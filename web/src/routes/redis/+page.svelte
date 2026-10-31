<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Zap, Database, RefreshCw, Trash2, FileText, Users, MemoryStick, Target, Clock, Server, LoaderCircle } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let stats = $state<any>(null);
  let loading = $state(false);
  let loadError = $state('');
  let info = $state('');
  let infoLoading = $state(false);
  let showInfo = $state(false);
  let flushing = $state(false);
  let confirmOpen = $state(false);
  let confirmKind = $state<'all' | 'db0'>('db0');
  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

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
      loadStats();
    }
  });

  function loadStats() {
    if (!selectedServer) return;
    loading = true;
    loadError = '';
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/redis/stats`)
      .then((d: any) => { stats = d?.stats ?? d; })
      .catch((e: any) => {
        stats = null;
        if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load Redis stats';
      })
      .finally(() => (loading = false));
  }

  function askFlush(kind: 'all' | 'db0') {
    confirmKind = kind;
    confirmOpen = true;
  }

  async function doFlush() {
    flushing = true;
    try {
      if (confirmKind === 'all') await apiFetch(auth.token, 'POST', `servers/${selectedServer}/redis/flush`, {});
      else await apiFetch(auth.token, 'POST', `servers/${selectedServer}/redis/flushdb`, { db: 0 });
      toasts.success(confirmKind === 'all' ? 'All Redis data flushed' : 'Database 0 flushed');
      loadStats();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Flush failed');
    }
    flushing = false;
  }

  async function viewInfo() {
    showInfo = true;
    infoLoading = true;
    try {
      const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/redis/info`);
      info = d?.info ?? '';
    } catch {
      info = 'Failed to load info';
    }
    infoLoading = false;
  }

  function formatBytes(bytes: string) {
    const b = parseInt(bytes);
    if (isNaN(b)) return bytes;
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    return (b / 1048576).toFixed(1) + ' MB';
  }

  function formatUptime(sec: string | number) {
    const s = Number(sec) || 0;
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m ${Math.floor(s % 60)}s`;
  }

  const s = $derived(stats?.stats || {});
</script>

<DashboardLayout active="Redis">
  <PageHeader title="Redis" description={loading ? 'Loading…' : 'In-memory data store'}>
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={loadStats} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as srv (srv.id)}<option value={srv.id}>{srv.name}</option>{/each}
        </select>
      {/if}
      <button onclick={loadStats} class="btn btn-secondary btn-sm" disabled={loading || !selectedServer}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if stats && stats.ok !== false}
        <button onclick={viewInfo} class="btn btn-secondary btn-sm"><FileText size={14} /> Full info</button>
        {#if canManage}
          <button onclick={() => askFlush('db0')} class="btn btn-secondary btn-sm" disabled={flushing}><Database size={14} /> Flush DB 0</button>
          <button onclick={() => askFlush('all')} class="btn btn-danger-ghost btn-sm" disabled={flushing}>
            {#if flushing}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if} Flush all
          </button>
        {/if}
      {/if}
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — Redis is managed through its agent.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else if loading && !stats}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">{#each Array(4) as _, i (i)}<div class="skeleton h-[88px]"></div>{/each}</div>
    <div class="card space-y-3 p-4">{#each Array(6) as _, i (i)}<div class="skeleton h-6"></div>{/each}</div>
  {:else if loadError}
    <div class="card">
      <EmptyState icon={Zap} title="Couldn't reach Redis" description={loadError} actionLabel="Retry" onaction={loadStats} />
    </div>
  {:else if stats?.ok === false}
    <div class="card">
      <EmptyState icon={Zap} title="Redis not available" description={stats?.message || 'Redis is not installed or not running.'} />
    </div>
  {:else if stats}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="Connected clients" value={s.connected_clients || '0'} icon={Users} />
      <StatCard label="Used memory" value={formatBytes(s.used_memory || '0')} icon={MemoryStick} />
      <StatCard label="Keyspace hits" value={s.keyspace_hits || '0'} hint={s.keyspace_misses ? `${s.keyspace_misses} misses` : ''} icon={Target} />
      <StatCard label="Uptime" value={formatUptime(s.uptime_in_seconds)} icon={Clock} />
    </div>

    <div class="card">
      <div class="card-header"><h2 class="card-title">Statistics</h2></div>
      <dl class="divide-y divide-line">
        {#each Object.entries(s) as [key, val] (key)}
          <div class="flex items-center justify-between gap-4 px-5 py-2.5">
            <dt class="truncate font-mono text-xs text-muted">{key}</dt>
            <dd class="truncate font-mono text-xs text-fg tabular-nums">{val}</dd>
          </div>
        {/each}
      </dl>
    </div>
  {/if}

  <Modal bind:open={showInfo} title="Redis INFO" maxWidth="2xl">
    {#if infoLoading}
      <div class="space-y-2">{#each Array(10) as _, i (i)}<div class="skeleton h-3.5" style="width: {[60, 82, 74, 90, 55, 68, 88, 72, 64, 80][i]}%"></div>{/each}</div>
    {:else}
      <pre class="max-h-[60vh] overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{info}</pre>
    {/if}
  </Modal>

  <ConfirmDialog
    bind:open={confirmOpen}
    title={confirmKind === 'all' ? 'Flush all Redis data?' : 'Flush database 0?'}
    message={confirmKind === 'all' ? 'Every key in every Redis database on this server is deleted. Caches and sessions stored in Redis will be lost.' : 'Every key in Redis database 0 is deleted.'}
    confirmLabel={confirmKind === 'all' ? 'Flush all' : 'Flush DB 0'}
    destructive
    onconfirm={doFlush}
  />
</DashboardLayout>
