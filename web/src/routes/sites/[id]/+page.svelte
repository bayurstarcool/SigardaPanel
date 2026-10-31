<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    Globe, ShieldCheck, Database, Folder, Zap, Settings, Users, Package,
    Lock, LoaderCircle, ExternalLink, Trash2, Copy, Check, Pencil, Save,
    UserPlus, Mail, Code, Shield, Clock, ScrollText, Upload, Key, GitBranch, TriangleAlert, Server,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  // Light tabs load with the page. Heavy ones (Files/Vhost pull CodeMirror,
  // SSH pulls xterm) are imported the first time their tab is opened.
  import SecurityTab from '$components/sites/SecurityTab.svelte';
  import LogsTab from '$components/sites/LogsTab.svelte';
  import CronTab from '$components/sites/CronTab.svelte';
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

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));
  const hasSSL = $derived(!!site?.ssl_status && site.ssl_status !== 'none');

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
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}`); goto('/sites'); }
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

  const pendingReason = $derived(
    !site ? '' : !site.conf_file ? 'Nginx belum terdaftar' : !hasSSL ? 'SSL belum aktif' : 'Provisioning incomplete',
  );
</script>

{#snippet row(label: string, value: string, opts: { mono?: boolean; copy?: string } = {})}
  <div class="flex items-center justify-between gap-4 py-2.5">
    <dt class="shrink-0 text-[13px] text-muted">{label}</dt>
    <dd class="flex min-w-0 items-center gap-1 text-right {opts.mono ? 'font-mono text-[13px]' : 'text-sm'} text-fg">
      <span class="truncate" title={value}>{value}</span>
      {#if opts.copy}
        <button onclick={() => copyText(value, opts.copy!)} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy {label.toLowerCase()}">
          {#if copiedField === opts.copy}<Check size={12} class="text-success" />{:else}<Copy size={12} />{/if}
        </button>
      {/if}
    </dd>
  </div>
{/snippet}

{#snippet tabLoading()}
  <div class="card space-y-3 p-5">
    <div class="skeleton h-5 w-40"></div>
    {#each Array(6) as _, i (i)}<div class="skeleton h-8"></div>{/each}
  </div>
{/snippet}

{#snippet tabError()}
  <EmptyState compact icon={TriangleAlert} title="This tab failed to load" description="Check your connection and reload the page." />
{/snippet}

<DashboardLayout active="Sites">
  {#if loading}
    <div class="mb-6 space-y-2">
      <div class="skeleton h-4 w-16"></div>
      <div class="skeleton h-8 w-64"></div>
      <div class="skeleton h-4 w-40"></div>
    </div>
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      {#each Array(4) as _, i (i)}<div class="skeleton h-[88px] rounded-xl"></div>{/each}
    </div>
    <div class="mb-6 flex gap-2">{#each Array(6) as _, i (i)}<div class="skeleton h-8 w-20"></div>{/each}</div>
    <div class="skeleton h-64 rounded-xl"></div>
  {:else if !site}
    <PageHeader title="Site" back="/sites" backLabel="Sites" />
    <div class="card">
      <EmptyState icon={Globe} title="Site not found" description="This site may have been deleted or you lack access." />
    </div>
  {:else}
    <PageHeader title={site.domain} back="/sites" backLabel="Sites">
      {#snippet meta()}
        <StatusPill status={site.status} />
        <span class="badge {site.runtime === 'docker' ? 'badge-info' : ''}">{site.runtime}</span>
        {#if hasSSL}<span class="badge badge-success"><Lock size={11} /> SSL {site.ssl_status}</span>{/if}
        {#if site.app_status}<span class="badge {site.app_status === 'running' ? 'badge-success' : ''}"><Zap size={11} /> App {site.app_status}</span>{/if}
        {#if site.status === 'pending'}
          <span class="badge badge-warning"><TriangleAlert size={11} /> {pendingReason}</span>
        {/if}
      {/snippet}
      {#snippet actions()}
        <a href="http://{site.domain}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary btn-sm"><ExternalLink size={14} /> Visit</a>
        {#if isAdmin && site.git_repo}
          <button onclick={triggerDeploy} disabled={deployBusy} class="btn btn-primary btn-sm">
            {#if deployBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Upload size={14} />{/if} Git pull
          </button>
        {/if}
        <button onclick={deleteSite} class="btn btn-danger-ghost btn-sm"><Trash2 size={14} /> Delete</button>
      {/snippet}
    </PageHeader>

    <!-- Quick stats -->
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="SSL" value={site.ssl_status || 'none'} icon={ShieldCheck} tone={hasSSL ? 'success' : ''} />
      <StatCard label="Runtime" value={site.runtime} icon={Zap} tone="accent" />
      <StatCard label="Nginx" value={site.conf_file ? 'Configured' : 'Not configured'} icon={Server} tone={site.conf_file ? 'success' : 'warning'} hint={site.conf_file ? site.conf_file.split('/').pop() : ''} />
      <StatCard label="Databases" value={dbsCount || '—'} icon={Database} />
    </div>

    <!-- Tabs -->
    <div class="tabs mb-6" role="tablist">
      {#each tabs as t (t.key)}
        <button role="tab" aria-selected={activeTab === t.key} onclick={() => (activeTab = t.key)} class="tab {activeTab === t.key ? 'tab-active' : ''}">
          <t.icon size={14} /> {t.label}
        </button>
      {/each}
    </div>

    {#if activeTab === 'info'}
      <div class="mb-4 grid grid-cols-1 gap-4 lg:grid-cols-5">
        <div class="card lg:col-span-3">
          <div class="card-header"><h2 class="card-title">Site details</h2></div>
          <dl class="divide-y divide-line px-5">
            {@render row('Domain', site.domain, { mono: true, copy: 'domain' })}
            {@render row('Runtime', site.runtime)}
            {@render row('Root path', site.root_path, { mono: true, copy: 'root' })}
            {#if site.system_user}{@render row('System user', site.system_user, { mono: true })}{/if}
            {#if site.conf_file}{@render row('Nginx config', site.conf_file, { mono: true })}{/if}
            {#if site.proxy_pass_url}{@render row('Proxy pass', site.proxy_pass_url, { mono: true })}{/if}
            {#if site.runtime === 'docker' && dockerContainer}
              {@render row('Container', `${dockerContainer.name} (${dockerContainer.id?.slice(0, 12)})`, { mono: true })}
              {@render row('Image', dockerContainer.image, { mono: true })}
              <div class="flex items-center justify-between gap-4 py-2.5">
                <dt class="text-[13px] text-muted">State</dt>
                <dd><span class="badge {dockerContainer.state === 'running' ? 'badge-success' : ''}"><span class="dot"></span>{dockerContainer.state}</span></dd>
              </div>
              {@render row('Ports', dockerContainer.ports || '—', { mono: true })}
            {/if}
            {@render row('Managed by', site.managed_by || 'sigardapanel')}
            {@render row('Created', fmtDate(site.created_at))}
          </dl>
        </div>

        <div class="flex flex-col gap-4 lg:col-span-2">
          <div class="card">
            <div class="card-header">
              <h2 class="card-title">SSL status</h2>
              <ShieldCheck size={16} class={hasSSL ? 'text-success' : 'text-subtle'} />
            </div>
            {#if hasSSL}
              <dl class="divide-y divide-line px-5">
                <div class="flex items-center justify-between gap-4 py-2.5">
                  <dt class="text-[13px] text-muted">Status</dt>
                  <dd><StatusPill status={site.ssl_status} /></dd>
                </div>
                {@render row('Issuer', site.ssl_issuer || '—')}
                {@render row('Expires', site.ssl_expires_at ? fmtDate(site.ssl_expires_at) : '—')}
              </dl>
            {:else}
              <EmptyState compact icon={ShieldCheck} title="No SSL configured" description="Issue a certificate from the SSL tab." actionLabel="Open SSL tab" onaction={() => (activeTab = 'ssl')} />
            {/if}
          </div>

          {#if isAdmin}
            <div class="card">
              <div class="card-header">
                <h2 class="card-title">Transfer ownership</h2>
                <Users size={16} class="text-subtle" />
              </div>
              <div class="card-body">
                <p class="mb-3 text-[13px] text-muted">Transfer this site to another user.</p>
                {#if !showTransfer}
                  <button onclick={() => (showTransfer = true)} class="btn btn-secondary btn-sm"><UserPlus size={14} /> Transfer</button>
                {:else}
                  <div class="space-y-3">
                    <div class="relative">
                      <label for="transfer-email" class="sr-only">New owner email</label>
                      <Mail size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
                      <input id="transfer-email" bind:value={transferEmail} type="email" placeholder="newowner@email.com" class="input pl-9" />
                    </div>
                    <div class="flex gap-2">
                      <button onclick={doTransfer} disabled={transferBusy || !transferEmail} class="btn btn-primary btn-sm">{transferBusy ? 'Transferring…' : 'Transfer'}</button>
                      <button onclick={() => { showTransfer = false; transferEmail = ''; }} class="btn btn-ghost btn-sm">Cancel</button>
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      </div>

      <!-- Git deploy -->
      <div class="card">
        <div class="card-header">
          <div class="flex items-center gap-2">
            <GitBranch size={16} class="text-subtle" />
            <h2 class="card-title">Git deploy</h2>
          </div>
          {#if !gitEditing}
            <button onclick={startEditGit} class="btn btn-ghost btn-sm"><Pencil size={14} /> {site.git_repo ? 'Edit' : 'Configure'}</button>
          {/if}
        </div>
        {#if gitEditing}
          <div class="card-body space-y-4">
            <div>
              <label for="git-repo" class="label">Repository URL</label>
              <input id="git-repo" bind:value={gitRepo} type="url" class="input font-mono text-[13px]" placeholder="https://github.com/user/repo.git" />
            </div>
            <div>
              <label for="git-branch" class="label">Branch</label>
              <input id="git-branch" bind:value={gitBranch} type="text" class="input font-mono text-[13px]" placeholder="main" />
            </div>
            <div class="flex items-center gap-2">
              <button onclick={saveGitConfig} disabled={gitBusy || !gitRepo} class="btn btn-primary btn-sm">
                {#if gitBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save
              </button>
              <button onclick={() => { gitEditing = false; }} class="btn btn-ghost btn-sm">Cancel</button>
            </div>
          </div>
        {:else if site.git_repo}
          <dl class="divide-y divide-line px-5">
            {@render row('Repository', site.git_repo, { mono: true })}
            {@render row('Branch', site.git_branch || 'main', { mono: true })}
            {@render row('Last commit', site.git_commit?.slice(0, 8) || '—', { mono: true })}
          </dl>
        {:else}
          <p class="card-body text-[13px] text-muted">No git repository configured. Click Configure to set up git deploy.</p>
        {/if}
      </div>
    {:else if activeTab === 'vhost'}
      {#await import('$components/sites/VhostTab.svelte')}
        {@render tabLoading()}
      {:then m}
        <m.default {siteId} {site} onTriggerDeploy={triggerDeploy} />
      {:catch}
        {@render tabError()}
      {/await}
    {:else if activeTab === 'ssh'}
      {#await import('$components/sites/SshFtpTab.svelte')}
        {@render tabLoading()}
      {:then m}
        <m.default {siteId} {site} />
      {:catch}
        {@render tabError()}
      {/await}
    {:else if activeTab === 'files'}
      {#await import('$components/sites/FileManagerTab.svelte')}
        {@render tabLoading()}
      {:then m}
        <m.default {siteId} />
      {:catch}
        {@render tabError()}
      {/await}
    {:else if activeTab === 'security'}<SecurityTab {siteId} />
    {:else if activeTab === 'logs'}<LogsTab {siteId} />
    {:else if activeTab === 'cron'}<CronTab {siteId} />
    {:else if activeTab === 'users'}<UsersTab {siteId} />
    {:else if activeTab === 'app'}<AppTab {siteId} />
    {:else if activeTab === 'backups'}<BackupTab {siteId} />
    {:else if activeTab === 'ssl'}<SslTab {siteId} />
    {:else if activeTab === 'databases'}<DatabaseTab {siteId} />
    {:else if activeTab === 'settings'}<SettingsTab {siteId} />
    {/if}
  {/if}
</DashboardLayout>
