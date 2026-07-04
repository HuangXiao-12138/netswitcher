<script lang="ts">
  // Phase 0 placeholder shell. Phase 6 replaces this with the full
  // left-nav layout (Status / Profiles / Routes / Diagnostics / Logs) and
  // wires the real Wails bindings (frontend/wailsjs/go/main/API.js).
  let pong = "（Phase 6 接入完整 API）";
</script>

<main class="shell">
  <header class="topbar">
    <div class="brand">
      <span class="logo">⇄</span>
      <div>
        <h1>NetSwitcher</h1>
        <p class="tagline">内外网路由管理工具</p>
      </div>
    </div>
    <span class="version">v0.1.0 — 骨架</span>
  </header>

  <section class="card">
    <h2>Phase 0 — 项目骨架</h2>
    <p>
      单二进制多角色已就绪：<code>netswitcher service run</code> /
      <code>gui</code> / <code>apply</code> / <code>dump</code>。
    </p>
    <p class="muted">后端绑定: {pong}</p>
  </section>
</main>

<style>
  :global(html, body) {
    margin: 0;
    height: 100%;
    background: #0f1115;
    color: #e6e8ee;
    font-family: "Segoe UI", system-ui, -apple-system, sans-serif;
  }
  :global(#app) {
    height: 100vh;
  }
  .shell {
    padding: 24px 28px;
  }
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid #232733;
    padding-bottom: 16px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .logo {
    font-size: 30px;
    color: #5fb8ff;
  }
  h1 {
    font-size: 20px;
    margin: 0;
  }
  .tagline {
    margin: 2px 0 0;
    color: #8b93a4;
    font-size: 12px;
  }
  .version {
    color: #6b7280;
    font-size: 12px;
  }
  .card {
    margin-top: 22px;
    background: #161a22;
    border: 1px solid #232733;
    border-radius: 10px;
    padding: 18px 20px;
  }
  .card h2 {
    margin: 0 0 8px;
    font-size: 15px;
  }
  code {
    background: #0b0d12;
    padding: 1px 5px;
    border-radius: 4px;
    font-size: 12px;
    color: #9dd0ff;
  }
  .muted {
    color: #8b93a4;
    font-size: 13px;
  }
</style>
