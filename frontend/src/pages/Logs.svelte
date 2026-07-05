<script lang="ts">
  import { onMount, onDestroy, afterUpdate } from "svelte";
  import { api, events, EVT } from "../lib/ipc";

  type Level = "debug" | "info" | "warn" | "error";
  let level: Level = "info";
  let filter = "";
  let entries: any[] = [];
  let subscribed = false;
  let loading = true;

  // Auto-scroll-to-bottom that yields to manual scroll-up: stays pinned to the
  // tail as new logs arrive, but if the user scrolls up to read history it
  // stops fighting them; scrolling back to the bottom re-arms it.
  let listEl: HTMLElement;
  let autoScroll = true;

  const levels: Level[] = ["debug", "info", "warn", "error"];
  const levelRank: Record<string, number> = { debug: 0, info: 1, warn: 2, warning: 2, error: 3 };

  onMount(() => { subscribe(); });
  onDestroy(() => { try { events.off(EVT.logLine); } catch {} });

  afterUpdate(() => {
    if (listEl && autoScroll) listEl.scrollTop = listEl.scrollHeight;
  });

  function onScroll() {
    if (!listEl) return;
    const distFromBottom = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight;
    autoScroll = distFromBottom < 30;
  }

  async function subscribe() {
    try { events.off(EVT.logLine); } catch {}
    events.on(EVT.logLine, onLine);
    // Load recent history first (covers logs emitted before the page opened,
    // e.g. startup apply), then start the live stream.
    loading = true;
    entries = [];
    autoScroll = true;
    try {
      const lines = await api.recentLogs(500);
      for (const line of lines) onLine(line);
    } catch { /* file may not exist yet */ }
    api.subscribeLogs(level);
    subscribed = true;
    loading = false;
  }

  function onLine(raw: string) {
    let rec: any;
    try { rec = JSON.parse(raw); } catch { rec = { msg: raw, level: "INFO" }; }
    entries = [...entries.slice(-999), rec];
  }

  // Client-side level filter (history is loaded all-levels; live is server-filtered
  // but we re-filter to stay consistent when level changes mid-stream).
  $: visible = entries.filter((e) => {
    const lr = levelRank[(e.level ?? "info").toLowerCase()] ?? 1;
    if (lr < levelRank[level]) return false;
    if (!filter.trim()) return true;
    const q = filter.toLowerCase();
    return (e.msg ?? "").toLowerCase().includes(q) || JSON.stringify(e).toLowerCase().includes(q);
  });

  function levelColor(l: string) {
    switch ((l ?? "").toUpperCase()) {
      case "ERROR": return "bad";
      case "WARN": case "WARNING": return "warn";
      case "DEBUG": return "faint";
      default: return "accent";
    }
  }
</script>

<div class="logs-page">
  <div class="head">
    <h2>日志</h2>
    <div class="controls">
      <select bind:value={level} on:change={subscribe}>
        {#each levels as l}<option value={l}>{l}</option>{/each}
      </select>
      <input placeholder="过滤…" bind:value={filter} />
      <button class="ghost" on:click={() => (entries = [])}>清空</button>
    </div>
  </div>

  <div class="card log-list" bind:this={listEl} on:scroll={onScroll}>
    {#each visible as e}
      <div class="log-row">
        <span class="log-time">{e.time ? new Date(e.time).toLocaleTimeString() : ""}</span>
        <span class="tag {levelColor(e.level)}">{(e.level ?? "INFO").toUpperCase()}</span>
        <span class="log-msg">{e.msg}</span>
        {#if Object.keys(e).length > 3}
          <span class="log-attrs mono">{JSON.stringify({ ...e, time: undefined, level: undefined, msg: undefined })}</span>
        {/if}
      </div>
    {:else}
      <div class="faint" style="padding:8px">{loading ? "加载历史日志…" : `等待日志…（级别 ≥ ${level}）`}</div>
    {/each}
  </div>
</div>

<style>
  /* height:100% resolves against .content's definite (flex-resolved) height,
     so the page fills exactly without making .content overflow/scroll. The
     head takes its natural height; the log-list flexes to fill the rest and
     scrolls internally (single scrollbar, pinned to this element). */
  .logs-page { height: 100%; display: flex; flex-direction: column; min-height: 0; }
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; gap: 12px; flex-shrink: 0; }
  h2 { margin: 0; font-size: 18px; }
  .controls { display: flex; gap: 8px; }
  .log-list { flex: 1; min-height: 0; overflow: auto; background: #07090d; }
  .log-row { display: flex; align-items: flex-start; gap: 8px; padding: 2px 6px; font-size: 12px; }
  .log-row:hover { background: var(--bg-1); }
  .log-time { color: var(--text-faint); font-family: var(--font-mono); width: 70px; flex-shrink: 0; }
  .log-msg { flex: 1; word-break: break-word; }
  .log-attrs { color: var(--text-faint); word-break: break-all; }
  .accent { color: var(--accent); }
</style>
