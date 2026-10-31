<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    Zap, X, RefreshCw, Globe, Trash2, Rocket, ShieldCheck, Server, Database, Users, UserMinus,
    Play, Square, HardDrive, Download, RotateCcw,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let jobs = $state<any[]>([]);
  let loading = $state(true);
  let refreshing = $state(false);
  let inFlight = false;
  let statusFilter = $state('');
  let cancelling = $state<string | null>(null);

  onMount(() => {
    if (!auth.user) { loading = false; return; }
    loadJobs();
    // Poll every 5s, but only while the tab is visible.
    const interval = setInterval(() => { if (!document.hidden) loadJobs(); }, 5000);
    const onVisible = () => { if (!document.hidden) loadJobs(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      clearInterval(interval);
      document.removeEventListener('visibilitychange', onVisible);
    };
  });

  async function loadJobs() {
    if (!auth.user || inFlight) return;
    inFlight = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'jobs');
      jobs = d?.jobs || [];
    } catch {}
    inFlight = false;
    if (loading) loading = false;
  }

  async function manualRefresh() {
    refreshing = true;
    await loadJobs();
    refreshing = false;
  }

  async function cancelJob(id: string) {
    cancelling = id;
    try {
      await apiFetch(auth.token, 'POST', `jobs/${id}/cancel`);
      toasts.success('Job cancelled');
      await loadJobs();
    } catch (e: any) {
      toasts.error(e.message);
    } finally { cancelling = null; }
  }

  const statusOptions = [
    { value: '', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'queued', label: 'Queued' },
    { value: 'failed', label: 'Failed' },
    { value: 'succeeded', label: 'Completed' },
  ];

  const counts = $derived(
    jobs.reduce((acc: Record<string, number>, j: any) => { acc[j.status] = (acc[j.status] || 0) + 1; return acc; }, {}),
  );
  const filteredJobs = $derived(statusFilter ? jobs.filter((j: any) => j.status === statusFilter) : jobs);

  const jobTypes: Record<string, { label: string; icon: any; danger?: boolean }> = {
    'site.create': { label: 'Create Site', icon: Globe },
    'site.delete': { label: 'Delete Site', icon: Trash2, danger: true },
    'site.deploy': { label: 'Deploy Site', icon: Rocket },
    'ssl.issue': { label: 'Issue SSL', icon: ShieldCheck },
    'ssl.renew': { label: 'Renew SSL', icon: ShieldCheck },
    'ssl.auto-renew': { label: 'Auto-Renew SSL', icon: ShieldCheck },
    'nginx.reload': { label: 'Reload Nginx', icon: RotateCcw },
    'backup.create': { label: 'Create Backup', icon: HardDrive },
    'backup.restore': { label: 'Restore Backup', icon: Download },
    'backup.upload': { label: 'Upload Backup', icon: HardDrive },
    'database.create': { label: 'Create Database', icon: Database },
    'database.delete': { label: 'Delete Database', icon: Trash2, danger: true },
    'dbuser.create': { label: 'Create DB User', icon: Users },
    'dbuser.delete': { label: 'Delete DB User', icon: UserMinus, danger: true },
    'app.start': { label: 'Start App', icon: Play },
    'app.stop': { label: 'Stop App', icon: Square },
    'app.restart': { label: 'Restart App', icon: RefreshCw },
    'db_backup.create': { label: 'Backup Database', icon: Database },
  };

  function jobType(type: string) {
    return jobTypes[type] ?? { label: type, icon: Zap };
  }
</script>

<DashboardLayout active="Jobs">
  <PageHeader title="Jobs" description={loading ? 'Loading…' : `${jobs.length} job(s) total`}>
    {#snippet actions()}
      <button onclick={manualRefresh} class="btn btn-secondary btn-sm" disabled={refreshing}>
        <RefreshCw size={14} class={refreshing ? 'animate-spin' : ''} /> Refresh
      </button>
    {/snippet}
  </PageHeader>

  <div class="card">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
      <div class="segmented max-w-full overflow-x-auto">
        {#each statusOptions as opt (opt.value)}
          <button class="segment {statusFilter === opt.value ? 'segment-active' : ''}" onclick={() => (statusFilter = opt.value)}>
            {opt.label}
            {#if opt.value && counts[opt.value]}<span class="text-subtle tabular-nums">{counts[opt.value]}</span>{/if}
          </button>
        {/each}
      </div>
      <span class="text-xs text-subtle max-sm:hidden">Auto-refreshes every 5s</span>
    </div>

    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(5) as _, i (i)}<div class="skeleton h-10"></div>{/each}
      </div>
    {:else if filteredJobs.length === 0}
      <EmptyState
        compact
        icon={Zap as any}
        title={statusFilter ? `No ${statusOptions.find((o) => o.value === statusFilter)?.label.toLowerCase()} jobs` : 'No jobs'}
        description="Jobs will appear here when tasks are running." />
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>Job</th>
              <th class="max-sm:hidden">Target</th>
              <th>Status</th>
              <th class="max-sm:hidden">Created</th>
              <th class="text-right"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each filteredJobs as j (j.id)}
              {@const t = jobType(j.type)}
              {@const canCancel = j.status === 'queued' || j.status === 'running'}
              <tr>
                <td class="max-w-0 min-w-56">
                  <div class="flex items-start gap-3">
                    <div class="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-surface-2 {t.danger ? 'text-danger' : 'text-muted'}">
                      <t.icon size={16} />
                    </div>
                    <div class="min-w-0">
                      <div class="font-medium text-fg">{t.label}</div>
                      {#if j.description}
                        <div class="truncate text-[13px] text-muted" title={j.description}>{j.description}</div>
                      {/if}
                      {#if j.status === 'failed' && j.error}
                        <div class="truncate font-mono text-xs text-danger" title={j.error}>{j.error}</div>
                      {/if}
                      <div class="text-xs text-subtle sm:hidden">{fmtRelative(j.created_at)}</div>
                    </div>
                  </div>
                </td>
                <td class="whitespace-nowrap text-[13px] text-muted max-sm:hidden">
                  {#if j.site_id}
                    <a href="/sites/{j.site_id}" class="inline-flex items-center gap-1.5 hover:text-accent"><Globe size={14} /> Site #{j.site_id}</a>
                  {:else if j.server_id}
                    <span class="inline-flex items-center gap-1.5"><Server size={14} /> Server #{j.server_id}</span>
                  {:else}
                    <span class="text-subtle">—</span>
                  {/if}
                  {#if j.attempts > 1}
                    <div class="text-xs text-subtle tabular-nums">Attempt {j.attempts}/{j.max_attempts}</div>
                  {/if}
                </td>
                <td><StatusPill status={j.status} /></td>
                <td class="whitespace-nowrap text-muted tabular-nums max-sm:hidden" title={j.created_at ? new Date(j.created_at).toLocaleString('id-ID') : ''}>
                  {fmtRelative(j.created_at)}
                </td>
                <td class="text-right">
                  {#if canCancel}
                    <button onclick={() => cancelJob(j.id)} disabled={cancelling === j.id} class="btn btn-danger-ghost btn-xs">
                      <X size={14} />
                      {cancelling === j.id ? 'Cancelling…' : 'Cancel'}
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</DashboardLayout>
