<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import { api } from "../lib/ipc";
  import type { StatusResponse, Interface, ApplyResult, Conflict } from "../../wailsjs/go/models";

  export let status: StatusResponse | null = null;
  export let serviceUp = false;
  const dispatch = createEventDispatcher();

  let applying = false;

  async function applyNow() {
    applying = true;
    try {
      await api.applyNow();
    } catch (e: any) {
      alert("应用失败：" + (e?.message ?? e));
    } finally {
      applying = false;
    }
  }

  $: last = status?.lastResult as ApplyResult | undefined;
  $: applied = last?.applied ?? [];
  // Conflicts the user has dismissed this session (by type:interface sig).
  // Reset implicitly when the process restarts. Reappears if the underlying
  // conflict set changes (different signature).
  let dismissed = new Set<string>();
  $: allConflicts = (status?.conflicts ?? []) as Conflict[];
  const conflictSig = (c: Conflict) => `${c.type}:${c.interface}`;
  $: conflicts = allConflicts.filter((c) => !dismissed.has(conflictSig(c)));
  $: interfaces = (status?.interfaces ?? []) as Interface[];

  function stateTag(ifc: Interface) {
    if (!ifc.IsUp) return { cls: "bad", text: "已断开" };
    return { cls: "good", text: "已连接" };
  }

  function dismissConflict(c: Conflict) {
    dismissed = new Set(dismissed);
    dismissed.add(conflictSig(c));
  }
</script>

<div class="head">
  <div>
    <h2>状态</h2>
    <div class="muted" style="margin-top:2px">
      活动配置：
      {#if status?.activeProfile}
        <strong>{status.activeProfile.name}</strong>
        <span class="faint">({status.activeProfile.id})</span>
      {:else}
        <strong>（无）</strong>
        <span class="faint">— 未配置自动分流，路由走系统默认</span>
      {/if}
    </div>
  </div>
  <button class="primary" on:click={applyNow} disabled={!serviceUp || applying}>
    {applying ? "应用中…" : "立即重新应用"}
  </button>
</div>

{#if conflicts.length > 0}
  <div class="conflicts">
    <h3>网络提示 ({conflicts.length})</h3>
    {#each conflicts as c}
      <div class="conflict-row">
        <span class="tag {c.type === 'vpn_present' ? 'vpn' : 'warn'}">
          {c.type === "vpn_present" ? "VPN" : "外部覆盖"}
        </span>
        <span class="conflict-text">{c.description}</span>
        <button class="dismiss" title="本次会话不再显示" on:click={() => dismissConflict(c)}>×</button>
      </div>
    {/each}
  </div>
{:else if allConflicts.length > 0}
  <div class="conflicts muted-bar">
    已忽略 {allConflicts.length} 条提示（重启或网络变化后重新评估）。
  </div>
{/if}

<section>
  <h3>网络接口 ({interfaces.length})</h3>
  {#if interfaces.length === 0}
    <p class="muted">暂未枚举到接口。</p>
  {:else}
    <div class="iface-grid">
      {#each interfaces as ifc}
        <div class="card iface" class:down={!ifc.IsUp}>
          <div class="iface-head">
            <span class="iface-name">{ifc.Name}</span>
            <span class="tag {stateTag(ifc).cls}">{stateTag(ifc).text}</span>
          </div>
          <div class="iface-desc faint">{ifc.FriendlyName}</div>
          <dl>
            <dt>IPv4</dt>
            <dd class="mono">{ifc.IPv4?.length ? ifc.IPv4.join(", ") : "—"}</dd>
            <dt>网关</dt>
            <dd class="mono">{ifc.Gateways?.length ? ifc.Gateways.join(", ") : "—"}</dd>
            <dt>类型 / Index</dt>
            <dd><span class="tag">{ifc.MediaType}</span> <span class="faint mono">#{ifc.Index}</span></dd>
          </dl>
        </div>
      {/each}
    </div>
  {/if}
</section>

<section>
  <h3>已下发路由 ({applied.length})</h3>
  {#if !applied.length}
    <p class="muted">本工具当前未下发任何路由。</p>
  {:else}
    <div class="card" style="padding:0">
      <table>
        <thead>
          <tr><th>目标</th><th>下一跳</th><th>接口</th><th>Index</th><th>Metric</th></tr>
        </thead>
        <tbody>
          {#each applied as r}
            <tr>
              <td class="mono">{r.destination}</td>
              <td class="mono">{r.gateway}</td>
              <td>{r.interface}</td>
              <td class="mono faint">{r.ifIndex}</td>
              <td class="mono">{r.metric}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
  {#if last?.skipped?.length}
    <div class="skipped">
      <strong>跳过 {last.skipped.length} 条规则：</strong>
      {#each last.skipped as s}
        <span class="skip">{s.destination} → {s.reason}</span>
      {/each}
    </div>
  {/if}
  {#if last?.errors?.length}
    <div class="skipped bad-bg">
      <strong>{last.errors.length} 条错误：</strong>
      {#each last.errors as e}
        <span class="skip">{e.op}: {e.message}</span>
      {/each}
    </div>
  {/if}
</section>

<section class="muted faint" style="margin-top:18px">
  最近一次 apply: {last?.at ? new Date(last.at).toLocaleString() : "—"}
  · 原因: <span class="mono">{last?.reason ?? "—"}</span>
</section>

<style>
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
  h2 { margin: 0; font-size: 18px; }
  h3 { margin: 18px 0 10px; font-size: 14px; color: var(--text-dim); text-transform: uppercase; letter-spacing: 0.05em; }
  .conflicts { background: rgba(192,132,252,0.06); border: 1px solid rgba(192,132,252,0.25); border-radius: var(--radius); padding: 12px 14px; margin-bottom: 8px; }
  .conflicts h3 { margin: 0 0 8px; } /* override the 18px section-title top margin */
  .conflict-row { display: flex; align-items: center; gap: 10px; padding: 4px 0; font-size: 13px; }
  .conflict-text { flex: 1; }
  .dismiss { background: transparent; border: none; color: var(--text-faint); font-size: 16px; line-height: 1; padding: 0 6px; cursor: pointer; }
  .dismiss:hover { color: var(--text); }
  .muted-bar { color: var(--text-faint); font-size: 12px; }
  .iface-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 12px; }
  .iface.down { opacity: 0.6; }
  .iface-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .iface-name { font-weight: 600; font-size: 15px; }
  .iface-desc { font-size: 11px; margin: 2px 0 8px; min-height: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  dl { margin: 0; display: grid; grid-template-columns: 72px 1fr; gap: 4px 8px; }
  dt { color: var(--text-faint); font-size: 11px; }
  dd { margin: 0; font-size: 12px; }
  .skipped { margin-top: 10px; padding: 10px 12px; background: var(--bg-2); border: 1px solid var(--border); border-radius: var(--radius-sm); font-size: 12px; }
  .skip { display: inline-block; margin: 2px 8px 2px 0; color: var(--text-dim); font-family: var(--font-mono); }
  .bad-bg { background: rgba(248,113,113,0.06); border-color: rgba(248,113,113,0.25); }
</style>
