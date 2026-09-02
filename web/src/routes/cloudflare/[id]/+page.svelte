<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { ArrowLeft, Plus, Trash2, X, Edit, Globe, RefreshCw, Check, AlertTriangle } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  const zoneId = $derived($page.params.id);

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

  // Bulk select
  let bulkMode = $state(false);
  let selectedIds = $state<Set<string>>(new Set());
  let bulkDeleting = $state(false);

  // Filter
  let filterType = $state('');

  function typeBadgeColor(type: string): string {
    const t = (type || '').toUpperCase();
    if (t === 'A') return 'bg-sky-400/15 text-sky-400 border-sky-400/30';
    if (t === 'AAAA') return 'bg-indigo-400/15 text-indigo-400 border-indigo-400/30';
    if (t === 'CNAME') return 'bg-pink-400/15 text-pink-400 border-pink-400/30';
    if (t === 'MX') return 'bg-amber-400/15 text-amber-400 border-amber-400/30';
    if (t === 'TXT') return 'bg-emerald-400/15 text-emerald-400 border-emerald-400/30';
    return 'bg-slate-400/15 text-slate-400 border-slate-400/30';
  }

  function formatTTL(ttl: number): string {
    if (!ttl || ttl === 1) return 'Auto';
    if (ttl < 60) return `${ttl}s`;
    if (ttl < 3600) return `${Math.round(ttl / 60)}m`;
    if (ttl < 86400) return `${Math.round(ttl / 3600)}h`;
    return `${Math.round(ttl / 86400)}d`;
  }

  function formatDate(d: string): string {
    if (!d) return '-';
    try { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }); }
    catch { return d; }
  }

  async function loadData() {
    if (!auth.token) { loading = false; return; }
    loading = true;
    try {
      const [zoneData, dnsData] = await Promise.all([
        apiFetch(auth.token, 'GET', `cloudflare/zones/${zoneId}`),
        apiFetch(auth.token, 'GET', `cloudflare/zones/${zoneId}/dns${filterType ? `?type=${filterType}` : ''}`)
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
      proxied: record.proxied ?? true
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
      if (editingRecord) {
        await apiFetch(auth.token, 'PUT', `cloudflare/zones/${zoneId}/dns/${editingRecord.id}`, {
          name: form.name, type: form.type, content: form.content, ttl: form.ttl, proxied: form.proxied
        });
        toasts.success('Record updated');
      } else {
        await apiFetch(auth.token, 'POST', `cloudflare/zones/${zoneId}/dns`, {
          name: form.name, type: form.type, content: form.content, ttl: form.ttl, proxied: form.proxied
        });
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
      records = records.filter(r => r.id !== recordId);
      toasts.success('Record deleted');
    } catch (e: any) { toasts.error(e.message); }
  }

  function confirmDelete(recordId: string) {
    if (!confirm('Delete this DNS record? This cannot be undone.')) return;
    deleteRecord(recordId);
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
      selectedIds = new Set(records.map(r => r.id));
    }
  }

  async function bulkDelete() {
    if (selectedIds.size === 0) return;
    if (!confirm(`Delete ${selectedIds.size} selected record(s)? This cannot be undone.`)) return;
    bulkDeleting = true;
    try {
      await apiFetch(auth.token, 'POST', `cloudflare/zones/${zoneId}/dns/bulk-delete`, {
        record_ids: Array.from(selectedIds)
      });
      const removed = selectedIds;
      records = records.filter(r => !removed.has(r.id));
      selectedIds = new Set();
      bulkMode = false;
      toasts.success(`Deleted ${removed.size} record(s)`);
    } catch (e: any) { toasts.error(e.message); }
    finally { bulkDeleting = false; }
  }
</script>

<DashboardLayout active="Cloudflare">
  <div class="max-w-7xl mx-auto">
    <!-- Header -->
    <a href="/cloudflare" class="inline-flex items-center gap-1.5 text-sm text-slate-400 hover:text-sky-400 transition mb-6">
      <ArrowLeft size={14} />
      Back to Cloudflare
    </a>

    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent flex items-center gap-2">
          <Globe size={22} class="text-sky-400" />
          {zone?.name || 'Loading...'}
        </h1>
        <p class="text-sm text-slate-500 mt-1">
          {loading ? 'Loading...' : `${records.length} DNS record${records.length !== 1 ? 's' : ''}`}
          {#if zone?.status}
            <span class="ml-2 inline-flex items-center gap-1">
              <StatusPill status={zone.status} />
            </span>
          {/if}
        </p>
      </div>
    </div>

    <!-- Actions Bar -->
    <div class="flex flex-wrap items-center gap-3 mb-5">
      <button
        onclick={openCreateModal}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-sky-500/20 hover:bg-sky-500/30 text-sky-400 text-sm font-medium border border-sky-500/20 transition"
      >
        <Plus size={15} />
        Create Record
      </button>

      <button
        onclick={() => showPurgeConfirm = true}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-indigo-500/15 hover:bg-indigo-500/25 text-indigo-400 text-sm font-medium border border-indigo-500/20 transition"
      >
        <RefreshCw size={15} />
        Purge Cache
      </button>

      <button
        onclick={toggleBulkMode}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium border transition
          {bulkMode
            ? 'bg-amber-500/20 text-amber-400 border-amber-500/30 hover:bg-amber-500/30'
            : 'bg-white/[0.04] text-slate-400 border-white/[0.08] hover:bg-white/[0.08]'}"
      >
        {#if bulkMode}
          <X size={15} />
          Cancel Select
        {:else}
          <Check size={15} />
          Bulk Select
        {/if}
      </button>

      {#if bulkMode && selectedIds.size > 0}
        <button
          onclick={bulkDelete}
          disabled={bulkDeleting}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-red-500/20 hover:bg-red-500/30 text-red-400 text-sm font-medium border border-red-500/20 transition disabled:opacity-50"
        >
          <Trash2 size={15} />
          Delete {selectedIds.size} Selected
        </button>
      {/if}

      <!-- Filter -->
      <div class="ml-auto">
        <select
          bind:value={filterType}
          onchange={loadData}
          class="bg-white/[0.04] border border-white/[0.08] rounded-xl px-3 py-2 text-sm text-slate-300 outline-none focus:border-sky-500/40 transition appearance-none cursor-pointer"
        >
          <option value="">All Types</option>
          <option value="A">A</option>
          <option value="AAAA">AAAA</option>
          <option value="CNAME">CNAME</option>
          <option value="MX">MX</option>
          <option value="TXT">TXT</option>
          <option value="SRV">SRV</option>
          <option value="NS">NS</option>
          <option value="CAA">CAA</option>
        </select>
      </div>
    </div>

    <!-- DNS Records Table -->
    {#if loading}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6 text-center py-20">
        <div class="animate-pulse text-slate-500 flex items-center justify-center gap-2">
          <RefreshCw size={16} class="animate-spin" />
          Loading DNS records...
        </div>
      </div>
    {:else if records.length === 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-12 text-center">
        <Globe size={40} class="mx-auto text-slate-600 mb-4" />
        <p class="text-slate-400 text-sm">No DNS records found</p>
        <button onclick={openCreateModal} class="mt-4 inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-sky-500/20 hover:bg-sky-500/30 text-sky-400 text-sm font-medium border border-sky-500/20 transition">
          <Plus size={15} />
          Create your first record
        </button>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead>
              <tr class="text-left">
                {#if bulkMode}
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] w-10">
                    <input
                      type="checkbox"
                      checked={selectedIds.size === records.length && records.length > 0}
                      onchange={toggleSelectAll}
                      class="rounded border-white/20 bg-white/5 text-sky-500 focus:ring-sky-500/40 cursor-pointer"
                    />
                  </th>
                {/if}
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Type</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Content</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">TTL</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Proxied</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Created</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each records as r (r.id)}
                <tr class="hover:bg-white/[0.02] transition group">
                  {#if bulkMode}
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle w-10">
                      <input
                        type="checkbox"
                        checked={selectedIds.has(r.id)}
                        onchange={() => toggleSelect(r.id)}
                        class="rounded border-white/20 bg-white/5 text-sky-500 focus:ring-sky-500/40 cursor-pointer"
                      />
                    </td>
                  {/if}
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="inline-flex items-center px-2.5 py-0.5 rounded-lg text-[0.72rem] font-semibold border {typeBadgeColor(r.type)}">
                      {r.type}
                    </span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] font-medium text-sm align-middle">
                    {r.name}
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-400 align-middle max-w-xs truncate" title={r.content}>
                    {r.content}
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-500 align-middle">
                    {formatTTL(r.ttl)}
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <button
                      onclick={() => toggleProxied(r)}
                      class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors {r.proxied ? 'bg-sky-500' : 'bg-white/10'}"
                      title={r.proxied ? 'Proxied (click to disable)' : 'DNS only (click to enable)'}
                    >
                      <span class="inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform {r.proxied ? 'translate-x-[18px]' : 'translate-x-[3px]'}" />
                    </button>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-500 align-middle">
                    {formatDate(r.created_on || r.created)}
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onclick={() => openEditModal(r)}
                        class="text-xs text-slate-400 hover:text-sky-400 transition p-1.5 rounded-lg hover:bg-sky-400/10"
                        title="Edit"
                      >
                        <Edit size={14} />
                      </button>
                      <button
                        onclick={() => confirmDelete(r.id)}
                        class="text-xs text-slate-400 hover:text-red-400 transition p-1.5 rounded-lg hover:bg-red-400/10"
                        title="Delete"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  </div>

  <!-- Purge Cache Confirm Dialog -->
  {#if showPurgeConfirm}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" role="dialog" onkeydown={(e) => e.key === 'Escape' && (showPurgeConfirm = false)}>
      <div class="bg-[#0a1122] border border-white/[0.1] rounded-2xl p-6 w-full max-w-md shadow-2xl shadow-black/50">
        <div class="flex items-center gap-3 mb-4">
          <div class="p-2 rounded-xl bg-amber-500/15">
            <AlertTriangle size={20} class="text-amber-400" />
          </div>
          <h3 class="text-lg font-semibold text-white">Purge Cache</h3>
        </div>
        <p class="text-sm text-slate-400 mb-6">
          This will purge all cached content for <span class="text-white font-medium">{zone?.name || 'this zone'}</span>.
          All visitors will need to re-fetch resources. This action cannot be undone.
        </p>
        <div class="flex justify-end gap-3">
          <button
            onclick={() => showPurgeConfirm = false}
            class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 bg-white/[0.05] hover:bg-white/[0.1] border border-white/[0.08] transition"
          >
            Cancel
          </button>
          <button
            onclick={purgeCache}
            disabled={purging}
            class="px-4 py-2 rounded-xl text-sm font-medium text-white bg-amber-500 hover:bg-amber-600 transition disabled:opacity-50 flex items-center gap-2"
          >
            {#if purging}
              <RefreshCw size={14} class="animate-spin" />
              Purging...
            {:else}
              <RefreshCw size={14} />
              Purge Everything
            {/if}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Create / Edit Modal -->
  {#if showModal}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" role="dialog" onkeydown={(e) => e.key === 'Escape' && closeModal()}>
      <div class="bg-[#0a1122] border border-white/[0.1] rounded-2xl p-6 w-full max-w-lg shadow-2xl shadow-black/50">
        <!-- Modal Header -->
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-lg font-semibold text-white">
            {editingRecord ? 'Edit DNS Record' : 'Create DNS Record'}
          </h3>
          <button onclick={closeModal} class="p-1.5 rounded-lg text-slate-500 hover:text-slate-300 hover:bg-white/[0.05] transition">
            <X size={18} />
          </button>
        </div>

        <!-- Form -->
        <div class="space-y-4">
          <!-- Name -->
          <div>
            <label for="dns-name" class="block text-xs font-medium text-slate-400 mb-1.5">Name</label>
            <input
              id="dns-name"
              type="text"
              bind:value={form.name}
              placeholder="e.g. subdomain or @ for root"
              class="w-full px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/[0.08] text-sm text-white placeholder-slate-600 outline-none focus:border-sky-500/40 transition"
            />
          </div>

          <!-- Type & Proxied row -->
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label for="dns-type" class="block text-xs font-medium text-slate-400 mb-1.5">Type</label>
              <select
                id="dns-type"
                bind:value={form.type}
                class="w-full px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/[0.08] text-sm text-white outline-none focus:border-sky-500/40 transition appearance-none cursor-pointer"
              >
                {#each ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'NS', 'CAA'] as t}
                  <option value={t}>{t}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="dns-ttl" class="block text-xs font-medium text-slate-400 mb-1.5">TTL</label>
              <select
                id="dns-ttl"
                bind:value={form.ttl}
                class="w-full px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/[0.08] text-sm text-white outline-none focus:border-sky-500/40 transition appearance-none cursor-pointer"
              >
                <option value={1}>Auto</option>
                <option value={60}>60s</option>
                <option value={300}>5 min</option>
                <option value={3600}>1 hour</option>
                <option value={86400}>1 day</option>
              </select>
            </div>
          </div>

          <!-- Content -->
          <div>
            <label for="dns-content" class="block text-xs font-medium text-slate-400 mb-1.5">Content</label>
            <input
              id="dns-content"
              type="text"
              bind:value={form.content}
              placeholder="e.g. 1.2.3.4 or target.example.com"
              class="w-full px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/[0.08] text-sm text-white placeholder-slate-600 outline-none focus:border-sky-500/40 transition"
            />
          </div>

          <!-- Proxied toggle -->
          <div class="flex items-center justify-between rounded-xl bg-white/[0.03] border border-white/[0.06] px-4 py-3">
            <div>
              <p class="text-sm text-white font-medium">Proxy</p>
              <p class="text-xs text-slate-500">Route traffic through Cloudflare</p>
            </div>
            <button
              type="button"
              onclick={() => form.proxied = !form.proxied}
              class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors {form.proxied ? 'bg-sky-500' : 'bg-white/10'}"
            >
              <span class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform {form.proxied ? 'translate-x-[22px]' : 'translate-x-[4px]'}" />
            </button>
          </div>
        </div>

        <!-- Modal Footer -->
        <div class="flex justify-end gap-3 mt-6">
          <button
            onclick={closeModal}
            class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 bg-white/[0.05] hover:bg-white/[0.1] border border-white/[0.08] transition"
          >
            Cancel
          </button>
          <button
            onclick={saveRecord}
            disabled={saving || !form.name || !form.content}
            class="px-4 py-2 rounded-xl text-sm font-medium text-white bg-sky-500 hover:bg-sky-600 transition disabled:opacity-50 flex items-center gap-2"
          >
            {#if saving}
              <RefreshCw size={14} class="animate-spin" />
              Saving...
            {:else}
              {editingRecord ? 'Update Record' : 'Create Record'}
            {/if}
          </button>
        </div>
      </div>
    </div>
  {/if}
</DashboardLayout>
