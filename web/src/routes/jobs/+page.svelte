<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    Zap, X, RefreshCw,
    Globe, Trash2, Rocket,
    ShieldCheck, RotateCw,
    Server, Database, Users, UserMinus,
    Play, Square, RefreshCw as RefreshIcon,
    HardDrive, Download, Upload,
    RotateCcw
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let jobs = $state<any[]>([]);
  let loading = $state(true);
  let statusFilter = $state('');
  let cancelling = $state<string | null>(null);

  let refreshInterval: ReturnType<typeof setInterval> | null = null;

  onMount(() => {
    if (!auth.user) { loading = false; return; }
    loadJobs();
    refreshInterval = setInterval(loadJobs, 5000);
    return () => { if (refreshInterval) clearInterval(refreshInterval); };
  });

  async function loadJobs() {
    if (!auth.user) return;
    try {
      const d = await apiFetch(auth.token, 'GET', 'jobs');
      jobs = d?.jobs || [];
    } catch {}
    if (loading) loading = false;
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

  const filteredJobs = $derived(statusFilter ? jobs.filter((j: any) => j.status === statusFilter) : jobs);

  const statusOptions = [
    { value: '', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'queued', label: 'Queued' },
    { value: 'failed', label: 'Failed' },
    { value: 'succeeded', label: 'Completed' },
  ];

  const jobTypeLabels: Record<string, string> = {
    'site.create': 'Create Site',
    'site.delete': 'Delete Site',
    'site.deploy': 'Deploy Site',
    'ssl.issue': 'Issue SSL',
    'ssl.renew': 'Renew SSL',
    'ssl.auto-renew': 'Auto-Renew SSL',
    'nginx.reload': 'Reload Nginx',
    'backup.create': 'Create Backup',
    'backup.restore': 'Restore Backup',
    'backup.upload': 'Upload Backup',
    'database.create': 'Create Database',
    'database.delete': 'Delete Database',
    'dbuser.create': 'Create DB User',
    'dbuser.delete': 'Delete DB User',
    'app.start': 'Start App',
    'app.stop': 'Stop App',
    'app.restart': 'Restart App',
    'db_backup.create': 'Backup Database',
  };

  function jobLabel(type: string): string {
    return jobTypeLabels[type] || type;
  }

  const jobTypeColors: Record<string, string> = {
    'site.create': 'text-emerald-400 bg-emerald-500/10',
    'site.delete': 'text-red-400 bg-red-500/10',
    'site.deploy': 'text-sky-400 bg-sky-500/10',
    'ssl.issue': 'text-amber-400 bg-amber-500/10',
    'ssl.renew': 'text-amber-400 bg-amber-500/10',
    'ssl.auto-renew': 'text-amber-400 bg-amber-500/10',
    'nginx.reload': 'text-purple-400 bg-purple-500/10',
    'backup.create': 'text-cyan-400 bg-cyan-500/10',
    'backup.restore': 'text-cyan-400 bg-cyan-500/10',
    'backup.upload': 'text-cyan-400 bg-cyan-500/10',
    'database.create': 'text-violet-400 bg-violet-500/10',
    'database.delete': 'text-red-400 bg-red-500/10',
    'dbuser.create': 'text-indigo-400 bg-indigo-500/10',
    'dbuser.delete': 'text-red-400 bg-red-500/10',
    'app.start': 'text-emerald-400 bg-emerald-500/10',
    'app.stop': 'text-orange-400 bg-orange-500/10',
    'app.restart': 'text-sky-400 bg-sky-500/10',
    'db_backup.create': 'text-cyan-400 bg-cyan-500/10',
  };

  function jobColor(type: string): string {
    return jobTypeColors[type] || 'text-slate-400 bg-slate-500/10';
  }

  const jobIconBg: Record<string, string> = {
    'site.create': 'bg-emerald-500/15 text-emerald-400',
    'site.delete': 'bg-red-500/15 text-red-400',
    'site.deploy': 'bg-sky-500/15 text-sky-400',
    'ssl.issue': 'bg-amber-500/15 text-amber-400',
    'ssl.renew': 'bg-amber-500/15 text-amber-400',
    'ssl.auto-renew': 'bg-amber-500/15 text-amber-400',
    'nginx.reload': 'bg-purple-500/15 text-purple-400',
    'backup.create': 'bg-cyan-500/15 text-cyan-400',
    'backup.restore': 'bg-cyan-500/15 text-cyan-400',
    'backup.upload': 'bg-cyan-500/15 text-cyan-400',
    'database.create': 'bg-violet-500/15 text-violet-400',
    'database.delete': 'bg-red-500/15 text-red-400',
    'dbuser.create': 'bg-indigo-500/15 text-indigo-400',
    'dbuser.delete': 'bg-red-500/15 text-red-400',
    'app.start': 'bg-emerald-500/15 text-emerald-400',
    'app.stop': 'bg-orange-500/15 text-orange-400',
    'app.restart': 'bg-sky-500/15 text-sky-400',
    'db_backup.create': 'bg-cyan-500/15 text-cyan-400',
  };

  function iconBg(type: string): string {
    return jobIconBg[type] || 'bg-slate-500/15 text-slate-400';
  }
</script>

<DashboardLayout active="Jobs">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Jobs</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${jobs.length} job(s) total`}</p>
    </div>
    <div class="flex items-center gap-3">
      <select bind:value={statusFilter}
        class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-300 focus:outline-none focus:border-sky-400/40">
        {#each statusOptions as opt}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
      <button onclick={loadJobs}
        class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition cursor-pointer border border-white/[0.07]">
        <RefreshCw size={14} /> Refresh
      </button>
    </div>
  </div>

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading...</div>
    </div>
  {:else if filteredJobs.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Zap size={48} class="mb-4 opacity-30 mx-auto" />
      <h2 class="text-lg font-semibold mb-2">{statusFilter ? `No ${statusFilter} jobs` : 'No Jobs'}</h2>
      <p class="text-sm text-slate-500">Jobs will appear here when tasks are running.</p>
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="divide-y divide-white/5">
        {#each filteredJobs as j}
          {@const canCancel = j.status === 'queued' || j.status === 'running'}
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-6 py-4 hover:bg-white/[0.02] transition">
            <div class="flex items-start gap-3 min-w-0 flex-1">
              <div class="shrink-0 w-9 h-9 rounded-lg flex items-center justify-center {iconBg(j.type)}">
                {#if j.type === 'site.create'}
                  <Globe size={16} />
                {:else if j.type === 'site.delete'}
                  <Trash2 size={16} />
                {:else if j.type === 'site.deploy'}
                  <Rocket size={16} />
                {:else if j.type === 'ssl.issue' || j.type === 'ssl.renew' || j.type === 'ssl.auto-renew'}
                  <ShieldCheck size={16} />
                {:else if j.type === 'nginx.reload'}
                  <RotateCcw size={16} />
                {:else if j.type === 'backup.create' || j.type === 'backup.upload'}
                  <HardDrive size={16} />
                {:else if j.type === 'backup.restore'}
                  <Download size={16} />
                {:else if j.type === 'database.create'}
                  <Database size={16} />
                {:else if j.type === 'database.delete'}
                  <Trash2 size={16} />
                {:else if j.type === 'dbuser.create'}
                  <Users size={16} />
                {:else if j.type === 'dbuser.delete'}
                  <UserMinus size={16} />
                {:else if j.type === 'app.start'}
                  <Play size={16} />
                {:else if j.type === 'app.stop'}
                  <Square size={16} />
                {:else if j.type === 'app.restart'}
                  <RefreshCw size={16} />
                {:else if j.type === 'db_backup.create'}
                  <Database size={16} />
                {:else}
                  <Zap size={16} />
                {/if}
              </div>
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-slate-200">{jobLabel(j.type)}</span>
                </div>
                {#if j.description}
                  <div class="text-xs text-slate-400 mt-0.5 truncate">{j.description}</div>
                {/if}
                <div class="flex items-center gap-1.5 mt-1 text-xs text-slate-500">
                  {#if j.server_id}
                    <Server size={11} />
                    <span>#{j.server_id}</span>
                  {/if}
                  {#if j.site_id}
                    <span class="text-slate-600">&bull;</span>
                    <Globe size={11} />
                    <span>Site #{j.site_id}</span>
                  {/if}
                  <span class="text-slate-600">&bull;</span>
                  <span>{fmtRelative(j.created_at)}</span>
                  {#if j.attempts > 1}
                    <span class="text-slate-600">&bull;</span>
                    <span>Attempt {j.attempts}/{j.max_attempts}</span>
                  {/if}
                </div>
              </div>
            </div>
            <div class="flex items-center gap-3 shrink-0">
              <StatusPill status={j.status} />
              {#if canCancel}
                <button onclick={() => cancelJob(j.id)} disabled={cancelling === j.id}
                  class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 hover:text-red-300 transition-all duration-150 cursor-pointer border-0 whitespace-nowrap disabled:opacity-40">
                  <X size={12} />
                  {cancelling === j.id ? 'Cancelling...' : 'Cancel'}
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</DashboardLayout>
