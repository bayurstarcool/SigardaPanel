<script lang="ts">
  import { page } from '$app/stores';
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    ArrowLeft, Globe, ShieldCheck, Database, Folder, Zap, Settings, Users, Package,
    RefreshCw, Lock, Loader2, ExternalLink, Trash2, Copy, Check, Pencil, Save,
    UserPlus, Mail, Code, Shield, Clock, ScrollText, Upload, Key,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  // Tab components
  import VhostTab from '$components/sites/VhostTab.svelte';
  import SecurityTab from '$components/sites/SecurityTab.svelte';
  import LogsTab from '$components/sites/LogsTab.svelte';
  import CronTab from '$components/sites/CronTab.svelte';
  import SshFtpTab from '$components/sites/SshFtpTab.svelte';
  import FileManagerTab from '$components/sites/FileManagerTab.svelte';
  import UsersTab from '$components/sites/UsersTab.svelte';
  import AppTab from '$components/sites/AppTab.svelte';
  import BackupTab from '$components/sites/BackupTab.svelte';
  import SslTab from '$components/sites/SslTab.svelte';
  import DatabaseTab from '$components/sites/DatabaseTab.svelte';
  import SettingsTab from '$components/sites/SettingsTab.svelte';

  const siteId = $derived(Number($page.params.id));
  let site = $state<any>(null);
  let loading = $state(true);
  let activeTab = $state<string>('info');

  // Deploy state
  let deployBusy = $state(false);

  // Git config state
  let gitEditing = $state(false);
  let gitRepo = $state('');
  let gitBranch = $state('main');
  let gitBusy = $state(false);

  // Transfer state
  let showTransfer = $state(false);
  let transferEmail = $state('');
  let transferBusy = $state(false);

  // Databases count (for stats)
  let dbsCount = $state(0);
  let dockerContainer = $state<any>(null);

  // Copy helper
  let copiedField = $state('');
  function copyText(text: string, field: string) { navigator.clipboard.writeText(text); copiedField = field; setTimeout(() => copiedField = '', 1500); }

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      site = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally { loading = false; }
    // Load db count for stats
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/databases`); dbsCount = (d?.databases || []).length; } catch {}
    // Load Docker container info for docker runtime sites
    if (site?.runtime === 'docker' && site?.server_id) {
      try {
        const containerName = `site-${site.name}`;
        const domainBase = site.domain?.split(".")[0] || "";
        const containers = await apiFetch(auth.token, 'GET', `servers/${site.server_id}/docker/containers`);
        const containerList = containers?.containers || [];
        // Try exact name match first, then partial match
        let container = containerList.find((c: any) => c.name === containerName);
        if (!container) container = containerList.find((c: any) => c.name?.includes(site.name) || c.name?.includes(domainBase) || c.name?.includes(`site-${site.name}`));
        if (container) {
          dockerContainer = container;
        }
      } catch {}
    }
  });

  async function triggerDeploy() {
    if (!confirm('Deploy this site now?')) return;
    deployBusy = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/deploy`); toasts.success(`Deploy queued — Job #${d?.job_id}`); }
    catch (e: any) { toasts.error(e.message); } finally { deployBusy = false; }
  }

  async function saveGitConfig() {
    gitBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/config`, { git_repo: gitRepo, git_branch: gitBranch || 'main' }); site = { ...site, git_repo: gitRepo, git_branch: gitBranch || 'main' }; gitEditing = false; toasts.success('Git config saved.'); }
    catch (e: any) { toasts.error(e.message || 'Failed'); } finally { gitBusy = false; }
  }
  function startEditGit() { gitRepo = site.git_repo || ''; gitBranch = site.git_branch || 'main'; gitEditing = true; }

  async function doTransfer() {
    if (!transferEmail) return; transferBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/transfer`, { new_owner_email: transferEmail }); toasts.success('Site transferred'); showTransfer = false; transferEmail = ''; }
    catch (e: any) { toasts.error(e.message); } finally { transferBusy = false; }
  }

  async function deleteSite() {
    if (!confirm(`DELETE site "${site?.domain}" permanently? This cannot be undone.`)) return;
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}`); window.location.href = '/sites'; }
    catch (e: any) { toasts.error(e.message); }
  }

  const allTabs = [
    { key: 'info', label: 'Info', icon: Globe },
    { key: 'vhost', label: 'Vhost', icon: Code },
    { key: 'ssl', label: 'SSL', icon: ShieldCheck },
    { key: 'security', label: 'Security', icon: Shield },
    { key: 'app', label: 'App', icon: Zap },
    { key: 'logs', label: 'Logs', icon: ScrollText },
    { key: 'cron', label: 'Cron', icon: Clock },
    { key: 'databases', label: 'Databases', icon: Database },
    { key: 'ssh', label: 'SSH', icon: Key },
    { key: 'files', label: 'Files', icon: Folder },
    { key: 'users', label: 'Users', icon: Users },
    { key: 'backups', label: 'Backups', icon: Package },
    { key: 'settings', label: 'Settings', icon: Settings },
  ] as const;

  const appRuntimes = ['node', 'go', 'golang', 'python', 'docker'];
  const tabs = $derived(
    !site ? allTabs.filter((t) => t.key !== 'app')
    : site.runtime === 'php' ? allTabs.filter((t) => t.key !== 'app')
    : appRuntimes.includes(site.runtime) ? allTabs
    : allTabs.filter((t) => t.key !== 'app')
  );
</script>

<DashboardLayout active="Sites">
  {#if loading}
    <div class="space-y-6">
      <div class="relative overflow-hidden bg-gradient-to-br from-sky-400/[0.07] via-indigo-500/[0.04] to-violet-500/[0.06] border border-white/[0.07] rounded-2xl shadow-xl p-5 lg:p-6">
        <div class="absolute top-0 right-0 w-48 h-48 bg-sky-400/[0.04] rounded-full blur-3xl"></div>
        <div class="relative flex items-center gap-4"><div class="w-12 h-12 rounded-2xl bg-white/[0.05] animate-pulse"></div><div class="space-y-2"><div class="h-6 w-48 bg-white/[0.05] rounded-lg animate-pulse"></div><div class="h-4 w-24 bg-white/[0.04] rounded-lg animate-pulse"></div></div></div>
      </div>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">{#each Array(4) as _}<div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-3.5"><div class="flex items-center gap-2 mb-1.5"><div class="w-6 h-6 rounded-lg bg-white/[0.04] animate-pulse"></div></div><div class="h-4 w-20 bg-white/[0.05] rounded animate-pulse"></div></div>{/each}</div>
      <div class="flex gap-1">{#each Array(6) as _}<div class="h-9 w-20 bg-white/[0.04] rounded-xl animate-pulse"></div>{/each}</div>
    </div>
  {:else if !site}
    <div class="flex flex-col items-center justify-center py-32">
      <div class="w-20 h-20 rounded-3xl bg-white/[0.03] border border-white/[0.07] flex items-center justify-center mb-6"><Globe size={36} class="text-white/[0.08]" /></div>
      <h2 class="text-lg font-semibold text-slate-300 mb-1">Site not found</h2>
      <p class="text-sm text-slate-500">This site may have been deleted or you lack access.</p>
    </div>
  {:else}
    <!-- Header -->
    <div class="relative overflow-hidden bg-gradient-to-br from-sky-400/[0.07] via-indigo-500/[0.04] to-violet-500/[0.06] border border-white/[0.07] rounded-2xl shadow-xl p-3 sm:p-5 lg:p-6 mb-4 sm:mb-6">
      <div class="absolute top-0 right-0 w-48 h-48 bg-sky-400/[0.04] rounded-full blur-3xl"></div>
      <div class="absolute bottom-0 left-0 w-32 h-32 bg-indigo-400/[0.04] rounded-full blur-3xl"></div>
      <div class="relative flex flex-col sm:flex-row sm:items-center justify-between gap-3 sm:gap-4">
        <div class="flex items-center gap-3 sm:gap-4 min-w-0">
          <div class="w-10 h-10 sm:w-12 sm:h-12 rounded-xl sm:rounded-2xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20 flex items-center justify-center shadow-lg ring-1 ring-white/[0.08] shrink-0"><Globe size={20} class="text-sky-400" /></div>
          <div class="min-w-0">
            <h1 class="text-lg sm:text-xl lg:text-2xl font-bold bg-gradient-to-r from-white via-sky-100 to-indigo-200 bg-clip-text text-transparent truncate">{site.domain}</h1>
            <div class="flex items-center gap-1.5 sm:gap-2 mt-1 sm:mt-1.5 flex-wrap">
              <StatusPill status={site.status} />
              <span class="text-[0.6rem] sm:text-[0.65rem] font-medium {site.runtime === 'docker' ? 'text-sky-400 bg-sky-400/10 border-sky-400/20' : 'text-slate-500 bg-white/[0.05] border-white/[0.06]'} px-1.5 sm:px-2 py-0.5 rounded-full border">{site.runtime}</span>
              {#if site.ssl_status && site.ssl_status !== 'none'}<span class="inline-flex items-center gap-1 text-[0.65rem] font-medium text-emerald-400/80 bg-emerald-400/[0.08] px-2 py-0.5 rounded-full border border-emerald-400/10"><Lock size={9} /> SSL {site.ssl_status}</span>{/if}
              {#if site.app_status}<span class="inline-flex items-center gap-1 text-[0.65rem] font-medium {site.app_status === 'running' ? 'text-emerald-400/80 bg-emerald-400/[0.08] border-emerald-400/10' : 'text-slate-500 bg-white/[0.05] border-white/[0.06]'} px-2 py-0.5 rounded-full border"><Zap size={9} /> App {site.app_status}</span>{/if}
              {#if site.status === 'pending'}
                <span class="inline-flex items-center gap-1 text-[0.65rem] font-medium text-amber-400/80 bg-amber-400/[0.08] px-2 py-0.5 rounded-full border border-amber-400/10">
                  ⚠ {!site.conf_file ? 'Nginx belum terdaftar' : !site.ssl_status || site.ssl_status === 'none' ? 'SSL belum aktif' : 'Provisioning incomplete'}
                </span>
              {/if}
            </div>
          </div>
        </div>
        <div class="flex items-center gap-1.5 sm:gap-2 flex-shrink-0">
          <a href="http://{site.domain}" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1.5 px-3 sm:px-4 py-1.5 sm:py-2 rounded-xl text-[0.7rem] sm:text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.25)] hover:shadow-[0_6px_20px_rgba(56,189,248,0.35)] hover:-translate-y-0.5 transition-all duration-200"><ExternalLink size={12} /> Visit</a>
          {#if hasRole(auth.user?.role, 'admin') && site.git_repo}
            <button onclick={triggerDeploy} disabled={deployBusy} class="inline-flex items-center gap-1.5 px-3 sm:px-4 py-1.5 sm:py-2 rounded-xl text-[0.7rem] sm:text-xs font-semibold bg-emerald-400/[0.1] text-emerald-400 border border-emerald-400/20 hover:bg-emerald-400/[0.15] hover:-translate-y-0.5 transition-all duration-200 disabled:opacity-40">{#if deployBusy}<Loader2 size={12} class="animate-spin" />{:else}<Upload size={12} />{/if} Git Pull</button>
          {/if}
          <button onclick={deleteSite} class="inline-flex items-center gap-1.5 px-3 sm:px-4 py-1.5 sm:py-2 rounded-xl text-[0.7rem] sm:text-xs font-medium text-red-400/50 hover:text-red-400 hover:bg-red-400/10 hover:border-red-400/20 border border-transparent transition-all duration-200"><Trash2 size={12} /> Delete</button>
        </div>
      </div>
    </div>

    <!-- Quick Stats -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 sm:gap-3 mb-4 sm:mb-6">
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-2.5 sm:p-3.5 hover:border-white/[0.1] transition-all">
        <div class="flex items-center gap-1.5 sm:gap-2 mb-1 sm:mb-1.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg {site.ssl_status && site.ssl_status !== 'none' ? 'bg-emerald-400/10 text-emerald-400' : 'bg-slate-400/10 text-slate-500'} flex items-center justify-center"><ShieldCheck size={12} /></div><span class="text-[0.65rem] text-slate-500 font-medium uppercase tracking-wider">SSL</span></div>
        <p class="text-sm font-semibold {site.ssl_status && site.ssl_status !== 'none' ? 'text-emerald-400' : 'text-slate-500'}">{site.ssl_status || 'none'}</p>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-2.5 sm:p-3.5 hover:border-white/[0.1] transition-all">
        <div class="flex items-center gap-1.5 sm:gap-2 mb-1 sm:mb-1.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 text-sky-400 flex items-center justify-center"><Zap size={12} /></div><span class="text-[0.65rem] text-slate-500 font-medium uppercase tracking-wider">Runtime</span></div>
        <p class="text-sm font-semibold text-slate-200">{site.runtime}</p>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-2.5 sm:p-3.5 hover:border-white/[0.1] transition-all">
        <div class="flex items-center gap-1.5 sm:gap-2 mb-1 sm:mb-1.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg {site.conf_file ? 'bg-emerald-400/10 text-emerald-400' : 'bg-amber-400/10 text-amber-400'} flex items-center justify-center"><Folder size={12} /></div><span class="text-[0.65rem] text-slate-500 font-medium uppercase tracking-wider">Nginx</span></div>
        <p class="text-sm font-semibold {site.conf_file ? 'text-emerald-400' : 'text-amber-400'}">{site.conf_file ? 'configured' : 'not configured'}</p>
        {#if site.conf_file}<p class="text-[0.6rem] text-slate-500 truncate mt-0.5" title={site.conf_file}>{site.conf_file.split('/').pop()}</p>{/if}
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-2.5 sm:p-3.5 hover:border-white/[0.1] transition-all">
        <div class="flex items-center gap-1.5 sm:gap-2 mb-1 sm:mb-1.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-violet-400/10 text-violet-400 flex items-center justify-center"><Database size={12} /></div><span class="text-[0.65rem] text-slate-500 font-medium uppercase tracking-wider">Databases</span></div>
        <p class="text-sm font-semibold text-slate-200">{dbsCount || '—'}</p>
      </div>
    </div>

    <!-- Tabs -->
    <div class="flex gap-1 mb-4 sm:mb-6 overflow-x-auto pb-1 scrollbar-hide -mx-1 px-1">
      {#each tabs as t}
        {@const Icon = t.icon}
        <button onclick={() => activeTab = t.key}
          class="inline-flex items-center gap-1 sm:gap-1.5 px-2.5 sm:px-3.5 py-1.5 sm:py-2 text-[0.65rem] sm:text-[0.7rem] font-medium rounded-xl transition-all duration-200 whitespace-nowrap {activeTab === t.key ? 'bg-gradient-to-r from-sky-400/12 to-indigo-400/12 text-sky-300 border border-sky-400/25 shadow-[0_0_20px_rgba(56,189,248,0.06)]' : 'text-slate-500 hover:text-slate-300 hover:bg-white/[0.04] border border-transparent hover:border-white/[0.06]'}">
          <Icon size={12} /> {t.label}
        </button>
      {/each}
    </div>

    <!-- ═══════ Tab: Info ═══════ -->
    {#if activeTab === 'info'}
      <div class="grid grid-cols-1 lg:grid-cols-5 gap-3 sm:gap-4 lg:gap-5 mb-4 sm:mb-5">
        <div class="lg:col-span-3 bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
          <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Globe size={12} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Site Details</span></div>
          <div class="p-3.5 sm:p-5">
            <div class="space-y-0">
              <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Domain</span><span class="text-xs sm:text-sm text-slate-200 font-mono flex items-center gap-1 min-w-0"><span class="truncate">{site.domain}</span><button onclick={() => copyText(site.domain, 'domain')} class="text-slate-500 hover:text-slate-300 transition shrink-0">{#if copiedField === 'domain'}<Check size={11} class="text-emerald-400" />{:else}<Copy size={11} />{/if}</button></span></div>
              <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">Runtime</span><span class="text-xs sm:text-sm text-slate-200">{site.runtime}</span></div>
              <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Root Path</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono flex items-center gap-1 min-w-0"><span class="truncate max-w-[140px] sm:max-w-[200px]" title={site.root_path}>{site.root_path}</span><button onclick={() => copyText(site.root_path, 'root')} class="text-slate-500 hover:text-slate-300 transition shrink-0">{#if copiedField === 'root'}<Check size={11} class="text-emerald-400" />{:else}<Copy size={11} />{/if}</button></span></div>
              {#if site.system_user}<div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">System User</span><span class="text-xs sm:text-sm text-slate-200 font-mono">{site.system_user}</span></div>{/if}
              {#if site.conf_file}<div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Nginx Config</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[140px] sm:max-w-[200px]" title={site.conf_file}>{site.conf_file}</span></div>{/if}
              {#if site.proxy_pass_url}<div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Proxy Pass</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[140px] sm:max-w-[200px]">{site.proxy_pass_url}</span></div>{/if}
              {#if site.runtime === 'docker' && dockerContainer}
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Container</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[140px] sm:max-w-[200px]">{dockerContainer.name} ({dockerContainer.id?.slice(0,12)})</span></div>
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Image</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[140px] sm:max-w-[200px]">{dockerContainer.image}</span></div>
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">State</span>
                  <span class="text-[0.65rem] font-medium {dockerContainer.state === 'running' ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20' : 'text-slate-500 bg-white/5 border-white/10'} px-2 py-0.5 rounded-full border">{dockerContainer.state}</span>
                </div>
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Ports</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[140px] sm:max-w-[200px]">{dockerContainer.ports || '—'}</span></div>
              {/if}
              <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">Managed By</span><span class="text-xs sm:text-sm text-slate-200">{site.managed_by || 'sigardapanel'}</span></div>
              <div class="flex justify-between py-2 sm:py-2.5"><span class="text-xs sm:text-sm text-slate-500">Created</span><span class="text-sm text-slate-200">{fmtDate(site.created_at)}</span></div>
            </div>
          </div>
        </div>
        <div class="lg:col-span-2 flex flex-col gap-3 sm:gap-4 lg:gap-5">
          <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
            <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg {site.ssl_status && site.ssl_status !== 'none' ? 'bg-emerald-400/10 text-emerald-400' : 'bg-slate-400/10 text-slate-500'} flex items-center justify-center"><ShieldCheck size={12} /></div><span class="text-sm font-semibold text-slate-200">SSL Status</span></div>
            <div class="p-3.5 sm:p-5">
              {#if site.ssl_status && site.ssl_status !== 'none'}
                <div class="space-y-0">
                  <div class="flex justify-between py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">Status</span><StatusPill status={site.ssl_status} /></div>
                  <div class="flex justify-between py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">Issuer</span><span class="text-xs sm:text-sm text-slate-200 truncate max-w-[120px]">{site.ssl_issuer || '—'}</span></div>
                  <div class="flex justify-between py-2.5"><span class="text-xs sm:text-sm text-slate-500">Expires</span><span class="text-xs sm:text-sm text-slate-200">{site.ssl_expires_at ? fmtDate(site.ssl_expires_at) : '—'}</span></div>
                </div>
              {:else}
                <div class="flex flex-col items-center justify-center py-8"><div class="w-14 h-14 rounded-2xl bg-white/[0.03] border border-white/[0.06] flex items-center justify-center mb-3"><ShieldCheck size={22} class="text-white/[0.08]" /></div><p class="text-sm font-medium text-slate-400 mb-0.5">No SSL configured</p><p class="text-xs text-slate-600">Issue a certificate from the SSL tab</p></div>
              {/if}
            </div>
          </div>
          {#if hasRole(auth.user?.role, 'admin')}
            <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
              <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-violet-400/10 flex items-center justify-center"><Users size={12} class="text-violet-400" /></div><span class="text-sm font-semibold text-slate-200">Transfer Ownership</span></div>
              <div class="p-3.5 sm:p-5">
                <p class="text-[0.7rem] sm:text-xs text-slate-500 mb-2.5 sm:mb-3">Transfer this site to another user.</p>
                {#if !showTransfer}<button onclick={() => showTransfer = true} class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-[0.7rem] sm:text-xs font-medium bg-sky-400/[0.08] text-sky-400 border border-sky-400/15 hover:bg-sky-400/[0.12] hover:border-sky-400/25 transition-all duration-200"><UserPlus size={12} /> Transfer</button>{/if}
                {#if showTransfer}
                  <div class="space-y-3">
                    <div class="relative"><Mail size={13} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" /><input bind:value={transferEmail} type="email" placeholder="newowner@email.com" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl pl-9 pr-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" /></div>
                    <div class="flex gap-2">
                      <button onclick={doTransfer} disabled={transferBusy || !transferEmail} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{transferBusy ? 'Transferring...' : 'Transfer'}</button>
                      <button onclick={() => { showTransfer = false; transferEmail = ''; }} class="px-3 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      </div>
      <!-- Git Deploy -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
        <div class="flex items-center justify-between px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
          <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Folder size={12} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Git Deploy</span></div>
          {#if !gitEditing}<button onclick={startEditGit} class="inline-flex items-center gap-1 px-3 py-1.5 rounded-xl text-xs font-medium text-sky-400 hover:bg-sky-400/10 transition cursor-pointer"><Pencil size={12} /> {site.git_repo ? 'Edit' : 'Configure'}</button>{/if}
        </div>
        <div class="p-3.5 sm:p-5">
          {#if gitEditing}
            <div class="space-y-3 sm:space-y-4">
              <div><label class="block text-[0.7rem] sm:text-xs text-slate-500 font-medium mb-1 sm:mb-1.5">Repository URL</label><input bind:value={gitRepo} type="url" class="w-full bg-white/[0.04] border border-white/[0.08] rounded-xl px-3.5 sm:px-4 py-2 sm:py-2.5 text-[0.8rem] sm:text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all placeholder:text-slate-600" placeholder="https://github.com/user/repo.git" /></div>
              <div><label class="block text-[0.7rem] sm:text-xs text-slate-500 font-medium mb-1 sm:mb-1.5">Branch</label><input bind:value={gitBranch} type="text" class="w-full bg-white/[0.04] border border-white/[0.08] rounded-xl px-3.5 sm:px-4 py-2 sm:py-2.5 text-[0.8rem] sm:text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all placeholder:text-slate-600" placeholder="main" /></div>
              <div class="flex items-center gap-2">
                <button onclick={saveGitConfig} disabled={gitBusy || !gitRepo} class="inline-flex items-center gap-1.5 px-3.5 sm:px-4 py-1.5 sm:py-2 rounded-xl text-[0.8rem] sm:text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{#if gitBusy}<Loader2 size={12} class="animate-spin" />{:else}<Save size={12} />{/if} Save</button>
                <button onclick={() => { gitEditing = false; }} class="px-2.5 sm:px-3 py-1.5 sm:py-2 rounded-xl text-[0.8rem] sm:text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
              </div>
            </div>
          {:else}
            {#if site.git_repo}
              <div class="space-y-0">
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04] gap-2"><span class="text-xs sm:text-sm text-slate-500 shrink-0">Repository</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono truncate max-w-[60%] text-right">{site.git_repo}</span></div>
                <div class="flex justify-between py-2 sm:py-2.5 border-b border-white/[0.04]"><span class="text-xs sm:text-sm text-slate-500">Branch</span><span class="text-xs sm:text-sm text-slate-200">{site.git_branch || 'main'}</span></div>
                <div class="flex justify-between py-2 sm:py-2.5"><span class="text-xs sm:text-sm text-slate-500">Last Commit</span><span class="text-[0.7rem] sm:text-xs text-slate-200 font-mono">{site.git_commit?.slice(0, 8) || '—'}</span></div>
              </div>
            {:else}
              <p class="text-[0.8rem] sm:text-sm text-slate-500">No git repository configured. Click Configure to set up git deploy.</p>
            {/if}
          {/if}
        </div>
      </div>
    {/if}

    <!-- ═══════ Inline Tabs ═══════ -->
    {#if activeTab === 'vhost'}<VhostTab {siteId} {site} onTriggerDeploy={triggerDeploy} />{/if}
    {#if activeTab === 'security'}<SecurityTab {siteId} />{/if}
    {#if activeTab === 'logs'}<LogsTab {siteId} />{/if}
    {#if activeTab === 'cron'}<CronTab {siteId} />{/if}
    {#if activeTab === 'ssh'}<SshFtpTab {siteId} {site} />{/if}
    {#if activeTab === 'files'}<FileManagerTab {siteId} />{/if}
    {#if activeTab === 'users'}<UsersTab {siteId} />{/if}
    {#if activeTab === 'app'}<AppTab {siteId} />{/if}
    {#if activeTab === 'backups'}<BackupTab {siteId} />{/if}
    {#if activeTab === 'ssl'}<SslTab {siteId} />{/if}
    {#if activeTab === 'databases'}<DatabaseTab {siteId} />{/if}
    {#if activeTab === 'settings'}<SettingsTab {siteId} />{/if}
  {/if}
</DashboardLayout>
