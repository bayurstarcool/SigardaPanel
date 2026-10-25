<script lang="ts">
  import { auth, bearerHeaders } from '$lib/auth.svelte';
  import { page } from '$app/stores';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Cpu, HardDrive, MemoryStick, Wifi, Globe, Server, Zap, RefreshCw, Clock, Cloud } from 'lucide-svelte';
  import { onMount } from 'svelte';

  const serverId = $page.params.id;

  let specs = $state<any>(null);
  let benchmark = $state<any>(null);
  let loadingSpecs = $state(true);
  let loadingBench = $state(false);
  let error = $state('');
  let lastUpdate = $state<Date | null>(null);

  const memPct = $derived(specs?.memory?.total_bytes ? ((specs.memory.used_bytes || 0) / specs.memory.total_bytes * 100) : 0);
  const swapPct = $derived(specs?.memory?.swap_total ? ((specs.memory.swap_used || 0) / specs.memory.swap_total * 100) : 0);

  function diskPct(d: any): number {
    return d.size_bytes ? ((d.used_bytes || 0) / d.size_bytes * 100) : 0;
  }

  async function fetchSpecs() {
    if (!auth.user) { window.location.href = '/login'; return; }
    loadingSpecs = true;
    error = '';
    try {
      const res = await fetch(`/api/proxy/servers/${serverId}/specs`, {
        headers: { ...bearerHeaders(), 'Content-Type': 'application/json' }
      });
      const json = await res.json();
      if (json.error) throw new Error(json.error.message);
      specs = json.data;
      lastUpdate = new Date();
    } catch (e: any) {
      error = e.message || 'Failed to load specs';
    } finally {
      loadingSpecs = false;
    }
  }

  async function runBenchmark() {
    if (!auth.user) return;
    loadingBench = true;
    error = '';
    try {
      const res = await fetch(`/api/proxy/servers/${serverId}/benchmark`, {
        headers: { ...bearerHeaders(), 'Content-Type': 'application/json' }
      });
      const json = await res.json();
      if (json.error) throw new Error(json.error.message);
      benchmark = json.data;
    } catch (e: any) {
      error = e.message || 'Benchmark failed';
    } finally {
      loadingBench = false;
    }
  }

  onMount(() => fetchSpecs());

  function fmtBytes(b: number): string {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }

  function fmtIOPS(iops: number): string {
    if (!iops) return 'N/A';
    if (iops >= 1000000) return (iops / 1000000).toFixed(1) + 'M';
    if (iops >= 1000) return (iops / 1000).toFixed(1) + 'K';
    return iops.toFixed(0);
  }

  function pctColor(pct: number): string {
    if (pct > 80) return '#ef4444';
    if (pct > 60) return '#f59e0b';
    return '#34d399';
  }
</script>

<DashboardLayout active="Servers">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <div class="flex items-center gap-2 text-xs text-slate-500 mb-1">
        <a href="/servers" class="hover:text-sky-400 transition">Servers</a>
        <span>›</span>
        <span>Server #{serverId}</span>
        <span>›</span>
        <span class="text-slate-400">Specs & Benchmark</span>
      </div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Server Specifications</h1>
      <p class="text-sm text-slate-500 mt-1">{lastUpdate ? `Updated: ${lastUpdate.toLocaleTimeString()}` : 'Loading...'}</p>
    </div>
    <div class="flex gap-2">
      <button onclick={fetchSpecs} disabled={loadingSpecs} class="px-4 py-2 rounded-lg text-sm bg-white/10 text-white hover:bg-white/20 transition flex items-center gap-2 border border-white/10 cursor-pointer disabled:opacity-50">
        <RefreshCw size={14} class={loadingSpecs ? 'animate-spin' : ''} /> Refresh
      </button>
      <button onclick={runBenchmark} disabled={loadingBench} class="px-4 py-2 rounded-lg text-sm bg-sky-600 text-white hover:bg-sky-500 transition flex items-center gap-2 cursor-pointer disabled:opacity-50">
        <Zap size={14} class={loadingBench ? 'animate-pulse' : ''} /> {loadingBench ? 'Running...' : 'Run Benchmark'}
      </button>
    </div>
  </div>

  {#if error}
    <div class="bg-red-500/10 border border-red-500/20 rounded-xl p-4 mb-6 text-red-400 text-sm">{error}</div>
  {/if}

  {#if loadingSpecs}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading server specifications...</div>
    </div>
  {:else if specs}
    <!-- Quick Info Cards -->
    <div class="grid gap-4 grid-cols-1 md:grid-cols-2 lg:grid-cols-5 mb-6">
      {#if specs.cloud}
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="flex items-center gap-2 mb-3"><Cloud size={16} class="text-emerald-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Cloud Provider</span></div>
          <div class="text-sm font-bold text-emerald-400">{specs.cloud.provider}</div>
          {#if specs.cloud.instance_type}<div class="text-xs text-slate-500 mt-1">Type: {specs.cloud.instance_type}</div>{/if}
          {#if specs.cloud.region}<div class="text-xs text-slate-500">Region: {specs.cloud.region}</div>{/if}
          {#if specs.cloud.instance_id}<div class="text-xs text-slate-600 mt-1 font-mono">{specs.cloud.instance_id}</div>{/if}
        </div>
      {/if}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Server size={16} class="text-sky-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">OS</span></div>
        <div class="text-sm font-medium text-white">{specs.os?.pretty_name || specs.os?.name || 'Unknown'}</div>
        <div class="text-xs text-slate-500 mt-1">Arch: {specs.os?.arch || '—'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Clock size={16} class="text-emerald-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Uptime</span></div>
        <div class="text-sm font-medium text-white">{specs.uptime || '—'}</div>
        <div class="text-xs text-slate-500 mt-1">Kernel: {specs.kernel_version || '—'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Globe size={16} class="text-amber-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Hostname</span></div>
        <div class="text-sm font-medium text-white truncate">{specs.hostname || '—'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Wifi size={16} class="text-violet-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Network</span></div>
        <div class="text-sm font-medium text-white">{specs.network?.length || 0} interface(s)</div>
        <div class="text-xs text-slate-500 mt-1">{specs.network?.[0]?.ip_address || '—'}</div>
      </div>
    </div>

    <!-- CPU -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
      <div class="flex items-center gap-2 mb-4"><Cpu size={18} class="text-sky-400" /><h2 class="text-lg font-semibold text-white">CPU</h2></div>
      <div class="grid gap-4 grid-cols-2 md:grid-cols-4">
        <div><div class="text-xs text-slate-500 mb-1">Model</div><div class="text-sm font-medium text-white">{specs.cpu?.model_name || '—'}</div></div>
        <div><div class="text-xs text-slate-500 mb-1">Vendor</div><div class="text-sm font-medium text-white">{specs.cpu?.vendor_id || '—'}</div></div>
        <div><div class="text-xs text-slate-500 mb-1">Architecture</div><div class="text-sm font-medium text-white">{specs.cpu?.architecture || '—'}</div></div>
        <div><div class="text-xs text-slate-500 mb-1">Cores / Threads</div><div class="text-sm font-medium text-white">{specs.cpu?.cores || 0}C / {specs.cpu?.threads || 0}T</div></div>
        <div><div class="text-xs text-slate-500 mb-1">Sockets</div><div class="text-sm font-medium text-white">{specs.cpu?.sockets || 0}</div></div>
        {#if specs.cpu?.max_mhz}<div><div class="text-xs text-slate-500 mb-1">Max MHz</div><div class="text-sm font-medium text-white">{specs.cpu.max_mhz}</div></div>{/if}
        {#if specs.cpu?.cache_size}<div><div class="text-xs text-slate-500 mb-1">Cache</div><div class="text-sm font-medium text-white">{specs.cpu.cache_size}</div></div>{/if}
      </div>
    </div>

    <!-- GPU -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
      <div class="flex items-center gap-2 mb-4"><Cpu size={18} class="text-fuchsia-400" /><h2 class="text-lg font-semibold text-white">GPU</h2></div>
      {#if specs.gpu?.detected && specs.gpu?.count > 0}
        <div class="grid gap-4 grid-cols-2 md:grid-cols-4">
          <div><div class="text-xs text-slate-500 mb-1">GPU Count</div><div class="text-lg font-bold text-fuchsia-400">{specs.gpu.count}</div></div>
          <div class="col-span-2"><div class="text-xs text-slate-500 mb-1">Device</div><div class="text-sm font-medium text-white truncate" title={specs.gpu.name}>{specs.gpu.name || 'Detected GPU'}</div></div>
          <div><div class="text-xs text-slate-500 mb-1">Usage</div><div class="text-lg font-bold text-fuchsia-400">{specs.gpu.usage_available ? `${Number(specs.gpu.usage_percent || 0).toFixed(1)}%` : 'N/A'}</div><div class="text-xs text-slate-600">{specs.gpu.usage_available ? 'average live utilization' : 'usage API unavailable'}</div></div>
        </div>
        {#if specs.gpu.usage_available}
          <div class="mt-4 h-2 bg-white/5 rounded-full overflow-hidden"><div class="h-full bg-fuchsia-500 rounded-full transition-all duration-500" style="width: {Math.min(Number(specs.gpu.usage_percent || 0), 100)}%"></div></div>
        {/if}
      {:else}
        <div class="text-sm text-slate-500">No GPU detected</div>
      {/if}
    </div>

    <!-- Memory -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
      <div class="flex items-center gap-2 mb-4"><MemoryStick size={18} class="text-emerald-400" /><h2 class="text-lg font-semibold text-white">Memory</h2></div>
      <div class="grid gap-6 md:grid-cols-2">
        <div>
          <div class="flex justify-between text-xs text-slate-500 mb-2"><span>RAM</span><span>{fmtBytes(specs.memory?.used_bytes || 0)} / {fmtBytes(specs.memory?.total_bytes || 0)}</span></div>
          <div class="h-3 bg-white/5 rounded-full overflow-hidden">
            <div class="h-full rounded-full transition-all duration-500" style="width: {Math.min(memPct, 100)}%; background: {pctColor(memPct)}"></div>
          </div>
          <div class="text-right text-xs text-slate-500 mt-1">{memPct.toFixed(1)}% used</div>
        </div>
        <div>
          <div class="flex justify-between text-xs text-slate-500 mb-2"><span>Swap</span><span>{fmtBytes(specs.memory?.swap_used || 0)} / {fmtBytes(specs.memory?.swap_total || 0)}</span></div>
          <div class="h-3 bg-white/5 rounded-full overflow-hidden">
            <div class="h-full bg-amber-500 rounded-full transition-all duration-500" style="width: {Math.min(swapPct, 100)}%"></div>
          </div>
          <div class="text-right text-xs text-slate-500 mt-1">{swapPct.toFixed(1)}% used</div>
        </div>
      </div>
    </div>

    <!-- Disk -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
      <div class="flex items-center gap-2 mb-4"><HardDrive size={18} class="text-violet-400" /><h2 class="text-lg font-semibold text-white">Disk</h2></div>
      {#if specs.disk?.length > 0}
        <div class="space-y-3">
          {#each specs.disk as d}
            <div class="bg-white/[0.02] rounded-lg p-4 border border-white/[0.05]">
              <div class="flex items-center justify-between mb-2">
                <div>
                  <span class="text-sm font-medium text-white">{d.name}</span>
                  {#if d.model}<span class="text-xs text-slate-500 ml-2">{d.model}</span>{/if}
                  <span class="text-xs text-slate-600 ml-2">({d.type})</span>
                  {#if d.disk_type}
                    <span class="ml-2 px-1.5 py-0.5 rounded text-[0.6rem] font-semibold {d.disk_type === 'NVMe' ? 'bg-purple-500/20 text-purple-400' : d.disk_type === 'SSD' ? 'bg-emerald-500/20 text-emerald-400' : d.disk_type === 'HDD' ? 'bg-amber-500/20 text-amber-400' : d.disk_type === 'Virtual' ? 'bg-sky-500/20 text-sky-300' : 'bg-slate-500/20 text-slate-300'}">{d.disk_type}</span>
                    {#if d.disk_type_confidence}<span class="ml-1 text-[0.6rem] text-slate-600">{d.disk_type_confidence}</span>{/if}
                  {/if}
                </div>
                {#if d.detection_reason}<div class="text-[0.65rem] text-slate-600 mt-1" title={d.detection_reason}>{d.detection_reason}</div>{/if}
                <div class="text-[0.65rem] text-slate-600 mt-1">
                  {#if d.transport}Transport: {d.transport} · {/if}
                  {#if d.vendor}Vendor: {d.vendor.trim()} · {/if}
                  {#if d.serial}ID: {d.serial}{/if}
                  {#if d.rotational !== undefined} · Rotational: {d.rotational ? 'yes' : 'no'}{/if}
                </div>
                <span class="text-sm text-slate-400">{fmtBytes(d.size_bytes)}</span>
              </div>
              {#if d.mounted}
                <div class="flex justify-between text-xs text-slate-500 mb-1"><span>{d.mount_point}</span><span>{fmtBytes(d.used_bytes || 0)} / {fmtBytes(d.size_bytes)}</span></div>
                <div class="h-2 bg-white/5 rounded-full overflow-hidden">
                  <div class="h-full bg-violet-500 rounded-full transition-all duration-500" style="width: {Math.min(diskPct(d), 100)}%"></div>
                </div>
                <div class="text-right text-xs text-slate-500 mt-1">{diskPct(d).toFixed(1)}% used</div>
              {/if}
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-sm text-slate-500">No disk information available</div>
      {/if}
    </div>

    <!-- Network -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
      <div class="flex items-center gap-2 mb-4"><Wifi size={18} class="text-amber-400" /><h2 class="text-lg font-semibold text-white">Network Interfaces</h2></div>
      {#if specs.network?.length > 0}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead><tr class="text-left text-xs text-slate-500">
              <th class="pb-2 font-medium">Interface</th><th class="pb-2 font-medium">IP Address</th><th class="pb-2 font-medium">Status</th><th class="pb-2 font-medium">IPv6</th>
            </tr></thead>
            <tbody>
              {#each specs.network as net}
                <tr class="border-t border-white/[0.03]">
                  <td class="py-2 text-white font-medium">{net.interface}</td>
                  <td class="py-2 text-slate-400 font-mono text-xs">{net.ip_address || '—'}</td>
                  <td class="py-2">
                    <span class="inline-flex items-center gap-1.5">
                      <span class="w-2 h-2 rounded-full {net.status === 'UP' ? 'bg-emerald-400' : 'bg-red-400'}"></span>
                      <span class="{net.status === 'UP' ? 'text-emerald-400' : 'text-red-400'}">{net.status}</span>
                    </span>
                  </td>
                  <td class="py-2 text-slate-500 text-xs font-mono">{net.ipv6_addresses?.length > 0 ? net.ipv6_addresses[0] : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="text-sm text-slate-500">No network information available</div>
      {/if}
    </div>

    <!-- Benchmark -->
    {#if benchmark}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-5 mb-6">
        <div class="flex items-center gap-2 mb-4">
          <Zap size={18} class="text-sky-400" /><h2 class="text-lg font-semibold text-white">Benchmark Results</h2>
          <span class="text-xs text-slate-500 ml-auto">Run at: {new Date(benchmark.run_at).toLocaleString()}</span>
        </div>
        <div class="grid gap-6 md:grid-cols-3">
          <!-- CPU -->
          <div class="bg-white/[0.02] rounded-xl p-4 border border-white/[0.05]">
            <div class="flex items-center gap-2 mb-3"><Cpu size={14} class="text-sky-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">CPU (sysbench)</span></div>
            <div class="space-y-2">
              <div>
                <div class="text-xs text-slate-500">Single Thread</div>
                <div class="text-lg font-bold text-sky-400">{benchmark.cpu?.single_thread?.events_per_sec?.toFixed(0) || 'N/A'} <span class="text-xs text-slate-500">events/s</span></div>
                <div class="text-xs text-slate-600">Latency avg: {benchmark.cpu?.single_thread?.latency_avg_ms?.toFixed(2) || 'N/A'} ms</div>
              </div>
              <div class="border-t border-white/[0.05] pt-2">
                <div class="text-xs text-slate-500">Multi Thread ({specs.cpu?.threads || '?'}t)</div>
                <div class="text-lg font-bold text-emerald-400">{benchmark.cpu?.multi_thread?.events_per_sec?.toFixed(0) || 'N/A'} <span class="text-xs text-slate-500">events/s</span></div>
                <div class="text-xs text-slate-600">Latency avg: {benchmark.cpu?.multi_thread?.latency_avg_ms?.toFixed(2) || 'N/A'} ms</div>
              </div>
            </div>
          </div>
          <!-- Disk -->
          <div class="bg-white/[0.02] rounded-xl p-4 border border-white/[0.05]">
            <div class="flex items-center gap-2 mb-3"><HardDrive size={14} class="text-violet-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Disk I/O</span></div>
            <div class="space-y-2">
              <div><div class="text-xs text-slate-500">Sequential Write</div><div class="text-lg font-bold text-violet-400">{benchmark.disk?.seq_write_mbs?.toFixed(1) || 'N/A'} <span class="text-xs text-slate-500">MB/s</span></div></div>
              <div><div class="text-xs text-slate-500">Sequential Read</div><div class="text-lg font-bold text-violet-400">{benchmark.disk?.seq_read_mbs?.toFixed(1) || 'N/A'} <span class="text-xs text-slate-500">MB/s</span></div></div>
              {#if benchmark.disk?.random_read_iops}
                <div><div class="text-xs text-slate-500">Random Read IOPS</div><div class="text-lg font-bold text-amber-400">{fmtIOPS(benchmark.disk.random_read_iops)} <span class="text-xs text-slate-500">IOPS</span></div><div class="text-xs text-slate-600">{benchmark.disk?.random_read_mbs?.toFixed(1) || 'N/A'} MB/s</div></div>
              {/if}
            </div>
          </div>
          <!-- Network -->
          <div class="bg-white/[0.02] rounded-xl p-4 border border-white/[0.05]">
            <div class="flex items-center gap-2 mb-3"><Globe size={14} class="text-amber-400" /><span class="text-xs font-semibold uppercase tracking-wider text-slate-500">Network</span></div>
            <div class="space-y-2">
              <div><div class="text-xs text-slate-500">Download Speed</div><div class="text-lg font-bold text-amber-400">{benchmark.network?.download_mbs?.toFixed(1) || 'N/A'} <span class="text-xs text-slate-500">MB/s</span></div><div class="text-xs text-slate-600">{benchmark.network?.download_mbs ? (benchmark.network.download_mbs * 8).toFixed(0) : 'N/A'} Mbps</div></div>
              <div>
                <div class="text-xs text-slate-500">Ping (Google DNS)</div>
                <div class="text-lg font-bold text-sky-400">{benchmark.network?.ping_ms_avg?.toFixed(2) || 'N/A'} <span class="text-xs text-slate-500">ms avg</span></div>
                <div class="text-xs text-slate-600">min: {benchmark.network?.ping_ms_min?.toFixed(2) || '—'} | max: {benchmark.network?.ping_ms_max?.toFixed(2) || '—'} ms</div>
              </div>
              <div><div class="text-xs text-slate-500">Packet Loss</div><div class="text-sm font-medium {benchmark.network?.packet_loss === '0%' ? 'text-emerald-400' : 'text-red-400'}">{benchmark.network?.packet_loss || 'N/A'}</div></div>
            </div>
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Server size={48} class="mb-4 opacity-30 mx-auto" />
      <h2 class="text-lg font-semibold mb-2">No Server Data</h2>
      <p class="text-sm text-slate-500 mb-6">Unable to load server specifications. Check if the agent is online.</p>
      <button onclick={fetchSpecs} class="px-4 py-2 rounded-lg text-sm bg-sky-600 text-white hover:bg-sky-500 transition cursor-pointer">Retry</button>
    </div>
  {/if}

  <div class="mt-10 text-center text-xs text-slate-600/60">Specs auto-loaded on page visit · Benchmark requires sysbench (CPU) &amp; fio (IOPS) on agent</div>
</DashboardLayout>
