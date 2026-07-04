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
  let checking = false;
  let status: StatusResponse | null = null;

  async function refreshServiceState() {
    checking = true;
    try {
      serviceUp = await api.serviceAvailable();
    } catch {
      serviceUp = false;
    } finally {
      checking = false;
    }
  }

  async function startService() {
    try {
      await api.startServiceElevated();
      // Give SCM a moment, then re-check.
      setTimeout(refreshServiceState, 2500);
    } catch (e: any) {
      alert("无法启动服务： " + (e?.message ?? e));
    }
  }

  onMount(async () => {
    await refreshServiceState();
    try {
      status = await api.getStatus();
    } catch {
      status = null;
    }
    // Live status updates pushed by the service.
    events.on(EVT.statusChanged, (st: StatusResponse) => {
      status = st;
      serviceUp = true;
    });
    // Re-check service health when the window regains focus.
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

{#if !serviceUp}
  <div class="banner">
    <div class="banner-text">
      <strong>NetSwitcher 服务未运行。</strong> 路由不会被维护。
      <span class="faint">以管理员身份启动服务后即可恢复。</span>
    </div>
    <button class="primary" on:click={startService}>以管理员身份启动服务</button>
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

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 18px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-1);
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
    color: var(--text);
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
  .nav-item.active {
    background: rgba(95,184,255,0.1);
    border-color: rgba(95,184,255,0.25);
    color: var(--accent);
  }
  .nav-icon { width: 16px; text-align: center; opacity: 0.85; }

  .content { flex: 1; overflow: auto; padding: 18px 22px; }
</style>
