<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Plus, Trash2, X, Pencil, Globe, RefreshCw, ListChecks, LoaderCircle } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  const zoneId = $derived($page.params.id);
  const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'NS', 'CAA'];

  let zone = $state<any>(null);
  let records = $state<any[]>([]);
  let loading = $state(true);

  // Create/Edit modal
  let showModal = $state(false);
  let editingRecord = $state<any>(null);
  let form = $state({ name: '', type: 'A', content: '', ttl: 1, proxied: true });
  let saving = $state(false);

  // Purge cache
  let showPurgeConfirm = $state(false);
  let purging = $state(false);

  // Delete confirmations
  let deleteTarget = $state<any>(null);
  let showDeleteConfirm = $state(false);
  let showBulkConfirm = $state(false);

  // Bulk select
  let bulkMode = $state(false);
  let selectedIds = $state<Set<string>>(new Set());
  let bulkDeleting = $state(false);

  // Filter
  let filterType = $state('');

  const typeTone: Record<string, string> = {
    A: 'badge-info', AAAA: 'badge-accent', CNAME: 'badge-success', MX: 'badge-warning',
  };

  function formatTTL(ttl: number): string {
    if (!ttl || ttl === 1) return 'Auto';
    if (ttl < 60) return `${ttl}s`;
    if (ttl < 3600) return `${Math.round(ttl / 60)}m`;
    if (ttl < 86400) return `${Math.round(ttl / 3600)}h`;
    return `${Math.round(ttl / 86400)}d`;
  }

  function formatDate(d: string): string {
    if (!d) return '—';
    try { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }); }
    catch { return d; }
  }

  async function loadData() {
    if (!auth.user) { loading = false; return; }
    loading = true;
    try {
      const [zoneData, dnsData] = await Promise.all([
        apiFetch(auth.token, 'GET', `cloudflare/zones/${zoneId}`),
        apiFetch(auth.token, 'GET', `cloudflare/zones/${zoneId}/dns${filterType ? `?type=${filterType}` : ''}`),
      ]);
      zone = zoneData;
      records = dnsData?.records || [];
    } catch (e: any) { toasts.error(e.message); }
    finally { loading = false; }
  }

  onMount(loadData);

  function openCreateModal() {
    editingRecord = null;
    form = { name: '', type: 'A', content: '', ttl: 1, proxied: true };
    showModal = true;
  }

  function openEditModal(record: any) {
    editingRecord = record;
    form = {
      name: record.name || '',
      type: record.type || 'A',
      content: record.content || '',
      ttl: record.ttl || 1,
      proxied: record.proxied ?? true,
    };
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editingRecord = null;
    saving = false;
  }

  async function saveRecord() {
    saving = true;
    try {
      const body = { name: form.name, type: form.type, content: form.content, ttl: form.ttl, proxied: form.proxied };
      if (editingRecord) {
        await apiFetch(auth.token, 'PUT', `cloudflare/zones/${zoneId}/dns/${editingRecord.id}`, body);
        toasts.success('Record updated');
      } else {
        await apiFetch(auth.token, 'POST', `cloudflare/zones/${zoneId}/dns`, body);
        toasts.success('Record created');
      }
      closeModal();
      await loadData();
    } catch (e: any) { toasts.error(e.message); }
    finally { saving = false; }
  }

  async function deleteRecord(recordId: string) {
    try {
      await apiFetch(auth.token, 'DELETE', `cloudflare/zones/${zoneId}/dns/${recordId}`);
      records = records.filter((r) => r.id !== recordId);
      toasts.success('Record deleted');
    } catch (e: any) { toasts.error(e.message); }
  }

  function confirmDelete(record: any) {
    deleteTarget = record;
    showDeleteConfirm = true;
  }

  async function toggleProxied(record: any) {
    try {
      const newVal = !record.proxied;
      await apiFetch(auth.token, 'PATCH', `cloudflare/zones/${zoneId}/dns/${record.id}`, { proxied: newVal });
      record.proxied = newVal;
      records = [...records];
    } catch (e: any) { toasts.error(e.message); }
  }

  async function purgeCache() {
    purging = true;
    try {
      await apiFetch(auth.token, 'POST', `cloudflare/zones/${zoneId}/purge-cache`, { purge_everything: true });
      showPurgeConfirm = false;
      toasts.success('Cache purged successfully');
    } catch (e: any) { toasts.error(e.message); }
    finally { purging = false; }
  }

  function toggleBulkMode() {
    bulkMode = !bulkMode;
    selectedIds = new Set();
  }

  function toggleSelect(id: string) {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selectedIds = next;
  }

  function toggleSelectAll() {
    if (selectedIds.size === records.length) {
      selectedIds = new Set();
    } else {
      selectedIds = new Set(records.map((r) => r.id));
    }
  }

  async function bulkDelete() {
    if (selectedIds.size === 0) return;
    bulkDeleting = true;
    try {
      await apiFetch(auth.token, 'POST', `cloudflare/zones/${zoneId}/dns/bulk-delete`, {
        record_ids: Array.from(selectedIds),
      });
      const removed = selectedIds;
      records = records.filter((r) => !removed.has(r.id));
      selectedIds = new Set();
      bulkMode = false;
      toasts.success(`Deleted ${removed.size} record(s)`);
    } catch (e: any) { toasts.error(e.message); }
    finally { bulkDeleting = false; }
  }
</script>

{#snippet proxySwitch(on: boolean, onclick: () => void, label: string)}
  <button
    type="button"
    role="switch"
    aria-checked={on}
    aria-label={label}
    title={on ? 'Proxied (click to disable)' : 'DNS only (click to enable)'}
    {onclick}
    class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-150 {on ? 'bg-accent' : 'bg-line-strong'}">
    <span class="inline-block size-3.5 rounded-full bg-accent-fg transition-transform duration-150 {on ? 'translate-x-[18px]' : 'translate-x-[3px]'}"></span>
  </button>
{/snippet}

<DashboardLayout active="Cloudflare">
  <PageHeader
    title={zone?.name || 'Loading…'}
    description={loading ? 'Loading…' : `${records.length} DNS record${records.length !== 1 ? 's' : ''}`}
    back="/cloudflare"
    backLabel="Cloudflare">
    {#snippet meta()}
      {#if zone?.status}<StatusPill status={zone.status} />{/if}
    {/snippet}
    {#snippet actions()}
      <button onclick={() => (showPurgeConfirm = true)} disabled={purging} class="btn btn-secondary btn-sm">
        <RefreshCw size={14} class={purging ? 'animate-spin' : ''} /> {purging ? 'Purging…' : 'Purge cache'}
      </button>
      <button onclick={openCreateModal} class="btn btn-primary btn-sm"><Plus size={14} /> Create record</button>
    {/snippet}
  </PageHeader>

  <div class="card">
    <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3">
      <button onclick={toggleBulkMode} class="btn btn-sm {bulkMode ? 'btn-secondary' : 'btn-ghost'}">
        {#if bulkMode}<X size={14} /> Cancel select{:else}<ListChecks size={14} /> Bulk select{/if}
      </button>
      {#if bulkMode && selectedIds.size > 0}
        <button onclick={() => (showBulkConfirm = true)} disabled={bulkDeleting} class="btn btn-danger btn-sm">
          <Trash2 size={14} /> Delete {selectedIds.size}
        </button>
      {/if}
      <div class="ml-auto w-36">
        <label for="dns-filter-type" class="sr-only">Record type</label>
        <select id="dns-filter-type" bind:value={filterType} onchange={loadData} class="input h-8 text-[13px]">
          <option value="">All types</option>
          {#each RECORD_TYPES as t (t)}<option value={t}>{t}</option>{/each}
        </select>
      </div>
    </div>

    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(6) as _, i (i)}<div class="skeleton h-9"></div>{/each}
      </div>
    {:else if records.length === 0}
      <EmptyState
        compact
        icon={Globe as any}
        title="No DNS records found"
        description={filterType ? `No ${filterType} records in this zone.` : 'This zone has no DNS records yet.'}
        actionLabel="Create record"
        onaction={openCreateModal} />
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              {#if bulkMode}
                <th class="w-10">
                  <input
                    type="checkbox"
                    class="checkbox"
                    aria-label="Select all records"
                    checked={selectedIds.size === records.length && records.length > 0}
                    onchange={toggleSelectAll} />
                </th>
              {/if}
              <th class="w-20">Type</th>
              <th>Name</th>
              <th class="max-sm:hidden">Content</th>
              <th class="max-sm:hidden">TTL</th>
              <th>Proxy</th>
              <th class="max-md:hidden">Created</th>
              <th class="text-right"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each records as r (r.id)}
              <tr class="group">
                {#if bulkMode}
                  <td>
                    <input
                      type="checkbox"
                      class="checkbox"
                      aria-label="Select {r.name}"
                      checked={selectedIds.has(r.id)}
                      onchange={() => toggleSelect(r.id)} />
                  </td>
                {/if}
                <td><span class="badge font-mono {typeTone[(r.type || '').toUpperCase()] ?? ''}">{r.type}</span></td>
                <td class="max-w-0 min-w-36">
                  <div class="truncate font-medium text-fg" title={r.name}>{r.name}</div>
                  <div class="truncate font-mono text-xs text-subtle sm:hidden" title={r.content}>{r.content}</div>
                </td>
                <td class="max-w-xs truncate font-mono text-[13px] text-muted max-sm:hidden" title={r.content}>{r.content}</td>
                <td class="whitespace-nowrap text-muted tabular-nums max-sm:hidden">{formatTTL(r.ttl)}</td>
                <td>{@render proxySwitch(!!r.proxied, () => toggleProxied(r), `Proxy ${r.name}`)}</td>
                <td class="whitespace-nowrap text-muted max-md:hidden">{formatDate(r.created_on || r.created)}</td>
                <td class="text-right">
                  <div class="flex items-center justify-end gap-0.5 transition-opacity duration-150 sm:opacity-0 sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
                    <button onclick={() => openEditModal(r)} class="btn btn-ghost btn-icon btn-xs" title="Edit" aria-label="Edit {r.name}">
                      <Pencil size={14} />
                    </button>
                    <button onclick={() => confirmDelete(r)} class="btn btn-danger-ghost btn-icon btn-xs" title="Delete" aria-label="Delete {r.name}">
                      <Trash2 size={14} />
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

  <ConfirmDialog
    bind:open={showPurgeConfirm}
    title="Purge cache"
    message={`This will purge all cached content for ${zone?.name || 'this zone'}. All visitors will need to re-fetch resources. This action cannot be undone.`}
    confirmLabel="Purge everything"
    destructive
    onconfirm={purgeCache} />

  <ConfirmDialog
    bind:open={showDeleteConfirm}
    title="Delete DNS record?"
    message={`Delete ${deleteTarget?.type ?? ''} record ${deleteTarget?.name ?? ''}? This cannot be undone.`}
    confirmLabel="Delete"
    destructive
    onconfirm={() => deleteTarget && deleteRecord(deleteTarget.id)}
    oncancel={() => (deleteTarget = null)} />

  <ConfirmDialog
    bind:open={showBulkConfirm}
    title="Delete selected records?"
    message={`Delete ${selectedIds.size} selected record(s)? This cannot be undone.`}
    confirmLabel="Delete"
    destructive
    onconfirm={bulkDelete} />

  <Modal bind:open={showModal} title={editingRecord ? 'Edit DNS record' : 'Create DNS record'} maxWidth="lg" onclose={closeModal}>
    <form id="dns-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); saveRecord(); }}>
      <div>
        <label for="dns-name" class="label">Name</label>
        <input id="dns-name" type="text" bind:value={form.name} placeholder="e.g. subdomain or @ for root" class="input" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="dns-type" class="label">Type</label>
          <select id="dns-type" bind:value={form.type} class="input">
            {#each RECORD_TYPES as t (t)}<option value={t}>{t}</option>{/each}
          </select>
        </div>
        <div>
          <label for="dns-ttl" class="label">TTL</label>
          <select id="dns-ttl" bind:value={form.ttl} class="input">
            <option value={1}>Auto</option>
            <option value={60}>60s</option>
            <option value={300}>5 min</option>
            <option value={3600}>1 hour</option>
            <option value={86400}>1 day</option>
          </select>
        </div>
      </div>
      <div>
        <label for="dns-content" class="label">Content</label>
        <input id="dns-content" type="text" bind:value={form.content} placeholder="e.g. 1.2.3.4 or target.example.com" class="input font-mono" />
      </div>
      <div class="flex items-center justify-between gap-3 rounded-lg border border-line bg-surface-2 px-4 py-3">
        <div>
          <p class="text-sm font-medium text-fg">Proxy</p>
          <p class="text-xs text-subtle">Route traffic through Cloudflare</p>
        </div>
        {@render proxySwitch(form.proxied, () => (form.proxied = !form.proxied), 'Proxy through Cloudflare')}
      </div>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={closeModal}>Cancel</button>
      <button type="submit" form="dns-form" class="btn btn-primary btn-sm" disabled={saving || !form.name || !form.content}>
        {#if saving}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}{editingRecord ? 'Update record' : 'Create record'}{/if}
      </button>
    {/snippet}
  </Modal>
</DashboardLayout>
