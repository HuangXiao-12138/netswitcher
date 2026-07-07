<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, events, EVT, wc, getTheme, setTheme } from "./lib/ipc";
  import type { StatusResponse, UpdateInfo } from "../wailsjs/go/models";
  import Status from "./pages/Status.svelte";
  import Profiles from "./pages/Profiles.svelte";
  import Routes from "./pages/Routes.svelte";
  import Diagnostics from "./pages/Diagnostics.svelte";
  import Logs from "./pages/Logs.svelte";
  import Settings from "./pages/Settings.svelte";

  type PageId = "status" | "profiles" | "routes" | "diagnostics" | "logs" | "settings";
  const nav: { id: PageId; label: string; icon: string }[] = [
    { id: "status", label: "状态", icon: "◉" },
    { id: "profiles", label: "配置", icon: "☰" },
    { id: "routes", label: "路由表", icon: "⇄" },
    { id: "diagnostics", label: "诊断", icon: "⌕" },
    { id: "logs", label: "日志", icon: "≣" },
    { id: "settings", label: "设置", icon: "⚙" },
  ];

  let page: PageId = "status";
  let elevated = false;
  let engineActive = false;
  let autoStart = false;
  let checking = false;
  let status: StatusResponse | null = null;
  let busy = false;
  let maximised = false;
  let updateInfo: UpdateInfo | null = null;
  let lastUpdateCheck = 0;
  // Upgrade modal lives here (global) so the topbar badge can open it from any
  // page without navigating to Settings first.
  let upgradeModal = false;
  let upgradeStage: "preparing" | "downloading" | "installing" | "armed" | "failed" = "preparing";
  let downloadPct = -1;
  let upgradeErr = "";
  // Auto-restart countdown once the swap is armed (the helper batch can only
  // complete once we exit, so we don't offer "later" — just a few seconds of
  // grace before forcing it, clickable to skip).
  let restartTimer: ReturnType<typeof setInterval> | null = null;
  let restartCountdown = 0;

  async function refreshState() {
    checking = true;
    try {
      elevated = await api.isElevated().catch(() => false);
      engineActive = await api.engineActive().catch(() => false);
      autoStart = await api.autoStartInstalled().catch(() => false);
      maximised = await api.isMaximised().catch(() => false);
    } finally {
      checking = false;
    }
  }

  // Re-check for updates when the window regains focus (e.g. reopened from the
  // tray), throttled to 30 min so we don't hammer GitHub on every focus.
  function maybeRecheckUpdate() {
    const now = Date.now();
    if (now - lastUpdateCheck < 30 * 60 * 1000) return;
    lastUpdateCheck = now;
    api.checkUpdate().then((info: UpdateInfo) => {
      if (info.hasUpdate) updateInfo = info;
    }).catch(() => {});
  }

  async function toggleMax() {
    await wc.toggleMax();
    setTimeout(async () => { maximised = await api.isMaximised().catch(() => false); }, 60);
  }

  async function loadStatus() {
    try {
      status = await api.getStatus();
    } catch {
      status = null;
    }
  }

  async function relaunchElevated() {
    busy = true;
    try {
      await api.relaunchElevated();
      setTimeout(() => window.close(), 800);
    } catch (e: any) {
      alert("无法以管理员身份重启：" + (e?.message ?? e));
      busy = false;
    }
  }

  async function quitApp() {
    try { await api.quit(); } catch {}
  }

  async function installAutoStart() {
    busy = true;
    try {
      await api.installAutoStart();
      await refreshState();
    } catch (e: any) {
      alert("设置开机自启失败：" + (e?.message ?? e));
    } finally {
      busy = false;
    }
  }

  // ---------- Upgrade ----------

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
        startRestartCountdown();
        break;
      case "failed":
        upgradeStage = "failed";
        upgradeErr = p?.error ?? "升级失败";
        events.off(EVT.updateProgress);
        break;
    }
  }

  async function openUpgrade() {
    // Ensure we have update info (badge may be clicked before the startup
    // check finishes). If there's no update after all, fall back to Settings.
    if (!updateInfo?.hasUpdate) {
      try { updateInfo = await api.checkUpdate(); } catch { /* surfaced in Settings */ }
    }
    if (!updateInfo?.hasUpdate) {
      page = "settings";
      return;
    }
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

  function startRestartCountdown() {
    restartCountdown = 5;
    if (restartTimer) clearInterval(restartTimer);
    restartTimer = setInterval(() => {
      restartCountdown -= 1;
      if (restartCountdown <= 0) {
        restartNow();
      }
    }, 1000);
  }

  function restartNow() {
    if (restartTimer) {
      clearInterval(restartTimer);
      restartTimer = null;
    }
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

  // Non-elevated runs are blocked entirely — admin is required to touch routes.
  $: showElevationModal = !elevated;
  // Auto-start nudge: elevated but not configured for boot launch.
  $: showAutoStartNudge = elevated && !autoStart;

  onMount(async () => {
    setTheme(getTheme());
    await refreshState();
    if (elevated) await loadStatus();
    events.on(EVT.statusChanged, (st: StatusResponse) => {
      status = st;
      engineActive = true;
    });
    events.on(EVT.updateAvailable, (info: UpdateInfo) => {
      updateInfo = info;
    });
    events.on(EVT.updateProgress, onProgress);
    // Treat the startup auto-check as the first "check" so the first focus
    // within 30 min doesn't immediately re-query GitHub.
    lastUpdateCheck = Date.now();
    window.addEventListener("focus", () => {
      refreshState();
      maybeRecheckUpdate();
    });
  });

  onDestroy(() => {
    if (restartTimer) clearInterval(restartTimer);
    try { events.off(EVT.statusChanged); events.off(EVT.updateProgress); } catch {}
  });
</script>

<header class="topbar" on:dblclick={wc.toggleMax}>
  <div class="brand">
    <img class="brand-logo" src="/logo.png" alt="NetSwitcher" />
    <div class="brand-text">
      <span class="title">NetSwitcher</span>
      <span class="subtitle">内外网路由管理</span>
    </div>
  </div>
  <div class="top-right">
    <div class="pills">
      {#if !elevated}
        <span class="pill down"><span class="dot"></span>未提权</span>
      {:else if engineActive}
        <span class="pill up"><span class="dot"></span>路由引擎在线</span>
      {:else}
        <span class="pill warn"><span class="dot"></span>引擎未启动</span>
      {/if}
    </div>
    {#if updateInfo?.hasUpdate}
      <button
        class="upd-badge"
        title="发现新版本 {updateInfo.latestVersion}，点击升级"
        on:click={openUpgrade}
      >
        <span class="upd-badge-dot"></span>有新版本 {updateInfo.latestVersion}
      </button>
    {/if}
    <div class="win-ctrl">
      <button class="win-btn" title="最小化" on:click={wc.minimise}><span class="ico-min"></span></button>
      <button class="win-btn" title={maximised ? "还原" : "最大化"} on:click={toggleMax}>
        {#if maximised}<span class="ico-restore"></span>{:else}<span class="ico-max"></span>{/if}
      </button>
      <button class="win-btn close" title="最小化到托盘" on:click={wc.hide}><span class="ico-close"></span></button>
    </div>
  </div>
</header>

{#if showAutoStartNudge}
  <div class="banner info">
    <div class="banner-text">
      <strong>建议设置开机自启。</strong>
      下次登录自动以管理员启动，无需每次 UAC。
    </div>
    <button on:click={installAutoStart} disabled={busy}>{busy ? "设置中…" : "设置开机自启"}</button>
    <button class="ghost" on:click={() => (autoStart = true)} disabled={busy}>跳过</button>
  </div>
{/if}

<main class="shell">
  <nav class="sidebar">
    {#each nav as item}
      <button
        class="nav-item"
        class:active={page === item.id}
        on:click={() => (page = item.id)}
      >
        <span class="nav-icon">{item.icon}</span>
        <span>{item.label}</span>
      </button>
    {/each}
  </nav>

  <section class="content">
    {#if page === "status"}
      <Status {status} serviceUp={engineActive} on:refresh={loadStatus} />
    {:else if page === "profiles"}
      <Profiles />
    {:else if page === "routes"}
      <Routes />
    {:else if page === "diagnostics"}
      <Diagnostics />
    {:else if page === "logs"}
      <Logs />
    {:else if page === "settings"}
      <Settings onUpgrade={openUpgrade} />
    {/if}
  </section>
</main>

{#if showElevationModal}
  <div class="modal-backdrop">
    <div class="modal">
      <div class="modal-icon">⇄</div>
      <h2>需要管理员权限</h2>
      <p>
        修改 Windows 路由表需要管理员权限。当前 NetSwitcher 是普通用户权限启动，
        <strong>无法下发路由</strong>。请以管理员身份重启。
      </p>
      <ul class="modal-bullets">
        <li>点击下方按钮 → 弹 UAC → 同意 → 自动以管理员重启</li>
        <li>重启后即可在「配置」页添加路由规则</li>
        <li>建议之后设置「开机自启」，下次登录免 UAC</li>
      </ul>
      <div class="modal-actions">
        <button class="ghost" on:click={quitApp} disabled={busy}>退出</button>
        <button class="primary" on:click={relaunchElevated} disabled={busy}>
          {busy ? "等待 UAC…" : "以管理员身份重启"}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if upgradeModal}
  <div class="upd-backdrop">
    <div class="upd-modal">
      <h3>升级到 {updateInfo?.latestVersion ?? ""}</h3>

      <div class="upd-version">
        <span class="cur">{updateInfo?.currentVersion ?? ""}</span>
        <span class="arrow">→</span>
        <span class="new">{updateInfo?.latestVersion ?? ""}</span>
      </div>

      {#if updateInfo?.releaseNotes}
        <div class="upd-notes">
          <div class="upd-notes-title">更新内容</div>
          <div class="upd-notes-body">{updateInfo.releaseNotes}</div>
        </div>
      {/if}

      <div class="upd-stage">
        {#if upgradeStage === "downloading" && downloadPct >= 0}
          <div class="upd-stage-label">正在下载 {downloadPct}%</div>
          <div class="upd-bar"><div class="upd-fill" style="width:{downloadPct}%"></div></div>
        {:else if upgradeStage === "armed"}
          <div class="upd-done">新版本已就绪，{restartCountdown} 秒后自动重启。</div>
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
  .topbar {
    display: flex; align-items: center; justify-content: space-between;
    height: 44px; padding: 0 0 0 16px; border-bottom: 1px solid var(--border); background: var(--bg-1);
    --wails-draggable: drag;
    user-select: none;
  }
  .brand { display: flex; align-items: center; gap: 10px; }
  .brand-logo { width: 24px; height: 24px; -webkit-app-region: no-drag; pointer-events: none; }
  .brand-text { display: flex; flex-direction: column; line-height: 1.15; }
  .title { font-weight: 700; font-size: 15px; }
  .subtitle { font-size: 11px; color: var(--text-dim); }
  .top-right { display: flex; align-items: center; gap: 14px; }
  .pills { display: flex; gap: 6px; }
  .pill {
    display: flex; align-items: center; gap: 7px;
    font-size: 12px; padding: 4px 11px; border-radius: 999px;
    border: 1px solid var(--border);
  }
  .pill .dot { width: 8px; height: 8px; border-radius: 50%; }
  .pill.up { color: var(--good); border-color: rgba(74,222,128,0.3); }
  .pill.up .dot { background: var(--good); box-shadow: 0 0 6px var(--good); }
  .pill.down { color: var(--bad); border-color: rgba(248,113,113,0.3); }
  .pill.down .dot { background: var(--bad); }
  .pill.warn { color: var(--warn); border-color: rgba(251,191,36,0.3); }
  .pill.warn .dot { background: var(--warn); }

  /* Update-available badge in the topbar. */
  .upd-badge {
    display: flex; align-items: center; gap: 6px; cursor: pointer;
    font-size: 12px; padding: 4px 11px; border-radius: 999px;
    border: 1px solid rgba(95,184,255,0.4); color: var(--accent);
    background: rgba(95,184,255,0.08);
  }
  .upd-badge:hover { background: rgba(95,184,255,0.16); }
  .upd-badge-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 6px var(--accent); }

  /* Custom window controls (frameless). */
  .win-ctrl { display: flex; align-items: stretch; --wails-draggable: no-drag; }
  .win-btn {
    width: 46px; height: 44px; padding: 0; background: transparent;
    border: none; border-radius: 0; color: var(--text-dim);
    display: flex; align-items: center; justify-content: center;
    --wails-draggable: no-drag;
  }
  .win-btn:hover { background: var(--bg-3); color: var(--text); }
  .win-btn.close:hover { background: #e81123; color: #fff; }
  .win-btn span { display: inline-block; }
  .ico-min { width: 12px; height: 1.5px; background: currentColor; }
  .ico-max { width: 11px; height: 11px; border: 1.5px solid currentColor; }
  .ico-restore { width: 14px; height: 14px; position: relative; }
  .ico-restore::before, .ico-restore::after {
    content: ""; position: absolute; width: 9px; height: 9px; border: 1.5px solid currentColor;
  }
  .ico-restore::before { left: 0; top: 4px; }
  .ico-restore::after { left: 4px; top: 0; background: var(--bg-1); }
  .ico-close { width: 14px; height: 14px; position: relative; }
  .ico-close::before, .ico-close::after {
    content: ""; position: absolute; left: 6px; top: 1px; width: 1.5px; height: 12px;
    background: currentColor; border-radius: 1px;
  }
  .ico-close::before { transform: rotate(45deg); }
  .ico-close::after { transform: rotate(-45deg); }

  .banner { display: flex; align-items: center; gap: 12px; padding: 10px 18px; }
  .banner.info { background: rgba(95,184,255,0.08); border-bottom: 1px solid rgba(95,184,255,0.25); }
  .banner-text { flex: 1; font-size: 13px; }

  .shell { flex: 1; display: flex; min-height: 0; }
  .sidebar {
    width: 152px; padding: 12px 8px;
    background: var(--bg-1); border-right: 1px solid var(--border);
    display: flex; flex-direction: column; gap: 2px;
  }
  .nav-item {
    display: flex; align-items: center; justify-content: flex-start; gap: 10px;
    background: transparent; border: 1px solid transparent;
    text-align: left; padding: 9px 11px; border-radius: var(--radius-sm);
    color: var(--text-dim); line-height: 1; width: 100%;
  }
  .nav-item:hover { background: var(--bg-2); color: var(--text); }
  .nav-item.active { background: rgba(95,184,255,0.1); border-color: rgba(95,184,255,0.25); color: var(--accent); }
  .nav-icon { width: 16px; height: 16px; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center; opacity: 0.85; font-size: 14px; }
  .content { flex: 1; overflow: auto; padding: 18px 22px; }

  .modal-backdrop {
    position: fixed; inset: 0; background: rgba(8,10,15,0.72);
    display: flex; align-items: center; justify-content: center; z-index: 50;
    -webkit-app-region: no-drag;
  }
  .modal {
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 12px;
    padding: 28px 30px; max-width: 460px; box-shadow: 0 10px 40px rgba(0,0,0,0.5);
  }
  .modal-icon { font-size: 40px; color: var(--bad); margin-bottom: 6px; line-height: 1; }
  .modal h2 { margin: 0 0 10px; font-size: 19px; }
  .modal p { margin: 6px 0; font-size: 13px; line-height: 1.55; }
  .modal-bullets { margin: 10px 0; padding-left: 20px; font-size: 12.5px; color: var(--text-dim); line-height: 1.7; }
  .modal-bullets li::marker { color: var(--accent); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }

  /* Upgrade modal (global, opened from topbar badge or Settings). */
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
  .upd-err { color: var(--bad); font-size: 12.5px; margin: 4px 0; line-height: 1.5; }
  .upd-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
  .upd-actions .primary { background: var(--accent); color: #fff; border-color: var(--accent); }
  .upd-actions .ghost { background: transparent; }
</style>
