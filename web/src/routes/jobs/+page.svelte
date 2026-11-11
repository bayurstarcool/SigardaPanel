<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative, fmtDateTime } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    Zap, X, RefreshCw, Globe, Trash2, Rocket, ShieldCheck, Server, Database, Users, UserMinus,
    Play, Square, HardDrive, Download, RotateCcw, ChevronDown, Copy, Check, AlertTriangle,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let jobs = $state<any[]>([]);
  let loading = $state(true);
  let refreshing = $state(false);
  let inFlight = false;
  let statusFilter = $state('');
  let typeFilter = $state('');
  let cancelling = $state<string | null>(null);
  let loadError = $state('');
  let expanded = $state<Record<string, boolean>>({});
  let copiedId = $state<string | null>(null);

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
      const d = await apiFetch(auth.token, 'GET', 'jobs?limit=200');
      jobs = d?.jobs || [];
      loadError = '';
    } catch (e: any) {
      if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load jobs';
    }
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

  async function copyError(j: any) {
    try {
      await navigator.clipboard.writeText(j.error);
      copiedId = j.id;
      setTimeout(() => { if (copiedId === j.id) copiedId = null; }, 2000);
    } catch {
      toasts.error('Copy failed — select the text manually');
    }
  }

  // First non-empty line of an error, for the collapsed row.
  function errorSummary(err: string): string {
    return err.split('\n').map((l) => l.trim()).find(Boolean) ?? err;
  }
  function isLongError(err: string): boolean {
    return err.length > 140 || err.includes('\n');
  }

  const statusOptions = [
    { value: '', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'queued', label: 'Queued' },
    { value: 'failed', label: 'Failed' },
    { value: 'succeeded', label: 'Completed' },
    { value: 'canceled', label: 'Canceled' },
  ];

  const counts = $derived(
    jobs.reduce((acc: Record<string, number>, j: any) => { acc[j.status] = (acc[j.status] || 0) + 1; return acc; }, {}),
  );
  const typeOptions = $derived(
    [...new Set(jobs.map((j: any) => j.type as string))]
      .map((t) => ({ value: t, label: jobType(t).label }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  );
  const filteredJobs = $derived(
    jobs.filter((j: any) => (!statusFilter || j.status === statusFilter) && (!typeFilter || j.type === typeFilter)),
  );

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
  <PageHeader title="Jobs" description={loading ? 'Loading…' : `Latest ${jobs.length} job${jobs.length === 1 ? '' : 's'}`}>
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
      <div class="flex w-full items-center gap-3 sm:w-auto">
        <label for="job-type" class="sr-only">Job type</label>
        <select id="job-type" bind:value={typeFilter} class="input h-8 w-full text-[13px] sm:w-48">
          <option value="">All types</option>
          {#each typeOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
        </select>
        <span class="shrink-0 text-xs text-subtle max-lg:hidden">Auto-refreshes every 5s</span>
      </div>
    </div>

    {#if loadError && jobs.length > 0}
      <div class="flex items-center gap-2 border-b border-line bg-danger/10 px-4 py-2 text-[13px] text-danger" role="alert">
        <AlertTriangle size={14} class="shrink-0" /> <span class="min-w-0 break-words">Refresh failed: {loadError}</span>
      </div>
    {/if}
    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(5) as _, i (i)}<div class="skeleton h-10"></div>{/each}
      </div>
    {:else if loadError && jobs.length === 0}
      <EmptyState compact icon={AlertTriangle as any} title="Could not load jobs" description={loadError} actionLabel="Try again" onaction={manualRefresh} />
    {:else if filteredJobs.length === 0}
      <EmptyState
        compact
        icon={Zap as any}
        title={statusFilter || typeFilter ? 'No jobs match these filters' : 'No jobs'}
        description={statusFilter || typeFilter ? 'Try another status or type.' : 'Jobs will appear here when tasks are running.'} />
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
                        {#if !expanded[j.id]}
                          <div class="mt-0.5 line-clamp-2 font-mono text-xs break-all text-danger">{errorSummary(j.error)}</div>
                        {/if}
                        <button
                          class="mt-0.5 inline-flex cursor-pointer items-center gap-1 text-xs font-medium text-muted hover:text-fg"
                          aria-expanded={!!expanded[j.id]}
                          aria-controls="job-err-{j.id}"
                          onclick={() => (expanded[j.id] = !expanded[j.id])}>
                          <ChevronDown size={12} class="transition-transform duration-150 {expanded[j.id] ? 'rotate-180' : ''}" />
                          {expanded[j.id] ? 'Hide error' : isLongError(j.error) ? 'Show full error' : 'Show error'}
                        </button>
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
                <td class="whitespace-nowrap text-muted tabular-nums max-sm:hidden" title={fmtDateTime(j.created_at)}>
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
              {#if j.status === 'failed' && j.error && expanded[j.id]}
                <tr id="job-err-{j.id}">
                  <td colspan="5" class="bg-surface-2">
                    <!-- On narrow screens the table scrolls sideways; keep the panel
                         pinned to the visible width so long logs wrap instead. -->
                    <div class="sticky left-4 max-w-[calc(100vw-4.5rem)] sm:max-w-[calc(100vw-6.5rem)] lg:max-w-none">
                    <div class="mb-2 flex items-center justify-between gap-2">
                      <span class="text-xs font-medium text-muted">Error output</span>
                      <button class="btn btn-ghost btn-xs" onclick={() => copyError(j)}>
                        {#if copiedId === j.id}<Check size={12} class="text-success" /> Copied{:else}<Copy size={12} /> Copy{/if}
                      </button>
                    </div>
                    <pre class="max-h-80 overflow-auto rounded-lg border border-line bg-surface p-3 font-mono text-xs wrap-anywhere whitespace-pre-wrap text-fg">{j.error}</pre>
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</DashboardLayout>
