<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, events, EVT } from "../lib/ipc";

  type Level = "debug" | "info" | "warn" | "error";
  let level: Level = "info";
  let filter = "";
  let entries: any[] = [];
  let subscribed = false;

  const levels: Level[] = ["debug", "info", "warn", "error"];

  onMount(() => { subscribe(); });
  onDestroy(() => { try { events.off(EVT.logLine); } catch {} });

  function subscribe() {
    entries = [];
    try { events.off(EVT.logLine); } catch {}
    events.on(EVT.logLine, onLine);
    api.subscribeLogs(level);
    subscribed = true;
  }

  function onLine(raw: string) {
    let rec: any;
    try { rec = JSON.parse(raw); } catch { rec = { msg: raw, level: "INFO" }; }
    entries = [...entries.slice(-499), rec];
  }

  $: visible = entries.filter((e) => {
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

<div class="card log-list" style="padding:8px">
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
    <div class="faint" style="padding:8px">等待日志…（订阅级别：{level}）</div>
  {/each}
</div>

<style>
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; gap: 12px; }
  h2 { margin: 0; font-size: 18px; }
  .controls { display: flex; gap: 8px; }
  .log-list { max-height: 65vh; overflow: auto; background: #07090d; }
  .log-row { display: flex; align-items: flex-start; gap: 8px; padding: 2px 6px; font-size: 12px; }
  .log-row:hover { background: var(--bg-1); }
  .log-time { color: var(--text-faint); font-family: var(--font-mono); width: 70px; flex-shrink: 0; }
  .log-msg { flex: 1; word-break: break-word; }
  .log-attrs { color: var(--text-faint); word-break: break-all; }
  .accent { color: var(--accent); }
</style>
