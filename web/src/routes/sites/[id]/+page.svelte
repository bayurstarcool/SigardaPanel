<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth, hasRole, hasPermission } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate, fmtDaysLeft, fmtBytesFull } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import Modal from '$components/Modal.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import LoadError from '$components/LoadError.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    Globe, ShieldCheck, Database, Folder, Zap, Settings, Users, Package,
    Lock, LoaderCircle, ExternalLink, Trash2, Pause, Play, Copy, Check, Pencil, Save,
    UserPlus, Mail, Code, Shield, Clock, ScrollText, Upload, Key, GitBranch, TriangleAlert, Server,
    RefreshCw,
  } from 'lucide-svelte';
  import { onMount, untrack } from 'svelte';
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
  import ProvisioningSteps from '$components/sites/ProvisioningSteps.svelte';
  import DatabaseTab from '$components/sites/DatabaseTab.svelte';
  import SettingsTab from '$components/sites/SettingsTab.svelte';

  const siteId = $derived(Number($page.params.id));
  let site = $state<any>(null);
  let loading = $state(true);
  let activeTab = $state<string>('info');
  // The open tab lives in ?tab= so a reload or a shared link keeps it.
  function setTab(key: string) {
    activeTab = key;
    const url = new URL(window.location.href);
    if (key === 'info') url.searchParams.delete('tab'); else url.searchParams.set('tab', key);
    history.replaceState(history.state, '', url);
  }

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
  let dbNames = $state<string[]>([]);
  // Disk usage, measured by du on the site's server: null = loading,
  // '' = could not be measured.
  let diskUsed = $state<number | null | ''>(null);
  async function loadDiskUsage() {
    const id = siteId;
    diskUsed = null;
    try {
      const d = await apiFetch(auth.token, 'GET', `sites/${id}/usage`);
      if (id === siteId) diskUsed = typeof d?.disk_used_bytes === 'number' ? d.disk_used_bytes : '';
    } catch { if (id === siteId) diskUsed = ''; }
  }
  let dockerContainer = $state<any>(null);

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));
  const canSuspend = $derived(hasPermission('site.suspend'));
  const canManageUsers = $derived(hasPermission('user.manage'));
  // What the caller may do on this site, as the API decides it (GET /sites/:id).
  const canWrite = $derived(!!site?.can_write);
  const canDelete = $derived(!!site?.can_delete);
  const canTransfer = $derived(!!site?.can_transfer);
  const noRoot = $derived(site?.runtime === 'proxy'); // a reverse proxy serves no files of its own
  const noGit = $derived(site?.runtime === 'proxy' || site?.runtime === 'docker');
  const hasSSL = $derived(!!site?.ssl_status && site.ssl_status !== 'none');

  // Copy helper
  let copiedField = $state('');
  function copyText(text: string, field: string) { navigator.clipboard.writeText(text); copiedField = field; setTimeout(() => copiedField = '', 1500); }

  let loadError = $state('');

  async function loadSite() {
    loading = true;
    loadError = '';
    try {
      site = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      // 404/403 render as "not found"; anything else is a load failure worth retrying.
      if (!/not found|forbidden|access/i.test(e.message || '')) loadError = e.message || 'Failed to load site';
    } finally { loading = false; }
  }

  // The page component is reused when navigating from one site to another
  // (e.g. global search): reload everything whenever the id changes, so the
  // header, tabs and actions never belong to different sites.
  let loadedFor = 0;
  $effect(() => {
    const id = siteId;
    if (id && id !== loadedFor) {
      loadedFor = id;
      untrack(() => { void initSite(); });
    }
  });

  // A new site is "pending" until its site.create job has run: refresh it
  // quietly (no loading state) so the header and Nginx card update without
  // a manual reload. Stops once it leaves pending or after ~5 minutes.
  $effect(() => {
    if (site?.status !== 'pending') return;
    const id = siteId;
    let tries = 0;
    const timer = setInterval(async () => {
      if (++tries > 75 || id !== siteId) { clearInterval(timer); return; }
      try {
        const fresh = await apiFetch(auth.token, 'GET', `sites/${id}`);
        if (id === siteId && fresh && fresh.status !== 'pending') site = fresh;
      } catch {}
    }, 4000);
    return () => clearInterval(timer);
  });

  async function initSite() {
    if (!auth.user) { window.location.href = '/login'; return; }
    site = null; loading = true; activeTab = new URL(window.location.href).searchParams.get('tab') || 'info'; dockerContainer = null; dbsCount = 0; dbNames = [];
    await loadSite();
    if (site) { void loadDiskUsage(); void loadDNS(); }
    // Load db count for stats
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/databases`); dbNames = (d?.databases || []).map((x: any) => x.name); dbsCount = dbNames.length; } catch {}
    // Load Docker container info for docker runtime sites
    if (site?.runtime === 'docker' && site?.server_id) {
      try {
        // sp-site-<id>; older sites used site-<name>. Exact names only:
        // a partial match could show (and act on) another site's container.
        const names = [`sp-site-${site.id}`, `site-${String(site.name).replace(/ /g, '-')}`];
        const containers = await apiFetch(auth.token, 'GET', `servers/${site.server_id}/docker/containers`);
        const containerList = containers?.containers || [];
        const container = names.map((n) => containerList.find((c: any) => String(c.name || '').replace(/^\//, '') === n)).find(Boolean);
        if (container) {
          dockerContainer = container;
        }
      } catch {}
    }
  }

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
  // Deploy webhook: URL + secret for GitHub/Gitea/GitLab, shown on demand.
  let webhook = $state<{ url: string; secret: string } | null>(null);
  let webhookBusy = $state(false);
  async function loadWebhook(rotate = false) {
    if (rotate && !confirm('Rotate the webhook secret? The old secret stops working; update it in your git host.')) return;
    webhookBusy = true;
    try {
      const d = await apiFetch(auth.token, rotate ? 'POST' : 'GET', rotate ? `sites/${siteId}/webhook/rotate` : `sites/${siteId}/webhook`);
      webhook = { url: `${window.location.origin}${d.path}`, secret: d.secret };
      if (rotate) toasts.success('Webhook secret rotated');
    } catch (e: any) { toasts.error(e.message || 'Failed'); } finally { webhookBusy = false; }
  }
  function startEditGit() { gitRepo = site.git_repo || ''; gitBranch = site.git_branch || 'main'; gitEditing = true; }

  async function doTransfer() {
    if (!transferEmail) return; transferBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/transfer`, { new_owner_email: transferEmail }); toasts.success('Transfer queued: the files move on the server, then ownership changes'); showTransfer = false; transferEmail = ''; }
    catch (e: any) { toasts.error(e.message); } finally { transferBusy = false; }
  }

  // Suspend / resume (admin only): vhost + SSL renewal are disabled, nothing is deleted.
  let toggleBusy = $state(false);
  async function toggleSuspend() {
    const suspending = site.status !== 'suspended';
    const msg = suspending
      ? `Suspend "${site.domain}"? The site goes offline (nginx vhost and SSL auto-renew are disabled). Files, databases and certificates are kept.`
      : `Resume "${site.domain}"? The vhost and SSL auto-renew are re-enabled.`;
    if (!confirm(msg)) return;
    toggleBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/${suspending ? 'suspend' : 'resume'}`);
      toasts.success(`${suspending ? 'Suspend' : 'Resume'} queued — Job #${d?.job_id}`);
      // Poll briefly so the status pill reflects the result without a reload.
      for (let i = 0; i < 15; i++) {
        await new Promise((r) => setTimeout(r, 2000));
        const fresh = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
        if (fresh && fresh.status !== site.status) { site = fresh; break; }
      }
    } catch (e: any) { toasts.error(e.message); } finally { toggleBusy = false; }
  }

  // Cloudflare DNS for the site (with the viewer's Cloudflare account).
  let dns = $state<any>(null);
  let dnsBusy = $state(false);
  let showDnsReplace = $state(false);
  async function loadDNS() {
    try { dns = await apiFetch(auth.token, 'GET', `sites/${siteId}/dns`); } catch { dns = null; }
  }
  async function syncDNS(overwrite = false) {
    dnsBusy = true;
    try {
      const r = await apiFetch(auth.token, 'POST', `sites/${siteId}/dns`, { overwrite });
      dns = r;
      if (r?.status === 'conflict') toasts.warning(r.message); else toasts.success(r?.message || 'DNS updated');
      showDnsReplace = false;
    } catch (e: any) { toasts.error(e.message); await loadDNS(); } finally { dnsBusy = false; }
  }
  const dnsTone = $derived(
    !dns ? '' : ['managed', 'exists', 'created', 'updated', 'replaced'].includes(dns.status) ? 'success' : dns.status === 'conflict' ? 'warning' : dns.status === 'error' ? 'danger' : '',
  );
  const dnsLabel = $derived(
    !dns ? '' : ({ managed: 'Managed', exists: 'Points here', created: 'Created', updated: 'Updated', replaced: 'Replaced', conflict: 'Conflict', missing: 'Not set', error: 'Problem', skipped: 'Not connected' } as Record<string, string>)[dns.status] || dns.status,
  );

  // Delete dialog: everything that belongs to the site goes; its files only
  // when asked (unchecked by default, so a mistaken delete keeps the data).
  let showDelete = $state(false);
  let deleteFiles = $state(false);
  let deleteConfirm = $state('');
  let deleteBusy = $state(false);
  function openDelete() { deleteFiles = false; deleteConfirm = ''; showDelete = true; }
  async function deleteSite() {
    if (!site || deleteConfirm.trim().toLowerCase() !== site.domain.toLowerCase()) return;
    deleteBusy = true;
    try {
      await apiFetch(auth.token, 'DELETE', `sites/${siteId}?delete_files=${deleteFiles}`);
      toasts.success(deleteFiles ? `${site.domain} and its files are being deleted` : `${site.domain} deleted; its files were kept`);
      showDelete = false;
      goto('/sites');
    } catch (e: any) { toasts.error(e.message); } finally { deleteBusy = false; }
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
  // Tabs that fit the site's runtime and the caller's permission:
  // App only for app runtimes; no Databases, Cron, Files or SSH for a reverse
  // proxy (it has no files or code of its own); Users only for admins (the
  // assignment API is admin-only).
  const tabs = $derived(
    allTabs.filter((t) => {
      if (t.key === 'app') return !!site && appRuntimes.includes(site.runtime);
      if (noRoot && ['databases', 'cron', 'files', 'ssh'].includes(t.key)) return false;
      if (t.key === 'users') return canManageUsers;
      return true;
    }),
  );
  // A tab from the URL that does not exist for this site falls back to Info.
  $effect(() => { if (site && !tabs.some((t) => t.key === activeTab)) activeTab = 'info'; });

  const pendingReason = $derived(
    !site ? '' : !site.conf_file ? 'Nginx vhost not created yet' : !hasSSL ? 'SSL not active yet' : 'Provisioning incomplete',
  );
</script>

{#snippet row(label: string, value: string, opts: { mono?: boolean; copy?: string } = {})}
  <div class="flex items-center justify-between gap-4 py-2.5">
    <dt class="shrink-0 text-[13px] text-muted">{label}</dt>
    <dd class="flex min-w-0 items-center gap-1 text-right {opts.mono ? 'font-mono text-[12.5px]' : 'text-[13px]'} text-fg">
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
{#key siteId}
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
  {:else if !site && loadError}
    <PageHeader title="Site" back="/sites" backLabel="Sites" />
    <div class="card">
      <LoadError title="Could not load this site" message={loadError} onretry={loadSite} compact={false} />
    </div>
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
        {#if canWrite && site.git_repo && !noGit}
          <button onclick={triggerDeploy} disabled={deployBusy} class="btn btn-primary btn-sm">
            {#if deployBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Upload size={14} />{/if} Git pull
          </button>
        {/if}
        {#if canSuspend && (site.status === 'active' || site.status === 'suspended')}
          <button onclick={toggleSuspend} disabled={toggleBusy} class="btn btn-secondary btn-sm">
            {#if toggleBusy}<LoaderCircle size={14} class="animate-spin" />{:else if site.status === 'suspended'}<Play size={14} />{:else}<Pause size={14} />{/if}
            {site.status === 'suspended' ? 'Resume' : 'Suspend'}
          </button>
        {/if}
        {#if canDelete}
          <button onclick={openDelete} class="btn btn-danger-ghost btn-sm"><Trash2 size={14} /> Delete</button>
        {/if}
      {/snippet}
    </PageHeader>

    {#if site.status === 'suspended'}
      <div class="card mb-6 flex items-start gap-3 border-warning/40 p-4 text-sm">
        <Pause size={16} class="mt-0.5 shrink-0 text-warning" />
        <div>
          <p class="font-medium text-fg">This site is suspended</p>
          <p class="text-muted">The nginx vhost and SSL auto-renewal are disabled. Files, databases and certificates are untouched{canSuspend ? ' — use Resume to bring it back online' : ''}.</p>
        </div>
      </div>
    {/if}

    <!-- Quick stats -->
    {#key site.id}
      <ProvisioningSteps {site} {dns} onsite={(fresh) => { site = fresh; }} />
    {/key}

    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="SSL" value={site.ssl_status || 'none'} icon={ShieldCheck} tone={hasSSL ? 'success' : ''} />
      <StatCard label="Runtime" value={site.runtime} icon={Zap} tone="accent" />
      <StatCard label="Nginx" value={site.conf_file ? 'Configured' : 'Not configured'} icon={Server} tone={site.conf_file ? 'success' : 'warning'} hint={site.conf_file ? site.conf_file.split('/').pop() : ''} />
      <StatCard label="Databases" value={dbsCount || '—'} icon={Database} />
    </div>

    <!-- Tabs -->
    <div class="tabs mb-6" role="tablist">
      {#each tabs as t (t.key)}
        <button role="tab" aria-selected={activeTab === t.key} onclick={() => setTab(t.key)} class="tab {activeTab === t.key ? 'tab-active' : ''}">
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
            <div class="flex items-center justify-between gap-4 py-2.5">
              <dt class="text-[13px] text-muted">Disk usage</dt>
              <dd class="flex items-center gap-2 text-[13px] text-fg tabular-nums">
                {#if diskUsed === null}<LoaderCircle size={13} class="animate-spin text-subtle" />
                {:else if diskUsed === ''}<span class="text-subtle" title="The site's server did not answer">Unavailable</span>
                {:else}{fmtBytesFull(diskUsed)}{/if}
                <button onclick={loadDiskUsage} disabled={diskUsed === null} class="btn btn-ghost btn-xs btn-icon" aria-label="Refresh disk usage" title="Refresh"><RefreshCw size={12} /></button>
              </dd>
            </div>
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
                {@render row('Expires', site.ssl_expires_at ? `${fmtDate(site.ssl_expires_at)} · ${fmtDaysLeft(site.ssl_expires_at)}` : '—')}
              </dl>
            {:else}
              <EmptyState compact icon={ShieldCheck} title="No SSL configured" description="Issue a certificate from the SSL tab." actionLabel="Open SSL tab" onaction={() => setTab('ssl')} />
            {/if}
          </div>

          {#if dns && dns.status !== 'skipped'}
            <div class="card">
              <div class="card-header">
                <h2 class="card-title flex items-center gap-2">DNS <span class="font-normal text-subtle">· Cloudflare</span></h2>
                <span class="badge {dnsTone ? 'badge-' + dnsTone : ''}">{dnsLabel}</span>
              </div>
              <div class="card-body space-y-3 text-[13px]">
                <p class="text-muted">{dns.message}</p>
                {#if dns.zone}
                  <dl class="space-y-1.5">
                    <div class="flex justify-between gap-3"><dt class="text-subtle">Zone</dt><dd class="font-mono text-[12.5px] text-fg">{dns.zone}</dd></div>
                    {#if dns.content}<div class="flex justify-between gap-3"><dt class="text-subtle">Server IP</dt><dd class="font-mono text-[12.5px] text-fg">{dns.content}</dd></div>{/if}
                  </dl>
                {/if}
                {#if dns.existing?.length}
                  <div class="rounded-lg border border-warning/30 bg-warning/5 p-3">
                    <div class="mb-1 text-xs font-medium text-warning">Existing records for {dns.name}</div>
                    {#each dns.existing as r, i (i)}
                      <div class="font-mono text-[12px] text-fg">{r.type} → {r.content}{r.proxied ? ' (proxied)' : ''}</div>
                    {/each}
                  </div>
                {/if}
                {#if canWrite}
                  <div class="flex flex-wrap gap-2 pt-1">
                    {#if dns.status === 'missing' || dns.status === 'error'}
                      <button onclick={() => syncDNS(false)} disabled={dnsBusy} class="btn btn-secondary btn-sm">{dnsBusy ? 'Working…' : dns.status === 'error' ? 'Try again' : 'Create DNS record'}</button>
                    {:else if dns.status === 'conflict'}
                      <button onclick={() => syncDNS(false)} disabled={dnsBusy} class="btn btn-secondary btn-sm">Sync</button>
                      <button onclick={() => (showDnsReplace = true)} disabled={dnsBusy} class="btn btn-danger-ghost btn-sm">Replace with this server</button>
                    {/if}
                  </div>
                {/if}
              </div>
            </div>
          {/if}

          {#if canTransfer}
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

      <!-- Git deploy (not for reverse proxies or Docker sites: nothing to pull) -->
      {#if !noGit}
      <div class="card">
        <div class="card-header">
          <div class="flex items-center gap-2">
            <GitBranch size={16} class="text-subtle" />
            <h2 class="card-title">Git deploy</h2>
          </div>
          {#if !gitEditing && canWrite}
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
          {#if isAdmin}
            <div class="border-t border-line px-5 py-3">
              {#if webhook}
                <dl class="space-y-2 text-[13px]">
                  <div><dt class="label">Webhook URL</dt><dd class="font-mono break-all text-fg">{webhook.url}</dd></div>
                  <div><dt class="label">Secret</dt><dd class="font-mono break-all text-fg">{webhook.secret}</dd></div>
                </dl>
                <p class="hint mt-2">Add this URL as a push webhook (content type JSON) with the secret. GitHub and Gitea sign with it; GitLab sends it as the token.</p>
                <div class="mt-2 flex gap-2">
                  <button onclick={() => loadWebhook(true)} disabled={webhookBusy} class="btn btn-secondary btn-sm"><RefreshCw size={14} /> Rotate secret</button>
                  <button onclick={() => (webhook = null)} class="btn btn-ghost btn-sm">Hide</button>
                </div>
              {:else}
                <button onclick={() => loadWebhook()} disabled={webhookBusy} class="btn btn-ghost btn-sm">Show deploy webhook</button>
              {/if}
            </div>
          {/if}
        {:else}
          <p class="card-body text-[13px] text-muted">No git repository configured. Click Configure to set up git deploy.</p>
        {/if}
      </div>
      {/if}
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
{/key}
</DashboardLayout>

{#if site && dns}
  <Modal bind:open={showDnsReplace} title="Replace DNS records for {dns.name}" maxWidth="md">
    <div class="space-y-3 text-[13px]">
      <p class="text-muted">These records are deleted in Cloudflare and replaced by one A record to this server ({dns.content}), proxied:</p>
      <div class="rounded-lg border border-line p-3">
        {#each dns.existing || [] as r, i (i)}
          <div class="font-mono text-[12px] text-fg">{r.type} → {r.content}</div>
        {/each}
      </div>
      <p class="text-xs text-subtle">Other record types (MX, TXT, …) are not touched. The change is recorded in the audit log with the old values.</p>
    </div>
    {#snippet footer()}
      <button onclick={() => (showDnsReplace = false)} class="btn btn-ghost btn-sm">Cancel</button>
      <button onclick={() => syncDNS(true)} disabled={dnsBusy} class="btn btn-danger btn-sm">{dnsBusy ? 'Replacing…' : 'Replace records'}</button>
    {/snippet}
  </Modal>
{/if}

{#if site}
  <Modal bind:open={showDelete} title="Delete {site.domain}" maxWidth="md">
    <div class="space-y-4 text-[13px]">
      <div>
        <p class="text-muted">Always removed from the panel and the server:</p>
        <ul class="mt-2 space-y-1 text-fg">
          <li class="flex gap-2"><span class="text-subtle">•</span> Nginx vhost, SSL certificate and logs</li>
          <li class="flex gap-2"><span class="text-subtle">•</span> {site.runtime === 'php' ? 'PHP-FPM pool' : 'App service'}, cron jobs and SSH/SFTP accounts</li>
          <li class="flex gap-2"><span class="text-subtle">•</span>
            {#if dbNames.length}Databases: <span class="font-mono">{dbNames.join(', ')}</span>{:else}Databases: none{/if}
          </li>
        </ul>
      </div>

      <label class="flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors {deleteFiles ? 'border-danger/40 bg-danger/5' : 'border-line'}">
        <input type="checkbox" bind:checked={deleteFiles} class="checkbox mt-0.5" />
        <span class="min-w-0">
          <span class="block font-medium text-fg">Also delete all site files</span>
          <span class="mt-0.5 block font-mono text-[12px] break-all text-muted">{site.root_path}</span>
          <span class="mt-1 block text-xs text-subtle">
            {deleteFiles ? 'The folder and everything in it (including local backups inside it) is deleted for good.' : 'Unchecked: the folder and its files stay on the server, and a new site can use it again later.'}
          </span>
        </span>
      </label>

      <div>
        <label for="delete-confirm" class="label">Type <span class="font-mono text-fg">{site.domain}</span> to confirm</label>
        <input id="delete-confirm" bind:value={deleteConfirm} class="input font-mono" autocomplete="off" spellcheck="false" placeholder={site.domain} />
      </div>
    </div>
    {#snippet footer()}
      <button onclick={() => (showDelete = false)} class="btn btn-ghost btn-sm">Cancel</button>
      <button onclick={deleteSite} disabled={deleteBusy || deleteConfirm.trim().toLowerCase() !== site.domain.toLowerCase()} class="btn btn-danger btn-sm">
        <Trash2 size={14} /> {deleteFiles ? 'Delete site and files' : 'Delete site'}
      </button>
    {/snippet}
  </Modal>
{/if}

