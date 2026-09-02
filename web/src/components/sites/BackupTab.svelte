<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative, fmtBytes } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import { Package, HardDrive, Cloud, Database, Trash2, RotateCw, Download, Loader2, RefreshCw } from 'lucide-svelte';

  let { siteId }: { siteId: number } = $props();
  let backups = $state<any[]>([]);
  let loading = $state(false);
  let loaded = $state(false);
  let busyId = $state<number | null>(null);
  let creating = $state(false);
  let confirmDelete = $state<any | null>(null);

  $effect(() => {
    if (!loading && backups.length === 0 && !loaded) { loaded = true; loadBackups(); }
  });

  async function loadBackups() {
    loading = true;
    try { const d = await apiFetch(auth.token, 'GET', `backups?site_id=${siteId}`); backups = d?.backups || []; }
    catch (e: any) { toasts.error(e.message || 'Failed to load backups'); }
    finally { loading = false; }
  }

  async function createBackup() {
    if (!confirm('Create a new backup for this site?')) return;
    creating = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/backups`); toasts.success(`Backup created — Job #${d?.job_id}`); await loadBackups(); }
    catch (e: any) { toasts.error(e.message || 'Failed to create backup'); }
    finally { creating = false; }
  }

  async function restoreBackup(b: any) {
    if (!confirm(`Restore backup #${b.id} from ${fmtRelative(b.created_at)}? This will overwrite current site files.`)) return;
    busyId = b.id;
    try { await apiFetch(auth.token, 'POST', `backups/${b.id}/restore`); toasts.success('Restore job queued'); }
    catch (e: any) { toasts.error(e.message || 'Restore failed'); }
    finally { busyId = null; }
  }

  async function deleteBackup(b: any) {
    confirmDelete = null; busyId = b.id;
    try { await apiFetch(auth.token, 'DELETE', `backups/${b.id}`); backups = backups.filter(x => x.id !== b.id); toasts.success('Backup deleted'); }
    catch (e: any) { toasts.error(e.message || 'Delete failed'); }
    finally { busyId = null; }
  }

  function statusClass(status: string): string {
    if (status === 'completed' || status === 'succeeded') return 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/20';
    if (status === 'failed') return 'bg-red-500/15 text-red-400 border border-red-500/20';
    if (status === 'running' || status === 'processing') return 'bg-blue-500/15 text-blue-400 border border-blue-500/20';
    return 'bg-yellow-500/15 text-yellow-400 border border-yellow-500/20';
  }
</script>

<div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
  <div class="flex items-center justify-between px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
    <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Package size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Backups</span><span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">{backups.length}</span></div>
    <div class="flex items-center gap-2"><button onclick={loadBackups} class="inline-flex items-center justify-center w-8 h-8 rounded-xl text-xs text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0" title="Refresh"><RefreshCw size={13} /></button><button onclick={createBackup} disabled={creating} class="inline-flex items-center gap-1 px-3 py-1.5 rounded-xl text-xs font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{creating ? 'Creating...' : '+ Create Backup'}</button></div>
  </div>
  {#if loading}
    <div class="divide-y divide-white/5">{#each Array(3) as _}<div class="px-5 py-4 flex items-center gap-4"><div class="h-8 w-8 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="flex-1 space-y-1.5"><div class="h-4 w-24 bg-white/[0.04] rounded animate-pulse"></div><div class="h-3 w-32 bg-white/[0.03] rounded animate-pulse"></div></div><div class="h-6 w-16 bg-white/[0.04] rounded animate-pulse"></div></div>{/each}</div>
  {:else if backups.length === 0}
    <div class="flex flex-col items-center justify-center py-14"><Package size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">No backups</p><p class="text-xs text-slate-600">Create a backup to protect your site data</p></div>
  {:else}
    <div class="divide-y divide-white/[0.04]">{#each backups as b (b.id)}
      {@const isDB = (b.storage_type || '').startsWith('database')}
      {@const hasLocal = !!b.storage_path}
      {@const hasCloud = !!b.s3_path}
      <div class="flex items-center justify-between px-4 sm:px-5 py-3.5 hover:bg-white/[0.02] transition">
        <div class="flex items-center gap-3 flex-1 min-w-0"><div class="w-8 h-8 rounded-lg bg-white/[0.05] border border-white/[0.08] flex items-center justify-center shrink-0">{#if isDB}<Database size={14} class="text-indigo-400" />{:else}<HardDrive size={14} class="text-sky-400" />{/if}</div><div class="min-w-0"><div class="flex items-center gap-2 flex-wrap"><span class="text-xs font-semibold text-slate-200">Backup #{b.id}</span><span class="inline-flex items-center px-2 py-0.5 rounded-md text-[0.6rem] font-semibold {statusClass(b.status)}">{b.status}</span>{#if isDB}<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[0.6rem] font-semibold bg-purple-500/20 text-purple-400 border border-purple-500/20"><Database size={8} class="mr-0.5" /> DB</span>{/if}{#if hasCloud}<span class="inline-flex items-center gap-0.5 px-2 py-0.5 rounded-md text-[0.6rem] font-semibold bg-cyan-500/20 text-cyan-400 border border-cyan-500/20"><Cloud size={8} /> Cloud</span>{:else if hasLocal}<span class="inline-flex items-center gap-0.5 px-2 py-0.5 rounded-md text-[0.6rem] font-semibold bg-yellow-500/20 text-yellow-400 border border-yellow-500/20"><HardDrive size={8} /> Local</span>{/if}</div><div class="flex items-center gap-3 mt-0.5"><span class="text-[0.65rem] text-slate-500">{fmtRelative(b.created_at)}</span><span class="text-[0.65rem] text-slate-500">{fmtBytes(b.size_bytes)}</span>{#if b.file_count != null}<span class="text-[0.65rem] text-slate-500">{b.file_count} files</span>{/if}{#if b.progress && b.status !== 'completed' && b.status !== 'failed'}<span class="text-[0.65rem] text-slate-500">{b.progress}</span>{/if}</div></div></div>
        <div class="flex items-center gap-1 shrink-0">{#if b.status === 'completed' || b.status === 'succeeded'}<a href="/api/proxy/backups/{b.id}/download" class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-slate-400 hover:text-sky-400 hover:bg-sky-400/10 transition" title="Download"><Download size={13} /></a><button onclick={() => restoreBackup(b)} disabled={busyId === b.id} class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-slate-400 hover:text-amber-400 hover:bg-amber-400/10 transition disabled:opacity-40 cursor-pointer border-0" title="Restore">{#if busyId === b.id}<Loader2 size={13} class="animate-spin" />{:else}<RotateCw size={13} />{/if}</button>{/if}<button onclick={() => confirmDelete = b} class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-slate-400 hover:text-red-400 hover:bg-red-400/10 transition cursor-pointer border-0" title="Delete"><Trash2 size={13} /></button></div>
      </div>
    {/each}</div>
  {/if}
</div>

{#if confirmDelete}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onclick={() => confirmDelete = null}><div class="bg-[#0f1a2e] border border-white/[0.08] rounded-2xl shadow-2xl p-6 max-w-md w-full" onclick={(e) => e.stopPropagation()}><h3 class="text-base font-semibold text-slate-200 mb-2">Delete Backup</h3><p class="text-sm text-slate-400 mb-5">Delete backup <span class="text-white font-medium">#{confirmDelete.id}</span> created {fmtRelative(confirmDelete.created_at)}? This action cannot be undone.</p><div class="flex items-center justify-end gap-2"><button onclick={() => confirmDelete = null} class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button><button onclick={() => deleteBackup(confirmDelete)} disabled={busyId === confirmDelete.id} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-red-500/20 text-red-400 border border-red-500/20 hover:bg-red-500/30 transition disabled:opacity-40 cursor-pointer">Delete</button></div></div></div>
{/if}
