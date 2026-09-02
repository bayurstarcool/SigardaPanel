<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { HardDrive, Cpu, MemoryStick, RefreshCw, Package, ArrowUpCircle } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let metrics = $state<any>(null);
  let specs = $state<any>(null);
  let loading = $state(false);
  let updates = $state('');
  let showUpdates = $state(false);

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadMetrics();
      loadSpecs();
    }
  });

  function loadMetrics() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/metrics/latest`)
      .then((d: any) => { metrics = d; })
      .catch(() => {})
      .finally(() => loading = false);
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
    updates = 'Checking for updates...';
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/command`, {
      command: "bash -c 'apt-get update -qq 2>/dev/null && apt list --upgradable 2>/dev/null | head -30'"
    }).then((d: any) => {
      updates = d?.output || 'No updates available';
    }).catch(() => { updates = 'Failed to check updates'; });
  }

  function fmtBytes(b: number) {
    if (!b) return '0 B';
    if (b < 1024**2) return (b/1024).toFixed(1) + ' KB';
    if (b < 1024**3) return (b/1024**2).toFixed(1) + ' MB';
    return (b/1024**3).toFixed(1) + ' GB';
  }

  function fmtPercent(used: number, total: number) {
    if (!total) return '0%';
    return (used/total*100).toFixed(1) + '%';
  }

  function usageColor(pct: number) {
    if (pct >= 90) return 'bg-red-500';
    if (pct >= 70) return 'bg-amber-500';
    return 'bg-emerald-500';
  }
</script>

<DashboardLayout active="System">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-violet-100 bg-clip-text text-transparent">System Overview</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : 'Server resource monitoring'}</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => { loadMetrics(); loadSpecs(); }}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => { loadMetrics(); loadSpecs(); }} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} />
      </button>
    </div>
  </div>

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading metrics...</div>
    </div>
  {:else if metrics}
    <!-- Resource Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
      <!-- CPU -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3">
          <Cpu size={18} class="text-blue-400" />
          <span class="text-sm font-medium text-white">CPU</span>
        </div>
        <div class="text-3xl font-bold text-white mb-2">{(metrics.cpu_percent || 0).toFixed(1)}%</div>
        <div class="w-full bg-white/[0.06] rounded-full h-2">
          <div class="h-2 rounded-full {usageColor(metrics.cpu_percent || 0)}" style="width: {Math.min(metrics.cpu_percent || 0, 100)}%"></div>
        </div>
        <div class="text-xs text-slate-500 mt-2">{specs?.cpu?.cores ?? "—"} core(s) · {specs?.cpu?.threads ?? "—"} thread(s)</div>
      </div>

      <!-- Memory -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3">
          <MemoryStick size={18} class="text-purple-400" />
          <span class="text-sm font-medium text-white">Memory</span>
        </div>
        <div class="text-3xl font-bold text-white mb-2">{fmtBytes(metrics.mem_used || 0)}</div>
        <div class="text-xs text-slate-500 mb-2">of {fmtBytes(metrics.mem_total || 0)} ({fmtPercent(metrics.mem_used || 0, metrics.mem_total || 0)})</div>
        <div class="w-full bg-white/[0.06] rounded-full h-2">
          <div class="h-2 rounded-full {usageColor((metrics.mem_used||0)/(metrics.mem_total||1)*100)}" style="width: {Math.min((metrics.mem_used||0)/(metrics.mem_total||1)*100, 100)}%"></div>
        </div>
      </div>

      <!-- Disk -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3">
          <HardDrive size={18} class="text-amber-400" />
          <span class="text-sm font-medium text-white">Disk</span>
        </div>
        <div class="text-3xl font-bold text-white mb-2">{fmtBytes(metrics.disk_used || 0)}</div>
        <div class="text-xs text-slate-500 mb-2">of {fmtBytes(metrics.disk_total || 0)} ({fmtPercent(metrics.disk_used || 0, metrics.disk_total || 0)})</div>
        <div class="w-full bg-white/[0.06] rounded-full h-2">
          <div class="h-2 rounded-full {usageColor((metrics.disk_used||0)/(metrics.disk_total||1)*100)}" style="width: {Math.min((metrics.disk_used||0)/(metrics.disk_total||1)*100, 100)}%"></div>
        </div>
      </div>
    </div>

    <!-- Hardware information -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Cpu size={18} class="text-cyan-400" /><span class="text-sm font-medium text-white">CPU Cores</span></div>
        <div class="text-3xl font-bold text-white">{specs?.cpu?.cores ?? '—'}</div>
        <div class="text-xs text-slate-500 mt-1">{specs?.cpu?.threads ?? '—'} threads · {specs?.cpu?.sockets ?? '—'} socket(s)</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Cpu size={18} class="text-fuchsia-400" /><span class="text-sm font-medium text-white">GPU</span></div>
        <div class="text-3xl font-bold text-white">{specs?.gpu?.count ?? '—'}</div>
        <div class="text-xs text-slate-500 mt-1 truncate" title={specs?.gpu?.name}>{specs?.gpu?.name || 'No GPU detected'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><Cpu size={18} class="text-violet-400" /><span class="text-sm font-medium text-white">GPU Usage</span></div>
        <div class="text-3xl font-bold text-white">{specs?.gpu?.usage_available ? `${Number(specs.gpu.usage_percent || 0).toFixed(1)}%` : 'N/A'}</div>
        <div class="text-xs text-slate-500 mt-1">{specs?.gpu?.usage_available ? 'live utilization' : 'driver/API unavailable'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center gap-2 mb-3"><HardDrive size={18} class="text-amber-400" /><span class="text-sm font-medium text-white">Storage</span></div>
        <div class="text-sm font-bold text-white">{specs?.disk?.length ?? '—'} device(s)</div>
        <div class="text-xs text-slate-500 mt-1">{specs?.disk?.map((d: any) => d.disk_type).filter(Boolean).join(', ') || 'No disk info'}</div>
      </div>
    </div>

    <!-- Network + Swap -->
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Network In</div>
        <div class="text-lg font-bold text-white">{fmtBytes(metrics.net_in_bytes || 0)}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Network Out</div>
        <div class="text-lg font-bold text-white">{fmtBytes(metrics.net_out_bytes || 0)}</div>
      </div>
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <HardDrive size={48} class="mb-4 opacity-30" />
      <h2 class="text-lg font-semibold mb-2">No Metrics</h2>
      <p class="text-sm text-slate-500">No metrics available for this server.</p>
    </div>
  {/if}

  <!-- Updates Section -->
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
    <div class="flex items-center justify-between mb-4">
      <div class="flex items-center gap-2">
        <Package size={18} class="text-cyan-400" />
        <span class="text-sm font-medium text-white">System Updates</span>
      </div>
      <button onclick={loadUpdates}
        class="px-3 py-1.5 bg-white/[0.05] border border-white/[0.1] rounded-lg text-xs font-medium hover:bg-white/[0.1] transition flex items-center gap-1.5">
        <ArrowUpCircle size={14} /> Check Updates
      </button>
    </div>
    {#if showUpdates}
      <pre class="text-xs text-slate-300 font-mono whitespace-pre-wrap bg-black/20 rounded-lg p-4">{updates}</pre>
    {:else}
      <p class="text-xs text-slate-500">Click "Check Updates" to see available system updates.</p>
    {/if}
  </div>
</DashboardLayout>
