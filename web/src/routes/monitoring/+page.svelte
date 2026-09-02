<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import MetricsChart from '$components/MetricsChart.svelte';
  import MetricsGauge from '$components/MetricsGauge.svelte';
  import { Activity, Server, Clock, RefreshCw, Copy, Check, Globe, Cpu, HardDrive, MemoryStick, Zap, Wifi, Shield } from 'lucide-svelte';
  import { onMount } from 'svelte';

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
  let stopped = false;

  const ranges = [
    { label: '1H', value: '1h' },
    { label: '6H', value: '6h' },
    { label: '24H', value: '24h' },
    { label: '7D', value: '7d' },
  ];

  async function fetchServers() {
    if (stopped || !auth.token) return;
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
    if (stopped || !selectedServer || !auth.token) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/metrics/latest`);
      latest = data;
      lastUpdate = new Date();
    } catch (e: any) {
      if (e.message === 'session expired') { stopped = true; stopAutoRefresh(); return; }
      latest = null;
    }
  }

  async function fetchHistory() {
    if (stopped || !selectedServer || !auth.token) return;
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

  function startAutoRefresh() {
    stopAutoRefresh();
    if (autoRefresh) {
      interval = setInterval(refresh, 10000);
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

  $effect(() => {
    selectedServer;
    range;
    refresh();
  });

  onMount(() => {
    (async () => {
      await fetchServers();
      await refresh();
      startAutoRefresh();
    })();
    return () => stopAutoRefresh();
  });

  function copyToClipboard(text: string, field: string) {
    navigator.clipboard.writeText(text);
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

  const cpuData = $derived(() => {
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    return { labels, values: history.map((m: any) => m.cpu_percent || 0) };
  });

  const memData = $derived(() => {
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    const usedGB = history.map((m: any) => m.mem_total ? (m.mem_used || 0) / (1024**3) : 0);
    const totalGB = history.map((m: any) => m.mem_total ? m.mem_total / (1024**3) : 0);
    return { labels, usedGB, totalGB };
  });

  const swapData = $derived(() => {
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    const usedGB = history.map((m: any) => m.swap_total ? (m.swap_used || 0) / (1024**3) : 0);
    const totalGB = history.map((m: any) => m.swap_total ? m.swap_total / (1024**3) : 0);
    return { labels, usedGB, totalGB };
  });

  const diskData = $derived(() => {
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    const usedGB = history.map((m: any) => m.disk_total ? (m.disk_used || 0) / (1024**3) : 0);
    const totalGB = history.map((m: any) => m.disk_total ? m.disk_total / (1024**3) : 0);
    return { labels, usedGB, totalGB };
  });

  const netData = $derived(() => {
    const labels = history.map((m: any) => {
      const d = new Date(m.collected_at);
      return range === '7d' ? d.toLocaleDateString('id', { day: '2-digit', month: 'short' })
        : d.toLocaleTimeString('id', { hour: '2-digit', minute: '2-digit' });
    });
    const inGB = history.map((m: any) => (m.net_in_bytes || 0) / (1024**3));
    const outGB = history.map((m: any) => (m.net_out_bytes || 0) / (1024**3));
    return { labels, inGB, outGB };
  });
</script>

<DashboardLayout active="Monitoring">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Monitoring</h1>
      <p class="text-sm text-slate-500 mt-1">Real-time system metrics & historical data</p>
    </div>
    <div class="flex items-center gap-3">
      <!-- Server selector -->
      {#if servers.length > 1}
        <select bind:value={selectedServer}
          class="px-3 py-2 rounded-xl text-sm bg-white/[0.04] border border-white/[0.07] text-slate-200 outline-none focus:border-sky-400/40 transition-colors">
          {#each servers as s}
            <option value={s.id}>{s.name}</option>
          {/each}
        </select>
      {/if}
      
      <!-- Range selector -->
      <div class="flex bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        {#each ranges as r}
          <button onclick={() => range = r.value}
            class="px-3 py-2 text-xs font-medium transition-all {range === r.value ? 'bg-sky-500/15 text-sky-400' : 'text-slate-500 hover:text-slate-300'}">
            {r.label}
          </button>
        {/each}
      </div>
      
      <!-- Auto-refresh toggle -->
      <button onclick={toggleAutoRefresh}
        class="p-2 rounded-xl transition-all {autoRefresh ? 'bg-sky-500/10 text-sky-400' : 'bg-white/[0.03] text-slate-500 hover:text-slate-300'}"
        title={autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}>
        <RefreshCw size={16} class="{autoRefresh ? 'animate-spin' : ''}" style="animation-duration: 3s" />
      </button>
    </div>
  </div>

  {#if loading && !latest}
    <div class="flex items-center justify-center py-32">
      <div class="flex flex-col items-center gap-4">
        <div class="relative">
          <div class="absolute inset-0 rounded-2xl bg-sky-400/20 blur-xl animate-pulse"></div>
          <div class="relative w-12 h-12 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-lg">
            <Activity size={20} class="text-white animate-pulse" />
          </div>
        </div>
        <span class="text-sm text-slate-500">Loading metrics...</span>
      </div>
    </div>
  {:else if !latest}
    <!-- No data state -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-12 text-center">
      <Activity size={48} class="text-white/10 mx-auto mb-4" />
      <h3 class="text-lg font-medium text-slate-200 mb-2">No Metrics Data</h3>
      <p class="text-sm text-slate-500 max-w-md mx-auto">
        Metrics will appear once the server agent starts reporting data. 
        Make sure the agent is running on your server.
      </p>
      <div class="mt-6 px-4 py-2 bg-sky-500/10 text-sky-400 rounded-lg text-xs font-mono border border-sky-500/20">
        Waiting for agent data...
      </div>
    </div>
  {:else}
    <!-- Server Info Card -->
    {#if selectedServerInfo}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6 mb-6">
        <div class="flex items-center gap-2 mb-4">
          <Server size={16} class="text-sky-400" />
          <h2 class="text-sm font-semibold text-slate-200">Server Information</h2>
        </div>
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <!-- IPv4 -->
          <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
            <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-2">IPv4 Address</div>
            <div class="flex items-center gap-2">
              <span class="text-sm font-mono text-slate-200 truncate">{selectedServerInfo.ip_address || 'N/A'}</span>
              {#if selectedServerInfo.ip_address}
                <button onclick={() => copyToClipboard(selectedServerInfo.ip_address, 'ipv4')}
                  class="p-1 rounded-lg hover:bg-white/[0.05] transition-colors {copiedField === 'ipv4' ? 'text-emerald-400' : 'text-slate-500 hover:text-slate-300'}">
                  {#if copiedField === 'ipv4'}<Check size={12} />{:else}<Copy size={12} />{/if}
                </button>
              {/if}
            </div>
          </div>
          
          <!-- IPv6 -->
          <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
            <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-2">IPv6 Address</div>
            <div class="flex items-center gap-2">
              <span class="text-sm font-mono text-slate-200 truncate">{selectedServerInfo.ipv6 || 'N/A'}</span>
              {#if selectedServerInfo.ipv6}
                <button onclick={() => copyToClipboard(selectedServerInfo.ipv6, 'ipv6')}
                  class="p-1 rounded-lg hover:bg-white/[0.05] transition-colors {copiedField === 'ipv6' ? 'text-emerald-400' : 'text-slate-500 hover:text-slate-300'}">
                  {#if copiedField === 'ipv6'}<Check size={12} />{:else}<Copy size={12} />{/if}
                </button>
              {/if}
            </div>
          </div>
          
          <!-- Provider -->
          <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
            <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-2">Cloud Provider</div>
            <div class="flex items-center gap-2">
              <Globe size={14} class="text-sky-400" />
              <span class="text-sm font-medium text-slate-200">{selectedServerInfo.provider || 'Unknown'}</span>
            </div>
          </div>
          
          <!-- Status -->
          <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
            <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-2">Status</div>
            <div class="flex items-center gap-2">
              <div class="w-2 h-2 rounded-full {selectedServerInfo.status === 'active' ? 'bg-emerald-400' : 'bg-amber-400'}"></div>
              <span class="text-sm font-medium text-slate-200 capitalize">{selectedServerInfo.status}</span>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Real-time gauges -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6 mb-6">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-sm font-semibold text-slate-200 flex items-center gap-2">
          <Activity size={14} class="text-sky-400" /> Live Metrics
        </h2>
        {#if lastUpdate}
          <span class="text-[0.65rem] text-slate-500 flex items-center gap-1">
            <Clock size={10} /> {lastUpdate.toLocaleTimeString('id')}
          </span>
        {/if}
      </div>
      
      <!-- CPU Gauge -->
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-7 gap-4">
        <MetricsGauge value={latest.cpu_percent || 0} label="CPU" color="#38bdf8" />
        
        <!-- Memory with GB labels -->
        <div class="flex flex-col items-center gap-2">
          <div class="relative w-24 h-24">
            <svg class="w-full h-full -rotate-90" viewBox="0 0 80 80">
              <circle cx="40" cy="40" r="36" fill="none" stroke="currentColor" stroke-width="6" class="text-white/[0.05]" />
              <circle cx="40" cy="40" r="36" fill="none" stroke="#34d399" stroke-width="6" stroke-linecap="round"
                stroke-dasharray={2 * Math.PI * 36}
                stroke-dashoffset={2 * Math.PI * 36 * (1 - memPercent / 100)}
                class="transition-all duration-700 ease-out" />
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center">
              <span class="text-lg font-bold tabular-nums text-emerald-400">{Math.round(memPercent)}%</span>
            </div>
          </div>
          <div class="text-center">
            <div class="text-xs font-medium text-slate-300">Memory</div>
            <div class="text-[0.65rem] text-slate-500">{fmtBytes(latest.mem_used || 0)} / {fmtBytes(latest.mem_total || 0)}</div>
            <div class="text-[0.6rem] text-slate-600">Free: {fmtBytes((latest.mem_total || 0) - (latest.mem_used || 0))}</div>
          </div>
        </div>
        
        <!-- Disk with GB labels -->
        <div class="flex flex-col items-center gap-2">
          <div class="relative w-24 h-24">
            <svg class="w-full h-full -rotate-90" viewBox="0 0 80 80">
              <circle cx="40" cy="40" r="36" fill="none" stroke="currentColor" stroke-width="6" class="text-white/[0.05]" />
              <circle cx="40" cy="40" r="36" fill="none" stroke="#a78bfa" stroke-width="6" stroke-linecap="round"
                stroke-dasharray={2 * Math.PI * 36}
                stroke-dashoffset={2 * Math.PI * 36 * (1 - diskPercent / 100)}
                class="transition-all duration-700 ease-out" />
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center">
              <span class="text-lg font-bold tabular-nums text-violet-400">{Math.round(diskPercent)}%</span>
            </div>
          </div>
          <div class="text-center">
            <div class="text-xs font-medium text-slate-300">Disk</div>
            <div class="text-[0.65rem] text-slate-500">{fmtBytes(latest.disk_used || 0)} / {fmtBytes(latest.disk_total || 0)}</div>
            <div class="text-[0.6rem] text-slate-600">Free: {fmtBytes((latest.disk_total || 0) - (latest.disk_used || 0))}</div>
          </div>
        </div>
        
        <!-- Swap with GB labels -->
        <div class="flex flex-col items-center gap-2">
          <div class="relative w-24 h-24">
            <svg class="w-full h-full -rotate-90" viewBox="0 0 80 80">
              <circle cx="40" cy="40" r="36" fill="none" stroke="currentColor" stroke-width="6" class="text-white/[0.05]" />
              <circle cx="40" cy="40" r="36" fill="none" stroke="#f59e0b" stroke-width="6" stroke-linecap="round"
                stroke-dasharray={2 * Math.PI * 36}
                stroke-dashoffset={2 * Math.PI * 36 * (1 - swapPercent / 100)}
                class="transition-all duration-700 ease-out" />
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center">
              <span class="text-lg font-bold tabular-nums text-amber-400">{Math.round(swapPercent)}%</span>
            </div>
          </div>
          <div class="text-center">
            <div class="text-xs font-medium text-slate-300">Swap</div>
            <div class="text-[0.65rem] text-slate-500">{fmtBytes(latest.swap_used || 0)} / {fmtBytes(latest.swap_total || 0)}</div>
            <div class="text-[0.6rem] text-slate-600">Free: {fmtBytes((latest.swap_total || 0) - (latest.swap_used || 0))}</div>
          </div>
        </div>
        
        <!-- Net In -->
        <MetricsGauge 
          value={latest.net_in_bytes || 0} 
          max={Math.max(latest.net_in_bytes || 1, latest.net_out_bytes || 1) * 1.2} 
          label="Net In" 
          color="#f59e0b" 
          displayUnit="bytes" />
        
        <!-- Net Out -->
        <MetricsGauge 
          value={latest.net_out_bytes || 0} 
          max={Math.max(latest.net_in_bytes || 1, latest.net_out_bytes || 1) * 1.2} 
          label="Net Out" 
          color="#f472b6" 
          displayUnit="bytes" />
      </div>
    </div>

    <!-- Charts -->
    <div class="grid gap-6 lg:grid-cols-2 mb-6">
      <!-- CPU Chart -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5">
        <MetricsChart 
          title="CPU Usage"
          labels={cpuData().labels}
          datasets={[{ label: 'CPU %', data: cpuData().values, color: '#38bdf8', fillColor: 'rgba(56,189,248,0.08)' }]}
          yLabel="%"
          height="220px" />
      </div>

      <!-- Memory Chart -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5">
        <MetricsChart 
          title="Memory Usage"
          labels={memData().labels}
          datasets={[{ label: 'Memory GB', data: memData().usedGB, color: '#34d399', fillColor: 'rgba(52,211,153,0.08)' }]}
          yLabel="GB"
          height="220px" />
      </div>

      <!-- Swap Chart -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5">
        <MetricsChart 
          title="Swap Usage"
          labels={swapData().labels}
          datasets={[{ label: 'Swap GB', data: swapData().usedGB, color: '#f59e0b', fillColor: 'rgba(245,158,11,0.08)' }]}
          yLabel="GB"
          height="220px" />
      </div>

      <!-- Disk Chart -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5">
        <MetricsChart 
          title="Disk Usage"
          labels={diskData().labels}
          datasets={[{ label: 'Disk GB', data: diskData().usedGB, color: '#a78bfa', fillColor: 'rgba(167,139,250,0.08)' }]}
          yLabel="GB"
          height="220px" />
      </div>

      <!-- Network Chart -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5">
        <MetricsChart 
          title="Network I/O"
          labels={netData().labels}
          datasets={[
            { label: 'Inbound', data: netData().inGB, color: '#f59e0b', fillColor: 'rgba(245,158,11,0.08)' },
            { label: 'Outbound', data: netData().outGB, color: '#f472b6', fillColor: 'rgba(244,114,182,0.08)' },
          ]}
          yLabel="GB"
          height="220px" />
      </div>
    </div>

    <!-- Quick stats -->
    <div class="grid gap-4 grid-cols-2 lg:grid-cols-6">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Data Points</div>
        <div class="text-xl font-bold text-white tabular-nums">{history.length}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Avg CPU</div>
        <div class="text-xl font-bold text-sky-400 tabular-nums">
          {history.length > 0 ? (history.reduce((s: number, m: any) => s + (m.cpu_percent || 0), 0) / history.length).toFixed(1) : '0'}%
        </div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Memory</div>
        <div class="text-xl font-bold text-emerald-400 tabular-nums">
          {fmtBytes(latest?.mem_used || 0)}
        </div>
        <div class="text-[0.6rem] text-slate-600">{fmtBytes(latest?.mem_total || 0)} total</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Disk</div>
        <div class="text-xl font-bold text-violet-400 tabular-nums">
          {fmtBytes(latest?.disk_used || 0)}
        </div>
        <div class="text-[0.6rem] text-slate-600">{fmtBytes(latest?.disk_total || 0)} total</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Swap</div>
        <div class="text-xl font-bold text-amber-400 tabular-nums">
          {fmtBytes(latest?.swap_used || 0)}
        </div>
        <div class="text-[0.6rem] text-slate-600">{fmtBytes(latest?.swap_total || 0)} total</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
        <div class="text-[0.65rem] font-semibold uppercase tracking-wider text-slate-500 mb-1">Network</div>
        <div class="text-xl font-bold text-amber-400 tabular-nums">
          {fmtBytes(latest?.net_in_bytes || 0)}
        </div>
        <div class="text-[0.6rem] text-slate-600">↑ {fmtBytes(latest?.net_out_bytes || 0)}</div>
      </div>
    </div>
  {/if}

  <div class="mt-10 text-center text-xs text-slate-600/60">
    Auto-refresh: {autoRefresh ? 'ON (10s)' : 'OFF'} &bull; Range: {range}
  </div>
</DashboardLayout>
