<script lang="ts">
  import { onDestroy } from "svelte";
  import { api, events, EVT } from "../lib/ipc";

  let target = "8.8.8.8";
  let mode: "ping" | "tracert" = "ping";
  let running = false;
  let lines: string[] = [];
  let errorText = "";

  // Auto-scroll the output to the tail as new lines arrive (diagnostics is
  // always a tail view, so we don't need the logs page's "yield on scroll-up").
  let outEl: HTMLElement;
  const scrollToBottom = () => {
    if (outEl) outEl.scrollTop = outEl.scrollHeight;
  };

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
    requestAnimationFrame(scrollToBottom);
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

<div class="diag-page">
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

  <div class="card diag-out" bind:this={outEl}>
    {#if lines.length === 0 && !errorText}
      <span class="faint">运行 {mode} 查看输出…</span>
    {:else}
      {#each lines as l}<div class="mono out-line">{l}</div>{/each}
      {#if errorText}<div class="mono out-line err-line">{errorText}</div>{/if}
    {/if}
  </div>
</div>

<style>
  /* Fill .content exactly: head + input-row take natural height, the output
     box flexes to fill the rest and scrolls internally. */
  .diag-page { height: 100%; display: flex; flex-direction: column; min-height: 0; }
  .head { margin-bottom: 14px; flex-shrink: 0; }
  h2 { margin: 0; font-size: 18px; }
  .input-row { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; flex-shrink: 0; }
  .input-row input { flex: 1; }
  .diag-out { flex: 1; min-height: 0; overflow: auto; background: #07090d; padding: 12px; }
  .out-line { white-space: pre-wrap; word-break: break-all; padding: 1px 0; }
  .err-line { color: var(--bad); }
</style>
