<script lang="ts">
  import { auth, bearerHeaders } from '$lib/auth.svelte';
  import { page } from '$app/stores';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Cpu, HardDrive, MemoryStick, Wifi, Globe, Server, Zap, RefreshCw, Clock, Cloud, CircuitBoard, Loader2 } from 'lucide-svelte';
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

  // Progress fill tone: theme token classes instead of hard-coded hex.
  function pctTone(pct: number): string {
    if (pct > 80) return 'bg-danger';
    if (pct > 60) return 'bg-warning';
    return 'bg-success';
  }
</script>

<DashboardLayout active="Servers">
  <PageHeader
    title="Server specifications"
    back="/servers"
    backLabel="Servers"
    description={lastUpdate ? `Server #${serverId} · updated ${lastUpdate.toLocaleTimeString()}` : `Server #${serverId}`}
  >
    {#snippet actions()}
      <button onclick={fetchSpecs} disabled={loadingSpecs} class="btn btn-secondary btn-sm">
        <RefreshCw size={14} class={loadingSpecs ? 'animate-spin' : ''} /> Refresh
      </button>
      <button onclick={runBenchmark} disabled={loadingBench} class="btn btn-primary btn-sm">
        {#if loadingBench}<Loader2 size={14} class="animate-spin" /> Running…{:else}<Zap size={14} /> Run benchmark{/if}
      </button>
    {/snippet}
  </PageHeader>

  {#if error}
    <div class="mb-6 rounded-lg bg-danger/10 px-4 py-3 text-[13px] text-danger">{error}</div>
  {/if}

  {#if loadingSpecs}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      {#each Array(4) as _, i (i)}<div class="skeleton h-24 rounded-xl"></div>{/each}
    </div>
    <div class="space-y-4">
      {#each Array(3) as _, i (i)}<div class="skeleton h-32 rounded-xl"></div>{/each}
    </div>
  {:else if specs}
    <div class="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 {specs.cloud ? 'lg:grid-cols-5' : 'lg:grid-cols-4'}">
      {#if specs.cloud}
        <div class="card p-4">
          <div class="stat-label flex items-center gap-1.5"><Cloud size={14} /> Cloud provider</div>
          <div class="mt-1.5 text-sm font-semibold text-fg">{specs.cloud.provider}</div>
          {#if specs.cloud.instance_type}<div class="mt-1 text-xs text-muted">Type: {specs.cloud.instance_type}</div>{/if}
          {#if specs.cloud.region}<div class="text-xs text-muted">Region: {specs.cloud.region}</div>{/if}
          {#if specs.cloud.instance_id}<div class="mt-1 truncate font-mono text-xs text-subtle">{specs.cloud.instance_id}</div>{/if}
        </div>
      {/if}
      <div class="card p-4">
        <div class="stat-label flex items-center gap-1.5"><Server size={14} /> OS</div>
        <div class="mt-1.5 text-sm font-semibold text-fg">{specs.os?.pretty_name || specs.os?.name || 'Unknown'}</div>
        <div class="mt-1 text-xs text-muted">Arch: {specs.os?.arch || '—'}</div>
      </div>
      <div class="card p-4">
        <div class="stat-label flex items-center gap-1.5"><Clock size={14} /> Uptime</div>
        <div class="mt-1.5 text-sm font-semibold text-fg">{specs.uptime || '—'}</div>
        <div class="mt-1 truncate text-xs text-muted">Kernel: {specs.kernel_version || '—'}</div>
      </div>
      <div class="card p-4">
        <div class="stat-label flex items-center gap-1.5"><Globe size={14} /> Hostname</div>
        <div class="mt-1.5 truncate text-sm font-semibold text-fg">{specs.hostname || '—'}</div>
      </div>
      <div class="card p-4">
        <div class="stat-label flex items-center gap-1.5"><Wifi size={14} /> Network</div>
        <div class="mt-1.5 text-sm font-semibold text-fg">{specs.network?.length || 0} interface(s)</div>
        <div class="mt-1 font-mono text-xs text-muted">{specs.network?.[0]?.ip_address || '—'}</div>
      </div>
    </div>

    <div class="space-y-4">
      <!-- CPU -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><Cpu size={16} class="text-subtle" /> CPU</h2></div>
        <dl class="card-body grid grid-cols-2 gap-4 md:grid-cols-4">
          <div class="col-span-2 md:col-span-1"><dt class="stat-label">Model</dt><dd class="mt-1 text-sm text-fg">{specs.cpu?.model_name || '—'}</dd></div>
          <div><dt class="stat-label">Vendor</dt><dd class="mt-1 text-sm text-fg">{specs.cpu?.vendor_id || '—'}</dd></div>
          <div><dt class="stat-label">Architecture</dt><dd class="mt-1 text-sm text-fg">{specs.cpu?.architecture || '—'}</dd></div>
          <div><dt class="stat-label">Cores / threads</dt><dd class="mt-1 text-sm text-fg tabular-nums">{specs.cpu?.cores || 0}C / {specs.cpu?.threads || 0}T</dd></div>
          <div><dt class="stat-label">Sockets</dt><dd class="mt-1 text-sm text-fg tabular-nums">{specs.cpu?.sockets || 0}</dd></div>
          {#if specs.cpu?.max_mhz}<div><dt class="stat-label">Max MHz</dt><dd class="mt-1 text-sm text-fg tabular-nums">{specs.cpu.max_mhz}</dd></div>{/if}
          {#if specs.cpu?.cache_size}<div><dt class="stat-label">Cache</dt><dd class="mt-1 text-sm text-fg">{specs.cpu.cache_size}</dd></div>{/if}
        </dl>
      </section>

      <!-- GPU -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><CircuitBoard size={16} class="text-subtle" /> GPU</h2></div>
        <div class="card-body">
          {#if specs.gpu?.detected && specs.gpu?.count > 0}
            <dl class="grid grid-cols-2 gap-4 md:grid-cols-4">
              <div><dt class="stat-label">GPU count</dt><dd class="stat-value text-lg">{specs.gpu.count}</dd></div>
              <div class="col-span-2"><dt class="stat-label">Device</dt><dd class="mt-1 truncate text-sm text-fg" title={specs.gpu.name}>{specs.gpu.name || 'Detected GPU'}</dd></div>
              <div>
                <dt class="stat-label">Usage</dt>
                <dd class="stat-value text-lg">{specs.gpu.usage_available ? `${Number(specs.gpu.usage_percent || 0).toFixed(1)}%` : 'N/A'}</dd>
                <dd class="text-xs text-subtle">{specs.gpu.usage_available ? 'average live utilization' : 'usage API unavailable'}</dd>
              </div>
            </dl>
            {#if specs.gpu.usage_available}
              <div class="progress mt-4"><div class="progress-bar" style="width: {Math.min(Number(specs.gpu.usage_percent || 0), 100)}%"></div></div>
            {/if}
          {:else}
            <p class="text-[13px] text-subtle">No GPU detected</p>
          {/if}
        </div>
      </section>

      <!-- Memory -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><MemoryStick size={16} class="text-subtle" /> Memory</h2></div>
        <div class="card-body grid gap-6 md:grid-cols-2">
          <div>
            <div class="mb-2 flex justify-between text-xs"><span class="font-medium text-muted">RAM</span><span class="text-subtle tabular-nums">{fmtBytes(specs.memory?.used_bytes || 0)} / {fmtBytes(specs.memory?.total_bytes || 0)}</span></div>
            <div class="progress"><div class="progress-bar {pctTone(memPct)}" style="width: {Math.min(memPct, 100)}%"></div></div>
            <div class="mt-1 text-right text-xs text-subtle tabular-nums">{memPct.toFixed(1)}% used</div>
          </div>
          <div>
            <div class="mb-2 flex justify-between text-xs"><span class="font-medium text-muted">Swap</span><span class="text-subtle tabular-nums">{fmtBytes(specs.memory?.swap_used || 0)} / {fmtBytes(specs.memory?.swap_total || 0)}</span></div>
            <div class="progress"><div class="progress-bar {pctTone(swapPct)}" style="width: {Math.min(swapPct, 100)}%"></div></div>
            <div class="mt-1 text-right text-xs text-subtle tabular-nums">{swapPct.toFixed(1)}% used</div>
          </div>
        </div>
      </section>

      <!-- Disk -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><HardDrive size={16} class="text-subtle" /> Disk</h2></div>
        {#if specs.disk?.length > 0}
          <div class="divide-y divide-line">
            {#each specs.disk as d, i (i)}
              <div class="px-5 py-4">
                <div class="flex flex-wrap items-start justify-between gap-2">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="text-sm font-medium text-fg">{d.name}</span>
                      {#if d.model}<span class="text-xs text-muted">{d.model}</span>{/if}
                      <span class="text-xs text-subtle">({d.type})</span>
                      {#if d.disk_type}
                        <span class="badge {d.disk_type === 'NVMe' ? 'badge-accent' : d.disk_type === 'SSD' ? 'badge-success' : d.disk_type === 'HDD' ? 'badge-warning' : d.disk_type === 'Virtual' ? 'badge-info' : ''}">{d.disk_type}</span>
                        {#if d.disk_type_confidence}<span class="text-[11px] text-subtle">{d.disk_type_confidence}</span>{/if}
                      {/if}
                    </div>
                    {#if d.detection_reason}<div class="mt-1 truncate text-xs text-subtle" title={d.detection_reason}>{d.detection_reason}</div>{/if}
                    <div class="mt-0.5 text-xs text-subtle">
                      {#if d.transport}Transport: {d.transport} · {/if}
                      {#if d.vendor}Vendor: {d.vendor.trim()} · {/if}
                      {#if d.serial}ID: {d.serial}{/if}
                      {#if d.rotational !== undefined} · Rotational: {d.rotational ? 'yes' : 'no'}{/if}
                    </div>
                  </div>
                  <span class="text-sm text-muted tabular-nums">{fmtBytes(d.size_bytes)}</span>
                </div>
                {#if d.mounted}
                  <div class="mt-3 mb-1.5 flex justify-between text-xs text-subtle"><span class="font-mono">{d.mount_point}</span><span class="tabular-nums">{fmtBytes(d.used_bytes || 0)} / {fmtBytes(d.size_bytes)}</span></div>
                  <div class="progress"><div class="progress-bar {pctTone(diskPct(d))}" style="width: {Math.min(diskPct(d), 100)}%"></div></div>
                  <div class="mt-1 text-right text-xs text-subtle tabular-nums">{diskPct(d).toFixed(1)}% used</div>
                {/if}
              </div>
            {/each}
          </div>
        {:else}
          <p class="card-body text-[13px] text-subtle">No disk information available</p>
        {/if}
      </section>

      <!-- Network -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><Wifi size={16} class="text-subtle" /> Network interfaces</h2></div>
        {#if specs.network?.length > 0}
          <div class="table-wrap">
            <table class="table">
              <thead><tr><th>Interface</th><th>IP address</th><th>Status</th><th class="max-sm:hidden">IPv6</th></tr></thead>
              <tbody>
                {#each specs.network as net, i (i)}
                  <tr>
                    <td class="font-medium">{net.interface}</td>
                    <td class="font-mono text-[13px] text-muted">{net.ip_address || '—'}</td>
                    <td><span class="inline-flex items-center gap-1.5 text-xs {net.status === 'UP' ? 'text-success' : 'text-danger'}"><span class="dot"></span>{net.status}</span></td>
                    <td class="font-mono text-xs text-subtle max-sm:hidden">{net.ipv6_addresses?.length > 0 ? net.ipv6_addresses[0] : '—'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="card-body text-[13px] text-subtle">No network information available</p>
        {/if}
      </section>

      <!-- Benchmark -->
      {#if benchmark}
        <section class="card">
          <div class="card-header">
            <h2 class="card-title flex items-center gap-2"><Zap size={16} class="text-subtle" /> Benchmark results</h2>
            <span class="text-xs text-subtle">Run at {new Date(benchmark.run_at).toLocaleString()}</span>
          </div>
          <div class="card-body grid gap-4 md:grid-cols-3">
            <div class="rounded-lg border border-line p-4">
              <div class="stat-label mb-3 flex items-center gap-1.5"><Cpu size={14} /> CPU (sysbench)</div>
              <div class="space-y-3">
                <div>
                  <div class="text-xs text-muted">Single thread</div>
                  <div class="text-lg font-semibold text-fg tabular-nums">{benchmark.cpu?.single_thread?.events_per_sec?.toFixed(0) || 'N/A'} <span class="text-xs font-normal text-subtle">events/s</span></div>
                  <div class="text-xs text-subtle">Latency avg: {benchmark.cpu?.single_thread?.latency_avg_ms?.toFixed(2) || 'N/A'} ms</div>
                </div>
                <div class="border-t border-line pt-3">
                  <div class="text-xs text-muted">Multi thread ({specs.cpu?.threads || '?'}t)</div>
                  <div class="text-lg font-semibold text-fg tabular-nums">{benchmark.cpu?.multi_thread?.events_per_sec?.toFixed(0) || 'N/A'} <span class="text-xs font-normal text-subtle">events/s</span></div>
                  <div class="text-xs text-subtle">Latency avg: {benchmark.cpu?.multi_thread?.latency_avg_ms?.toFixed(2) || 'N/A'} ms</div>
                </div>
              </div>
            </div>
            <div class="rounded-lg border border-line p-4">
              <div class="stat-label mb-3 flex items-center gap-1.5"><HardDrive size={14} /> Disk I/O</div>
              <div class="space-y-3">
                <div><div class="text-xs text-muted">Sequential write</div><div class="text-lg font-semibold text-fg tabular-nums">{benchmark.disk?.seq_write_mbs?.toFixed(1) || 'N/A'} <span class="text-xs font-normal text-subtle">MB/s</span></div></div>
                <div><div class="text-xs text-muted">Sequential read</div><div class="text-lg font-semibold text-fg tabular-nums">{benchmark.disk?.seq_read_mbs?.toFixed(1) || 'N/A'} <span class="text-xs font-normal text-subtle">MB/s</span></div></div>
                {#if benchmark.disk?.random_read_iops}
                  <div><div class="text-xs text-muted">Random read IOPS</div><div class="text-lg font-semibold text-fg tabular-nums">{fmtIOPS(benchmark.disk.random_read_iops)} <span class="text-xs font-normal text-subtle">IOPS</span></div><div class="text-xs text-subtle">{benchmark.disk?.random_read_mbs?.toFixed(1) || 'N/A'} MB/s</div></div>
                {/if}
              </div>
            </div>
            <div class="rounded-lg border border-line p-4">
              <div class="stat-label mb-3 flex items-center gap-1.5"><Globe size={14} /> Network</div>
              <div class="space-y-3">
                <div><div class="text-xs text-muted">Download speed</div><div class="text-lg font-semibold text-fg tabular-nums">{benchmark.network?.download_mbs?.toFixed(1) || 'N/A'} <span class="text-xs font-normal text-subtle">MB/s</span></div><div class="text-xs text-subtle">{benchmark.network?.download_mbs ? (benchmark.network.download_mbs * 8).toFixed(0) : 'N/A'} Mbps</div></div>
                <div>
                  <div class="text-xs text-muted">Ping (Google DNS)</div>
                  <div class="text-lg font-semibold text-fg tabular-nums">{benchmark.network?.ping_ms_avg?.toFixed(2) || 'N/A'} <span class="text-xs font-normal text-subtle">ms avg</span></div>
                  <div class="text-xs text-subtle">min {benchmark.network?.ping_ms_min?.toFixed(2) || '—'} · max {benchmark.network?.ping_ms_max?.toFixed(2) || '—'} ms</div>
                </div>
                <div><div class="text-xs text-muted">Packet loss</div><div class="text-sm font-medium {benchmark.network?.packet_loss === '0%' ? 'text-success' : 'text-danger'}">{benchmark.network?.packet_loss || 'N/A'}</div></div>
              </div>
            </div>
          </div>
        </section>
      {/if}
    </div>
  {:else}
    <div class="card">
      <EmptyState
        icon={Server as any}
        title="No server data"
        description="Unable to load server specifications. Check that the agent is online."
        actionLabel="Retry"
        onaction={fetchSpecs}
      />
    </div>
  {/if}

  <p class="mt-8 text-center text-xs text-subtle">Specs load on page visit · Benchmark requires sysbench (CPU) &amp; fio (IOPS) on the agent</p>
</DashboardLayout>
