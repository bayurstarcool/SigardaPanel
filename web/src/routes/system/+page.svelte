<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { HardDrive, Cpu, MemoryStick, RefreshCw, Package, ArrowUpCircle, Activity, ArrowDown, ArrowUp, Server, LoaderCircle, MonitorCog } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let metrics = $state<any>(null);
  let metricsState = $state<'ok' | 'empty' | 'error'>('ok');
  let specs = $state<any>(null);
  let loading = $state(false);
  let updates = $state('');
  let updatesLoading = $state(false);
  let showUpdates = $state(false);
  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));

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
    loadMetrics();
    loadSpecs();
  }

  function changeServer() {
    metrics = null;
    specs = null;
    showUpdates = false;
    refresh();
  }

  function loadMetrics() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/metrics/latest`)
      .then((d: any) => { metrics = d; metricsState = d ? 'ok' : 'empty'; })
      .catch((e: any) => {
        metrics = null;
        // 404 = the agent hasn't reported yet; anything else is a real failure.
        metricsState = /no metrics|not found/i.test(String(e?.message)) ? 'empty' : 'error';
      })
      .finally(() => (loading = false));
  }

  function loadSpecs() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/specs`)
      .then((d: any) => { specs = d; })
      .catch(() => { specs = null; });
  }

  function loadUpdates() {
    if (!selectedServer) return;
    showUpdates = true;
    updatesLoading = true;
    updates = '';
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/command`, {
      command: "bash -c 'apt-get update -qq 2>/dev/null && apt list --upgradable 2>/dev/null | head -30'",
    })
      .then((d: any) => { updates = d?.output || 'No updates available'; })
      .catch(() => { updates = 'Failed to check updates'; })
      .finally(() => (updatesLoading = false));
  }

  function fmtBytes(b: number) {
    if (!b) return '0 B';
    if (b < 1024 ** 2) return (b / 1024).toFixed(1) + ' KB';
    if (b < 1024 ** 3) return (b / 1024 ** 2).toFixed(1) + ' MB';
    return (b / 1024 ** 3).toFixed(1) + ' GB';
  }

  function pct(used: number, total: number) {
    if (!total) return 0;
    return Math.min(100, Math.max(0, (used / total) * 100));
  }

  function barTone(v: number) {
    if (v >= 90) return 'bg-danger';
    if (v >= 70) return 'bg-warning';
    return 'bg-accent';
  }

  const resources = $derived(
    metrics
      ? [
          {
            label: 'CPU', icon: Cpu, value: Math.min(100, Math.max(0, Number(metrics.cpu_percent) || 0)),
            big: `${(Number(metrics.cpu_percent) || 0).toFixed(1)}%`,
            detail: `${specs?.cpu?.cores ?? '—'} core(s) · ${specs?.cpu?.threads ?? '—'} thread(s)`,
          },
          {
            label: 'Memory', icon: MemoryStick, value: pct(metrics.mem_used || 0, metrics.mem_total || 0),
            big: fmtBytes(metrics.mem_used || 0),
            detail: `of ${fmtBytes(metrics.mem_total || 0)} · ${pct(metrics.mem_used || 0, metrics.mem_total || 0).toFixed(1)}%`,
          },
          {
            label: 'Disk', icon: HardDrive, value: pct(metrics.disk_used || 0, metrics.disk_total || 0),
            big: fmtBytes(metrics.disk_used || 0),
            detail: `of ${fmtBytes(metrics.disk_total || 0)} · ${pct(metrics.disk_used || 0, metrics.disk_total || 0).toFixed(1)}%`,
          },
        ]
      : [],
  );
</script>

<DashboardLayout active="System">
  <PageHeader title="System" description={loading ? 'Loading…' : 'Server resources and hardware'}>
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={changeServer} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      <button onclick={refresh} class="btn btn-secondary btn-sm" disabled={loading || !selectedServer}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server to see its resources here.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else}
    {#if loading && !metrics}
      <div class="mb-6 grid gap-3 sm:grid-cols-3">{#each Array(3) as _, i (i)}<div class="skeleton h-[124px]"></div>{/each}</div>
      <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">{#each Array(4) as _, i (i)}<div class="skeleton h-[88px]"></div>{/each}</div>
    {:else if metrics}
      <div class="mb-6 grid gap-3 sm:grid-cols-3">
        {#each resources as r (r.label)}
          <div class="card p-5">
            <div class="flex items-center justify-between">
              <span class="stat-label">{r.label}</span>
              <r.icon size={16} strokeWidth={1.75} class="text-subtle" />
            </div>
            <div class="stat-value">{r.big}</div>
            <div class="progress mt-3"><div class="progress-bar {barTone(r.value)}" style="width: {r.value}%"></div></div>
            <div class="mt-2 text-xs text-subtle tabular-nums">{r.detail}</div>
          </div>
        {/each}
      </div>

      <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="CPU cores" value={specs?.cpu?.cores ?? '—'} hint={`${specs?.cpu?.threads ?? '—'} threads · ${specs?.cpu?.sockets ?? '—'} socket(s)`} icon={Cpu} />
        <StatCard label="GPU" value={specs?.gpu?.count ?? '—'} hint={specs?.gpu?.name || 'No GPU detected'} icon={MonitorCog} />
        <StatCard
          label="GPU usage"
          value={specs?.gpu?.usage_available ? `${Number(specs.gpu.usage_percent || 0).toFixed(1)}%` : 'N/A'}
          hint={specs?.gpu?.usage_available ? 'Live utilization' : 'Driver/API unavailable'}
          icon={Activity}
        />
        <StatCard
          label="Storage"
          value={`${specs?.disk?.length ?? '—'} device(s)`}
          hint={specs?.disk?.map((d: any) => d.disk_type).filter(Boolean).join(', ') || 'No disk info'}
          icon={HardDrive}
        />
      </div>

      <div class="mb-6 grid grid-cols-2 gap-3">
        <StatCard label="Network in" value={fmtBytes(metrics.net_in_bytes || 0)} icon={ArrowDown} />
        <StatCard label="Network out" value={fmtBytes(metrics.net_out_bytes || 0)} icon={ArrowUp} />
      </div>
    {:else}
      <div class="card mb-6">
        <EmptyState
          compact
          icon={HardDrive}
          title={metricsState === 'error' ? 'Metrics unavailable' : 'No metrics yet'}
          description={metricsState === 'error' ? 'The panel could not load metrics for this server. Try again shortly.' : "The agent hasn't reported metrics for this server yet."}
          actionLabel="Retry"
          onaction={loadMetrics}
        />
      </div>
    {/if}

    {#if isAdmin}
      <div class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Package size={16} class="text-subtle" /> System updates</h2>
          <button onclick={loadUpdates} disabled={updatesLoading || !selectedServer} class="btn btn-secondary btn-sm">
            {#if updatesLoading}<LoaderCircle size={14} class="animate-spin" />{:else}<ArrowUpCircle size={14} />{/if} Check updates
          </button>
        </div>
        <div class="card-body">
          {#if !showUpdates}
            <p class="text-[13px] text-muted">Runs <code class="code">apt list --upgradable</code> on the server and shows up to 30 packages.</p>
          {:else if updatesLoading}
            <div class="space-y-2">{#each Array(5) as _, i (i)}<div class="skeleton h-3.5" style="width: {[70, 88, 64, 80, 56][i]}%"></div>{/each}</div>
          {:else}
            <pre class="max-h-[50vh] overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{updates}</pre>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</DashboardLayout>
