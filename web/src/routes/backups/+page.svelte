<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { fmtRelative } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import {
    Package, RefreshCw, RotateCcw, Download, Trash2,
    Database, HardDrive, Calendar, Cloud, Loader2,
    CheckCircle2, XCircle, Clock, AlertCircle, Archive,
    Settings, Plus, Edit3, Globe, Key, Play, X,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let backups = $state<any[]>([]);
  let providers = $state<any[]>([]);
  let configs = $state<any[]>([]);
  let loading = $state(true);
  let confirmDelete = $state<any>(null);
  let deletingId = $state<number | null>(null);
  let view = $state<'history' | 'settings'>('history');
  let activeTab = $state<'all' | 'files' | 'database'>('all');
  let settingsTab = $state<'providers' | 'configs'>('providers');
  let showProviderForm = $state(false);
  let editingProvider = $state<any>(null);
  let showConfigForm = $state(false);
  let editingConfig = $state<any>(null);
  let allSites = $state<any[]>([]);
  let allDatabases = $state<any[]>([]);
  let providerUsage = $state<Record<number, any>>({});
  let saving = $state(false);
  let testingProvider = $state(false);
  let testResult = $state<any>(null);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await load();
    await loadProviders();
    await loadConfigs();
    await loadSites();
    await loadDatabases();
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

  function downloadBackup(id: number) {
    window.location.href = `/api/proxy/backups/${id}/download`;
  }

  async function restoreBackup(id: number) {
    if (!confirm(`Restore backup #${id}? This will overwrite current site files/database.`)) return;
    try {
      await apiFetch(auth.token, 'POST', `backups/${id}/restore`);
      toasts.success('Restore job queued');
      await load();
    } catch (e: any) {
      toasts.error(e.message || 'Restore failed');
    }
  }

  async function deleteBackup(id: number) {
    deletingId = id;
    try {
      await apiFetch(auth.token, 'DELETE', `backups/${id}`);
      backups = backups.filter(b => b.id !== id);
      confirmDelete = null;
      toasts.success('Backup deleted');
    } catch (e: any) {
      toasts.error(e.message || 'Delete failed');
    } finally {
      deletingId = null;
    }
  }

  async function loadProviders() {
    try {
      const resp = await apiFetch(auth.token, 'GET', 'backup-providers');
      providers = resp?.providers || [];
      // Load usage for each provider
      for (const p of providers) {
        try {
          const uResp = await apiFetch(auth.token, 'GET', `backup-providers/${p.id}/usage?source=actual`);
          if (uResp) providerUsage[p.id] = uResp;
        } catch {}
      }
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

  async function saveProvider() {
    saving = true;
    try {
      const body = { ...editingProvider };
      if (editingProvider.id) {
        await apiFetch(auth.token, 'PATCH', `backup-providers/${editingProvider.id}`, body);
        toasts.success('Provider updated');
      } else {
        await apiFetch(auth.token, 'POST', 'backup-providers', body);
        toasts.success('Provider created');
      }
      showProviderForm = false;
      editingProvider = null;
      await loadProviders();
    } catch (e: any) { toasts.error(e.message); }
    finally { saving = false; }
  }

  async function testProviderConnection(id: number) {
    testingProvider = true;
    testResult = null;
    try {
      const resp = await apiFetch(auth.token, 'POST', `backup-providers/${id}/test`);
      testResult = resp;
    } catch (e: any) { toasts.error(e.message); }
    finally { testingProvider = false; }
  }

  async function deleteProvider(id: number) {
    try {
      await apiFetch(auth.token, 'DELETE', `backup-providers/${id}`);
      toasts.success('Provider deleted');
      await loadProviders();
    } catch (e: any) { toasts.error(e.message); }
  }

  async function saveConfig() {
    saving = true;
    try {
      const body = { ...editingConfig };
      // Convert exclude_patterns_text → array
      if (body.exclude_patterns_text !== undefined) {
        body.exclude_patterns = body.exclude_patterns_text
          ? body.exclude_patterns_text.split(',').map((s: string) => s.trim()).filter((s: string) => s)
          : [];
        delete body.exclude_patterns_text;
      }
      // Handle exclude_patterns from API response (array)
      if (Array.isArray(body.exclude_patterns)) {
        body.exclude_patterns_text = body.exclude_patterns.join(', ');
      }
      // Empty site_ids enables auto-discovery for database/full configs.
      if (body._autoDiscover && (body.scope_type === 'database' || body.scope_type === 'both')) {
        body.site_ids = [];
      }
      delete body._autoDiscover;
      delete body._allSites;
      delete body._preset;
      delete body._hour;
      // Clean up undefined values
      Object.keys(body).forEach(k => { if (body[k] === undefined) delete body[k]; });
      if (editingConfig.id) {
        await apiFetch(auth.token, 'PATCH', `backup-configs/${editingConfig.id}`, body);
        toasts.success('Config updated');
      } else {
        await apiFetch(auth.token, 'POST', 'backup-configs', body);
        toasts.success('Config created');
      }
      showConfigForm = false;
      editingConfig = null;
      await loadConfigs();
    } catch (e: any) { toasts.error(e.message); }
    finally { saving = false; }
  }

  async function deleteConfig(id: number) {
    try {
      await apiFetch(auth.token, 'DELETE', `backup-configs/${id}`);
      toasts.success('Config deleted');
      await loadConfigs();
    } catch (e: any) { toasts.error(e.message); }
  }

  async function runConfig(id: number) {
    try {
      await apiFetch(auth.token, 'POST', `backup-configs/${id}/run`);
      toasts.success('Backup run triggered');
    } catch (e: any) { toasts.error(e.message); }
  }

  let showManualBackup = $state(false);
  let manualSiteId = $state<number>(0);
  let manualType = $state<'files' | 'database' | 'both'>('both');
  let manualProviderId = $state<number>(0);
  let manualBusy = $state(false);

  const filtered = $derived(
    activeTab === 'all' ? backups : backups.filter(b => b.backup_type === activeTab)
  );

  let stats = $derived({
    total: backups.length,
    completed: backups.filter(b => b.status === 'completed').length,
    failed: backups.filter(b => b.status === 'failed').length,
    running: backups.filter(b => b.status === 'running' || b.status === 'archiving').length,
    totalSize: backups.reduce((s, b) => s + (b.size_bytes || 0), 0),
  });

  async function triggerManualBackup() {
    if (!manualSiteId) {
      toasts.error('Please select a site');
      return;
    }
    manualBusy = true;
    try {
      const endpoint = manualType === 'database'
        ? `sites/${manualSiteId}/backups/database`
        : `sites/${manualSiteId}/backups`;
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

  function statusConfig(s: string) {
    if (s === 'completed') return { color: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20', icon: CheckCircle2 };
    if (s === 'failed') return { color: 'bg-red-500/15 text-red-400 border-red-500/20', icon: XCircle };
    if (s === 'running' || s === 'archiving') return { color: 'bg-sky-500/15 text-sky-400 border-sky-500/20', icon: Clock };
    return { color: 'bg-slate-500/15 text-slate-400 border-slate-500/20', icon: AlertCircle };
  }

  function fmtBytes(bytes: number): string {
    if (!bytes || bytes === 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let size = bytes;
    while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
    return `${size.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
  }
</script>

<DashboardLayout active="Backups">
  <!-- Header -->
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-xl font-bold text-white">Backups</h1>
      <p class="text-sm text-slate-400 mt-0.5">Manage, configure, and monitor backups</p>
    </div>
    <div class="flex items-center gap-2">
      <button onclick={() => { showManualBackup = true; loadSites(); loadProviders(); }}
        class="px-4 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-amber-400 to-orange-400 text-[#0a1628] hover:-translate-y-0.5 transition-all shadow-sm cursor-pointer flex items-center gap-1.5 border-0">
        <Plus size={13} /> + Create Backup
      </button>
      <button onclick={load}
        class="px-4 py-2 rounded-xl text-xs font-medium bg-white/[0.04] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] hover:text-white transition-all duration-200 cursor-pointer flex items-center gap-1.5">
        <RefreshCw size={12} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
    </div>
  </div>

  <!-- View Toggle: History | Settings -->
  <div class="flex gap-1 mb-6 bg-white/[0.03] rounded-xl p-1 border border-white/[0.06]">
    <button onclick={() => view = 'history'}
      class="flex-1 flex items-center justify-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all duration-200 cursor-pointer
        {view === 'history'
          ? 'bg-gradient-to-r from-amber-400/20 to-orange-400/20 text-amber-300 shadow-sm border border-amber-400/20'
          : 'text-slate-400 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent'}">
      <Package size={12} /> Backup History
    </button>
    <button onclick={() => view = 'settings'}
      class="flex-1 flex items-center justify-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all duration-200 cursor-pointer
        {view === 'settings'
          ? 'bg-gradient-to-r from-amber-400/20 to-orange-400/20 text-amber-300 shadow-sm border border-amber-400/20'
          : 'text-slate-400 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent'}">
      <Settings size={12} /> Settings
    </button>
  </div>

  {#if view === 'history'}
    <!-- Stats Cards -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
      {#each [
        { label: 'Total', value: stats.total, color: 'from-sky-400 to-blue-500', bg: 'bg-sky-400/10' },
        { label: 'Completed', value: stats.completed, color: 'from-emerald-400 to-green-500', bg: 'bg-emerald-400/10' },
        { label: 'Failed', value: stats.failed, color: 'from-red-400 to-rose-500', bg: 'bg-red-400/10' },
        { label: 'Storage', value: fmtBytes(stats.totalSize), color: 'from-amber-400 to-orange-500', bg: 'bg-amber-400/10' },
      ] as stat}
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4 hover:border-white/[0.12] transition-all duration-300">
          <div class="text-2xl font-bold bg-gradient-to-r {stat.color} bg-clip-text text-transparent">{stat.value}</div>
          <div class="text-xs text-slate-500 mt-1">{stat.label}</div>
        </div>
      {/each}
    </div>

    <!-- Cloud Storage Status -->
    {#each providers as p}
      {#if providerUsage[p.id]}
        {@const u = providerUsage[p.id]}
        <div class="flex items-center gap-3 px-4 py-2.5 mb-4 bg-white/[0.03] border border-white/[0.07] rounded-xl">
          <Cloud size={14} class="text-sky-400" />
          <span class="text-xs text-slate-400">
            <strong class="text-slate-200">{p.name}</strong>
            {#if u.connected}
              <span class="text-emerald-400 ml-1">● Connected</span>
            {:else}
              <span class="text-red-400 ml-1">● Disconnected</span>
            {/if}
          </span>
          <span class="text-xs text-slate-500 ml-auto">
            {u.object_count || 0} objects · {u.total_mb ? u.total_mb.toFixed(0) + ' MB' : '0 MB'}
          </span>
        </div>
      {/if}
    {/each}

    <!-- Type Tabs -->
    <div class="flex gap-1 mb-4 bg-white/[0.03] rounded-xl p-1 border border-white/[0.06]">
      {#each [
        { id: 'all' as const, label: 'All Backups', icon: Package },
        { id: 'files' as const, label: 'Files', icon: HardDrive },
        { id: 'database' as const, label: 'Databases', icon: Database },
      ] as tab}
        <button onclick={() => activeTab = tab.id}
          class="flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all duration-200 cursor-pointer
            {activeTab === tab.id
              ? 'bg-gradient-to-r from-amber-400/20 to-orange-400/20 text-amber-300 shadow-sm border border-amber-400/20'
              : 'text-slate-400 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent'}">
          <svelte:component this={tab.icon} size={12} />
          {tab.label}
        </button>
      {/each}
    </div>

    <!-- Backup List -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
      <div class="px-5 py-3.5 border-b border-white/[0.05] flex items-center gap-2">
        <Archive size={14} class="text-amber-400" />
        <span class="text-sm font-semibold text-slate-200">History</span>
        {#if !loading}
          <span class="ml-auto text-xs text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full">{filtered.length} backup{filtered.length !== 1 ? 's' : ''}</span>
        {/if}
      </div>
      <div class="p-5">
        {#if loading}
          <div class="flex flex-col items-center justify-center py-16 gap-3">
            <Loader2 size={24} class="animate-spin text-sky-400" />
            <p class="text-sm text-slate-500">Loading backups...</p>
          </div>
        {:else if filtered.length === 0}
          <div class="flex flex-col items-center justify-center py-16 gap-3">
            <div class="w-14 h-14 rounded-2xl bg-white/[0.03] border border-white/[0.06] flex items-center justify-center">
              <Package size={28} class="text-slate-600" />
            </div>
            <p class="text-sm text-slate-500">No backups yet</p>
          </div>
        {:else}
          <div class="overflow-x-auto -mx-5">
            <table class="w-full text-xs">
              <thead>
                <tr class="text-slate-500 border-b border-white/[0.05]">
                  <th class="text-left py-3 px-5 font-medium">Status</th>
                  <th class="text-left py-3 px-5 font-medium">Site</th>
                  <th class="text-left py-3 px-5 font-medium">Type</th>
                  <th class="text-left py-3 px-5 font-medium">Storage</th>
                  <th class="text-left py-3 px-5 font-medium">Size</th>
                  <th class="text-left py-3 px-5 font-medium">Date</th>
                  <th class="text-right py-3 px-5 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {#each filtered as b}
                  {@const cfg = statusConfig(b.status)}
                  <tr class="border-b border-white/[0.03] hover:bg-white/[0.02] transition-colors duration-150">
                    <td class="py-3 px-5">
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium border {cfg.color}">
                        <svelte:component this={cfg.icon} size={10} />
                        {b.status}
                      </span>
                    </td>
                    <td class="py-3 px-5">
                      <span class="text-slate-200 font-medium">{b.site?.domain || b.site?.name || `Site #${b.site_id}`}</span>
                    </td>
                    <td class="py-3 px-5">
                      <span class="inline-flex items-center gap-1 text-slate-300">
                        {#if b.backup_type === 'database'}
                          <Database size={10} class="text-sky-400" /> Database
                        {:else}
                          <HardDrive size={10} class="text-amber-400" /> Files
                        {/if}
                      </span>
                    </td>
                    <td class="py-3 px-5">
                      {#if b.s3_path}
                        <span class="inline-flex items-center gap-1 text-sky-400"><Cloud size={10} /> B2</span>
                      {:else}
                        <span class="inline-flex items-center gap-1 text-slate-400"><HardDrive size={10} /> Local</span>
                      {/if}
                    </td>
                    <td class="py-3 px-5 font-mono text-slate-400">{b.size_bytes ? fmtBytes(b.size_bytes) : '—'}</td>
                    <td class="py-3 px-5 text-slate-400 whitespace-nowrap">
                      <Calendar size={10} class="inline mr-1 opacity-60" />{fmtRelative(b.created_at)}
                    </td>
                    <td class="py-3 px-5">
                      <div class="flex justify-end gap-0.5">
                        <button onclick={() => downloadBackup(b.id)} title="Download"
                          class="p-2 rounded-lg hover:bg-sky-500/10 text-slate-500 hover:text-sky-400 transition-all duration-200 cursor-pointer border-0 bg-transparent">
                          <Download size={12} />
                        </button>
                        <button onclick={() => restoreBackup(b.id)} title="Restore"
                          class="p-2 rounded-lg hover:bg-amber-500/10 text-slate-500 hover:text-amber-400 transition-all duration-200 cursor-pointer border-0 bg-transparent">
                          <RotateCcw size={12} />
                        </button>
                        <button onclick={() => confirmDelete = b} title="Delete"
                          class="p-2 rounded-lg hover:bg-red-500/10 text-slate-500 hover:text-red-400 transition-all duration-200 cursor-pointer border-0 bg-transparent">
                          <Trash2 size={12} />
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
    </div>
  {:else}
    <!-- Settings View -->
    <div class="flex gap-1 mb-4 bg-white/[0.03] rounded-xl p-1 border border-white/[0.06]">
      <button onclick={() => settingsTab = 'providers'}
        class="flex-1 flex items-center justify-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all duration-200 cursor-pointer
          {settingsTab === 'providers'
            ? 'bg-gradient-to-r from-amber-400/20 to-orange-400/20 text-amber-300 shadow-sm border border-amber-400/20'
            : 'text-slate-400 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent'}">
        <Cloud size={12} /> Storage Providers
      </button>
      <button onclick={() => settingsTab = 'configs'}
        class="flex-1 flex items-center justify-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all duration-200 cursor-pointer
          {settingsTab === 'configs'
            ? 'bg-gradient-to-r from-amber-400/20 to-orange-400/20 text-amber-300 shadow-sm border border-amber-400/20'
            : 'text-slate-400 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent'}">
        <Edit3 size={12} /> Backup Configs
      </button>
    </div>

    {#if settingsTab === 'providers'}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
        <div class="px-5 py-3.5 border-b border-white/[0.05] flex items-center gap-2">
          <Cloud size={14} class="text-sky-400" />
          <span class="text-sm font-semibold text-slate-200">Storage Providers</span>
          <button onclick={() => { editingProvider = {}; showProviderForm = true; }}
            class="ml-auto px-3 py-1.5 rounded-lg text-[10px] font-medium bg-sky-500/15 text-sky-400 border border-sky-500/20 hover:bg-sky-500/25 transition cursor-pointer flex items-center gap-1">
            <Plus size={10} /> Add Provider
          </button>
        </div>
        <div class="p-5 space-y-2">
          {#each providers as p}
            <div class="flex items-center justify-between p-3 bg-white/[0.02] border border-white/[0.05] rounded-xl">
              <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded-lg bg-sky-400/10 flex items-center justify-center">
                  <Cloud size={14} class="text-sky-400" />
                </div>
                <div>
                  <p class="text-xs font-medium text-slate-200">{p.name}</p>
                  <p class="text-[10px] text-slate-500">{p.provider_type} · {p.bucket}</p>
                  {#if providerUsage[p.id]}
                    {@const u = providerUsage[p.id]}
                    <div class="flex items-center gap-2 mt-1 text-[10px]">
                      {#if u.connected}
                        <span class="text-emerald-400">● Connected</span>
                      {:else}
                        <span class="text-red-400">● Disconnected</span>
                      {/if}
                      <span class="text-slate-500">{u.object_count || 0} objects</span>
                      <span class="text-slate-500">{u.total_mb ? u.total_mb.toFixed(0) + ' MB' : ''}</span>
                    </div>
                  {:else}
                    <div class="flex items-center gap-2 mt-1 text-[10px]">
                      <Loader2 size={8} class="animate-spin text-slate-500" />
                      <span class="text-slate-500">Loading usage...</span>
                    </div>
                  {/if}
                </div>
              </div>
              <div class="flex gap-1">
                <button onclick={() => { editingProvider = { ...p }; showProviderForm = true; }}
                  class="p-1.5 rounded-lg hover:bg-amber-500/10 text-slate-500 hover:text-amber-400 transition cursor-pointer border-0 bg-transparent">
                  <Edit3 size={12} />
                </button>
                <button onclick={() => deleteProvider(p.id)}
                  class="p-1.5 rounded-lg hover:bg-red-500/10 text-slate-500 hover:text-red-400 transition cursor-pointer border-0 bg-transparent">
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          {:else}
            <div class="text-center py-8 text-sm text-slate-500">No providers configured</div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
        <div class="px-5 py-3.5 border-b border-white/[0.05] flex items-center gap-2">
          <Edit3 size={14} class="text-amber-400" />
          <span class="text-sm font-semibold text-slate-200">Backup Configs</span>
          <button onclick={() => { editingConfig = { name: 'Daily Databases', scope_type: 'database', cron_expr: '0 0 * * *', enabled: true, site_ids: [], database_ids: [], retention_days: 7, _autoDiscover: true, _allSites: false }; showConfigForm = true; }}
            class="ml-auto px-3 py-1.5 rounded-lg text-[10px] font-medium bg-amber-500/15 text-amber-400 border border-amber-500/20 hover:bg-amber-500/25 transition cursor-pointer flex items-center gap-1">
            <Plus size={10} /> Add Config
          </button>
        </div>
        <div class="p-5 space-y-2">
          {#each configs as c}
            <div class="p-3 bg-white/[0.02] border border-white/[0.05] rounded-xl hover:border-white/[0.1] transition-all">
              <div class="flex items-center justify-between mb-1.5">
                <div class="flex items-center gap-2">
                  <span class="text-xs font-medium text-slate-200">{c.name || 'Unnamed Config'}</span>
                  <span class="text-[10px] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full">{c.scope_type}</span>
                  {#if c.enabled}
                    <span class="text-[10px] text-emerald-400">● active</span>
                  {:else}
                    <span class="text-[10px] text-slate-500">○ inactive</span>
                  {/if}
                </div>
                <div class="flex gap-1">
                  <button onclick={() => runConfig(c.id)} title="Run Now"
                    class="p-1.5 rounded-lg hover:bg-emerald-500/10 text-slate-500 hover:text-emerald-400 transition cursor-pointer border-0 bg-transparent">
                    <Play size={12} />
                  </button>
                  <button onclick={() => { const cfg = { ...c }; if (Array.isArray(cfg.exclude_patterns)) cfg.exclude_patterns_text = cfg.exclude_patterns.join(', '); cfg.site_ids = cfg.site_ids || []; cfg._autoDiscover = cfg.auto_discover_sites === true; cfg._allSites = !cfg._autoDiscover && cfg.site_ids.length === allSites.length; editingConfig = cfg; showConfigForm = true; }} title="Edit"
                    class="p-1.5 rounded-lg hover:bg-amber-500/10 text-slate-500 hover:text-amber-400 transition cursor-pointer border-0 bg-transparent">
                    <Edit3 size={12} />
                  </button>
                  <button onclick={() => deleteConfig(c.id)} title="Delete"
                    class="p-1.5 rounded-lg hover:bg-red-500/10 text-slate-500 hover:text-red-400 transition cursor-pointer border-0 bg-transparent">
                    <Trash2 size={12} />
                  </button>
                </div>
              </div>
              <p class="text-[10px] text-slate-500 flex items-center gap-2">
                <Clock size={8} /> {c.cron_expr || 'Not scheduled'}
                {#if c.retention_days}· {c.retention_days}d retention{/if}
                {#if c.provider_id}· Provider #{c.provider_id}{/if}
                {#if c.last_run_at}· Last: {fmtRelative(c.last_run_at)}{/if}
              </p>
            </div>
          {:else}
            <div class="text-center py-8 text-sm text-slate-500">No backup configs</div>
          {/each}
        </div>
      </div>
    {/if}
  {/if}
</DashboardLayout>

<!-- Delete Modal -->
{#if confirmDelete}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onclick={() => confirmDelete = null}>
    <div class="bg-[#0f1a2e] border border-white/[0.1] rounded-2xl shadow-2xl w-full max-w-sm p-6" onclick={(e) => e.stopPropagation()}>
      <div class="w-12 h-12 rounded-2xl bg-red-500/10 border border-red-500/20 flex items-center justify-center mx-auto mb-4">
        <Trash2 size={22} class="text-red-400" />
      </div>
      <h3 class="text-base font-semibold text-white text-center mb-2">Delete Backup</h3>
      <p class="text-xs text-slate-400 text-center mb-6">Delete backup for <strong class="text-slate-300">{confirmDelete.site?.domain || 'site'}</strong>?</p>
      <div class="flex gap-3">
        <button onclick={() => confirmDelete = null} class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-white/[0.05] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] transition cursor-pointer">Cancel</button>
        <button onclick={() => deleteBackup(confirmDelete.id)} disabled={deletingId === confirmDelete.id}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-gradient-to-r from-red-500 to-rose-500 text-white hover:shadow-lg hover:shadow-red-500/20 transition-all disabled:opacity-50 cursor-pointer border-0">
          {deletingId === confirmDelete.id ? 'Deleting...' : 'Delete'}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Provider Add/Edit Modal -->
{#if showProviderForm}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onclick={() => { showProviderForm = false; editingProvider = null; }}>
    <div class="bg-[#0f1a2e] border border-white/[0.1] rounded-2xl shadow-2xl w-full max-w-md p-6" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-base font-semibold text-white mb-1">{editingProvider?.id ? 'Edit Provider' : 'Add Provider'}</h3>
      <p class="text-xs text-slate-400 mb-5">{editingProvider?.id ? 'Update storage provider settings' : 'Configure a new backup storage provider'}</p>
      <div class="space-y-4">
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Provider Name</label>
          <input type="text" bind:value={editingProvider.name} placeholder="My Provider"
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Type</label>
          <select bind:value={editingProvider.provider_type}
            onchange={() => {
              if (editingProvider.provider_type === 'r2') {
                editingProvider.region = 'auto';
                editingProvider.endpoint = '';
              } else if (editingProvider.provider_type === 's3') {
                editingProvider.region = 'us-east-1';
              } else {
                editingProvider.region = '';
              }
              editingProvider.endpoint = editingProvider.provider_type === 'r2' ? '' : editingProvider.endpoint;
              testResult = null;
            }}
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white focus:outline-none focus:border-amber-400/40">
            <option value="s3">Amazon S3</option>
            <option value="r2">Cloudflare R2</option>
            <option value="b2">Backblaze B2</option>
            <option value="wasabi">Wasabi</option>
            <option value="digitalocean">DigitalOcean Spaces</option>
            <option value="generic">S3-Compatible (Generic)</option>
          </select>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Access Key</label>
            <input type="text" bind:value={editingProvider.access_key} placeholder="AKID..."
              class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
          </div>
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Secret Key</label>
            <input type="password" bind:value={editingProvider.secret_key} placeholder="••••••••"
              class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
          </div>
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Bucket</label>
          <input type="text" bind:value={editingProvider.bucket} placeholder="my-backups"
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Endpoint</label>
            {#if editingProvider.provider_type === 'r2'}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="<account-id>.r2.cloudflarestorage.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else if editingProvider.provider_type === 's3'}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="s3.amazonaws.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else if editingProvider.provider_type === 'b2'}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="s3.us-east-005.backblazeb2.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else if editingProvider.provider_type === 'wasabi'}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="s3.us-east-1.wasabisys.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else if editingProvider.provider_type === 'digitalocean'}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="sgp1.digitaloceanspaces.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else}
              <input type="text" bind:value={editingProvider.endpoint} placeholder="your-storage-endpoint.com"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {/if}
            {#if editingProvider.provider_type === 'r2'}
              <p class="text-[10px] text-slate-500 mt-1">Find in R2 Dashboard → Settings → S3 API</p>
            {/if}
          </div>
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Region</label>
            {#if editingProvider.provider_type === 'r2'}
              <input type="text" bind:value={editingProvider.region} placeholder="auto" disabled
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-slate-400 placeholder:text-slate-600 focus:outline-none cursor-not-allowed opacity-50" />
              <p class="text-[10px] text-slate-500 mt-1">Auto (R2 is global)</p>
            {:else if editingProvider.provider_type === 'digitalocean'}
              <input type="text" bind:value={editingProvider.region} placeholder="sgp1"
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {:else}
              <input type="text" bind:value={editingProvider.region} placeholder={editingProvider.provider_type === 's3' ? 'us-east-1' : 'us-east-005'}
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
            {/if}
          </div>
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Retention (days, 0 = forever)</label>
          <input type="number" bind:value={editingProvider.retention_days} placeholder="30" min="0"
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
        </div>
        <div class="flex items-center gap-2">
          <input type="checkbox" bind:checked={editingProvider.is_default} id="isDefault"
            class="rounded bg-white/[0.05] border border-white/[0.1] checked:bg-amber-400" />
          <label for="isDefault" class="text-xs text-slate-400">Set as default provider</label>
        </div>
        {#if editingProvider.id}
          <button onclick={() => testProviderConnection(editingProvider.id)} disabled={testingProvider}
            class="w-full px-3 py-2 rounded-xl text-xs font-medium bg-white/[0.05] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] transition cursor-pointer flex items-center justify-center gap-2">
            {#if testingProvider}
              <Loader2 size={12} class="animate-spin" /> Testing...
            {:else}
              <Globe size={12} /> Test Connection
            {/if}
          </button>
          {#if testResult}
            <div class="px-3 py-2 rounded-xl text-xs {testResult.connected ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-red-500/10 text-red-400 border border-red-500/20'}">
              {testResult.connected ? '✓ ' : '✕ '}{testResult.message || testResult.error || 'Unknown result'}
            </div>
          {/if}
        {/if}
      </div>
      <div class="flex gap-3 mt-6">
        <button onclick={() => { showProviderForm = false; editingProvider = null; }}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-white/[0.05] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] transition cursor-pointer">Cancel</button>
        <button onclick={saveProvider} disabled={saving}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-400 to-orange-400 text-[#0a1628] hover:shadow-lg hover:shadow-amber-400/20 transition-all disabled:opacity-50 cursor-pointer border-0">
          {saving ? 'Saving...' : editingProvider?.id ? 'Update' : 'Create'}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Config Add/Edit Modal -->
{#if showConfigForm}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onclick={() => { showConfigForm = false; editingConfig = null; }}>
    <div class="bg-[#0f1a2e] border border-white/[0.1] rounded-2xl shadow-2xl w-full max-w-lg p-6" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-base font-semibold text-white mb-1">{editingConfig?.id ? 'Edit Config' : 'Add Config'}</h3>
      <p class="text-xs text-slate-400 mb-5">Configure backup schedule and scope</p>
      <div class="space-y-4">
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Config Name</label>
          <input type="text" bind:value={editingConfig.name} placeholder="Daily Files"
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
        </div>
        <div class="col-span-2">
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="text-xs text-slate-400 mb-1.5 block">Scope Type</label>
              <select bind:value={editingConfig.scope_type}
                class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white focus:outline-none focus:border-amber-400/40">
                <option value="files">Files</option>
                <option value="database">Databases</option>
                <option value="both">Full (Files + DB)</option>
              </select>
            </div>
            <div></div>
          </div>
        </div>
        <div class="col-span-2">
          <label class="text-xs text-slate-400 mb-1.5 block">Schedule</label>
            <div class="grid grid-cols-2 gap-2 mb-2">
              {#each [
                { label: 'Every Hour', cron: '0 * * * *' },
                { label: 'Every 6 Hours', cron: '0 */6 * * *' },
                { label: 'Daily at', cron: 'daily' },
                { label: 'Weekly (Mon)', cron: '0 3 * * 1' },
                { label: 'Weekly (Sun)', cron: '0 3 * * 0' },
                { label: 'Monthly 1st', cron: '0 3 1 * *' },
                { label: 'Custom', cron: 'custom' },
              ] as preset}
                <button onclick={() => {
                  editingConfig._preset = preset.cron;
                  if (preset.cron === 'daily') {
                    editingConfig.cron_expr = `0 ${editingConfig._hour || 2} * * *`;
                  } else if (preset.cron !== 'custom') {
                    editingConfig.cron_expr = preset.cron;
                  }
                }}
                  class="px-2 py-1.5 rounded-lg text-[10px] font-medium transition-all duration-200 cursor-pointer border
                    {editingConfig._preset === preset.cron
                      ? 'bg-amber-500/20 border-amber-500/40 text-amber-300'
                      : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:border-white/[0.15]'}">
                  {preset.label}
                </button>
              {/each}
            </div>
            {#if editingConfig._preset === 'daily'}
              <div class="flex items-center gap-2 px-3 py-2 bg-white/[0.03] rounded-xl border border-white/[0.07]">
                <span class="text-xs text-slate-400">Every day at</span>
                <select bind:value={editingConfig._hour}
                  onchange={() => { editingConfig.cron_expr = `0 ${editingConfig._hour} * * *`; }}
                  class="px-2 py-1 rounded-lg bg-white/[0.05] border border-white/[0.1] text-xs text-white">
                  {#each Array.from({ length: 24 }, (_, i) => i) as h}
                    <option value={h}>{String(h).padStart(2,'0')}:00</option>
                  {/each}
                </select>
              </div>
            {/if}
            {#if editingConfig._preset === 'custom'}
              <div class="flex items-center gap-2">
                <input type="text" bind:value={editingConfig.cron_expr} placeholder="0 2 * * *"
                  class="flex-1 px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white font-mono placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
                <span class="text-[10px] text-slate-500">cron expression</span>
              </div>
            {/if}
            {#if editingConfig.cron_expr}
              <p class="text-[10px] text-slate-500 mt-1">
                Cron: <code class="text-amber-400">{editingConfig.cron_expr}</code>
              </p>
            {/if}
          </div>
        <!-- Site Selection -->
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Sites to Backup</label>
          {#if (editingConfig.scope_type === 'database' || editingConfig.scope_type === 'both') && editingConfig._autoDiscover}
            <div class="px-3 py-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-[10px] text-emerald-300">
              Auto-discovery active. All active sites and newly created sites included.
            </div>
          {:else}
          <div class="bg-white/[0.03] border border-white/[0.08] rounded-xl p-3 max-h-32 overflow-y-auto space-y-1.5">
            <label class="flex items-center gap-2 cursor-pointer hover:bg-white/[0.03] px-2 py-1 rounded-lg">
              <input type="checkbox" bind:checked={editingConfig._allSites}
                onchange={() => { editingConfig.site_ids = editingConfig._allSites ? allSites.map((s: any) => s.id) : []; }}
                class="rounded bg-white/[0.05] border border-white/[0.1] checked:bg-amber-400" />
              <span class="text-xs text-slate-300 font-medium">All Sites</span>
            </label>
            {#each allSites as s}
              <label class="flex items-center gap-2 cursor-pointer hover:bg-white/[0.03] px-2 py-1 rounded-lg">
                <input type="checkbox" checked={editingConfig.site_ids?.includes(s.id)}
                  onchange={() => {
                    if (editingConfig.site_ids?.includes(s.id)) {
                      editingConfig.site_ids = editingConfig.site_ids.filter((id: number) => id !== s.id);
                    } else {
                      editingConfig.site_ids = [...(editingConfig.site_ids || []), s.id];
                    }
                    editingConfig._allSites = editingConfig.site_ids?.length === allSites.length;
                  }}
                  class="rounded bg-white/[0.05] border border-white/[0.1] checked:bg-amber-400" />
                <span class="text-xs text-slate-300">{s.domain || s.name || `Site #${s.id}`}</span>
              </label>
            {/each}
          </div>
          {/if}
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Provider</label>
            <select bind:value={editingConfig.provider_id}
              class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white focus:outline-none focus:border-amber-400/40">
              <option value={undefined}>Local only</option>
              {#each providers as p}
                <option value={p.id}>{p.name}</option>
              {/each}
            </select>
          </div>
          <div>
            <label class="text-xs text-slate-400 mb-1.5 block">Retention (days)</label>
            <input type="number" bind:value={editingConfig.retention_days} placeholder="7" min="0"
              class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
          </div>
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Exclude Patterns (comma-separated)</label>
          <input type="text" bind:value={editingConfig.exclude_patterns_text} placeholder="node_modules, vendor, .git"
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white placeholder:text-slate-600 focus:outline-none focus:border-amber-400/40" />
        </div>
        <div class="flex items-center gap-2">
          <input type="checkbox" bind:checked={editingConfig.enabled} id="configEnabled"
            class="rounded bg-white/[0.05] border border-white/[0.1] checked:bg-amber-400" />
          <label for="configEnabled" class="text-xs text-slate-400">Schedule enabled</label>
        </div>
      </div>
      <div class="flex gap-3 mt-6">
        <button onclick={() => { showConfigForm = false; editingConfig = null; }}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-white/[0.05] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] transition cursor-pointer">Cancel</button>
        <button onclick={saveConfig} disabled={saving}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-400 to-orange-400 text-[#0a1628] hover:shadow-lg hover:shadow-amber-400/20 transition-all disabled:opacity-50 cursor-pointer border-0">
          {saving ? 'Saving...' : editingConfig?.id ? 'Update' : 'Create'}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Manual Backup Modal -->
{#if showManualBackup}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" onclick={() => showManualBackup = false} role="presentation">
    <div class="bg-[#0f1117] border border-white/[0.08] rounded-2xl p-6 w-full max-w-md shadow-2xl" onclick={(e) => e.stopPropagation()} role="dialog">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-sm font-semibold text-white">Create Manual Backup</h3>
        <button onclick={() => showManualBackup = false} class="text-slate-500 hover:text-white p-1 cursor-pointer border-0 bg-transparent">
          <X size={14} />
        </button>
      </div>
      <p class="text-xs text-slate-400 mb-5">Trigger an immediate backup for a specific site</p>
      <div class="space-y-4">
        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Select Site <span class="text-red-400">*</span></label>
          <select bind:value={manualSiteId}
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white focus:outline-none focus:border-amber-400/40">
            <option value={0}>Select a site...</option>
            {#each allSites as s}
              <option value={s.id}>{s.domain || s.name || `Site #${s.id}`}</option>
            {/each}
          </select>
        </div>

        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Backup Type</label>
          <div class="grid grid-cols-3 gap-2">
            {#each [
              { id: 'both', label: 'Full (Files+DB)' },
              { id: 'files', label: 'Files Only' },
              { id: 'database', label: 'DB Only' },
            ] as t}
              <button
                type="button"
                onclick={() => manualType = t.id as any}
                class="px-2 py-2 rounded-xl text-xs font-medium border transition-all cursor-pointer {manualType === t.id ? 'bg-amber-400/15 border-amber-400/30 text-amber-300' : 'bg-white/[0.03] border-white/[0.06] text-slate-400 hover:text-white'}">
                {t.label}
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label class="text-xs text-slate-400 mb-1.5 block">Storage Destination</label>
          <select bind:value={manualProviderId}
            class="w-full px-3 py-2 rounded-xl bg-white/[0.05] border border-white/[0.1] text-xs text-white focus:outline-none focus:border-amber-400/40">
            <option value={0}>Default Provider / Local</option>
            {#each providers as p}
              <option value={p.id}>{p.name} ({p.provider_type})</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="flex gap-3 mt-6">
        <button onclick={() => showManualBackup = false}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-white/[0.05] border border-white/[0.08] text-slate-300 hover:bg-white/[0.08] transition cursor-pointer">
          Cancel
        </button>
        <button onclick={triggerManualBackup} disabled={manualBusy || !manualSiteId}
          class="flex-1 px-4 py-2.5 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-400 to-orange-400 text-[#0a1628] hover:shadow-lg hover:shadow-amber-400/20 transition-all disabled:opacity-50 cursor-pointer border-0 font-semibold">
          {manualBusy ? 'Queueing...' : 'Start Backup'}
        </button>
      </div>
    </div>
  </div>
{/if}
