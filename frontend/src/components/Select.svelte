<script lang="ts" context="module">
  export interface SelectOption { value: string; label: string; }
</script>

<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import type { SelectOption } from "./Select.svelte";

  export let options: SelectOption[] = [];
  export let value = "";
  export let placeholder = "";
  export let disabled = false;

  let open = false;
  let triggerEl: HTMLButtonElement | undefined;
  let menuStyle = "";

  $: selected = options.find((o) => o.value === value);

  // Always open downward, cap height to available space. Simpler than
  // flipping — no gap calculation needed. The menu scrolls internally.
  function placeMenu() {
    if (!triggerEl) return;
    const r = triggerEl.getBoundingClientRect();
    const spaceBelow = window.innerHeight - r.bottom - 8;
    const maxH = Math.max(80, Math.min(280, spaceBelow));
    menuStyle =
      `position:fixed;` +
      `top:${r.bottom + 4}px;` +
      `left:${r.left}px;` +
      `min-width:${Math.max(r.width, 160)}px;` +
      `max-height:${maxH}px;` +
      `z-index:10000;` +
      `overflow:auto;`;
  }

  function toggle() {
    if (disabled) return;
    open = !open;
    if (open) placeMenu();
  }

  function pick(o: SelectOption) {
    value = o.value;     // bind:value propagates to parent automatically
    open = false;
  }

  // Portal: move to <body> so ancestor overflow doesn't clip the menu.
  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy() { node.remove(); } };
  }

  // Close on outside-click / Escape. NO scroll listener — the menu's own
  // overflow:auto handles scrolling; scroll-closing caused bugs.
  function onDocClick(e: MouseEvent) {
    if (!open) return;
    const t = e.target as Node;
    if (triggerEl?.contains(t)) return;
    const menu = document.querySelector(".ns-sel-menu");
    if (menu && menu.contains(t)) return;
    open = false;
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") open = false;
  }
  onMount(() => {
    document.addEventListener("click", onDocClick, true);
    document.addEventListener("keydown", onKey);
  });
  onDestroy(() => {
    document.removeEventListener("click", onDocClick, true);
    document.removeEventListener("keydown", onKey);
  });
</script>

<!-- The trigger stays in-place (inside the table cell). -->
<div class="ns-select">
  <button type="button" class="ns-sel-trigger" on:click={toggle}
    bind:this={triggerEl} {disabled}>
    <span class="ns-sel-label">{selected?.label || placeholder || "—"}</span>
    <span class="ns-sel-caret chev-up"></span>
  </button>
</div>

<!-- The menu is portaled to <body> + position:fixed so it floats above
     everything regardless of table overflow. -->
{#if open}
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <div class="ns-sel-menu" use:portal style={menuStyle}>
    {#each options as o}
      <button type="button" class="ns-sel-opt" class:sel={o.value === value}
        on:click={() => pick(o)}>
        {o.label}
      </button>
    {/each}
    {#if options.length === 0}
      <div class="ns-sel-empty">无选项</div>
    {/if}
  </div>
{/if}

<style>
  .ns-select { position: relative; display: inline-block; width: 100%; }
  .ns-sel-trigger {
    width: 100%; display: flex; align-items: center; justify-content: space-between;
    gap: 8px; text-align: left;
    font-family: var(--comp-font); font-size: 13px;
    background: var(--input-bg); color: var(--text);
    border: 1px solid var(--border);
    padding: 7px 12px; padding-right: 10px;
    border-radius: var(--comp-radius); outline: none; cursor: pointer;
  }
  .ns-sel-trigger:hover { border-color: var(--text-faint); }
  .ns-sel-trigger:focus { border-color: var(--accent); }
  .ns-sel-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .chev-up {
    width: 0; height: 0; flex-shrink: 0;
    border-left: 4px solid transparent; border-right: 4px solid transparent;
    border-top: 5px solid var(--text-faint); opacity: 0.7;
  }
</style>

<!-- Menu + option styles are global because the node is portaled to <body>. -->
<style>
  :global(.ns-sel-menu) {
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--comp-radius);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    padding: 4px; display: flex; flex-direction: column; gap: 1px;
  }
  :global(.ns-sel-opt) {
    text-align: left; width: 100%; flex-shrink: 0;
    background: transparent; border: none; color: var(--text-dim);
    padding: 7px 10px; border-radius: calc(var(--comp-radius) - 2px);
    cursor: pointer; font-family: var(--comp-font); font-size: 13px; line-height: 1.2;
  }
  :global(.ns-sel-opt:hover) { background: var(--bg-2); color: var(--text); }
  :global(.ns-sel-opt.sel) { background: var(--bg-2); color: var(--accent); }
  :global(.ns-sel-empty) { padding: 8px 10px; color: var(--text-faint); font-size: 12px; }
</style>
