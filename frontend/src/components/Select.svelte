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
  export let classExtra = "";

  const dispatch = createEventDispatcher();
  let open = false;
  let root: HTMLElement;

  const current = () => options.find((o) => o.value === value);

  function toggle() {
    if (disabled) return;
    open = !open;
  }
  function pick(o: SelectOption) {
    value = o.value;
    open = false;
    dispatch("change", o.value);
  }
  function onDocClick(e: MouseEvent) {
    if (root && !root.contains(e.target as Node)) open = false;
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") open = false;
  }
  onMount(() => {
    document.addEventListener("click", onDocClick);
    document.addEventListener("keydown", onKey);
  });
  onDestroy(() => {
    document.removeEventListener("click", onDocClick);
    document.removeEventListener("keydown", onKey);
  });
</script>

<div class="ns-select" bind:this={root}>
  <button
    type="button"
    class="ns-sel-trigger {classExtra}"
    on:click={toggle}
    {disabled}
    aria-haspopup="listbox"
    aria-expanded={open}
  >
    <span class="ns-sel-label">{current()?.label || placeholder || "—"}</span>
    <span class="ns-sel-caret" class:open></span>
  </button>
  {#if open}
    <div class="ns-sel-menu" role="listbox">
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
</div>

<style>
  .ns-select { position: relative; display: inline-block; width: 100%; }

  /* Trigger looks like a themed button + chevron. Uses CSS vars from app.css
     so it follows the active theme. */
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

  /* CSS chevron (no font glyph dependency). */
  .ns-sel-caret {
    width: 0; height: 0; flex-shrink: 0;
    border-left: 4px solid transparent; border-right: 4px solid transparent;
    border-top: 5px solid var(--text-faint); opacity: 0.7;
    transition: transform 150ms;
  }
  .ns-sel-caret.open { transform: rotate(180deg); }

  /* Popup menu — the part native <option> can't theme. */
  .ns-sel-menu {
    position: absolute; top: calc(100% + 4px); left: 0; z-index: 50;
    min-width: 100%; max-height: 280px; overflow: auto;
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--comp-radius);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
    padding: 4px;
    display: flex; flex-direction: column; gap: 1px;
  }
  .ns-sel-opt {
    text-align: left; width: 100%;
    background: transparent; border: none; color: var(--text-dim);
    padding: 7px 10px; border-radius: calc(var(--comp-radius) - 2px);
    cursor: pointer; font-family: var(--comp-font); font-size: 13px; line-height: 1.2;
    transition: background 100ms, color 100ms;
  }
  .ns-sel-opt:hover { background: var(--bg-2); color: var(--text); }
  .ns-sel-opt.sel { background: var(--bg-2); color: var(--accent); }
  .ns-sel-empty { padding: 8px 10px; color: var(--text-faint); font-size: 12px; }
</style>
