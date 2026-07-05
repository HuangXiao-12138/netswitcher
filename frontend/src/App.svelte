<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, events, EVT } from "./lib/ipc";
  import type { StatusResponse } from "../wailsjs/go/models";
  import Status from "./pages/Status.svelte";
  import Profiles from "./pages/Profiles.svelte";
  import Routes from "./pages/Routes.svelte";
  import Diagnostics from "./pages/Diagnostics.svelte";
  import Logs from "./pages/Logs.svelte";

  type PageId = "status" | "profiles" | "routes" | "diagnostics" | "logs";
  const nav: { id: PageId; label: string; icon: string }[] = [
    { id: "status", label: "状态", icon: "◉" },
    { id: "profiles", label: "配置", icon: "☰" },
    { id: "routes", label: "路由表", icon: "⇄" },
    { id: "diagnostics", label: "诊断", icon: "⌕" },
    { id: "logs", label: "日志", icon: "≣" },
  ];

  let page: PageId = "status";
  let serviceUp = false;
  let serviceInstalled = false;
  let checking = false;
  let status: StatusResponse | null = null;
  let installing = false;
  let firstRunDismissed = false; // user clicked "稍后" on the first-run modal

  async function refreshServiceState() {
    checking = true;
    try {
      // serviceInstalled queries SCM directly — works even before the service
      // is running, so we can tell "never installed" from "installed but stopped".
      serviceInstalled = await api.serviceInstalled().catch(() => false);
      serviceUp = await api.serviceAvailable().catch(() => false);
    } finally {
      checking = false;
    }
  }

  async function installService() {
    installing = true;
    try {
      await api.startServiceElevated();
      // Poll until the service comes up (UAC + install + start takes a few s).
      const deadline = Date.now() + 30000;
      while (Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 1000));
        await refreshServiceState();
        if (serviceUp) break;
      }
    } catch (e: any) {
      alert("安装失败：" + (e?.message ?? e));
    } finally {
      installing = false;
    }
  }

  // First-run modal shows when service is not installed and not yet dismissed.
  $: showFirstRun = !serviceInstalled && !serviceUp && !firstRunDismissed;
  // Smaller banner for the installed-but-stopped case.
  $: showStoppedBanner = serviceInstalled && !serviceUp;

  onMount(async () => {
    await refreshServiceState();
    try {
      status = await api.getStatus();
    } catch {
      status = null;
    }
    events.on(EVT.statusChanged, (st: StatusResponse) => {
      status = st;
      serviceUp = true;
    });
    window.addEventListener("focus", refreshServiceState);
  });

  onDestroy(() => {
    try { events.off(EVT.statusChanged); } catch {}
  });
</script>

<header class="topbar">
  <div class="brand">
    <span class="logo">⇄</span>
    <div class="brand-text">
      <span class="title">NetSwitcher</span>
      <span class="subtitle">内外网路由管理</span>
    </div>
  </div>
  <div class="service-pill" class:up={serviceUp} class:down={!serviceUp}>
    <span class="dot"></span>
    {serviceUp ? "服务在线" : (checking ? "检测中…" : "服务未运行")}
  </div>
</header>

{#if showStoppedBanner}
  <div class="banner">
    <div class="banner-text">
      <strong>服务未运行。</strong>路由不会被维护。
    </div>
    <button class="primary" on:click={installService} disabled={installing}>
      {installing ? "启动中…" : "启动服务"}
    </button>
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
      <Status {status} {serviceUp} on:refresh={refreshServiceState} />
    {:else if page === "profiles"}
      <Profiles />
    {:else if page === "routes"}
      <Routes />
    {:else if page === "diagnostics"}
      <Diagnostics />
    {:else if page === "logs"}
      <Logs />
    {/if}
  </section>
</main>

{#if showFirstRun}
  <div class="modal-backdrop">
    <div class="modal">
      <div class="modal-icon">⇄</div>
      <h2>欢迎使用 NetSwitcher</h2>
      <p>
        为了在 <strong>Wi-Fi 重连、网卡插拔、DHCP 续约</strong> 后自动恢复路由，
        需要安装一个后台服务。
      </p>
      <ul class="modal-bullets">
        <li>服务以 SYSTEM 权限常驻，<strong>关掉本窗口或注销用户也照常工作</strong></li>
        <li>开机自启 + 崩溃自动重启</li>
        <li>仅此一次安装，会请求管理员权限（UAC 弹窗）</li>
      </ul>
      <p class="faint" style="margin-top:6px">
        安装后即可在「配置」页添加路由规则。
      </p>
      <div class="modal-actions">
        <button class="ghost" on:click={() => (firstRunDismissed = true)} disabled={installing}>稍后</button>
        <button class="primary" on:click={installService} disabled={installing}>
          {installing ? "安装中…（请在 UAC 窗口点同意）" : "立即安装"}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .topbar {
    display: flex; align-items: center; justify-content: space-between;
    padding: 10px 18px; border-bottom: 1px solid var(--border); background: var(--bg-1);
    -webkit-app-region: drag;
  }
  .brand { display: flex; align-items: center; gap: 12px; }
  .logo { font-size: 26px; color: var(--accent); line-height: 1; }
  .brand-text { display: flex; flex-direction: column; line-height: 1.15; }
  .title { font-weight: 700; font-size: 16px; }
  .subtitle { font-size: 11px; color: var(--text-dim); }
  .service-pill {
    display: flex; align-items: center; gap: 7px;
    font-size: 12px; padding: 4px 11px; border-radius: 999px;
    border: 1px solid var(--border); -webkit-app-region: no-drag;
  }
  .service-pill .dot { width: 8px; height: 8px; border-radius: 50%; }
  .service-pill.up { color: var(--good); border-color: rgba(74,222,128,0.3); }
  .service-pill.up .dot { background: var(--good); box-shadow: 0 0 6px var(--good); }
  .service-pill.down { color: var(--bad); border-color: rgba(248,113,113,0.3); }
  .service-pill.down .dot { background: var(--bad); }

  .banner {
    display: flex; align-items: center; justify-content: space-between;
    gap: 16px; padding: 10px 18px;
    background: rgba(248,113,113,0.08); border-bottom: 1px solid rgba(248,113,113,0.3);
  }
  .banner-text { font-size: 13px; }

  .shell { flex: 1; display: flex; min-height: 0; }
  .sidebar {
    width: 152px; padding: 12px 8px;
    background: var(--bg-1); border-right: 1px solid var(--border);
    display: flex; flex-direction: column; gap: 2px;
  }
  .nav-item {
    display: flex; align-items: center; gap: 10px;
    background: transparent; border: 1px solid transparent;
    text-align: left; padding: 9px 11px; border-radius: var(--radius-sm);
    color: var(--text-dim);
  }
  .nav-item:hover { background: var(--bg-2); color: var(--text); }
  .nav-item.active { background: rgba(95,184,255,0.1); border-color: rgba(95,184,255,0.25); color: var(--accent); }
  .nav-icon { width: 16px; text-align: center; opacity: 0.85; }
  .content { flex: 1; overflow: auto; padding: 18px 22px; }

  /* First-run modal */
  .modal-backdrop {
    position: fixed; inset: 0; background: rgba(8,10,15,0.72);
    display: flex; align-items: center; justify-content: center; z-index: 50;
    -webkit-app-region: no-drag;
  }
  .modal {
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 12px;
    padding: 28px 30px; max-width: 460px; box-shadow: 0 10px 40px rgba(0,0,0,0.5);
  }
  .modal-icon { font-size: 40px; color: var(--accent); margin-bottom: 6px; line-height: 1; }
  .modal h2 { margin: 0 0 10px; font-size: 19px; }
  .modal p { margin: 6px 0; font-size: 13px; line-height: 1.55; }
  .modal-bullets { margin: 10px 0; padding-left: 20px; font-size: 12.5px; color: var(--text-dim); line-height: 1.7; }
  .modal-bullets li::marker { color: var(--accent); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }
</style>
