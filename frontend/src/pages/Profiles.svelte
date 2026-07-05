<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/ipc";
  import type { Config, Profile, Rule, Interface } from "../../wailsjs/go/models";

  let config: Config | null = null;
  let interfaces: Interface[] = [];
  let selectedId = "";
  let editing: Profile | null = null;
  let saving = false;
  let deleting = false;
  let pendingDelete = false;
  let errorText = "";
  let fieldErrors: Record<string, string> = {};

  $: activeId = config?.activeProfile ?? "";
  // Double optional chaining: config.profiles can be null when the on-disk
  // config has no profiles section (Go marshals a nil slice as `null`). A
  // bare `config?.profiles.find` would throw `null.find is not a function`
  // and break Svelte's reactivity for the whole app.
  $: selected = config?.profiles?.find((p) => p.id === selectedId) ?? null;

  onMount(load);

  async function load() {
    try {
      const [cfg, st] = await Promise.all([api.getConfig(), api.getStatus()]);
      config = cfg;
      interfaces = st.interfaces ?? [];
      if (!selectedId && cfg.profiles.length) selectedId = cfg.profiles[0].id;
      prepareEditing();
    } catch (e: any) {
      errorText = "加载配置失败：" + (e?.message ?? e);
    }
  }

  function prepareEditing() {
    if (selected) {
      // Deep-clone so edits are local until Save.
      editing = JSON.parse(JSON.stringify(selected));
    } else {
      editing = null;
    }
    fieldErrors = {};
    errorText = "";
  }

  $: if (selectedId) prepareEditing();

  function newProfile() {
    const id = "profile-" + Math.random().toString(36).slice(2, 8);
    const p: Profile = {
      id,
      name: "新配置",
      rules: [],
      defaultRouteInterface: "",
      autoManageMetrics: true,
      metricPolicy: { preferredMetric: 10, othersMetric: 50 },
    };
    // Reassign (not push) so Svelte sees the change and re-renders the list.
    config = { ...(config ?? { version: 1, activeProfile: "", profiles: [] }), profiles: [...(config?.profiles ?? []), p] };
    selectedId = id;
  }

  function addRule() {
    if (!editing) return;
    // Reassign the array (not push) so Svelte re-renders the rule table.
    editing = {
      ...editing,
      rules: [
        ...editing.rules,
        {
          id: "r" + (editing.rules.length + 1) + "-" + Math.random().toString(36).slice(2, 6),
          destination: "10.0.0.0/24",
          viaInterface: interfaces[0]?.Name ?? "",
          viaGateway: "auto",
          metric: 1,
          enabled: true,
        },
      ],
    };
  }

  function removeRule(idx: number) {
    if (!editing) return;
    editing.rules = editing.rules.filter((_, i) => i !== idx);
    editing = { ...editing };
  }

  function ruleField(idx: number, field: keyof Rule, value: any) {
    if (!editing) return;
    const rules = [...editing.rules];
    rules[idx] = { ...rules[idx], [field]: value };
    editing = { ...editing, rules };
  }

  // viaGateway is "auto" (use the NIC's primary gateway) or a literal IPv4.
  // Loaded configs are normalized by config.applyDefaults, so "" only appears
  // transiently after clicking "指定 IP" — it means "specify mode, awaiting
  // input", not auto. Without this distinction the 切换 button silently no-ops.
  function isAutoGateway(gw: string): boolean {
    return gw.toLowerCase() === "auto";
  }

  function resolvedGatewayFor(ifaceName: string): string {
    return interfaces.find((ifc) => ifc.Name === ifaceName)?.Gateways?.[0] ?? "";
  }

  async function save() {
    if (!editing) return;
    saving = true;
    errorText = "";
    fieldErrors = {};
    try {
      await api.saveProfile(editing);
      await load();
    } catch (e: any) {
      parseError(e);
    } finally {
      saving = false;
    }
  }

  function deleteProfile() {
    if (!editing) return;
    // Show the styled confirmation modal (Wails/WebView2 blocks the native
    // JS confirm() dialog, so we use our own).
    pendingDelete = true;
  }

  async function confirmDelete() {
    if (!editing) return;
    pendingDelete = false;
    deleting = true;
    errorText = "";
    try {
      await api.deleteProfile(editing.id);
      // If the deleted one was selected, fall back to the first remaining
      // profile (or clear selection if none remain).
      const rest = (config?.profiles ?? []).filter((p) => p.id !== editing!.id);
      selectedId = rest.length ? rest[0].id : "";
      await load();
    } catch (e: any) {
      parseError(e);
    } finally {
      deleting = false;
    }
  }

  async function setActive() {
    if (!editing) return;
    try {
      await api.setActiveProfile(editing.id);
      await load();
    } catch (e: any) {
      parseError(e);
    }
  }

  function parseError(e: any) {
    const msg = e?.message ?? String(e);
    errorText = msg;
    // IPC error strings look like:
    //   "INVALID_CONFIG: profiles[0].rules[1].destination: bad CIDR; ..."
    const body = msg.split(":").slice(1).join(":").trim();
    const parts = body.split(";");
    const fe: Record<string, string> = {};
    for (const p of parts) {
      const m = p.trim().match(/^(profiles\[[^\]]*\](?:\.rules\[[^\]]*\]\.[a-zA-Z]+)?)/);
      if (m) fe[m[1]] = p.trim();
    }
    fieldErrors = fe;
  }

  function ruleErr(idx: number, field: string) {
    return fieldErrors[`profiles[0].rules[${idx}].${field}`] ?? "";
  }
</script>

<div class="profiles-page">
  <div class="head">
    <h2>配置</h2>
    <button on:click={newProfile}>+ 新建配置</button>
  </div>

  {#if errorText}
    <div class="err">{errorText}</div>
  {/if}

  {#if (config?.profiles ?? []).length > 0}
  <div class="layout">
  <aside class="prof-list">
    {#each config?.profiles ?? [] as p}
      <button
        class="prof-item"
        class:active={p.id === selectedId}
        on:click={() => (selectedId = p.id)}
      >
        <span class="prof-name">{p.name}</span>
        {#if p.id === activeId}
          <span class="tag good">活动</span>
        {/if}
      </button>
    {/each}
  </aside>

  <div class="editor">
    {#if editing}
      <div class="form-row">
        <label>显示名 <input bind:value={editing.name} /></label>
      </div>

      <div class="form-row">
        <label>
          默认路由网卡
          <select bind:value={editing.defaultRouteInterface}>
            <option value="">(不管理默认路由)</option>
            {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name} ({ifc.MediaType})</option>{/each}
          </select>
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={editing.autoManageMetrics} />
          自动管理接口跃点数
        </label>
      </div>

      {#if editing.autoManageMetrics && editing.metricPolicy}
        <div class="form-row metric-policy">
          <label>
            首选网卡
            <select bind:value={editing.metricPolicy.preferredInterface}>
              <option value="">(用默认路由网卡)</option>
              {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name}</option>{/each}
            </select>
          </label>
          <label>首选 Metric <input type="number" min="1" bind:value={editing.metricPolicy.preferredMetric} /></label>
          <label>其他 Metric <input type="number" min="1" bind:value={editing.metricPolicy.othersMetric} /></label>
        </div>
      {/if}

      <div class="rules">
        <div class="rules-head">
          <h3>规则 ({editing.rules.length})</h3>
          <button on:click={addRule}>+ 添加规则</button>
        </div>
        <div class="card rule-table-wrap" style="padding:0">
          <table>
            <thead>
              <tr><th>目标 CIDR</th><th>接口</th><th>网关</th><th>Metric</th><th>启用</th><th></th></tr>
            </thead>
            <tbody>
              {#each editing.rules as r, i}
                <tr>
                  <td>
                    <input
                      class="cell-input mono {ruleErr(i, 'destination') ? 'invalid' : ''}"
                      value={r.destination}
                      on:input={(e) => ruleField(i, "destination", e.currentTarget.value)}
                    />
                    {#if ruleErr(i, "destination")}<div class="field-err">{ruleErr(i, "destination")}</div>{/if}
                  </td>
                  <td>
                    <select class="cell-input" on:change={(e) => ruleField(i, "viaInterface", e.currentTarget.value)}>
                      {#each interfaces as ifc}
                        <option value={ifc.Name} selected={ifc.Name === r.viaInterface}>{ifc.Name}</option>
                      {/each}
                    </select>
                  </td>
                  <td>
                    <div class="gw">
                      {#if isAutoGateway(r.viaGateway)}
                        <div class="gw-auto">
                          <span class="gw-resolved" title="用所选网卡的主网关">
                            ↳ {resolvedGatewayFor(r.viaInterface) || "网卡无网关"}
                          </span>
                          <button class="seg" on:click={() => ruleField(i, "viaGateway", "")}>指定 IP</button>
                        </div>
                      {:else}
                        <div class="gw-specify">
                          <input
                            class="cell-input mono {ruleErr(i, 'viaGateway') ? 'invalid' : ''}"
                            placeholder="192.168.1.1"
                            value={r.viaGateway}
                            on:input={(e) => ruleField(i, "viaGateway", e.currentTarget.value)}
                          />
                          <button class="seg" on:click={() => ruleField(i, "viaGateway", "auto")}>改自动</button>
                        </div>
                      {/if}
                    </div>
                    {#if !isAutoGateway(r.viaGateway) && ruleErr(i, "viaGateway")}
                      <div class="field-err">{ruleErr(i, "viaGateway")}</div>
                    {/if}
                  </td>
                  <td>
                    <input class="cell-input mono" type="number" min="1" value={r.metric ?? 1} on:input={(e) => ruleField(i, "metric", +e.currentTarget.value)} />
                  </td>
                  <td style="text-align:center">
                    <input type="checkbox" checked={r.enabled !== false} on:change={(e) => ruleField(i, "enabled", e.currentTarget.checked)} />
                  </td>
                  <td><button class="ghost danger" on:click={() => removeRule(i)}>×</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="actions">
        <button class="primary" on:click={save} disabled={saving}>{saving ? "保存中…" : "保存"}</button>
        <button on:click={setActive} disabled={editing.id === activeId}>设为活动</button>
        <button class="danger" on:click={deleteProfile} disabled={deleting}>{deleting ? "删除中…" : "删除"}</button>
      </div>
    {:else}
      <p class="muted">从左侧选择一个配置查看或编辑。</p>
    {/if}
  </div>
</div>
  {:else}
    <div class="empty-state">
      <h3>还没有路由配置</h3>
      <p class="muted">新建一个配置，添加"哪些网段走哪块网卡"的规则，<br />NetSwitcher 会自动维护，网络变化也会重新下发。</p>
      <button class="primary" on:click={newProfile}>+ 新建第一个配置</button>
    </div>
  {/if}
</div>

{#if pendingDelete}
  <div class="modal-backdrop" on:click={() => (pendingDelete = false)}>
    <div class="modal" role="dialog" aria-modal="true" on:click|stopPropagation>
      <h3>删除配置</h3>
      <p>确定删除配置 <strong>“{editing?.name}”</strong> 吗？</p>
      <ul class="modal-bullets">
        <li>该 profile 下的所有规则一并删除</li>
        <li>如果它是活动配置，活动状态会清空（路由回退到系统默认）</li>
        <li>操作不可撤销</li>
      </ul>
      <div class="modal-actions">
        <button on:click={() => (pendingDelete = false)} disabled={deleting}>取消</button>
        <button class="danger" on:click={confirmDelete} disabled={deleting}>
          {deleting ? "删除中…" : "确认删除"}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Page fills .content so the empty state can vertically center in the
     available space (same pattern as Logs/Diagnostics). */
  .profiles-page { height: 100%; display: flex; flex-direction: column; min-height: 0; }
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; flex-shrink: 0; }
  h2 { margin: 0; font-size: 18px; }
  h3 { margin: 0; font-size: 13px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-dim); }
  .err { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.3); padding: 9px 12px; border-radius: var(--radius-sm); font-size: 12px; margin-bottom: 12px; font-family: var(--font-mono); flex-shrink: 0; }
  .layout { display: grid; grid-template-columns: 220px 1fr; gap: 16px; flex: 1; min-height: 0; }
  .prof-list { display: flex; flex-direction: column; gap: 2px; }
  .prof-item { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; text-align: left; padding: 8px 10px; background: transparent; border: 1px solid transparent; }
  .prof-item:hover { background: var(--bg-2); }
  .prof-item.active { background: rgba(95,184,255,0.1); border-color: rgba(95,184,255,0.25); }
  .prof-name { font-weight: 600; }
  .editor { min-width: 0; }
  .form-row { display: flex; gap: 16px; margin-bottom: 12px; flex-wrap: wrap; }
  .form-row label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-dim); flex: 1; min-width: 180px; }
  .form-row label.check { flex-direction: row; align-items: center; gap: 8px; }
  /* Rule-table inputs fill their cell instead of forcing min-width (which
     made the row wider than the table and spilled out). */
  .cell-input {
    width: 100%;
    box-sizing: border-box;
    padding: 4px 6px;
    font-size: 12px;
  }
  /* Let the rule table scroll horizontally on very narrow windows instead of
     overflowing the card. */
  .rule-table-wrap { overflow-x: auto; }
  .rule-table-wrap table { min-width: 560px; }
  .rules-head { display: flex; align-items: center; justify-content: space-between; margin: 14px 0 8px; }
  .field-err { color: var(--bad); font-size: 11px; margin-top: 3px; }
  .actions { display: flex; gap: 8px; margin-top: 16px; }

  /* No-profiles empty state — rendered as a direct flex child of .profiles-page
     (the .layout grid is not rendered at all when there are no profiles), so
     flex:1 fills the whole page below the head and centers across the full
     width (not just the editor column). */
  .empty-state {
    flex: 1; min-height: 0;
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    text-align: center; gap: 12px; padding: 32px 24px;
  }
  .empty-state h3 { margin: 0; font-size: 16px; text-transform: none; letter-spacing: 0; color: var(--text); }
  .empty-state p { margin: 0; font-size: 13px; line-height: 1.6; max-width: 420px; }

  /* Gateway cell: switch between "auto" (use NIC gateway) and a literal IP. */
  .gw { display: flex; flex-direction: column; gap: 3px; }
  .gw-auto, .gw-specify { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .gw-specify input { flex: 1; min-width: 0; }
  .gw-resolved {
    font-family: var(--font-mono); font-size: 11px; color: var(--text-dim);
    flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .seg {
    padding: 3px 8px; font-size: 11px;
    background: transparent; border: 1px solid var(--border);
    color: var(--text-faint); border-radius: var(--radius-sm);
    cursor: pointer; white-space: nowrap; flex-shrink: 0;
  }
  .seg:hover { color: var(--accent); border-color: var(--accent-dim); background: var(--bg-2); }

  /* Delete confirmation modal. */
  .modal-backdrop {
    position: fixed; inset: 0; background: rgba(8,10,15,0.72);
    display: flex; align-items: center; justify-content: center; z-index: 60;
  }
  .modal {
    background: var(--bg-1); border: 1px solid var(--border); border-radius: 12px;
    padding: 24px 26px; max-width: 420px; box-shadow: 0 10px 40px rgba(0,0,0,0.5);
  }
  .modal h3 { margin: 0 0 10px; font-size: 16px; text-transform: none; letter-spacing: 0; color: var(--text); }
  .modal p { margin: 6px 0; font-size: 13px; line-height: 1.55; }
  .modal-bullets { margin: 8px 0; padding-left: 20px; font-size: 12px; color: var(--text-dim); line-height: 1.7; }
  .modal-bullets li::marker { color: var(--bad); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px; }
</style>
