<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Zap, Database, RefreshCw, Trash2, Activity } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let stats = $state<any>(null);
  let loading = $state(false);
  let info = $state('');
  let showInfo = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadStats();
    }
  });

  function loadStats() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/redis/stats`)
      .then((d: any) => { stats = d?.stats ?? d; })
      .catch(() => {})
      .finally(() => loading = false);
  }

  async function flushAll() {
    if (!confirm('Flush ALL Redis data?')) return;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/redis/flush`, {});
      loadStats();
    } catch (e) {}
  }

  async function flushDB(db: number) {
    if (!confirm(`Flush Redis database ${db}?`)) return;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/redis/flushdb`, { db });
      loadStats();
    } catch (e) {}
  }

  async function viewInfo() {
    showInfo = true;
    try {
      const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/redis/info`);
      info = d?.info ?? '';
    } catch (e) { info = 'Failed to load info'; }
  }

  function formatBytes(bytes: string) {
    const b = parseInt(bytes);
    if (isNaN(b)) return bytes;
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    return (b / 1048576).toFixed(1) + ' MB';
  }
</script>

<DashboardLayout active="Redis">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-red-100 bg-clip-text text-transparent">Redis Management</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : 'In-memory data store'}</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => loadStats()}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => loadStats()} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} />
      </button>
    </div>
  </div>

  {#if stats?.ok === false}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Zap size={48} class="mb-4 opacity-30" />
      <h2 class="text-lg font-semibold mb-2">Redis Not Available</h2>
      <p class="text-sm text-slate-500">{stats?.message || 'Redis is not installed or not running.'}</p>
    </div>
  {:else if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading Redis stats...</div>
    </div>
  {:else if stats}
    <!-- Stats Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Connected Clients</div>
        <div class="text-2xl font-bold text-white">{stats.stats?.connected_clients || '0'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Used Memory</div>
        <div class="text-2xl font-bold text-white">{formatBytes(stats.stats?.used_memory || '0')}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Total Keys</div>
        <div class="text-2xl font-bold text-white">{stats.stats?.keyspace_hits || '0'}</div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="text-xs text-slate-500 mb-1">Uptime</div>
        <div class="text-2xl font-bold text-white">{stats.stats?.uptime_in_seconds || '0'}s</div>
      </div>
    </div>

    <!-- Actions -->
    <div class="flex items-center gap-3 mb-6">
      <button onclick={viewInfo}
        class="px-4 py-2 bg-white/[0.05] border border-white/[0.1] rounded-lg text-sm font-medium hover:bg-white/[0.1] transition flex items-center gap-2">
        <Activity size={16} /> Full Info
      </button>
      {#if canManage}
        <button onclick={() => flushDB(0)}
          class="px-4 py-2 bg-amber-500/20 text-amber-400 rounded-lg text-sm font-medium hover:bg-amber-500/30 transition flex items-center gap-2">
          <Database size={16} /> Flush DB 0
        </button>
        <button onclick={flushAll}
          class="px-4 py-2 bg-red-500/20 text-red-400 rounded-lg text-sm font-medium hover:bg-red-500/30 transition flex items-center gap-2">
          <Trash2 size={16} /> Flush All
        </button>
      {/if}
    </div>

    <!-- Stats Table -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="px-5 py-3 border-b border-white/[0.06]">
        <h3 class="text-sm font-semibold text-white">Redis Statistics</h3>
      </div>
      <div class="divide-y divide-white/[0.04]">
        {#each Object.entries(stats.stats || {}) as [key, val]}
          <div class="flex justify-between px-5 py-2.5 hover:bg-white/[0.02]">
            <span class="text-xs text-slate-400 font-mono">{key}</span>
            <span class="text-xs text-white font-mono">{val}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Info Modal -->
  {#if showInfo}
    <div class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4" onclick={() => showInfo = false}>
      <div class="bg-[#0c0d0f] border border-white/[0.08] rounded-xl w-full max-w-4xl max-h-[80vh] flex flex-col" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <h3 class="text-sm font-semibold text-white">Redis Full Info</h3>
          <button onclick={() => showInfo = false} class="text-slate-500 hover:text-white text-lg">×</button>
        </div>
        <pre class="flex-1 overflow-auto p-5 text-xs text-slate-300 font-mono whitespace-pre-wrap">{info}</pre>
      </div>
    </div>
  {/if}
</DashboardLayout>
