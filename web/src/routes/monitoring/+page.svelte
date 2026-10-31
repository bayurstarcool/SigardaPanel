<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import MetricsChart from '$components/MetricsChart.svelte';
  import MetricsGauge from '$components/MetricsGauge.svelte';
  import { Activity, Server, Clock, Copy, Check, Globe, Cpu, HardDrive, MemoryStick, Wifi, Database, BarChart3 } from 'lucide-svelte';
  import { onMount, untrack } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number | null>(null);
  let latest = $state<any>(null);
  let history = $state<any[]>([]);
  let loading = $state(true);
  let range = $state('24h');
  let autoRefresh = $state(true);
  let interval: ReturnType<typeof setInterval>;
  let lastUpdate = $state<Date | null>(null);
  let copiedField = $state<string | null>(null);
  let latestError = $state('');
  let stopped = false;

  const ranges = [
    { label: '1H', value: '1h' },
    { label: '6H', value: '6h' },
    { label: '24H', value: '24h' },
    { label: '7D', value: '7d' },
  ];

  async function fetchServers() {
    if (stopped || !auth.user) return;
    try {
      const data = await apiFetch(auth.token, 'GET', 'servers');
      servers = data?.servers || [];
      if (servers.length > 0 && !selectedServer) {
        selectedServer = servers[0].id;
      }
    } catch (e: any) {
      if (e.message === 'session expired') { stopped = true; stopAutoRefresh(); }
    }
  }

  async function fetchLatest() {
    if (stopped || !selectedServer || !auth.user) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/metrics/latest`);
      latest = data;
      latestError = '';
      lastUpdate = new Date();
    } catch (e: any) {
      if (e.message === 'session expired') { stopped = true; stopAutoRefresh(); return; }
      latest = null;
      // 404 "no metrics yet" is the normal state for a freshly added server.
      latestError = /no metrics|not found/i.test(e.message || '') ? '' : (e.message || 'Failed to load metrics');
    }
  }

  async function fetchHistory() {
    if (stopped || !selectedServer || !auth.user) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/metrics?range=${range}`);
      history = data?.metrics || [];
    } catch (e: any) {
      if (e.message === 'session expired') { stopped = true; stopAutoRefresh(); return; }
      history = [];
    }
  }

  async function refresh() {
    if (stopped) return;
    await Promise.all([fetchLatest(), fetchHistory()]);
  }

  // Polling skips hidden tabs; a refresh runs as soon as the tab is visible again.
  function tick() {
    if (document.hidden) return;
    refresh();
  }

  function onVisibility() {
    if (!document.hidden && autoRefresh && !stopped) refresh();
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    if (autoRefresh) {
      interval = setInterval(tick, 10000);
    }
  }

  function stopAutoRefresh() {
    if (interval) clearInterval(interval);
  }

  function toggleAutoRefresh() {
    autoRefresh = !autoRefresh;
    if (autoRefresh) startAutoRefresh();
    else stopAutoRefresh();
  }

  // Fetches whenever the selected server or range changes (including the
  // initial selection made by fetchServers).
  $effect(() => {
    if (!selectedServer) return;
    range;
    untrack(() => { refresh().finally(() => { loading = false; }); });
  });

  onMount(() => {
    fetchServers().finally(() => { if (!selectedServer) loading = false; });
    startAutoRefresh();
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      stopped = true;
      stopAutoRefresh();
      document.removeEventListener('visibilitychange', onVisibility);
    };
  });

  function copyToClipboard(text: string, field: string) {
    navigator.clipboard?.writeText(text).catch(() => {});
    copiedField = field;
    setTimeout(() => { copiedField = null; }, 2000);
  }

  function fmtBytes(b: number): string {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }

  // Derived data
  const selectedServerInfo = $derived(servers.find((s: any) => s.id === selectedServer));

  const memPercent = $derived(latest?.mem_total ? ((latest.mem_used || 0) / latest.mem_total * 100) : 0);
  const diskPercent = $derived(latest?.disk_total ? ((latest.disk_used || 0) / latest.disk_total * 100) : 0);
  const swapPercent = $derived(latest?.swap_total ? ((latest.swap_used || 0) / latest.swap_total * 100) : 0);

  // One pass over history for all charts.
  const series = $derived.by(() => {
    const GB = 1024 ** 3;
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    return {
      labels,
      cpu: history.map((m: any) => m.cpu_percent || 0),
      mem: history.map((m: any) => m.mem_total ? (m.mem_used || 0) / GB : 0),
      swap: history.map((m: any) => m.swap_total ? (m.swap_used || 0) / GB : 0),
      disk: history.map((m: any) => m.disk_total ? (m.disk_used || 0) / GB : 0),
      netIn: history.map((m: any) => (m.net_in_bytes || 0) / GB),
      netOut: history.map((m: any) => (m.net_out_bytes || 0) / GB),
    };
  });

  const avgCpu = $derived(history.length > 0 ? (history.reduce((s: number, m: any) => s + (m.cpu_percent || 0), 0) / history.length).toFixed(1) : '0');
  const netMax = $derived(Math.max(latest?.net_in_bytes || 1, latest?.net_out_bytes || 1) * 1.2);
</script>

<DashboardLayout active="Monitoring">
  <PageHeader title="Monitoring" description="Real-time system metrics and historical data">
    {#snippet actions()}
      {#if servers.length > 1}
        <label for="mon-server" class="sr-only">Server</label>
        <select id="mon-server" bind:value={selectedServer} class="input h-8 w-auto text-[13px]">
          {#each servers as s (s.id)}
            <option value={s.id}>{s.name}</option>
          {/each}
        </select>
      {/if}
      <div class="segmented" role="group" aria-label="Range">
        {#each ranges as r (r.value)}
          <button onclick={() => (range = r.value)} class="segment {range === r.value ? 'segment-active' : ''}">{r.label}</button>
        {/each}
      </div>
      <button
        onclick={toggleAutoRefresh}
        class="btn btn-secondary btn-sm"
        aria-pressed={autoRefresh}
        title={autoRefresh ? 'Auto-refresh every 10s — click to pause' : 'Auto-refresh paused — click to resume'}
      >
        <span class="dot {autoRefresh ? 'text-success' : 'text-subtle'}"></span>
        {autoRefresh ? 'Live' : 'Paused'}
      </button>
    {/snippet}
  </PageHeader>

  {#if loading && !latest}
    <div class="skeleton mb-6 h-28 rounded-xl"></div>
    <div class="skeleton mb-6 h-44 rounded-xl"></div>
    <div class="grid gap-4 lg:grid-cols-2">
      {#each Array(2) as _, i (i)}<div class="skeleton h-64 rounded-xl"></div>{/each}
    </div>
  {:else if servers.length === 0}
    <div class="card">
      <EmptyState icon={Server as any} title="No servers" description="Add a server to start collecting metrics.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else if !latest}
    <div class="card">
      <EmptyState
        icon={Activity as any}
        title={latestError ? 'Metrics unavailable' : 'No metrics yet'}
        description={latestError || 'Metrics appear once the server agent starts reporting. Make sure the agent is running on your server.'}
      >
        <span class="badge badge-accent"><span class="dot"></span>Waiting for agent data</span>
      </EmptyState>
    </div>
  {:else}
    {#if selectedServerInfo}
      <section class="card mb-6">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Server size={16} class="text-subtle" /> {selectedServerInfo.name ?? 'Server'}</h2>
          <span class="inline-flex items-center gap-1.5 text-xs capitalize {selectedServerInfo.status === 'active' ? 'text-success' : 'text-warning'}">
            <span class="dot"></span>{selectedServerInfo.status}
          </span>
        </div>
        <dl class="grid grid-cols-1 divide-line max-sm:divide-y sm:grid-cols-3 sm:divide-x">
          <div class="px-5 py-3">
            <dt class="stat-label">IPv4 address</dt>
            <dd class="mt-1 flex items-center gap-1.5">
              <span class="truncate font-mono text-[13px] text-fg">{selectedServerInfo.ip_address || 'N/A'}</span>
              {#if selectedServerInfo.ip_address}
                <button onclick={() => copyToClipboard(selectedServerInfo.ip_address, 'ipv4')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy IPv4 address">
                  {#if copiedField === 'ipv4'}<Check size={12} class="text-success" />{:else}<Copy size={12} />{/if}
                </button>
              {/if}
            </dd>
          </div>
          <div class="px-5 py-3">
            <dt class="stat-label">IPv6 address</dt>
            <dd class="mt-1 flex items-center gap-1.5">
              <span class="truncate font-mono text-[13px] text-fg">{selectedServerInfo.ipv6 || 'N/A'}</span>
              {#if selectedServerInfo.ipv6}
                <button onclick={() => copyToClipboard(selectedServerInfo.ipv6, 'ipv6')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy IPv6 address">
                  {#if copiedField === 'ipv6'}<Check size={12} class="text-success" />{:else}<Copy size={12} />{/if}
                </button>
              {/if}
            </dd>
          </div>
          <div class="px-5 py-3">
            <dt class="stat-label">Cloud provider</dt>
            <dd class="mt-1 flex items-center gap-1.5 text-[13px] text-fg"><Globe size={14} class="text-subtle" /> {selectedServerInfo.provider || 'Unknown'}</dd>
          </div>
        </dl>
      </section>
    {/if}

    <section class="card mb-6">
      <div class="card-header">
        <h2 class="card-title flex items-center gap-2"><Activity size={16} class="text-subtle" /> Live metrics</h2>
        {#if lastUpdate}
          <span class="flex items-center gap-1 text-xs text-subtle tabular-nums"><Clock size={12} /> {lastUpdate.toLocaleTimeString('id')}</span>
        {/if}
      </div>
      <div class="card-body grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-6">
        <MetricsGauge value={latest.cpu_percent || 0} label="CPU" />
        <MetricsGauge
          value={memPercent}
          label="Memory"
          detail="{fmtBytes(latest.mem_used || 0)} / {fmtBytes(latest.mem_total || 0)}"
          sub="Free {fmtBytes((latest.mem_total || 0) - (latest.mem_used || 0))}"
        />
        <MetricsGauge
          value={diskPercent}
          label="Disk"
          detail="{fmtBytes(latest.disk_used || 0)} / {fmtBytes(latest.disk_total || 0)}"
          sub="Free {fmtBytes((latest.disk_total || 0) - (latest.disk_used || 0))}"
        />
        <MetricsGauge
          value={swapPercent}
          label="Swap"
          detail="{fmtBytes(latest.swap_used || 0)} / {fmtBytes(latest.swap_total || 0)}"
          sub="Free {fmtBytes((latest.swap_total || 0) - (latest.swap_used || 0))}"
        />
        <MetricsGauge value={latest.net_in_bytes || 0} max={netMax} label="Net in" tone="info" thresholds={false} displayUnit="bytes" />
        <MetricsGauge value={latest.net_out_bytes || 0} max={netMax} label="Net out" tone="info" thresholds={false} displayUnit="bytes" />
      </div>
    </section>

    <div class="mb-6 grid gap-4 lg:grid-cols-2">
      <div class="card card-body">
        <MetricsChart title="CPU usage" labels={series.labels} datasets={[{ label: 'CPU %', data: series.cpu }]} yLabel="%" height="220px" />
      </div>
      <div class="card card-body">
        <MetricsChart title="Memory usage" labels={series.labels} datasets={[{ label: 'Memory GB', data: series.mem }]} yLabel="GB" height="220px" />
      </div>
      <div class="card card-body">
        <MetricsChart title="Swap usage" labels={series.labels} datasets={[{ label: 'Swap GB', data: series.swap }]} yLabel="GB" height="220px" />
      </div>
      <div class="card card-body">
        <MetricsChart title="Disk usage" labels={series.labels} datasets={[{ label: 'Disk GB', data: series.disk }]} yLabel="GB" height="220px" />
      </div>
      <div class="card card-body lg:col-span-2">
        <MetricsChart
          title="Network I/O"
          labels={series.labels}
          datasets={[
            { label: 'Inbound', data: series.netIn, tone: 'accent' },
            { label: 'Outbound', data: series.netOut, tone: 'info' },
          ]}
          yLabel="GB"
          height="220px"
        />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
      <StatCard label="Data points" value={history.length} icon={BarChart3 as any} hint="in {range}" />
      <StatCard label="Avg CPU" value="{avgCpu}%" icon={Cpu as any} />
      <StatCard label="Memory" value={fmtBytes(latest?.mem_used || 0)} hint="{fmtBytes(latest?.mem_total || 0)} total" icon={MemoryStick as any} />
      <StatCard label="Disk" value={fmtBytes(latest?.disk_used || 0)} hint="{fmtBytes(latest?.disk_total || 0)} total" icon={HardDrive as any} />
      <StatCard label="Swap" value={fmtBytes(latest?.swap_used || 0)} hint="{fmtBytes(latest?.swap_total || 0)} total" icon={Database as any} />
      <StatCard label="Network in" value={fmtBytes(latest?.net_in_bytes || 0)} hint="out {fmtBytes(latest?.net_out_bytes || 0)}" icon={Wifi as any} />
    </div>
  {/if}

  <p class="mt-8 text-center text-xs text-subtle">
    Auto-refresh {autoRefresh ? 'on (10s, paused while the tab is hidden)' : 'off'} · Range {range}
  </p>
</DashboardLayout>
