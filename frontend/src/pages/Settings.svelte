<script lang="ts">
  import { onMount } from "svelte";
  import { api, events, EVT, getTheme, setTheme, type ThemeId } from "../lib/ipc";
  import type { AppInfo, UpdateInfo } from "../../wailsjs/go/models";

  const levels = ["debug", "info", "warn", "error"];
  const themes: { id: ThemeId; name: string; desc: string }[] = [
    { id: "a", name: "精炼优化", desc: "继承当前网络控制台风格" },
    { id: "b", name: "现代扁平", desc: "Linear / Notion 风克制现代" },
    { id: "c", name: "终端主题", desc: "等宽字体 + 角标 + 青光" },
  ];

  let info: AppInfo | null = null;
  let logLevel = "info";
  let autoStart = false;
  let elevated = false;
  let busy = false;
  let msg = "";
  let theme: ThemeId = "a";
  let upd: UpdateInfo | null = null;
  let checking = false;
  let releaseErr = "";
  let upgradeModal = false;
  let upgradeStage: "preparing" | "downloading" | "installing" | "armed" | "failed" = "preparing";
  let downloadPct = -1; // -1 when the total is unknown
  let upgradeErr = "";

  // Bumped by App when the tray "检查更新" item is clicked — each change
  // triggers a check. Starts at 0 so the reactive block is inert on mount.
  export let checkUpdateTrigger = 0;
  $: if (checkUpdateTrigger) checkUpdate();

  onMount(async () => {
    theme = getTheme();
    try {
      const [i, lvl, as, el] = await Promise.all([
        api.getAppInfo(),
        api.getLogLevel(),
        api.autoStartInstalled(),
        api.isElevated(),
      ]);
      info = i;
      logLevel = lvl;
      autoStart = as;
      elevated = el;
    } catch (e: any) {
      msg = "加载失败：" + (e?.message ?? e);
    }
  });

  function pickTheme(id: ThemeId) {
    theme = id;
    setTheme(id);
    msg = `主题已切换为「${themes.find((t) => t.id === id)?.name}」（仅本机界面，立即生效）。`;
  }

  async function toggleAutoStart() {
    busy = true;
    msg = "";
    try {
      if (autoStart) {
        await api.uninstallAutoStart();
        autoStart = false;
        msg = "已关闭开机自启。";
      } else {
        await api.installAutoStart();
        autoStart = true;
        msg = "已开启开机自启：下次登录自动以管理员启动，无需 UAC。";
      }
    } catch (e: any) {
      msg = "操作失败：" + (e?.message ?? e);
    } finally {
      busy = false;
    }
  }

  async function changeLevel(e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value;
    busy = true;
    msg = "";
    try {
      await api.setLogLevel(v);
      logLevel = v;
      msg = "日志级别已设为 " + v + "。";
    } catch (err: any) {
      msg = "设置失败：" + (err?.message ?? err);
    } finally {
      busy = false;
    }
  }

  async function applyNow() {
    busy = true;
    msg = "";
    try {
      const r: any = await api.applyNow();
      const ap = r?.applied?.length ?? 0;
      const sk = r?.skipped?.length ?? 0;
      msg = `应用完成：新增 ${ap} 条，跳过 ${sk} 条。`;
    } catch (e: any) {
      msg = "应用失败：" + (e?.message ?? e);
    } finally {
      busy = false;
    }
  }

  async function openLogs() {
    try {
      await api.openLogFolder();
    } catch (e: any) {
      msg = "无法打开：" + (e?.message ?? e);
    }
  }

  async function checkUpdate() {
    checking = true;
    releaseErr = "";
    try {
      upd = await api.checkUpdate();
    } catch (e: any) {
      // Backend categorizes failures into upd.errorKind, so a reject here can
      // only be an IPC-level fault — synthesize an error state as a fallback.
      upd = {
        currentVersion: info?.version ?? "",
        isDevBuild: true,
        errorKind: "unknown",
        error: "检查更新失败：" + (e?.message ?? e),
      } as UpdateInfo;
    } finally {
      checking = false;
    }
  }

  async function openRelease(url: string) {
    releaseErr = "";
    try {
      await api.openURL(url);
    } catch (e: any) {
      releaseErr = "无法打开页面：" + (e?.message ?? e);
    }
  }

  function onProgress(p: any) {
    switch (p?.stage) {
      case "preparing":
        upgradeStage = "preparing";
        break;
      case "downloading":
        upgradeStage = "downloading";
        downloadPct = p.total > 0 ? Math.min(100, Math.round((p.downloaded / p.total) * 100)) : -1;
        break;
      case "installing":
        upgradeStage = "installing";
        break;
      case "armed":
        upgradeStage = "armed";
        events.off(EVT.updateProgress);
        break;
      case "failed":
        upgradeStage = "failed";
        upgradeErr = p?.error ?? "升级失败";
        events.off(EVT.updateProgress);
        break;
    }
  }

  function openUpgrade() {
    upgradeModal = true;
    upgradeStage = "preparing";
    downloadPct = -1;
    upgradeErr = "";
    events.on(EVT.updateProgress, onProgress);
    api.performUpdate().catch((e: any) => {
      upgradeStage = "failed";
      upgradeErr = e?.message ?? String(e);
      events.off(EVT.updateProgress);
    });
  }

  function cancelUpgrade() {
    api.cancelUpdate().catch(() => {});
    events.off(EVT.updateProgress);
    upgradeModal = false;
  }

  function restartNow() {
    api.quit().catch(() => {});
  }

  function stageLabel(stage: string): string {
    switch (stage) {
      case "preparing":
        return "正在获取版本信息……";
      case "installing":
        return "正在准备安装……";
      default:
        return "升级中……";
    }
  }

  function formatDate(iso: string): string {
    const d = new Date(iso);
    return isNaN(d.getTime()) ? iso : d.toLocaleDateString("zh-CN");
  }

  $: levelDesc = {
    debug: "最详细（含每次 apply 的全部 route 命令）",
    info: "常规（apply / 网络变化 / 配置变化）",
    warn: "警告及以上（路由冲突、跳过、失败）",
    error: "仅错误",
  }[logLevel] ?? "";
</script>

<div class="head">
  <h2>设置</h2>
</div>

{#if msg}
  <div class="msg">{msg}</div>
{/if}

<div class="card section">
  <div class="section-head">
    <h3>主题</h3>
  </div>
  <div class="theme-grid">
    {#each themes as t}
      <button class="theme-card" class:active={theme === t.id} on:click={() => pickTheme(t.id)}>
        <div class="theme-name">{t.name}</div>
        <div class="theme-desc">{t.desc}</div>
        <span class="theme-tag">{t.id.toUpperCase()}</span>
      </button>
    {/each}
  </div>
</div>

<div class="card section">
  <div class="section-head">
    <h3>开机自启</h3>
    <label class="switch">
      <input type="checkbox" checked={autoStart} on:change={toggleAutoStart} disabled={busy || !elevated} />
      <span class="slider"></span>
    </label>
  </div>
  <p class="muted">
    开启后，下次登录 Windows 时由任务计划自动以管理员身份启动 NetSwitcher（不弹 UAC）。
  </p>
  {#if !elevated}
    <p class="warn-line">需要管理员权限才能配置开机自启 —— 请先以管理员身份重启。</p>
  {/if}
</div>

<div class="card section">
  <div class="section-head">
    <h3>日志级别</h3>
    <select value={logLevel} on:change={changeLevel} disabled={busy}>
      {#each levels as l}<option value={l}>{l}</option>{/each}
    </select>
  </div>
  <p class="muted">{levelDesc}</p>
  <p class="muted">改动立即生效，并写回 config.json（重启后保留）。</p>
</div>

<div class="card section">
  <div class="section-head">
    <h3>路由引擎</h3>
    <button on:click={applyNow} disabled={busy || !info?.engineOn}>立即重新应用</button>
  </div>
  <p class="muted">
    引擎状态：
    <span class="tag {info?.engineOn ? 'good' : 'bad'}">{info?.engineOn ? "运行中" : "未运行"}</span>
    · 按一次重发所有配置路由（解决"路由被外部改掉"等问题）。
  </p>
</div>

<div class="card section">
  <div class="section-head">
    <h3>日志文件</h3>
    <button on:click={openLogs} disabled={busy}>打开日志目录</button>
  </div>
  <p class="muted mono">{info?.logDir ?? "—"}</p>
</div>

<div class="card section about">
  <div class="section-head">
    <h3>关于</h3>
    <button on:click={checkUpdate} disabled={checking || !info}>
      {checking ? "检查中…" : "检查更新"}
    </button>
  </div>
  <dl>
    <dt>版本</dt><dd>{info?.version ?? "—"}</dd>
    <dt>权限</dt><dd>{info?.elevated ? "管理员" : "普通用户"}</dd>
    <dt>配置文件</dt><dd class="mono">{info?.configPath ?? "—"}</dd>
    <dt>状态文件</dt><dd class="mono">{info?.statePath ?? "—"}</dd>
  </dl>
  {#if upd}
    <div class="upd">
      {#if upd.errorKind}
        <p class="upd-err">{upd.error}</p>
        <button class="link" on:click={checkUpdate} disabled={checking}>重试</button>
      {:else if upd.isDevBuild}
        <p class="muted">
          开发版本（{upd.currentVersion}），最新发布版为 <strong>{upd.latestVersion}</strong>。
        </p>
        <button class="link" on:click={() => openRelease(upd.releaseURL)}>查看发布页 ↗</button>
      {:else if upd.hasUpdate}
        <p class="muted">
          发现新版本 <strong>{upd.latestVersion}</strong>（当前 {upd.currentVersion}）。
        </p>
        {#if info?.elevated}
          <button class="link" on:click={openUpgrade}>一键升级</button>
          <span class="muted"> · </span>
        {/if}
        <button class="link" on:click={() => openRelease(upd.releaseURL)}>前往下载 ↗</button>
      {:else}
        <p class="muted">已是最新版本（{upd.currentVersion}）。</p>
      {/if}
      {#if upd.publishedAt && !upd.errorKind}
        <p class="muted">发布于 {formatDate(upd.publishedAt)}</p>
      {/if}
      {#if releaseErr}
        <p class="upd-err">{releaseErr}</p>
      {/if}
    </div>
  {/if}
</div>

{#if upgradeModal}
  <div class="upd-backdrop">
    <div class="upd-modal">
      <h3>升级到 {upd?.latestVersion ?? ""}</h3>

      <div class="upd-version">
        <span class="cur">{upd?.currentVersion ?? ""}</span>
        <span class="arrow">→</span>
        <span class="new">{upd?.latestVersion ?? ""}</span>
      </div>

      {#if upd?.releaseNotes}
        <div class="upd-notes">
          <div class="upd-notes-title">更新内容</div>
          <div class="upd-notes-body">{upd.releaseNotes}</div>
        </div>
      {/if}

      <div class="upd-stage">
        {#if upgradeStage === "downloading" && downloadPct >= 0}
          <div class="upd-stage-label">正在下载 {downloadPct}%</div>
          <div class="upd-bar"><div class="upd-fill" style="width:{downloadPct}%"></div></div>
        {:else if upgradeStage === "armed"}
          <div class="upd-done">✓ 新版本已就绪，点击下方按钮重启完成安装。</div>
        {:else if upgradeStage === "failed"}
          <div class="upd-err">{upgradeErr}</div>
        {:else}
          <div class="upd-stage-label">{stageLabel(upgradeStage)}</div>
          <div class="upd-bar indeterminate"><div class="upd-fill"></div></div>
        {/if}
      </div>

      <div class="upd-actions">
        {#if upgradeStage === "armed"}
          <button class="primary" on:click={restartNow}>立即重启</button>
        {:else if upgradeStage === "failed"}
          <button on:click={openUpgrade}>重试</button>
          <button class="ghost" on:click={() => { upgradeModal = false; }}>关闭</button>
        {:else if upgradeStage === "installing"}
          <button disabled>正在准备安装…</button>
        {:else}
          <button class="ghost" on:click={cancelUpgrade}>取消</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .head { margin-bottom: 14px; }
  h2 { margin: 0; font-size: 18px; }
  h3 { margin: 0; font-size: 14px; }
  .section { margin-bottom: 12px; }
  .section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px; }
  .muted { color: var(--text-dim); font-size: 12.5px; margin: 4px 0; line-height: 1.5; }
  .mono { font-family: var(--font-mono); font-size: 12px; word-break: break-all; }
  .warn-line { color: var(--warn); font-size: 12px; margin-top: 6px; }
  .msg { background: rgba(95,184,255,0.08); border: 1px solid rgba(95,184,255,0.25); padding: 9px 12px; border-radius: var(--radius-sm); font-size: 12.5px; margin-bottom: 12px; }
  dl { margin: 8px 0 0; display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; }
  dt { color: var(--text-faint); font-size: 12px; }
  dd { margin: 0; font-size: 12.5px; }

  /* toggle switch */
  .switch { position: relative; display: inline-block; width: 40px; height: 22px; }
  .switch input { opacity: 0; width: 0; height: 0; }
  .slider {
    position: absolute; inset: 0; cursor: pointer; background: var(--bg-3);
    transition: 150ms; border-radius: 999px;
  }
  .slider::before {
    content: ""; position: absolute; height: 16px; width: 16px; left: 3px; top: 3px;
    background: var(--text-dim); transition: 150ms; border-radius: 50%;
  }
  .switch input:checked + .slider { background: var(--accent-dim); }
  .switch input:checked + .slider::before { transform: translateX(18px); background: var(--accent); }
  .switch input:disabled + .slider { opacity: 0.4; cursor: not-allowed; }

  /* Theme picker. */
  .theme-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
  .theme-card {
    text-align: left; background: var(--bg-2); border: 1px solid var(--border);
    border-radius: var(--radius-sm); padding: 12px 14px; cursor: pointer; position: relative;
    display: flex; flex-direction: column; gap: 4px;
  }
  .theme-card:hover { border-color: var(--accent-dim); }
  .theme-card.active { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }
  .theme-name { font-size: 13px; font-weight: 600; color: var(--text); }
  .theme-desc { font-size: 11.5px; color: var(--text-dim); line-height: 1.4; }
  .theme-tag { position: absolute; top: 8px; right: 10px; font-family: var(--font-mono); font-size: 10px; color: var(--text-faint); }

  /* Update-check block inside 关于. */
  .upd { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border); }
  .upd .muted { margin: 4px 0; }
  .upd-err { color: var(--bad); font-size: 12.5px; margin: 4px 0; line-height: 1.5; }
  .link { background: none; border: none; color: var(--accent); padding: 0; cursor: pointer; font-size: 12.5px; }
  .link:hover { text-decoration: underline; }

  /* Upgrade modal. */
  .upd-backdrop {
    position: fixed; inset: 0; background: rgba(8,10,15,0.72);
    display: flex; align-items: center; justify-content: center; z-index: 60;
  }
  .upd-modal {
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 12px;
    padding: 22px 24px; width: 440px; max-width: 90vw; max-height: 80vh; overflow-y: auto;
    box-shadow: 0 10px 40px rgba(0,0,0,0.5);
  }
  .upd-modal h3 { margin: 0 0 10px; font-size: 16px; }
  .upd-version { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; font-family: var(--font-mono); font-size: 13px; }
  .upd-version .cur { color: var(--text-dim); }
  .upd-version .new { color: var(--accent); font-weight: 600; }
  .upd-version .arrow { color: var(--text-faint); }
  .upd-notes { margin-bottom: 14px; }
  .upd-notes-title { font-size: 12px; color: var(--text-dim); margin-bottom: 6px; }
  .upd-notes-body {
    font-size: 12.5px; line-height: 1.6; white-space: pre-wrap;
    background: var(--bg-2); border: 1px solid var(--border); border-radius: 6px;
    padding: 10px 12px; max-height: 160px; overflow-y: auto;
  }
  .upd-stage { margin-bottom: 16px; }
  .upd-stage-label { font-size: 12.5px; color: var(--text-dim); margin-bottom: 6px; }
  .upd-bar { height: 6px; background: var(--bg-3); border-radius: 3px; overflow: hidden; }
  .upd-fill { height: 100%; background: var(--accent); border-radius: 3px; transition: width 200ms ease; }
  .upd-bar.indeterminate .upd-fill { width: 40%; animation: upd-indet 1.2s ease-in-out infinite; }
  @keyframes upd-indet { 0% { margin-left: -40%; } 100% { margin-left: 100%; } }
  .upd-done { color: var(--good); font-size: 13px; }
  .upd-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
  .upd-actions .primary { background: var(--accent); color: #fff; border-color: var(--accent); }
  .upd-actions .ghost { background: transparent; }
</style>
