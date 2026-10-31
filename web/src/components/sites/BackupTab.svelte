<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative, fmtBytes } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Package, HardDrive, Cloud, Database, Trash2, RotateCw, Download, LoaderCircle, RefreshCw, Plus } from 'lucide-svelte';

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
    if (status === 'completed' || status === 'succeeded') return 'badge-success';
    if (status === 'failed') return 'badge-danger';
    if (status === 'running' || status === 'processing') return 'badge-info';
    return 'badge-warning';
  }
</script>

<div class="card">
  <div class="card-header">
    <div class="flex items-center gap-2">
      <Package size={16} class="text-subtle" />
      <h3 class="card-title">Backups</h3>
      <span class="badge tabular-nums">{backups.length}</span>
    </div>
    <div class="flex items-center gap-1">
      <button onclick={loadBackups} class="btn btn-ghost btn-icon btn-sm" aria-label="Refresh backups" title="Refresh"><RefreshCw size={14} class={loading ? 'animate-spin' : ''} /></button>
      <button onclick={createBackup} disabled={creating} class="btn btn-primary btn-sm"><Plus size={14} /> {creating ? 'Creating…' : 'Create backup'}</button>
    </div>
  </div>
  {#if loading}
    <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-11"></div>{/each}</div>
  {:else if backups.length === 0}
    <EmptyState compact icon={Package} title="No backups" description="Create a backup to protect your site data." />
  {:else}
    <ul class="divide-y divide-line">
      {#each backups as b (b.id)}
        {@const isDB = (b.storage_type || '').startsWith('database')}
        {@const hasLocal = !!b.storage_path}
        {@const hasCloud = !!b.s3_path}
        {@const done = b.status === 'completed' || b.status === 'succeeded'}
        <li class="flex items-center justify-between gap-3 px-5 py-3 transition-colors duration-100 hover:bg-surface-2/60">
          <div class="flex min-w-0 flex-1 items-center gap-3">
            <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-subtle">
              {#if isDB}<Database size={15} />{:else}<HardDrive size={15} />{/if}
            </span>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="text-sm font-medium text-fg">Backup #{b.id}</span>
                <span class="badge {statusClass(b.status)}">{b.status}</span>
                {#if isDB}<span class="badge badge-accent"><Database size={11} /> DB</span>{/if}
                {#if hasCloud}<span class="badge badge-info"><Cloud size={11} /> Cloud</span>{:else if hasLocal}<span class="badge"><HardDrive size={11} /> Local</span>{/if}
              </div>
              <div class="mt-0.5 flex flex-wrap items-center gap-x-3 text-xs text-subtle tabular-nums">
                <span>{fmtRelative(b.created_at)}</span>
                <span>{fmtBytes(b.size_bytes)}</span>
                {#if b.file_count != null}<span>{b.file_count} files</span>{/if}
                {#if b.progress && b.status !== 'completed' && b.status !== 'failed'}<span>{b.progress}</span>{/if}
              </div>
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-0.5">
            {#if done}
              <a href="/api/proxy/backups/{b.id}/download" class="btn btn-ghost btn-icon btn-xs" aria-label="Download backup #{b.id}" title="Download"><Download size={14} /></a>
              <button onclick={() => restoreBackup(b)} disabled={busyId === b.id} class="btn btn-ghost btn-icon btn-xs" aria-label="Restore backup #{b.id}" title="Restore">
                {#if busyId === b.id}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCw size={14} />{/if}
              </button>
            {/if}
            <button onclick={() => (confirmDelete = b)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete backup #{b.id}" title="Delete"><Trash2 size={14} /></button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<ConfirmDialog
  open={!!confirmDelete}
  title="Delete backup"
  message={confirmDelete ? `Delete backup #${confirmDelete.id} created ${fmtRelative(confirmDelete.created_at)}? This action cannot be undone.` : ''}
  confirmLabel="Delete"
  destructive
  onconfirm={() => { const b = confirmDelete; if (b) deleteBackup(b); }}
  oncancel={() => (confirmDelete = null)}
/>
