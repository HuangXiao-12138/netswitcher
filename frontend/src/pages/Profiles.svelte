<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api, events, EVT } from "../lib/ipc";
  import type { Config, Profile, Rule, Interface, StatusResponse, NrptRule } from "../../wailsjs/go/models";

  let config: Config | null = null;
  let interfaces: Interface[] = [];
  let selectedId = "";
  let editing: Profile | null = null;
  let saving = false;
  let deleting = false;
  let pendingDelete = false;
  let errorText = "";
  let fieldErrors: Record<string, string> = {};
  let advOpen = false;
  let systemDefaultIf = ""; // the live default-route interface name

  $: activeId = config?.activeProfile ?? "";
  // Active profile is read-only in the editor — change it via "停用" first,
  // then edit, then "设为活动". Matches the user's mental model.
  $: isActive = !!selected && selected.id === activeId;
  // Double optional chaining: config.profiles can be null when the on-disk
  // config has no profiles section (Go marshals a nil slice as `null`).
  $: selected = config?.profiles?.find((p) => p.id === selectedId) ?? null;

  onMount(load);

  // Refresh the interface dropdowns when the network changes. The backend
  // pushes status:changed after every apply (startup / network_change /
  // config_change); mirror its interface list into the rule & advanced
  // dropdowns so enabling/disabling adapters shows up without reopening the
  // page. offStatus is Wails EventsOn's per-callback cancel (EventsOff(name)
  // would also remove App's listener).
  let offStatus: (() => void) | null = null;
  onMount(() => {
    offStatus = events.on(EVT.statusChanged, (st: StatusResponse) => {
      if (st?.interfaces) interfaces = st.interfaces;
    });
  });
  onDestroy(() => {
    try { offStatus?.(); } catch {}
  });

  async function load() {
    try {
      const [cfg, st, defaultIf] = await Promise.all([api.getConfig(), api.getStatus(), api.getDefaultRouteInterface().catch(() => "")]);
      config = cfg;
      interfaces = st.interfaces ?? [];
      systemDefaultIf = defaultIf || "";
      if (!selectedId && cfg.profiles.length) selectedId = cfg.profiles[0].id;
      // Set the working copy directly from the FRESH cfg. Don't call
      // prepareEditing() here: `selected` is a reactive that lags `config=cfg`
      // by a tick, so reading it synchronously returns the pre-save profile
      // and reverts the user's edits. (The `$: if (selectedId) prepareEditing`
      // reactive only depends on selectedId, so it does NOT re-run on a config
      // change either — load() is the only path that refreshes editing after
      // save.)
      setEditingFrom(cfg.profiles.find((p) => p.id === selectedId) ?? null);
    } catch (e: any) {
      errorText = "加载配置失败：" + (e?.message ?? e);
    }
  }

  function setEditingFrom(src: Profile | null) {
    editing = src ? JSON.parse(JSON.stringify(src)) : null;
    if (editing) {
      // Normalize optional fields from undefined→"" so <select bind:value>
      // doesn't sync undefined→"" on mount and falsely mark the form dirty
      // (the "open Advanced → 有未保存的修改" bug).
      editing.defaultRouteInterface = editing.defaultRouteInterface ?? "";
      if (editing.metricPolicy) {
        editing.metricPolicy.preferredInterface = editing.metricPolicy.preferredInterface ?? "";
      }
    }
    fieldErrors = {};
    errorText = "";
  }

  // Normalize optional fields for dirty comparison. The backend omits empty
  // defaultRouteInterface / preferredInterface (undefined in JS); setEditingFrom
  // normalizes editing to "" so <select bind:value> doesn't mutate it on mount.
  // To compare without false positives, BOTH sides go through norm() —
  // otherwise the "" vs undefined difference always reads as dirty.
  function norm(p: Profile | null): Profile | null {
    if (!p) return p;
    const c = JSON.parse(JSON.stringify(p));
    c.defaultRouteInterface = c.defaultRouteInterface ?? "";
    if (c.metricPolicy) {
      c.metricPolicy.preferredInterface = c.metricPolicy.preferredInterface ?? "";
    }
    return c;
  }

  // editing is derived directly from `selected` (which itself tracks selectedId
  // + config). New profile / switch / delete → selected changes → editing
  // re-derives. MUST be a direct call with `selected` inline so svelte tracks
  // the dependency — wrapping in a function ($: prepareEditing()) fails to
  // re-run because svelte doesn't see selected read inside the wrapper.
  $: setEditingFrom(selected);

  // Dirty flag: deep-compare the working copy to the saved original. Drives
  // the "有未保存的修改" pip and disables/enables the Save button.
  $: dirty = editing && selected ? JSON.stringify(norm(editing)) !== JSON.stringify(norm(selected)) : false;
  $: enabledCount = editing ? editing.rules.filter((r) => r.enabled !== false).length : 0;

  function newProfile() {
    if (config === null) return; // still loading; load() returning would clobber this
    const id = "profile-" + Math.random().toString(36).slice(2, 8);
    const p: Profile = {
      id,
      name: "新配置",
      rules: [],
      defaultRouteInterface: "",
      autoManageMetrics: true,
      metricPolicy: { preferredMetric: 10, othersMetric: 50 },
      nrptRules: [],
    };
    config = { ...(config ?? { version: 1, activeProfile: "", profiles: [] }), profiles: [...(config?.profiles ?? []), p] };
    selectedId = id;
  }

  function addRule() {
    if (!editing) return;
    editing = {
      ...editing,
      rules: [
        ...editing.rules,
        {
          id: "r" + (editing.rules.length + 1) + "-" + Math.random().toString(36).slice(2, 6),
          destination: "",
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

  // NRPT (domain-suffix → DNS) rule helpers. Mirrors rules[] helpers but for
  // nrptRules[]: a domain suffix + comma-separated DNS servers. The backend
  // turns domain "luculent.vip" into NRPT namespace ".luculent.vip" so it
  // matches *.luculent.vip.
  function addNrpt() {
    if (!editing) return;
    const nrpt = editing.nrptRules ?? [];
    editing = {
      ...editing,
      nrptRules: [...nrpt, { id: "n" + nrpt.length + "-" + Math.random().toString(36).slice(2, 5), domain: "", nameServers: [], enabled: true }],
    };
  }
  function removeNrpt(idx: number) {
    if (!editing?.nrptRules) return;
    editing.nrptRules = editing.nrptRules.filter((_, i) => i !== idx);
    editing = { ...editing };
  }
  function nrptField(idx: number, field: "domain" | "enabled", value: any) {
    if (!editing?.nrptRules) return;
    const ns = [...editing.nrptRules];
    ns[idx] = { ...ns[idx], [field]: value };
    editing = { ...editing, nrptRules: ns };
  }
  function nrptServers(idx: number, value: string) {
    if (!editing?.nrptRules) return;
    const servers = value.split(",").map((s) => s.trim()).filter(Boolean);
    const ns = [...editing.nrptRules];
    ns[idx] = { ...ns[idx], nameServers: servers };
    editing = { ...editing, nrptRules: ns };
  }

  function ruleField(idx: number, field: keyof Rule, value: any) {
    if (!editing) return;
    const rules = [...editing.rules];
    rules[idx] = { ...rules[idx], [field]: value };
    editing = { ...editing, rules };
  }

  function isAutoGateway(gw: string): boolean {
    return gw.toLowerCase() === "auto";
  }
  function resolvedGatewayFor(ifaceName: string): string {
    return interfaces.find((ifc) => ifc.Name === ifaceName)?.Gateways?.[0] ?? "";
  }
  // Gateway field is mode-driven via a <select>: "auto" (resolve from the
  // NIC) or "custom" (explicit IP). Switching to custom seeds the input with
  // the currently-resolved gateway so there's a starting value.
  function gatewayMode(gw: string): "auto" | "custom" {
    return isAutoGateway(gw) ? "auto" : "custom";
  }
  function setGatewayMode(idx: number, mode: "auto" | "custom") {
    if (!editing) return;
    const r = editing.rules[idx];
    if (mode === "auto") {
      ruleField(idx, "viaGateway", "auto");
    } else if (isAutoGateway(r.viaGateway)) {
      ruleField(idx, "viaGateway", resolvedGatewayFor(r.viaInterface) || "");
    }
  }

  async function save() {
    if (!editing || !dirty) return;
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
    pendingDelete = true;
  }
  async function confirmDelete() {
    if (!editing) return;
    deleting = true;
    errorText = "";
    try {
      await api.deleteProfile(editing.id);
      // Backend delete finished BEFORE we close the modal + refresh the list,
      // so the left-pane card disappears in the same frame as the modal close
      // (no async gap where the card lingers).
      const rest = (config?.profiles ?? []).filter((p) => p.id !== editing!.id);
      config = { ...(config as Config), profiles: rest };
      selectedId = rest.length ? rest[0].id : "";
    } catch (e: any) {
      parseError(e);
    } finally {
      deleting = false;
      pendingDelete = false;
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

  async function deactivate() {
    try {
      await api.deactivateProfile();
      await load();
    } catch (e: any) {
      parseError(e);
    }
  }

  function parseError(e: any) {
    const msg = e?.message ?? String(e);
    errorText = msg;
    const body = msg.split(":").slice(1).join(":").trim();
    const parts = body.split(";");
    const fe: Record<string, string> = {};
    for (const p of parts) {
      // Normalize the key to rules[idx].field regardless of which profiles[N]
      // the backend references — the editor only shows one profile at a time,
      // so the profile index is irrelevant for matching field errors to rows.
      const m = p.trim().match(/^profiles\[\d+\]\.rules\[(\d+)\]\.([a-zA-Z]+)/);
      if (m) fe[`rules[${m[1]}].${m[2]}`] = p.trim();
    }
    fieldErrors = fe;
  }
  function ruleErr(idx: number, field: string) {
    return fieldErrors[`rules[${idx}].${field}`] ?? "";
  }
</script>

<div class="profiles-page">
  <div class="page-head">
    <div>
      <h2>路由配置</h2>
      <div class="dim head-sub">
        {#if (config?.profiles ?? []).length}{(config?.profiles ?? []).length} 个配置 · 活动 {activeId ? 1 : 0}{:else}尚无配置{/if}
      </div>
    </div>
    <button class="btn primary" on:click={newProfile} disabled={config === null}>+ 新建配置</button>
  </div>

  {#if errorText}
    <div class="err">{errorText}</div>
  {/if}

  {#if config === null}
    <div class="empty-state"><p class="muted">加载配置中…</p></div>
  {:else if (config.profiles ?? []).length === 0}
    <div class="empty-state">
      <h3>还没有路由配置</h3>
      <p class="muted">新建一个配置，添加"哪些网段走哪块网卡"的规则，<br />NetSwitcher 会自动维护，网络变化也会重新下发。</p>
      <button class="btn primary" on:click={newProfile}>+ 新建第一个配置</button>
    </div>
  {:else}
  <div class="stage">
    <!-- Profile list -->
    <aside class="prof-list">
      {#each config?.profiles ?? [] as p}
        <button
          class="prof"
          class:active={p.id === selectedId}
          class:active-profile={p.id === activeId}
          on:click={() => (selectedId = p.id)}
        >
          <div class="prof-row1">
            <span class="prof-dot"></span>
            <span class="prof-name">{p.name}</span>
          </div>
          <div class="prof-meta">
            <span class="rules">{p.rules?.length ?? 0} 条</span>
            <span class="nic">{p.metricPolicy?.preferredInterface || p.defaultRouteInterface || '—'}</span>
          </div>
        </button>
      {/each}
    </aside>

    <!-- Editor -->
    <section class="editor">
      {#if editing}
        {#if isActive}
          <div class="readonly-banner" title="停用此配置后即可编辑">
            ⚠ 当前为活动配置，规则只读 — 点下方“停用”后才能编辑，改完再“设为活动”。
          </div>
        {/if}
        <fieldset class="editor-fs" disabled={isActive}>
        <!-- Overview card -->
        <div class="overview" class:is-active={editing.id === activeId}>
          <div class="ov-head">
            <div class="ov-titleblock">
              <div class="ov-status">
                <span class="pip"></span>
                {editing.id === activeId ? "当前活动配置" : "未激活（点“设为活动”启用）"}
              </div>
              <div class="ov-name">
                <input bind:value={editing.name} />
              </div>
              <div class="ov-id">{dirty ? "未保存" : "已保存"} · {editing.id}</div>
            </div>
            <div class="ov-stats">
              <div class="ov-stat"><span class="k">规则</span><span class="v accent">{editing.rules.length}</span></div>
              <div class="ov-stat"><span class="k">启用</span><span class="v">{enabledCount} / {editing.rules.length}</span></div>
              <div
                class="ov-stat"
                title={(editing.metricPolicy?.preferredInterface || editing.defaultRouteInterface)
                  ? ""
                  : `未配置默认路由网卡 —— 默认流量按系统当前 metric 走,本工具不会主动接管默认路由。展开下方“高级”→ 设置“首选网卡”或“默认路由网卡”后才会真正生效`}
              >
                <span class="k">默认出口</span>
                <span class="v" class:warn={!editing.metricPolicy?.preferredInterface && !editing.defaultRouteInterface}>{editing.metricPolicy?.preferredInterface || editing.defaultRouteInterface || (systemDefaultIf ? `系统 ${systemDefaultIf}` : "未配置")}</span>
              </div>
              <div class="ov-stat"><span class="k">Metric</span><span class="v">{editing.metricPolicy?.preferredMetric ?? '—'}</span></div>
            </div>
          </div>
        </div>

        <!-- Rules -->
        <div class="rules-region">
          <div class="region-head">
            <h2>路由规则 <span class="count">{editing.rules.length}</span><span class="tip" title={`配置指南\n• 把指定网段(CIDR)固定到某块网卡,例如 168.168.0.0/16 → 以太网3\n• 网关选"自动"自动取该网卡当前网关,或手动指定 IP\n• 默认路由(其余流量)走哪块网卡在下方"高级"里设\n• 这是 IP 路由,和"域名解析规则"独立`}>?</span></h2>
            <button class="btn small" on:click={addRule}>+ 添加规则</button>
          </div>
          <div class="rules-card">
            {#if editing.rules.length === 0}
              <div class="rules-empty">还没有规则。点“+ 添加规则”新建第一条。</div>
            {:else}
            <div class="rules-scroll">
              <table>
                <thead>
                  <tr><th class="col-dest">目标 CIDR</th><th class="col-if">接口</th><th class="col-gwm">网关模式</th><th class="col-gw">网关</th><th class="col-m">Metric</th><th class="col-en">启用</th><th class="col-x"></th></tr>
                </thead>
                <tbody>
                  {#each editing.rules as r, i}
                    <tr>
                      <td>
                        <input class="cell mono {ruleErr(i, 'destination') ? 'invalid' : ''}" value={r.destination} placeholder="如 168.168.0.0/16" on:input={(e) => ruleField(i, "destination", e.currentTarget.value)} />
                        {#if ruleErr(i, "destination")}<div class="field-err">{ruleErr(i, "destination")}</div>{/if}
                      </td>
                      <td>
                        <select class="cell" bind:value={r.viaInterface}>
                          {#if r.viaInterface && !interfaces.some((i) => i.Name === r.viaInterface)}
                            <option value={r.viaInterface} disabled>{r.viaInterface}（当前不可用）</option>
                          {/if}
                          {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name}</option>{/each}
                        </select>
                      </td>
                      <td>
                        <div class="seg">
                          <button type="button" class="seg-btn" class:active={gatewayMode(r.viaGateway) === "auto"} on:click={() => setGatewayMode(i, "auto")} title="自动取该网卡当前网关">自动</button>
                          <button type="button" class="seg-btn" class:active={gatewayMode(r.viaGateway) === "custom"} on:click={() => setGatewayMode(i, "custom")} title="手动指定网关 IP">指定</button>
                        </div>
                      </td>
                      <td>
                        {#if gatewayMode(r.viaGateway) === "auto"}
                          <span class="resolved-gw mono" title="该网卡当前的默认网关（auto 自动解析）">{resolvedGatewayFor(r.viaInterface) || "—"}</span>
                        {:else}
                          <input class="cell mono gw-ip {ruleErr(i, 'viaGateway') ? 'invalid' : ''}" value={r.viaGateway} placeholder="如 192.168.1.1" on:input={(e) => ruleField(i, "viaGateway", e.currentTarget.value)} />
                          {#if ruleErr(i, "viaGateway")}<div class="field-err">{ruleErr(i, "viaGateway")}</div>{/if}
                        {/if}
                      </td>
                      <td><input class="cell mono metric" type="number" min="1" value={r.metric ?? 1} on:input={(e) => ruleField(i, "metric", +e.currentTarget.value)} /></td>
                      <td><span class="toggle-sw" class:off={r.enabled === false} on:click={() => ruleField(i, "enabled", !(r.enabled !== false))} role="switch" tabindex="0"></span></td>
                      <td class="col-x"><button class="row-del" on:click={() => removeRule(i)} title="删除规则">×</button></td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
            {/if}
          </div>
        </div>

        <!-- NRPT (domain-suffix → DNS) rules -->
        <div class="rules-region">
          <div class="region-head">
            <h2>域名解析规则 <span class="count">{(editing.nrptRules ?? []).length}</span><span class="tip" title={`配置指南\n• 填 *.demo.com → 通配,匹配所有子域 *.demo.com(不含 demo.com 本身)\n• 填 demo.com → 匹配 demo.com 及其子域 *.demo.com\n• 想精确到单个主机,填完整域名(如 a.demo.com)\n• DNS 服务器填内网 DNS 的 IP,多个用逗号分隔\n• 和 IP 路由独立;若解析出的是内网 IP 想走内网网卡,再配上方的路由规则`}>?</span></h2>
            <button class="btn small" on:click={addNrpt}>+ 添加域名</button>
          </div>
          <div class="rules-card">
            {#if !(editing.nrptRules ?? []).length}
              <div class="rules-empty">还没有域名规则。让指定后缀的域名走指定 DNS 服务器解析(例如内网域名走内网 DNS);若解析出的是内网 IP,可再配上方的 IP 路由走内网网卡。</div>
            {:else}
            <div class="rules-scroll">
              <table>
                <thead>
                  <tr><th class="col-dom">域名后缀</th><th class="col-dns">DNS 服务器</th><th class="col-en">启用</th><th class="col-x"></th></tr>
                </thead>
                <tbody>
                  {#each editing.nrptRules as n, i}
                    <tr>
                      <td class="col-dom">
                        <input class="cell mono" value={n.domain} placeholder="demo.com 或 *.demo.com" on:input={(e) => nrptField(i, "domain", e.currentTarget.value)} />
                      </td>
                      <td class="col-dns">
                        <input class="cell mono" value={(n.nameServers ?? []).join(", ")} placeholder="10.0.0.1" on:input={(e) => nrptServers(i, e.currentTarget.value)} />
                      </td>
                      <td><span class="toggle-sw" class:off={n.enabled === false} on:click={() => nrptField(i, "enabled", !(n.enabled !== false))} role="switch" tabindex="0"></span></td>
                      <td class="col-x"><button class="row-del" on:click={() => removeNrpt(i)} title="删除">×</button></td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
            {/if}
          </div>
        </div>

        <!-- Advanced (collapsed) -->
        <div class="advanced" class:open={advOpen}>
          <div class="adv-head" on:click={() => (advOpen = !advOpen)}>
            <div class="lbl"><span class="caret">▸</span> 高级 · 默认路由与跃点数策略</div>
            <div class="summary">默认 {editing.metricPolicy?.preferredInterface || editing.defaultRouteInterface || '未设'} · metric {editing.autoManageMetrics && editing.metricPolicy ? `${editing.metricPolicy.preferredMetric ?? '-'}` : '关闭'}</div>
          </div>
          {#if advOpen}
          <div class="adv-body">
            <div class="adv-row">
              <div class="label">默认路由网卡<small>defaultRouteInterface</small></div>
              <div class="control">
                <select bind:value={editing.defaultRouteInterface}>
                  <option value="">（不管理默认路由）</option>
                  {#if editing.defaultRouteInterface && !interfaces.some((i) => i.Name === editing.defaultRouteInterface)}
                    <option value={editing.defaultRouteInterface} disabled>{editing.defaultRouteInterface}（当前不可用）</option>
                  {/if}
                  {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name} ({ifc.MediaType})</option>{/each}
                </select>
              </div>
            </div>
            <div class="adv-row">
              <div class="label">自动管理跃点数<small>autoManageMetrics</small></div>
              <div class="control">
                <label class="check-inline">
                  <span class="toggle-sw" class:off={!editing.autoManageMetrics} on:click={() => (editing.autoManageMetrics = !editing.autoManageMetrics)} role="switch" tabindex="0"></span>
                  启用 — 引擎持续维护接口 metric，让默认路由走指定网卡
                </label>
              </div>
            </div>
            {#if editing.autoManageMetrics && editing.metricPolicy}
            <div class="adv-row">
              <div class="label">首选网卡 + metric<small>preferredInterface / preferredMetric</small></div>
              <div class="control">
                <div style="flex:1; min-width:160px">
                  <select bind:value={editing.metricPolicy.preferredInterface}>
                    <option value="">（用默认路由网卡）</option>
                    {#if editing.metricPolicy.preferredInterface && !interfaces.some((i) => i.Name === editing.metricPolicy.preferredInterface)}
                      <option value={editing.metricPolicy.preferredInterface} disabled>{editing.metricPolicy.preferredInterface}（当前不可用）</option>
                    {/if}
                    {#each interfaces as ifc}<option value={ifc.Name}>{ifc.Name}</option>{/each}
                  </select>
                </div>
                <span class="mini">metric</span>
                <input class="num" type="number" min="1" bind:value={editing.metricPolicy.preferredMetric} />
              </div>
            </div>
            {/if}
          </div>
          {/if}
        </div>
        </fieldset>

        <!-- Action bar -->
        <div class="actionbar">
          {#if dirty}<span class="unsaved-pip">有未保存的修改</span>{/if}
          <span class="spacer"></span>
          <button class="btn primary" on:click={save} disabled={!dirty || saving}>{saving ? "保存中…" : "保存"}</button>
          {#if editing.id === activeId}
            <button class="btn" on:click={deactivate} title="停用此配置，清空活动状态，已下发路由会被移除">停用</button>
          {:else}
            <button class="btn" on:click={setActive}>设为活动</button>
          {/if}
          <span class="divider"></span>
          <button class="btn danger" on:click={deleteProfile} disabled={deleting}>{deleting ? "删除中…" : "删除"}</button>
        </div>
      {:else}
        <div class="select-hint">从左侧选择一个配置查看或编辑。</div>
      {/if}
    </section>
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
        <button class="btn ghost" on:click={() => (pendingDelete = false)} disabled={deleting}>取消</button>
        <button class="btn danger" on:click={confirmDelete} disabled={deleting}>{deleting ? "删除中…" : "确认删除"}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .profiles-page { height: 100%; display: flex; flex-direction: column; min-height: 0; }
  .page-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 18px; flex-shrink: 0; }
  h2 { margin: 0 0 2px; font-size: 18px; }
  h3 { margin: 0; font-size: 16px; }
  .head-sub { font-size: 12px; }
  .dim { color: var(--text-dim); }
  .muted { color: var(--text-dim); }
  .err { background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.3); padding: 9px 12px; border-radius: var(--radius-sm); font-size: 12px; margin-bottom: 12px; font-family: var(--font-mono); flex-shrink: 0; }

  /* Empty state */
  .empty-state { flex: 1; min-height: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; gap: 12px; padding: 32px 24px; }
  .empty-state p { margin: 0; font-size: 13px; line-height: 1.6; max-width: 420px; }

  /* Two-pane stage */
  .stage { display: grid; grid-template-columns: 240px 1fr; gap: 20px; flex: 1; min-height: 0; }
  .editor-fs { border: none; padding: 0; margin: 0; min-width: 0; display: flex; flex-direction: column; gap: 16px; }
  .editor-fs:disabled { opacity: 0.6; }
  .readonly-banner { background: rgba(250,204,21,0.08); border: 1px solid rgba(250,204,21,0.35); color: var(--warn); padding: 8px 12px; border-radius: var(--radius-sm); font-size: 12px; margin-bottom: 0; line-height: 1.5; }

  /* Profile list */
  .prof-list { display: flex; flex-direction: column; gap: 4px; }
  .prof {
    display: block; width: 100%; text-align: left;
    padding: 11px 12px; background: transparent; border: 1px solid transparent;
    border-radius: var(--radius-sm); cursor: pointer; color: var(--text-dim);
    transition: background 120ms, border-color 120ms; position: relative;
  }
  .prof:hover { background: var(--bg-2); }
  .prof.active { background: var(--bg-2); border-color: var(--accent-dim); }
  .prof-row1 { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
  .prof-dot { width: 7px; height: 7px; border-radius: 50%; border: 1.5px solid var(--text-faint); flex-shrink: 0; background: transparent; }
  .prof.active-profile .prof-dot { border-color: var(--good); background: var(--good); box-shadow: 0 0 8px rgba(74,222,128,0.5); }
  .prof-name { font-weight: 600; font-size: 13.5px; color: var(--text); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .prof-meta { font-family: var(--font-mono); font-size: 11px; color: var(--text-faint); padding-left: 15px; display: flex; gap: 10px; }
  .prof-meta .rules { color: var(--text-dim); }

  /* Editor */
  .editor { display: flex; flex-direction: column; gap: 16px; min-width: 0; }

  /* Overview card */
  .overview { background: var(--bg-1); border: 1px solid var(--border); border-radius: var(--radius); padding: 18px 22px; }
  .overview.is-active { border-color: rgba(74,222,128,0.4); }
  .ov-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .ov-titleblock { min-width: 0; flex: 1; }
  .ov-status { display: inline-flex; align-items: center; gap: 6px; font-family: var(--font-mono); font-size: 11px; color: var(--text-faint); margin-bottom: 6px; }
  .ov-status .pip { width: 6px; height: 6px; border-radius: 50%; background: var(--text-faint); }
  .overview.is-active .ov-status { color: var(--good); }
  .overview.is-active .ov-status .pip { background: var(--good); box-shadow: 0 0 6px var(--good); }
  .ov-name { font-size: 20px; font-weight: 700; }
  .ov-name input {
    background: transparent; border: 1px solid transparent; color: var(--text);
    font: inherit; padding: 2px 6px; margin-left: -6px; border-radius: 4px; min-width: 0; max-width: 100%;
  }
  .ov-name input:hover { border-color: var(--border); background: var(--bg-2); }
  .ov-name input:focus { outline: none; border-color: var(--accent); background: var(--bg-2); }
  .ov-id { font-family: var(--font-mono); font-size: 11px; color: var(--text-faint); margin-top: 3px; }
  .ov-stats { display: flex; gap: 24px; flex-wrap: wrap; }
  .ov-stat { display: flex; flex-direction: column; gap: 2px; }
  .ov-stat .k { font-size: 10px; text-transform: uppercase; letter-spacing: 0.1em; color: var(--text-faint); }
  .ov-stat .v { font-family: var(--font-mono); font-size: 14px; color: var(--text); }
  .ov-stat .v.accent { color: var(--accent); }
  .ov-stat .v.warn { color: var(--warn); }

  /* Rules region */
  .rules-region { display: flex; flex-direction: column; gap: 10px; }
  .region-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .region-head h2 { font-size: 13px; text-transform: uppercase; letter-spacing: 0.1em; color: var(--text-dim); font-weight: 600; }
  .region-head .count { font-family: var(--font-mono); color: var(--accent); margin-left: 6px; background: rgba(95,184,255,0.1); padding: 1px 7px; border-radius: 10px; font-size: 11px; }
  .rules-card { background: var(--bg-1); border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
  .rules-empty { padding: 18px; color: var(--text-faint); font-size: 13px; text-align: center; }
  .rules-scroll { overflow-x: auto; }
  table { border-collapse: collapse; width: 100%; min-width: 720px; }
  th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid var(--border-soft); vertical-align: middle; }
  th { font-size: 11px; font-weight: 600; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.06em; background: var(--bg-2); }
  tbody tr:last-child td { border-bottom: none; }
  tbody tr:hover td { background: rgba(95,184,255,0.04); }
  .col-dest { width: 19%; } .col-if { width: 13%; } .col-gwm { width: 16%; } .col-gw { width: 19%; } .col-m { width: 8%; } .col-en { width: 8%; } .col-x { width: 40px; text-align: center; }
  .col-dom { width: 38%; } .col-dns { width: 50%; }
  /* .cell / .seg / .seg-btn / .toggle-sw visual styling comes from the global
     app.css (so the theme switch applies). Here we only keep layout overrides. */
  .cell { width: 100%; box-sizing: border-box; }
  .resolved-gw { display: inline-block; font-size: 12px; color: var(--text-dim); padding: 5px 8px; background: var(--bg-0); border: 1px solid var(--border-soft); border-radius: var(--comp-radius); }
  .gw-ip { width: 100%; box-sizing: border-box; }
  .metric { width: 60px; }
  .field-err { color: var(--bad); font-size: 11px; margin-top: 3px; }
  .row-del { display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; vertical-align: middle; background: transparent; border: none; color: var(--text-faint); cursor: pointer; padding: 0; font-size: 15px; line-height: 1; border-radius: 4px; }
  .row-del:hover { color: var(--bad); background: rgba(248,113,113,0.08); }
  /* Keep all inline controls in a rule row aligned to the row's midline. */
  .rules-card td .cell { vertical-align: middle; }

  /* Advanced */
  .advanced { background: var(--bg-1); border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
  .adv-head { display: flex; align-items: center; justify-content: space-between; padding: 13px 18px; cursor: pointer; user-select: none; }
  .adv-head:hover { background: var(--bg-2); }
  .adv-head .lbl { font-size: 12.5px; color: var(--text-dim); display: flex; align-items: center; gap: 8px; }
  .adv-head .lbl .caret { transition: transform 150ms; color: var(--text-faint); }
  .advanced.open .adv-head .lbl .caret { transform: rotate(90deg); }
  .adv-head .summary { font-family: var(--font-mono); font-size: 11px; color: var(--text-faint); }
  .adv-body { padding: 4px 18px 16px; border-top: 1px solid var(--border-soft); }
  .adv-row { display: flex; align-items: center; gap: 14px; padding: 12px 0; border-bottom: 1px dashed var(--border-soft); }
  .adv-row:last-child { border-bottom: none; }
  .adv-row .label { width: 160px; font-size: 12.5px; color: var(--text-dim); flex-shrink: 0; }
  .adv-row .label small { display: block; font-size: 10.5px; color: var(--text-faint); font-family: var(--font-mono); margin-top: 1px; }
  .adv-row .control { flex: 1; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .adv-row select, .adv-row .num { background: var(--bg-2); border: 1px solid var(--border); color: var(--text); padding: 6px 10px; border-radius: 4px; font-family: var(--font-mono); font-size: 13px; outline: none; }
  .adv-row select:focus, .adv-row .num:focus { border-color: var(--accent); }
  .adv-row .num { width: 70px; }
  .adv-row .mini { font-family: var(--font-mono); font-size: 11px; color: var(--text-faint); }
  .check-inline { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; cursor: pointer; color: var(--text-dim); }

  /* Action bar */
  .actionbar { display: flex; align-items: center; gap: 8px; padding-top: 16px; border-top: 1px solid var(--border-soft); margin-bottom: 14px; }
  .actionbar .spacer { flex: 1; }
  .actionbar .divider { width: 1px; height: 22px; background: var(--border); margin: 0 6px; }
  .unsaved-pip { font-family: var(--font-mono); font-size: 11px; color: var(--warn); display: inline-flex; align-items: center; gap: 5px; }
  .unsaved-pip::before { content: '●'; font-size: 8px; }
  .tip { display: inline-flex; align-items: center; justify-content: center; width: 14px; height: 14px; border-radius: 50%; background: var(--bg-2); border: 1px solid var(--border); color: var(--text-faint); font-size: 10px; cursor: help; margin-left: 6px; vertical-align: middle; user-select: none; }
  .tip:hover { color: var(--text); border-color: var(--text-dim); }

  /* Buttons (local, since this page defines its own btn styles) */
  /* .btn visual styling comes from global app.css button rules (themed).
     Keep only size modifiers that are page-specific. */
  .btn.small { padding: 4px 10px; font-size: 12px; }

  .select-hint { color: var(--text-faint); padding: 40px; text-align: center; }

  /* Delete modal */
  .modal-backdrop { position: fixed; inset: 0; background: rgba(8,10,15,0.72); display: flex; align-items: center; justify-content: center; z-index: 60; }
  .modal { background: var(--bg-1); border: 1px solid var(--border); border-radius: 12px; padding: 24px 26px; max-width: 420px; box-shadow: 0 10px 40px rgba(0,0,0,0.5); }
  .modal h3 { margin: 0 0 10px; font-size: 16px; }
  .modal p { margin: 6px 0; font-size: 13px; line-height: 1.55; }
  .modal-bullets { margin: 8px 0; padding-left: 20px; font-size: 12px; color: var(--text-dim); line-height: 1.7; }
  .modal-bullets li::marker { color: var(--bad); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px; }
</style>
