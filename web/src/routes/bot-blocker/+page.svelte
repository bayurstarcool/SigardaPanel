<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import { Bot, Shield, ShieldCheck, ShieldOff, Trash2, LoaderCircle, Ban, RefreshCw, FileCode, Server, ChevronRight } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let enabled = $state(false);
  let loading = $state(true);
  let loadError = $state('');
  let busy = $state('');
  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

  let blockedAgents = $state<string[]>([]);
  let blockedIPs = $state<string[]>([]);
  let rawConfig = $state('');

  let showAddAgent = $state(false);
  let showAddIP = $state(false);
  let newAgent = $state('');
  let newIP = $state('');

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load servers');
    }
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadRules();
    } else {
      loading = false;
    }
  });

  function loadRules() {
    if (!selectedServer) return;
    loading = true;
    loadError = '';
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'list' })
      .then((d: any) => {
        enabled = d?.enabled ?? false;
        rawConfig = d?.config ?? '';
        parseConfig(rawConfig);
      })
      .catch((e: any) => { if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load rules'; })
      .finally(() => (loading = false));
  }

  function parseConfig(config: string) {
    const agentRegex = /~\*\(([^)]+)\)\s+1;/g;
    const a: string[] = [];
    let m;
    while ((m = agentRegex.exec(config)) !== null) a.push(m[1]);
    blockedAgents = a;

    const ipRegex = /deny\s+([^\s;]+);/g;
    const i: string[] = [];
    while ((m = ipRegex.exec(config)) !== null) i.push(m[1]);
    blockedIPs = i;
  }

  async function manage(key: string, body: any, success: string, after?: () => void) {
    busy = key;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, body);
      toasts.success(success);
      after?.();
      loadRules();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Action failed');
    } finally {
      busy = '';
    }
  }

  const toggle = (enable: boolean) =>
    manage('toggle', { action: enable ? 'enable' : 'disable' }, enable ? 'Bot blocker enabled' : 'Bot blocker disabled');

  function addAgent() {
    if (!newAgent.trim()) return;
    const list = newAgent.split(',').map((s) => s.trim()).filter(Boolean);
    manage('add-agent', { action: 'add-agent', agents: list }, `Added ${list.length} pattern(s)`, () => { newAgent = ''; showAddAgent = false; });
  }

  const removeAgent = (agent: string) => manage(`ra-${agent}`, { action: 'remove-agent', agents: [agent] }, 'Removed');

  function addIP() {
    if (!newIP.trim()) return;
    const list = newIP.split(',').map((s) => s.trim()).filter(Boolean);
    manage('add-ip', { action: 'add-ip', ips: list }, `Added ${list.length} IP(s)`, () => { newIP = ''; showAddIP = false; });
  }

  const removeIP = (ip: string) => manage(`ri-${ip}`, { action: 'remove-ip', ips: [ip] }, 'Removed');
</script>

<DashboardLayout active="Bot Blocker">
  <PageHeader title="Bot blocker" description="Block malicious bots, scrapers and unwanted traffic in nginx">
    {#snippet meta()}
      {#if !loading && !loadError && servers.length > 0}
        <span class="badge {enabled ? 'badge-success' : ''}"><span class="dot"></span>{enabled ? 'Active' : 'Inactive'}</span>
      {/if}
    {/snippet}
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={loadRules} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      <button onclick={loadRules} class="btn btn-secondary btn-sm" disabled={loading || !selectedServer}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if canManage && selectedServer}
        <button onclick={() => (showAddIP = true)} class="btn btn-secondary btn-sm"><Ban size={14} /> Block IP</button>
        <button onclick={() => (showAddAgent = true)} class="btn btn-primary btn-sm"><Bot size={14} /> Block user-agent</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if !loading && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — the bot blocker runs in its nginx.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else if loading}
    <div class="skeleton mb-6 h-[84px]"></div>
    <div class="grid gap-6 lg:grid-cols-2">
      {#each Array(2) as _, i (i)}<div class="card space-y-3 p-4">{#each Array(4) as _, j (j)}<div class="skeleton h-8"></div>{/each}</div>{/each}
    </div>
  {:else if loadError}
    <div class="card">
      <EmptyState icon={Shield} title="Couldn't load bot blocker" description={loadError} actionLabel="Retry" onaction={loadRules} />
    </div>
  {:else if !enabled && blockedAgents.length === 0 && blockedIPs.length === 0}
    <div class="card">
      <EmptyState icon={Shield} title="Bot blocker not configured" description="Enable the bot blocker to start filtering malicious traffic in nginx.">
        {#if canManage}
          <button onclick={() => toggle(true)} disabled={busy === 'toggle'} class="btn btn-primary btn-sm">
            {#if busy === 'toggle'}<LoaderCircle size={14} class="animate-spin" />{:else}<ShieldCheck size={14} />{/if} Enable bot blocker
          </button>
        {/if}
      </EmptyState>
    </div>
  {:else}
    <div class="card mb-6 flex flex-wrap items-center justify-between gap-4 p-5">
      <div class="flex items-center gap-3">
        <div class="flex size-10 items-center justify-center rounded-lg border border-line bg-surface-2 {enabled ? 'text-success' : 'text-subtle'}">
          {#if enabled}<ShieldCheck size={18} strokeWidth={1.75} />{:else}<ShieldOff size={18} strokeWidth={1.75} />{/if}
        </div>
        <div>
          <div class="text-sm font-semibold text-fg">{enabled ? 'Blocking is active' : 'Blocking is paused'}</div>
          <div class="text-[13px] text-muted tabular-nums">{blockedAgents.length} user-agent pattern(s) · {blockedIPs.length} IP(s)</div>
        </div>
      </div>
      {#if canManage}
        {#if enabled}
          <button onclick={() => toggle(false)} disabled={busy === 'toggle'} class="btn btn-danger-ghost btn-sm">
            {#if busy === 'toggle'}<LoaderCircle size={14} class="animate-spin" />{:else}<ShieldOff size={14} />{/if} Disable
          </button>
        {:else}
          <button onclick={() => toggle(true)} disabled={busy === 'toggle'} class="btn btn-primary btn-sm">
            {#if busy === 'toggle'}<LoaderCircle size={14} class="animate-spin" />{:else}<ShieldCheck size={14} />{/if} Enable
          </button>
        {/if}
      {/if}
    </div>

    <div class="mb-6 grid gap-6 lg:grid-cols-2">
      {#each [
        { key: 'agent', title: 'Blocked user-agents', icon: Bot, items: blockedAgents, empty: 'No user-agent patterns configured.', busyPrefix: 'ra-', remove: removeAgent },
        { key: 'ip', title: 'Blocked IP addresses', icon: Ban, items: blockedIPs, empty: 'No IPs blocked.', busyPrefix: 'ri-', remove: removeIP },
      ] as section (section.key)}
        <section class="card min-w-0">
          <div class="card-header">
            <h2 class="card-title flex items-center gap-2"><section.icon size={16} class="text-subtle" /> {section.title}</h2>
            <span class="badge tabular-nums">{section.items.length}</span>
          </div>
          {#if section.items.length === 0}
            <p class="px-5 py-8 text-center text-[13px] text-muted">{section.empty}</p>
          {:else}
            <ul class="max-h-[420px] divide-y divide-line overflow-y-auto">
              {#each section.items as item (item)}
                <li class="flex items-center justify-between gap-3 px-5 py-2.5 transition-colors hover:bg-surface-2/60">
                  <code class="code truncate">{item}</code>
                  {#if canManage}
                    <button onclick={() => section.remove(item)} disabled={busy === `${section.busyPrefix}${item}`} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove {item}" title="Remove">
                      {#if busy === `${section.busyPrefix}${item}`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                    </button>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/each}
    </div>

    {#if rawConfig}
      <details class="card group">
        <summary class="flex cursor-pointer list-none items-center gap-2 px-5 py-3.5 text-sm font-medium text-muted transition-colors hover:text-fg">
          <ChevronRight size={14} class="transition-transform group-open:rotate-90" />
          <FileCode size={14} /> Raw nginx config
        </summary>
        <div class="px-5 pb-5">
          <pre class="max-h-[50vh] overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{rawConfig}</pre>
        </div>
      </details>
    {/if}
  {/if}

  <Modal bind:open={showAddAgent} title="Block user-agent pattern" maxWidth="md">
    <form id="bb-agent" onsubmit={(e) => { e.preventDefault(); addAgent(); }}>
      <label class="label" for="bb-agent-input">Patterns</label>
      <input id="bb-agent-input" bind:value={newAgent} placeholder="semrushbot, ahrefsbot, dotbot" class="input font-mono text-[13px]" />
      <p class="hint">Case-insensitive regex matched against the User-Agent header. Comma-separate multiple patterns.</p>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showAddAgent = false)}>Cancel</button>
      <button type="submit" form="bb-agent" class="btn btn-primary btn-sm" disabled={busy === 'add-agent' || !newAgent.trim()}>
        {#if busy === 'add-agent'}<LoaderCircle size={14} class="animate-spin" />{/if} Add
      </button>
    {/snippet}
  </Modal>

  <Modal bind:open={showAddIP} title="Block IP address" maxWidth="md">
    <form id="bb-ip" onsubmit={(e) => { e.preventDefault(); addIP(); }}>
      <label class="label" for="bb-ip-input">IP addresses</label>
      <input id="bb-ip-input" bind:value={newIP} placeholder="1.2.3.4, 5.6.7.8" class="input font-mono text-[13px]" />
      <p class="hint">Requests from these addresses get a 403 from nginx. Comma-separate multiple IPs.</p>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showAddIP = false)}>Cancel</button>
      <button type="submit" form="bb-ip" class="btn btn-danger btn-sm" disabled={busy === 'add-ip' || !newIP.trim()}>
        {#if busy === 'add-ip'}<LoaderCircle size={14} class="animate-spin" />{:else}<Ban size={14} />{/if} Block
      </button>
    {/snippet}
  </Modal>
</DashboardLayout>
