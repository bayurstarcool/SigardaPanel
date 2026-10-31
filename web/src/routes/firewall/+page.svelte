<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Shield, ShieldCheck, ShieldOff, Plus, Trash2, RefreshCw, Zap, Globe, Server, Database, Ban, LoaderCircle, RotateCcw, Search } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let status = $state<any>(null);
  let statusLoading = $state(false);
  let rules = $state<any[]>([]);
  let loading = $state(false);
  let loadError = $state('');
  let busy = $state('');
  let showAddRule = $state(false);
  let showQuickBlock = $state(false);
  let showPresets = $state(false);
  let newPort = $state('');
  let newProtocol = $state('tcp');
  let newFrom = $state('');
  let newAction = $state('allow');
  let blockIP = $state('');
  let filter = $state<'all' | 'ALLOW' | 'DENY'>('all');
  let query = $state('');
  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

  // Confirm dialog state shared by delete / reset / disable.
  let confirmOpen = $state(false);
  let confirmCfg = $state<{ title: string; message: string; label: string; run: () => void }>({ title: '', message: '', label: '', run: () => {} });

  const presets = [
    { name: 'Web server', icon: Globe, ports: ['80/tcp', '443/tcp'], desc: 'HTTP + HTTPS' },
    { name: 'SSH', icon: Server, ports: ['22/tcp'], desc: 'Secure Shell' },
    { name: 'Database', icon: Database, ports: ['3306/tcp', '5432/tcp'], desc: 'MySQL + PostgreSQL' },
    { name: 'Redis', icon: Zap, ports: ['6379/tcp'], desc: 'Redis cache' },
    { name: 'Node.js', icon: Server, ports: ['3000/tcp', '8080/tcp'], desc: 'Common Node ports' },
  ];

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load servers');
    }
    serversLoaded = true;
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      refresh();
    }
  });

  function refresh() {
    loadStatus();
    loadRules();
  }

  function showError(e: any, fallback: string) {
    if (e?.message !== 'session expired') toasts.error(e?.message || fallback);
  }

  function loadStatus() {
    if (!selectedServer) return;
    statusLoading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/firewall/status`)
      .then((d: any) => { status = d; })
      .catch(() => { status = null; })
      .finally(() => (statusLoading = false));
  }

  function loadRules() {
    if (!selectedServer) return;
    loading = true;
    loadError = '';
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/firewall/rules`)
      .then((d: any) => { rules = d?.rules ?? []; })
      .catch((e: any) => { if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load rules'; })
      .finally(() => (loading = false));
  }

  function ask(title: string, message: string, label: string, run: () => void) {
    confirmCfg = { title, message, label, run };
    confirmOpen = true;
  }

  async function toggleFirewall(enable: boolean) {
    const action = enable ? 'enable' : 'disable';
    busy = action;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/${action}`, {});
      toasts.success(enable ? 'Firewall enabled' : 'Firewall disabled');
      loadStatus();
    } catch (e: any) {
      showError(e, `Failed to ${action} firewall`);
    }
    busy = '';
  }

  async function addRule() {
    if (!newPort) return;
    const endpoint = newAction === 'allow' ? 'allow' : 'deny';
    const body: any = { port: newPort, protocol: newProtocol };
    if (newFrom) body.from = newFrom;
    busy = 'add';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/${endpoint}`, body);
      toasts.success(`Rule added: ${endpoint} ${newPort}/${newProtocol}`);
      showAddRule = false;
      newPort = '';
      newFrom = '';
      loadRules();
    } catch (e: any) {
      showError(e, 'Failed to add rule');
    }
    busy = '';
  }

  async function applyPreset(preset: (typeof presets)[number]) {
    busy = `preset:${preset.name}`;
    try {
      for (const port of preset.ports) {
        const [p, proto] = port.split('/');
        await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/allow`, { port: p, protocol: proto || 'tcp', comment: preset.name });
      }
      toasts.success(`${preset.name} ports allowed`);
      showPresets = false;
    } catch (e: any) {
      showError(e, 'Failed to apply preset');
    }
    busy = '';
    loadRules();
  }

  async function blockIPAddr() {
    if (!blockIP) return;
    busy = 'block';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/deny`, { port: 'any', from: blockIP, comment: 'Blocked via panel' });
      toasts.success(`${blockIP} blocked`);
      showQuickBlock = false;
      blockIP = '';
      loadRules();
    } catch (e: any) {
      showError(e, 'Failed to block IP');
    }
    busy = '';
  }

  async function deleteRule(num: number) {
    busy = `del:${num}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/delete`, { number: num });
      toasts.success(`Rule #${num} deleted`);
      loadRules();
    } catch (e: any) {
      showError(e, 'Failed to delete rule');
    }
    busy = '';
  }

  async function resetFirewall() {
    busy = 'reset';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/reset`, {});
      toasts.success('Firewall reset to defaults');
      refresh();
    } catch (e: any) {
      showError(e, 'Failed to reset firewall');
    }
    busy = '';
  }

  function actionBadge(a: string) {
    if (a === 'ALLOW') return 'badge-success';
    if (a === 'DENY' || a === 'REJECT') return 'badge-danger';
    return 'badge-warning';
  }

  const visible = $derived(
    rules
      .filter((r) => filter === 'all' || r.action === filter)
      .filter((r) => !query || `${r.port} ${r.from} ${r.to} ${r.comment}`.toLowerCase().includes(query.toLowerCase())),
  );
</script>

<DashboardLayout active="Firewall">
  <PageHeader title="Firewall" description={`UFW · ${rules.length} rule(s)`}>
    {#snippet meta()}
      {#if status}
        <span class="badge {status.active ? 'badge-success' : 'badge-danger'}"><span class="dot"></span>{status.active ? 'Active' : 'Inactive'}</span>
      {/if}
    {/snippet}
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={refresh} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      <button onclick={refresh} class="btn btn-secondary btn-sm" disabled={loading || !selectedServer}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if canManage && selectedServer}
        <button onclick={() => (showQuickBlock = true)} class="btn btn-secondary btn-sm"><Ban size={14} /> Block IP</button>
        <button onclick={() => (showPresets = true)} class="btn btn-secondary btn-sm"><Zap size={14} /> Presets</button>
        <button onclick={() => (showAddRule = true)} class="btn btn-primary btn-sm"><Plus size={14} /> Add rule</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — the firewall is managed through its agent.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else}
    <div class="card mb-6 flex flex-wrap items-center justify-between gap-4 p-5">
      <div class="flex items-center gap-3">
        <div class="flex size-10 items-center justify-center rounded-lg border border-line bg-surface-2 {status?.active ? 'text-success' : 'text-danger'}">
          {#if status?.active}<ShieldCheck size={18} strokeWidth={1.75} />{:else}<ShieldOff size={18} strokeWidth={1.75} />{/if}
        </div>
        <div>
          {#if statusLoading && !status}
            <div class="skeleton mb-1.5 h-4 w-28"></div>
            <div class="skeleton h-3 w-48"></div>
          {:else}
            <div class="text-sm font-semibold text-fg">{status ? (status.active ? 'Firewall is active' : 'Firewall is inactive') : 'Status unavailable'}</div>
            <div class="text-[13px] text-muted">
              {status ? (status.active ? 'Incoming traffic is filtered by the rules below.' : 'All ports are reachable. Enable UFW to enforce rules.') : 'The agent did not report the firewall status.'}
            </div>
          {/if}
        </div>
      </div>
      {#if canManage && status}
        <div class="flex gap-2">
          {#if status.active}
            <button
              onclick={() => ask('Disable firewall?', 'Every port on this server becomes reachable until the firewall is enabled again.', 'Disable', () => toggleFirewall(false))}
              disabled={!!busy}
              class="btn btn-danger-ghost btn-sm"
            >
              {#if busy === 'disable'}<LoaderCircle size={14} class="animate-spin" />{:else}<ShieldOff size={14} />{/if} Disable
            </button>
          {:else}
            <button onclick={() => toggleFirewall(true)} disabled={!!busy} class="btn btn-primary btn-sm">
              {#if busy === 'enable'}<LoaderCircle size={14} class="animate-spin" />{:else}<ShieldCheck size={14} />{/if} Enable
            </button>
          {/if}
          <button
            onclick={() => ask('Reset firewall?', 'All rules are removed and UFW returns to its defaults.', 'Reset', resetFirewall)}
            disabled={!!busy}
            class="btn btn-secondary btn-sm"
          >
            {#if busy === 'reset'}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCcw size={14} />{/if} Reset
          </button>
        </div>
      {/if}
    </div>

    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <div class="segmented">
          {#each [['all', 'All'], ['ALLOW', 'Allow'], ['DENY', 'Deny']] as [key, label] (key)}
            <button class="segment {filter === key ? 'segment-active' : ''}" onclick={() => (filter = key as any)}>{label}</button>
          {/each}
        </div>
        <div class="relative w-full sm:w-64">
          <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
          <input bind:value={query} class="input h-8 pl-8" placeholder="Filter port, IP, comment…" aria-label="Filter rules" />
        </div>
      </div>

      {#if loading && rules.length === 0}
        <div class="space-y-3 p-4">{#each Array(5) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
      {:else if loadError && rules.length === 0}
        <EmptyState compact icon={Shield} title="Couldn't load rules" description={loadError} actionLabel="Retry" onaction={loadRules} />
      {:else if visible.length === 0}
        <EmptyState compact icon={Shield} title={rules.length ? 'No rules match' : 'No rules'} description={rules.length ? 'Try another filter.' : 'No firewall rules are configured yet.'} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th class="w-12">#</th>
                <th>Port</th>
                <th>Action</th>
                <th class="max-md:hidden">Direction</th>
                <th>From</th>
                <th class="max-lg:hidden">To</th>
                <th class="max-md:hidden">Comment</th>
                {#if canManage}<th class="text-right">Actions</th>{/if}
              </tr>
            </thead>
            <tbody>
              {#each visible as r (r.number)}
                <tr>
                  <td class="font-mono text-xs text-subtle tabular-nums">{r.number}</td>
                  <td class="whitespace-nowrap">
                    <span class="font-mono text-[13px] text-fg">{r.port || '—'}</span>
                    <span class="ml-1 text-xs text-subtle">{r.protocol || 'any'}</span>
                  </td>
                  <td><span class="badge {actionBadge(r.action)}">{r.action}</span></td>
                  <td class="text-xs text-muted max-md:hidden">{r.direction || '—'}</td>
                  <td class="font-mono text-[13px] text-muted">{r.from || 'Anywhere'}</td>
                  <td class="font-mono text-[13px] text-muted max-lg:hidden">{r.to || 'Anywhere'}</td>
                  <td class="max-w-[180px] truncate text-xs text-subtle max-md:hidden" title={r.comment}>{r.comment || '—'}</td>
                  {#if canManage}
                    <td class="text-right">
                      <button
                        onclick={() => ask(`Delete rule #${r.number}?`, `${r.action} ${r.port || 'any'}${r.from ? ` from ${r.from}` : ''} will be removed.`, 'Delete', () => deleteRule(r.number))}
                        disabled={busy === `del:${r.number}`}
                        class="btn btn-danger-ghost btn-icon btn-xs"
                        aria-label="Delete rule {r.number}"
                        title="Delete"
                      >
                        {#if busy === `del:${r.number}`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </td>
                  {/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <Modal bind:open={showAddRule} title="Add firewall rule" maxWidth="md">
    <form id="fw-add" class="grid grid-cols-2 gap-4" onsubmit={(e) => { e.preventDefault(); addRule(); }}>
      <div>
        <label class="label" for="fw-port">Port</label>
        <input id="fw-port" bind:value={newPort} placeholder="443 or 8000:8100" class="input font-mono text-[13px]" />
      </div>
      <div>
        <label class="label" for="fw-proto">Protocol</label>
        <select id="fw-proto" bind:value={newProtocol} class="input">
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
        </select>
      </div>
      <div class="col-span-2">
        <label class="label" for="fw-from">From IP</label>
        <input id="fw-from" bind:value={newFrom} placeholder="Anywhere" class="input font-mono text-[13px]" />
        <p class="hint">Optional — leave empty to apply to any source.</p>
      </div>
      <div class="col-span-2">
        <span class="label">Action</span>
        <div class="segmented">
          <button type="button" class="segment {newAction === 'allow' ? 'segment-active' : ''}" onclick={() => (newAction = 'allow')}>Allow</button>
          <button type="button" class="segment {newAction === 'deny' ? 'segment-active' : ''}" onclick={() => (newAction = 'deny')}>Deny</button>
        </div>
      </div>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showAddRule = false)}>Cancel</button>
      <button type="submit" form="fw-add" class="btn btn-primary btn-sm" disabled={!newPort || busy === 'add'}>
        {#if busy === 'add'}<LoaderCircle size={14} class="animate-spin" />{/if} Save rule
      </button>
    {/snippet}
  </Modal>

  <Modal bind:open={showPresets} title="Allow common ports" maxWidth="lg">
    <div class="grid gap-2 sm:grid-cols-2">
      {#each presets as preset (preset.name)}
        <button
          onclick={() => applyPreset(preset)}
          disabled={!!busy}
          class="flex items-center gap-3 rounded-lg border border-line p-3 text-left transition-colors hover:border-line-strong hover:bg-surface-2 disabled:opacity-50"
        >
          <div class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-subtle">
            {#if busy === `preset:${preset.name}`}<LoaderCircle size={16} class="animate-spin" />{:else}<preset.icon size={16} strokeWidth={1.75} />{/if}
          </div>
          <div class="min-w-0">
            <div class="text-sm font-medium text-fg">{preset.name}</div>
            <div class="truncate font-mono text-xs text-subtle">{preset.ports.join(', ')}</div>
          </div>
        </button>
      {/each}
    </div>
  </Modal>

  <Modal bind:open={showQuickBlock} title="Block IP address" maxWidth="md">
    <form id="fw-block" onsubmit={(e) => { e.preventDefault(); blockIPAddr(); }}>
      <label class="label" for="fw-ip">IP address</label>
      <input id="fw-ip" bind:value={blockIP} placeholder="1.2.3.4" class="input font-mono text-[13px]" />
      <p class="hint">Adds a deny rule for all ports from this address.</p>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showQuickBlock = false)}>Cancel</button>
      <button type="submit" form="fw-block" class="btn btn-danger btn-sm" disabled={!blockIP || busy === 'block'}>
        {#if busy === 'block'}<LoaderCircle size={14} class="animate-spin" />{:else}<Ban size={14} />{/if} Block
      </button>
    {/snippet}
  </Modal>

  <ConfirmDialog bind:open={confirmOpen} title={confirmCfg.title} message={confirmCfg.message} confirmLabel={confirmCfg.label} destructive onconfirm={() => confirmCfg.run()} />
</DashboardLayout>
