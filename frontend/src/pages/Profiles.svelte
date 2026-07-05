<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/ipc";
  import type { Config, Profile, Rule, Interface } from "../../wailsjs/go/models";

  let config: Config | null = null;
  let interfaces: Interface[] = [];
  let selectedId = "";
  let editing: Profile | null = null;
  let saving = false;
  let errorText = "";
  let fieldErrors: Record<string, string> = {};

  $: activeId = config?.activeProfile ?? "";
  $: selected = config?.profiles.find((p) => p.id === selectedId) ?? null;

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

  async function deleteProfile() {
    if (!editing || !config) return;
    if (!confirm(`删除配置 "${editing.name}"？`)) return;
    try {
      await api.deleteProfile(editing.id);
      await load();
    } catch (e: any) {
      parseError(e);
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

<div class="head">
  <h2>配置</h2>
  <button on:click={newProfile}>+ 新建配置</button>
</div>

{#if errorText}
  <div class="err">{errorText}</div>
{/if}

<div class="layout">
  <aside class="prof-list">
    {#each config?.profiles ?? [] as p}
      <button
        class="prof-item"
        class:active={p.id === selectedId}
        on:click={() => (selectedId = p.id)}
      >
        <span class="prof-name">{p.name}</span>
        <span class="faint mono">{p.id}</span>
        {#if p.id === activeId}
          <span class="tag good">活动</span>
        {/if}
      </button>
    {:else}
      <div class="muted" style="padding:12px">尚无配置，点击右上角新建。</div>
    {/each}
  </aside>

  <div class="editor">
    {#if editing}
      <div class="form-row">
        <label>显示名 <input bind:value={editing.name} /></label>
        <label>配置 ID <input value={editing.id} disabled class="mono" /></label>
      </div>

      <div class="form-row">
        <label>
          默认路由网卡
          <select bind:value={editing.defaultRouteInterface}>
            <option value="">（不管理默认路由）</option>
            {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name} ({ifc.MediaType})</option>{/each}
          </select>
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={editing.autoManageMetrics} />
          自动管理接口跃点数
        </label>
      </div>

      {#if editing.autoManageMetrics}
        <div class="form-row metric-policy">
          <label>
            首选网卡
            <select bind:value={editing.metricPolicy.preferredInterface}>
              <option value="">（用默认路由网卡）</option>
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
        <div class="card" style="padding:0">
          <table>
            <thead>
              <tr><th>目标 CIDR</th><th>接口</th><th>网关</th><th>Metric</th><th>启用</th><th></th></tr>
            </thead>
            <tbody>
              {#each editing.rules as r, i}
                <tr>
                  <td>
                    <input
                      class="mono small {ruleErr(i, 'destination') ? 'invalid' : ''}"
                      value={r.destination}
                      on:input={(e) => ruleField(i, "destination", e.currentTarget.value)}
                    />
                    {#if ruleErr(i, "destination")}<div class="field-err">{ruleErr(i, "destination")}</div>{/if}
                  </td>
                  <td>
                    <select on:change={(e) => ruleField(i, "viaInterface", e.currentTarget.value)}>
                      {#each interfaces as ifc}
                        <option value={ifc.Name} selected={ifc.Name === r.viaInterface}>{ifc.Name}</option>
                      {/each}
                    </select>
                  </td>
                  <td>
                    <input class="mono small" value={r.viaGateway} on:input={(e) => ruleField(i, "viaGateway", e.currentTarget.value)} />
                  </td>
                  <td>
                    <input class="mono small" type="number" min="1" value={r.metric ?? 1} on:input={(e) => ruleField(i, "metric", +e.currentTarget.value)} />
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
        <button class="danger" on:click={deleteProfile} disabled={config?.profiles.length === 1}>删除</button>
      </div>
    {:else}
      <p class="muted">从左侧选择一个配置，或新建一个。</p>
    {/if}
  </div>
</div>

<style>
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
  h2 { margin: 0; font-size: 18px; }
  h3 { margin: 0; font-size: 13px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-dim); }
  .err { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.3); padding: 9px 12px; border-radius: var(--radius-sm); font-size: 12px; margin-bottom: 12px; font-family: var(--font-mono); }
  .layout { display: grid; grid-template-columns: 220px 1fr; gap: 16px; }
  .prof-list { display: flex; flex-direction: column; gap: 2px; }
  .prof-item { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; text-align: left; padding: 8px 10px; background: transparent; border: 1px solid transparent; }
  .prof-item:hover { background: var(--bg-2); }
  .prof-item.active { background: rgba(95,184,255,0.1); border-color: rgba(95,184,255,0.25); }
  .prof-name { font-weight: 600; }
  .editor { min-width: 0; }
  .form-row { display: flex; gap: 16px; margin-bottom: 12px; flex-wrap: wrap; }
  .form-row label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-dim); flex: 1; min-width: 180px; }
  .form-row label.check { flex-direction: row; align-items: center; gap: 8px; }
  .small { padding: 4px 6px; font-size: 12px; min-width: 120px; }
  .rules-head { display: flex; align-items: center; justify-content: space-between; margin: 14px 0 8px; }
  .field-err { color: var(--bad); font-size: 11px; margin-top: 3px; }
  .actions { display: flex; gap: 8px; margin-top: 16px; }
</style>
