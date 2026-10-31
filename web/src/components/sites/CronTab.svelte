<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import { Clock, Plus, Trash2 } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();

  let cronJobs = $state<any[]>([]);
  let cronLoading = $state(false);
  let cronLoaded = $state(false);
  let showCreateCron = $state(false);
  let newCron = $state({ schedule: '0 0 * * *', command: '' });
  let cronBusy = $state(false);

  $effect(() => { if (!cronLoading && cronJobs.length === 0 && cronLoaded === false) { cronLoaded = true; loadCron(); } });

  async function loadCron() {
    cronLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/cron`); cronJobs = d?.jobs || []; }
    catch {} finally { cronLoading = false; }
  }
  async function createCron() {
    if (!newCron.command) return; cronBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/cron`, newCron); showCreateCron = false; newCron = { schedule: '0 0 * * *', command: '' }; await loadCron(); }
    catch (e: any) { toasts.error(e.message); } finally { cronBusy = false; }
  }
  async function deleteCron(job: any) {
    if (!confirm('Delete this cron job?')) return;
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/cron/${job.id}`); cronJobs = cronJobs.filter(j => j.id !== job.id); }
    catch (e: any) { toasts.error(e.message); }
  }
</script>

<div class="card">
  <div class="card-header">
    <div class="flex items-center gap-2">
      <Clock size={16} class="text-subtle" />
      <h3 class="card-title">Cron jobs</h3>
      <span class="badge tabular-nums">{cronJobs.length}</span>
    </div>
    <button onclick={() => (showCreateCron = !showCreateCron)} class="btn btn-secondary btn-sm"><Plus size={14} /> Add cron</button>
  </div>
  {#if showCreateCron}
    <div class="flex flex-col gap-3 border-b border-line bg-surface-2/50 px-5 py-4 sm:flex-row sm:items-end">
      <div class="sm:w-40">
        <label for="cron-schedule" class="label">Schedule</label>
        <input id="cron-schedule" bind:value={newCron.schedule} class="input font-mono text-[13px]" placeholder="0 0 * * *" />
      </div>
      <div class="flex-1">
        <label for="cron-command" class="label">Command</label>
        <input id="cron-command" bind:value={newCron.command} class="input font-mono text-[13px]" placeholder="/usr/bin/php artisan schedule:run" />
      </div>
      <div class="flex gap-2">
        <button onclick={createCron} disabled={cronBusy || !newCron.command} class="btn btn-primary">{cronBusy ? 'Creating…' : 'Create'}</button>
        <button onclick={() => (showCreateCron = false)} class="btn btn-ghost">Cancel</button>
      </div>
    </div>
  {/if}
  {#if cronLoading}
    <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
  {:else if cronJobs.length === 0}
    <EmptyState compact icon={Clock} title="No cron jobs" description="Add a cron job for this site." />
  {:else}
    <ul class="divide-y divide-line">
      {#each cronJobs as job (job.id)}
        <li class="flex items-center justify-between gap-3 px-5 py-3 transition-colors duration-100 hover:bg-surface-2/60">
          <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
            <span class="code shrink-0">{job.schedule}</span>
            <span class="font-mono text-[13px] break-all text-fg">{job.command}</span>
          </div>
          <button onclick={() => deleteCron(job)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete cron job"><Trash2 size={14} /></button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
