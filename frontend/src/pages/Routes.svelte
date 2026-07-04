<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/ipc";
  import type { RouteRow } from "../../wailsjs/go/models";

  let rows: RouteRow[] = [];
  let loading = false;
  let search = "";
  let errorText = "";

  onMount(refresh);

  async function refresh() {
    loading = true;
    errorText = "";
    try {
      rows = await api.getRouteTable();
    } catch (e: any) {
      errorText = "读取路由表失败：" + (e?.message ?? e);
    } finally {
      loading = false;
    }
  }

  $: filtered = rows.filter((r) => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return (
      r.destinationPrefix.toLowerCase().includes(q) ||
      r.nextHop.toLowerCase().includes(q) ||
      r.interfaceAlias.toLowerCase().includes(q)
    );
  });

  function sourceTag(s: string) {
    if (s === "managed") return { cls: "good", text: "本工具" };
    if (s === "suspect") return { cls: "vpn", text: "疑似 VPN" };
    return { cls: "", text: "系统" };
  }
</script>

<div class="head">
  <div>
    <h2>路由表</h2>
    <div class="muted" style="margin-top:2px">来自 Get-NetRoute，可搜索、按来源着色。</div>
  </div>
  <div class="actions">
    <input placeholder="搜索目标 / 下一跳 / 接口…" bind:value={search} />
    <button on:click={refresh} disabled={loading}>{loading ? "刷新中…" : "刷新"}</button>
  </div>
</div>

{#if errorText}
  <div class="err">{errorText}</div>
{/if}

<div class="card" style="padding:0">
  <table>
    <thead>
      <tr><th>来源</th><th>目标</th><th>下一跳</th><th>接口</th><th>Route</th><th>Iface</th></tr>
    </thead>
    <tbody>
      {#each filtered as r}
        <tr>
          <td><span class="tag {sourceTag(r.source).cls}">{sourceTag(r.source).text}</span></td>
          <td class="mono">{r.destinationPrefix}</td>
          <td class="mono">{r.nextHop}</td>
          <td>{r.interfaceAlias} <span class="faint mono">#{r.interfaceIndex}</span></td>
          <td class="mono faint">{r.routeMetric}</td>
          <td class="mono faint">{r.interfaceMetric}</td>
        </tr>
      {:else}
        <tr><td colspan="6" class="muted" style="text-align:center;padding:24px">{loading ? "加载中…" : "（无路由）"}</td></tr>
      {/each}
    </tbody>
  </table>
</div>
<p class="faint" style="margin-top:8px">共 {filtered.length} 条{search ? "（已筛选）" : ""}。</p>

<style>
  .head { display: flex; align-items: flex-end; justify-content: space-between; margin-bottom: 14px; gap: 12px; }
  h2 { margin: 0; font-size: 18px; }
  .actions { display: flex; gap: 8px; }
  .err { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.3); padding: 9px 12px; border-radius: var(--radius-sm); font-size: 12px; margin-bottom: 12px; }
</style>
