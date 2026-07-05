<script lang="ts" context="module">
  export interface SelectOption {
    value: string;
    label: string;
  }
</script>

<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from "svelte";
  import type { SelectOption } from "./Select.svelte";

  export let options: SelectOption[] = [];
  export let value = "";
  export let placeholder = "";
  export let disabled = false;

  const dispatch = createEventDispatcher();
  let open = false;
  let triggerEl: HTMLButtonElement;
  let menuEl: HTMLDivElement;
  let menuStyle = "";

  const current = () => options.find((o) => o.value === value);

  function placeMenu() {
    if (!triggerEl) return;
    const r = triggerEl.getBoundingClientRect();
    const spaceBelow = window.innerHeight - r.bottom - 8;
    const spaceAbove = r.top - 8;
    const menuMax = 280;
    // Flip upward when there's more room above the trigger than below.
    let top: number, maxH: number;
    if (spaceBelow >= Math.min(menuMax, 120) || spaceBelow >= spaceAbove) {
      top = r.bottom + 4;
      maxH = Math.min(menuMax, spaceBelow);
    } else {
      maxH = Math.min(menuMax, spaceAbove);
      top = r.top - 4 - maxH;
    }
    menuStyle = `position: fixed; top: ${Math.max(4, top)}px; left: ${r.left}px; min-width: ${Math.max(r.width, 160)}px; max-height: ${maxH}px;`;
  }
  function openMenu() {
    if (disabled) return;
    placeMenu();
    open = true;
  }
  function toggle() {
    if (open) open = false;
    else openMenu();
  }
  function pick(o: SelectOption) {
    value = o.value;
    open = false;
    dispatch("change", o.value);
  }
  // Portal action: move the menu node to <body> so it's outside the table's
  // DOM subtree (and any overflow / stacking-context traps there).
  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy() { node.remove(); } };
  }
  function onDocClick(e: MouseEvent) {
    if (!open) return;
    const t = e.target as Node;
    if (triggerEl?.contains(t)) return;
    if (menuEl?.contains(t)) return;
    open = false;
  }
  function onScroll() { if (open) open = false; } // close on any scroll (nested included)
  function onKey(e: KeyboardEvent) { if (e.key === "Escape") open = false; }
  onMount(() => {
    document.addEventListener("click", onDocClick, true);
    document.addEventListener("scroll", onScroll, true);
    document.addEventListener("keydown", onKey);
    window.addEventListener("resize", onScroll);
  });
  onDestroy(() => {
    document.removeEventListener("click", onDocClick, true);
    document.removeEventListener("scroll", onScroll, true);
    document.removeEventListener("keydown", onKey);
    window.removeEventListener("resize", onScroll);
  });
</script>

<div class="ns-select">
  <button
    type="button"
    class="ns-sel-trigger"
    on:click={toggle}
    bind:this={triggerEl}
    {disabled}
    aria-haspopup="listbox"
    aria-expanded={open}
  >
    <span class="ns-sel-label">{current()?.label || placeholder || "—"}</span>
    <span class="ns-sel-caret" class:open></span>
  </button>
</div>

{#if open}
  <div bind:this={menuEl} use:portal class="ns-sel-menu" style={menuStyle} role="listbox">
    {#each options as o}
      <button
        type="button"
        class="ns-sel-opt"
        class:sel={o.value === value}
        on:click={() => pick(o)}
        role="option"
        aria-selected={o.value === value}
      >
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
    border-radius: var(--comp-radius); outline: none;
    cursor: pointer;
  }
  .ns-sel-trigger:hover { border-color: var(--text-faint); }
  .ns-sel-trigger:focus { border-color: var(--accent); box-shadow: var(--focus-ring); }
  .ns-sel-trigger:disabled { opacity: 0.45; cursor: not-allowed; }
  .ns-sel-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .ns-sel-caret {
    width: 0; height: 0; flex-shrink: 0;
    border-left: 4px solid transparent; border-right: 4px solid transparent;
    border-top: 5px solid var(--text-faint); opacity: 0.7;
    transition: transform 150ms;
  }
  .ns-sel-caret.open { transform: rotate(180deg); }

  /* Menu — rendered in <body> (portal) + position: fixed (inline style), so
     no parent overflow clips it. z-index high to sit above the table. */
  :global(.ns-sel-menu) {
    z-index: 1000;
    max-height: 280px; overflow: auto;
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--comp-radius);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
    padding: 4px;
    display: flex; flex-direction: column; gap: 1px;
  }
  :global(.ns-sel-opt) {
    text-align: left; width: 100%;
    background: transparent; border: none; color: var(--text-dim);
    padding: 7px 10px; border-radius: calc(var(--comp-radius) - 2px);
    cursor: pointer; font-family: var(--comp-font); font-size: 13px; line-height: 1.2;
    transition: background 100ms, color 100ms;
  }
  :global(.ns-sel-opt:hover) { background: var(--bg-2); color: var(--text); }
  :global(.ns-sel-opt.sel) { background: var(--bg-2); color: var(--accent); }
  :global(.ns-sel-empty) { padding: 8px 10px; color: var(--text-faint); font-size: 12px; }
</style>
