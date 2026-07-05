<script lang="ts">
  import { onMount } from "svelte";
  import { api, getTheme, setTheme, type ThemeId } from "../lib/ipc";
  import type { AppInfo } from "../../wailsjs/go/models";

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
  <h3>关于</h3>
  <dl>
    <dt>版本</dt><dd>{info?.version ?? "—"}</dd>
    <dt>权限</dt><dd>{info?.elevated ? "管理员" : "普通用户"}</dd>
    <dt>配置文件</dt><dd class="mono">{info?.configPath ?? "—"}</dd>
    <dt>状态文件</dt><dd class="mono">{info?.statePath ?? "—"}</dd>
  </dl>
</div>

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
</style>
