<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, events, EVT, wc, getTheme, setTheme } from "./lib/ipc";
  import type { StatusResponse } from "../wailsjs/go/models";
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

  async function toggleMax() {
    await wc.toggleMax();
    // Update the icon after the toggle lands.
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
      // The elevated instance is starting; this one will quit on its own
      // (handled Go-side). Give it a moment then close the window.
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

  // Non-elevated runs are blocked entirely — admin is required to touch routes,
  // so there's no useful "read-only" mode. The modal can only be dismissed by
  // relaunching elevated (or closing the window).
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
    window.addEventListener("focus", refreshState);
  });

  onDestroy(() => {
    try { events.off(EVT.statusChanged); } catch {}
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
      <Settings />
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

<style>
  .topbar {
    display: flex; align-items: center; justify-content: space-between;
    /* Right padding is 0 so the window buttons sit flush at the edge (Windows
       convention); buttons keep their own hit area via fixed width. */
    padding: 8px 0 8px 16px; border-bottom: 1px solid var(--border); background: var(--bg-1);
    /* Wails frameless drag uses the --wails-draggable CSS property (NOT
       -webkit-app-region). Any descendant that should be clickable must
       override it to a non-"drag" value. */
    --wails-draggable: drag;
    /* user-select:none stops text selection from competing with the drag
       handler — without it clicks on the title text often show the "no" cursor
       and refuse to drag. */
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

  /* Custom window controls (frameless). Override the inherited drag property
     so the buttons are clickable, not drag-handles. */
  .win-ctrl { display: flex; align-items: stretch; --wails-draggable: no-drag; }
  .win-btn {
    width: 40px; height: 32px; padding: 0; background: transparent;
    border: none; border-radius: 0; color: var(--text-dim);
    display: flex; align-items: center; justify-content: center;
    --wails-draggable: no-drag;
  }
  .win-btn:hover { background: var(--bg-3); color: var(--text); }
  .win-btn.close:hover { background: #e81123; color: #fff; }
  .win-btn span { display: inline-block; }
  .ico-min { width: 12px; height: 1.5px; background: currentColor; }
  .ico-max { width: 11px; height: 11px; border: 1.5px solid currentColor; }
  /* Restore: two overlapping squares (the maximized → normal indicator). */
  .ico-restore { width: 14px; height: 14px; position: relative; }
  .ico-restore::before, .ico-restore::after {
    content: ""; position: absolute; width: 9px; height: 9px; border: 1.5px solid currentColor;
  }
  .ico-restore::before { left: 0; top: 4px; }
  .ico-restore::after { left: 4px; top: 0; background: var(--bg-1); }
  .ico-close {
    width: 14px; height: 14px; position: relative;
  }
  .ico-close::before, .ico-close::after {
    content: ""; position: absolute; left: 6px; top: 1px; width: 1.5px; height: 12px;
    background: currentColor; border-radius: 1px;
  }
  .ico-close::before { transform: rotate(45deg); }
  .ico-close::after { transform: rotate(-45deg); }

  .banner {
    display: flex; align-items: center; gap: 12px; padding: 10px 18px;
  }
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
</style>
