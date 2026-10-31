<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import SettingsPanel from './SettingsPanel.svelte';
  import {
    Package, RefreshCw, RotateCcw, Download, Trash2, Database, HardDrive, Cloud,
    CheckCircle2, XCircle, Plus, LoaderCircle,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let backups = $state<any[]>([]);
  let providers = $state<any[]>([]);
  let configs = $state<any[]>([]);
  let loading = $state(true);
  let view = $state<'history' | 'settings'>('history');
  let activeTab = $state<'all' | 'files' | 'database'>('all');
  let allSites = $state<any[]>([]);
  let allDatabases = $state<any[]>([]);
  let providerUsage = $state<Record<number, any>>({});

  // Delete / restore confirmations
  let deleteTarget = $state<any>(null);
  let showDeleteConfirm = $state(false);
  let deletingId = $state<number | null>(null);
  let restoreTarget = $state<any>(null);
  let showRestoreConfirm = $state(false);

  // Manual backup
  let showManualBackup = $state(false);
  let manualSiteId = $state<number>(0);
  let manualType = $state<'files' | 'database' | 'both'>('both');
  let manualProviderId = $state<number>(0);
  let manualBusy = $state(false);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await load();
    await Promise.all([loadProviders(), loadConfigs(), loadSites(), loadDatabases()]);
  });

  async function load() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'backups');
      backups = d?.backups || [];
    } catch (e: any) {
      if (e.message !== 'session expired') toasts.error(e.message);
    } finally { loading = false; }
  }

  async function loadProviders() {
    try {
      const resp = await apiFetch(auth.token, 'GET', 'backup-providers');
      providers = resp?.providers || [];
      // Load usage for each provider (in parallel).
      await Promise.all(
        providers.map(async (p) => {
          try {
            const uResp = await apiFetch(auth.token, 'GET', `backup-providers/${p.id}/usage?source=actual`);
            if (uResp) providerUsage[p.id] = uResp;
          } catch {}
        }),
      );
    } catch {}
  }

  async function loadConfigs() {
    try { configs = (await apiFetch(auth.token, 'GET', 'backup-configs'))?.configs || []; } catch {}
  }

  async function loadSites() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'sites');
      allSites = d?.sites || [];
    } catch {}
  }

  async function loadDatabases() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'databases');
      allDatabases = d?.databases || [];
    } catch {}
  }

  function askRestore(b: any) {
    restoreTarget = b;
    showRestoreConfirm = true;
  }

  async function restoreBackup(id: number) {
    try {
      await apiFetch(auth.token, 'POST', `backups/${id}/restore`);
      toasts.success('Restore job queued');
      await load();
    } catch (e: any) {
      toasts.error(e.message || 'Restore failed');
    }
  }

  function askDelete(b: any) {
    deleteTarget = b;
    showDeleteConfirm = true;
  }

  async function deleteBackup(id: number) {
    deletingId = id;
    try {
      await apiFetch(auth.token, 'DELETE', `backups/${id}`);
      backups = backups.filter((b) => b.id !== id);
      deleteTarget = null;
      toasts.success('Backup deleted');
    } catch (e: any) {
      toasts.error(e.message || 'Delete failed');
    } finally {
      deletingId = null;
    }
  }

  function openManualBackup() {
    showManualBackup = true;
    loadSites();
    loadProviders();
  }

  async function triggerManualBackup() {
    if (!manualSiteId) {
      toasts.error('Please select a site');
      return;
    }
    manualBusy = true;
    try {
      const endpoint = manualType === 'database' ? `sites/${manualSiteId}/backups/database` : `sites/${manualSiteId}/backups`;
      const body: any = { backup_type: manualType };
      if (manualProviderId > 0) body.provider_id = manualProviderId;
      const res = await apiFetch(auth.token, 'POST', endpoint, body);
      toasts.success(`Backup job #${res?.job_id || ''} queued`);
      showManualBackup = false;
      await load();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to trigger backup');
    } finally {
      manualBusy = false;
    }
  }

  const filtered = $derived(activeTab === 'all' ? backups : backups.filter((b) => b.backup_type === activeTab));

  const stats = $derived({
    total: backups.length,
    completed: backups.filter((b) => b.status === 'completed').length,
    failed: backups.filter((b) => b.status === 'failed').length,
    totalSize: backups.reduce((s, b) => s + (b.size_bytes || 0), 0),
  });

  function fmtBytes(bytes: number): string {
    if (!bytes || bytes === 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let size = bytes;
    while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
    return `${size.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
  }

  const siteName = (b: any) => b?.site?.domain || b?.site?.name || `Site #${b?.site_id}`;
  const usageList = $derived(providers.filter((p) => providerUsage[p.id]));
  const typeTabs = [
    { id: 'all' as const, label: 'All' },
    { id: 'files' as const, label: 'Files' },
    { id: 'database' as const, label: 'Databases' },
  ];
  const manualTypes = [
    { id: 'both' as const, label: 'Full' },
    { id: 'files' as const, label: 'Files only' },
    { id: 'database' as const, label: 'DB only' },
  ];
</script>

<DashboardLayout active="Backups">
  <PageHeader title="Backups" description="Manage, configure, and monitor backups">
    {#snippet actions()}
      <button onclick={load} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      <button onclick={openManualBackup} class="btn btn-primary btn-sm"><Plus size={14} /> Create backup</button>
    {/snippet}
  </PageHeader>

  <div class="tabs mb-6" role="tablist">
    <button role="tab" aria-selected={view === 'history'} class="tab {view === 'history' ? 'tab-active' : ''}" onclick={() => (view = 'history')}>History</button>
    <button role="tab" aria-selected={view === 'settings'} class="tab {view === 'settings' ? 'tab-active' : ''}" onclick={() => (view = 'settings')}>Settings</button>
  </div>

  {#if view === 'history'}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="Total" value={loading ? '–' : stats.total} icon={Package as any} />
      <StatCard label="Completed" value={loading ? '–' : stats.completed} icon={CheckCircle2 as any} tone="success" />
      <StatCard label="Failed" value={loading ? '–' : stats.failed} icon={XCircle as any} tone="danger" />
      <StatCard label="Storage used" value={loading ? '–' : fmtBytes(stats.totalSize)} icon={HardDrive as any} />
    </div>

    {#if usageList.length > 0}
      <div class="mb-6 flex flex-wrap gap-2">
        {#each usageList as p (p.id)}
          {@const u = providerUsage[p.id]}
          <div class="card flex items-center gap-2.5 px-3 py-2 text-[13px]">
            <Cloud size={14} class="text-subtle" />
            <span class="font-medium text-fg">{p.name}</span>
            <span class={u.connected ? 'text-success' : 'text-danger'}><span class="dot mr-1"></span>{u.connected ? 'Connected' : 'Disconnected'}</span>
            <span class="text-xs text-subtle tabular-nums">{u.object_count || 0} objects · {u.total_mb ? u.total_mb.toFixed(0) + ' MB' : '0 MB'}</span>
          </div>
        {/each}
      </div>
    {/if}

    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <div class="segmented">
          {#each typeTabs as t (t.id)}
            <button class="segment {activeTab === t.id ? 'segment-active' : ''}" onclick={() => (activeTab = t.id)}>{t.label}</button>
          {/each}
        </div>
        {#if !loading}
          <span class="text-xs text-subtle tabular-nums">{filtered.length} backup{filtered.length !== 1 ? 's' : ''}</span>
        {/if}
      </div>

      {#if loading}
        <div class="space-y-3 p-4">
          {#each Array(5) as _, i (i)}<div class="skeleton h-9"></div>{/each}
        </div>
      {:else if filtered.length === 0}
        <EmptyState compact icon={Package as any} title="No backups yet" description="Create a backup now, or add a schedule under Settings." actionLabel="Create backup" onaction={openManualBackup} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Site</th>
                <th>Status</th>
                <th class="max-sm:hidden">Type</th>
                <th class="max-md:hidden">Storage</th>
                <th class="max-sm:hidden">Size</th>
                <th class="max-sm:hidden">Created</th>
                <th class="text-right"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as b (b.id)}
                <tr>
                  <td>
                    <div class="font-medium text-fg">{siteName(b)}</div>
                    <div class="text-xs text-subtle sm:hidden">{b.backup_type === 'database' ? 'Database' : 'Files'} · {fmtRelative(b.created_at)}</div>
                  </td>
                  <td>
                    {#if b.status === 'archiving'}<StatusPill status="running" label="archiving" />{:else}<StatusPill status={b.status} />{/if}
                  </td>
                  <td class="max-sm:hidden">
                    <span class="inline-flex items-center gap-1.5 text-[13px] text-muted">
                      {#if b.backup_type === 'database'}<Database size={14} /> Database{:else}<HardDrive size={14} /> Files{/if}
                    </span>
                  </td>
                  <td class="max-md:hidden">
                    <span class="inline-flex items-center gap-1.5 text-[13px] text-muted">
                      {#if b.s3_path}<Cloud size={14} /> Cloud{:else}<HardDrive size={14} /> Local{/if}
                    </span>
                  </td>
                  <td class="text-muted tabular-nums max-sm:hidden">{b.size_bytes ? fmtBytes(b.size_bytes) : '—'}</td>
                  <td class="whitespace-nowrap text-muted max-sm:hidden" title={b.created_at ? new Date(b.created_at).toLocaleString('id-ID') : ''}>{fmtRelative(b.created_at)}</td>
                  <td class="text-right">
                    <div class="flex items-center justify-end gap-0.5">
                      <a href="/api/proxy/backups/{b.id}/download" download data-sveltekit-reload class="btn btn-ghost btn-icon btn-xs" title="Download" aria-label="Download backup #{b.id}">
                        <Download size={14} />
                      </a>
                      <button onclick={() => askRestore(b)} class="btn btn-ghost btn-icon btn-xs" title="Restore" aria-label="Restore backup #{b.id}">
                        <RotateCcw size={14} />
                      </button>
                      <button onclick={() => askDelete(b)} disabled={deletingId === b.id} class="btn btn-danger-ghost btn-icon btn-xs" title="Delete" aria-label="Delete backup #{b.id}">
                        {#if deletingId === b.id}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {:else}
    <SettingsPanel {providers} {providerUsage} {configs} {allSites} {loadProviders} {loadConfigs} />
  {/if}

  <ConfirmDialog
    bind:open={showRestoreConfirm}
    title="Restore backup #{restoreTarget?.id ?? ''}?"
    message="This will overwrite the current files/database of {siteName(restoreTarget)}."
    confirmLabel="Restore"
    destructive
    onconfirm={() => restoreTarget && restoreBackup(restoreTarget.id)} />

  <ConfirmDialog
    bind:open={showDeleteConfirm}
    title="Delete backup?"
    message="Delete backup for {deleteTarget?.site?.domain || 'site'}? This cannot be undone."
    confirmLabel="Delete"
    destructive
    onconfirm={() => deleteTarget && deleteBackup(deleteTarget.id)}
    oncancel={() => (deleteTarget = null)} />

  <Modal bind:open={showManualBackup} title="Create backup" maxWidth="md">
    <form id="manual-backup-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); triggerManualBackup(); }}>
      <p class="text-[13px] text-muted">Trigger an immediate backup for a site.</p>
      <div>
        <label for="mb-site" class="label">Site</label>
        <select id="mb-site" bind:value={manualSiteId} class="input">
          <option value={0}>Select a site…</option>
          {#each allSites as s (s.id)}
            <option value={s.id}>{s.domain || s.name || `Site #${s.id}`}</option>
          {/each}
        </select>
      </div>
      <fieldset>
        <legend class="label">Backup type</legend>
        <div class="segmented">
          {#each manualTypes as t (t.id)}
            <button type="button" class="segment {manualType === t.id ? 'segment-active' : ''}" onclick={() => (manualType = t.id)}>{t.label}</button>
          {/each}
        </div>
      </fieldset>
      <div>
        <label for="mb-provider" class="label">Storage destination</label>
        <select id="mb-provider" bind:value={manualProviderId} class="input">
          <option value={0}>Default provider / local</option>
          {#each providers as p (p.id)}
            <option value={p.id}>{p.name} ({p.provider_type})</option>
          {/each}
        </select>
      </div>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showManualBackup = false)}>Cancel</button>
      <button type="submit" form="manual-backup-form" class="btn btn-primary btn-sm" disabled={manualBusy || !manualSiteId}>
        {#if manualBusy}<LoaderCircle size={14} class="animate-spin" /> Queueing…{:else}Start backup{/if}
      </button>
    {/snippet}
  </Modal>
</DashboardLayout>
