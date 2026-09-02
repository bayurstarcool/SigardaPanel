<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { Clock, Plus, Trash2, Loader2 } from 'lucide-svelte';
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

<div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
  <div class="flex items-center justify-between px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
    <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Clock size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Cron Jobs</span><span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">{cronJobs.length}</span></div>
    <button onclick={() => showCreateCron = !showCreateCron} class="inline-flex items-center gap-1 px-3 py-1.5 rounded-xl text-xs font-medium bg-sky-400/[0.08] text-sky-400 border border-sky-400/15 hover:bg-sky-400/[0.12] hover:border-sky-400/25 transition-all duration-200">+ Add Cron</button>
  </div>
  {#if showCreateCron}
    <div class="px-5 py-4 border-b border-white/[0.05] bg-white/[0.01]">
      <div class="flex gap-3 items-end">
        <div class="w-40"><label class="block text-xs text-slate-500 mb-1">Schedule</label><input bind:value={newCron.schedule} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-3 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="0 0 * * *" /></div>
        <div class="flex-1"><label class="block text-xs text-slate-500 mb-1">Command</label><input bind:value={newCron.command} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-3 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="/usr/bin/php artisan schedule:run" /></div>
        <button onclick={createCron} disabled={cronBusy || !newCron.command} class="px-4 py-2.5 rounded-xl text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{cronBusy ? '...' : 'Create'}</button>
        <button onclick={() => showCreateCron = false} class="px-3 py-2.5 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
      </div>
    </div>
  {/if}
  {#if cronLoading}
    <div class="divide-y divide-white/5">{#each Array(3) as _}<div class="px-5 py-4 flex items-center gap-4"><div class="h-6 w-24 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="h-4 flex-1 bg-white/[0.03] rounded animate-pulse"></div></div>{/each}</div>
  {:else if cronJobs.length === 0}
    <div class="flex flex-col items-center justify-center py-14"><Clock size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">No cron jobs</p><p class="text-xs text-slate-600">Add a cron job for this site</p></div>
  {:else}
    <div class="divide-y divide-white/[0.04]">{#each cronJobs as job}<div class="flex items-center justify-between px-5 py-3.5 hover:bg-white/[0.02] transition"><div class="flex items-center gap-4"><span class="text-xs font-mono text-sky-300 bg-sky-400/10 px-2 py-1 rounded-lg">{job.schedule}</span><span class="text-sm text-slate-200 font-mono text-xs">{job.command}</span></div><button onclick={() => deleteCron(job)} class="text-xs text-red-400/60 hover:text-red-400 transition p-1 rounded-lg hover:bg-red-400/10"><Trash2 size={14} /></button></div>{/each}</div>
  {/if}
</div>
