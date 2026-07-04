<script lang="ts">
  import { onDestroy } from "svelte";
  import { api, events, EVT } from "../lib/ipc";

  let target = "8.8.8.8";
  let mode: "ping" | "tracert" = "ping";
  let running = false;
  let lines: string[] = [];
  let errorText = "";

  function clear() { lines = []; errorText = ""; }

  function start() {
    if (!target.trim() || running) return;
    clear();
    running = true;
    events.on(EVT.diagLine, onLine);
    events.on(EVT.diagEnd, onEnd);
    events.on(EVT.diagError, onError);
    if (mode === "ping") api.ping(target);
    else api.tracert(target);
  }

  function stop() {
    api.stopDiag();
    finish();
  }

  function onLine(line: string) {
    lines = [...lines, line];
    requestAnimationFrame(() => {
      const el = document.querySelector(".diag-out");
      if (el) el.scrollTop = el.scrollHeight;
    });
  }
  function onError(msg: string) {
    errorText = msg;
  }
  function onEnd() {
    finish();
  }
  function finish() {
    running = false;
    try { events.off(EVT.diagLine); events.off(EVT.diagEnd); events.off(EVT.diagError); } catch {}
  }

  onDestroy(finish);
</script>

<div class="head">
  <h2>诊断</h2>
</div>

<div class="card input-row">
  <select bind:value={mode} disabled={running}>
    <option value="ping">ping</option>
    <option value="tracert">tracert</option>
  </select>
  <input class="mono" placeholder="目标 IP / 域名" bind:value={target} disabled={running} />
  {#if running}
    <button class="danger" on:click={stop}>停止</button>
  {:else}
    <button class="primary" on:click={start}>运行</button>
  {/if}
  <button class="ghost" on:click={clear} disabled={running}>清空</button>
</div>

<div class="card diag-out" style="padding:12px">
  {#if lines.length === 0 && !errorText}
    <span class="faint">运行 {mode} 查看输出…</span>
  {:else}
    {#each lines as l}<div class="mono out-line">{l}</div>{/each}
    {#if errorText}<div class="mono out-line err-line">{errorText}</div>{/if}
  {/if}
</div>

<style>
  .head { margin-bottom: 14px; }
  h2 { margin: 0; font-size: 18px; }
  .input-row { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; }
  .input-row input { flex: 1; }
  .diag-out { min-height: 320px; max-height: 60vh; overflow: auto; background: #07090d; }
  .out-line { white-space: pre-wrap; word-break: break-all; padding: 1px 0; }
  .err-line { color: var(--bad); }
</style>
