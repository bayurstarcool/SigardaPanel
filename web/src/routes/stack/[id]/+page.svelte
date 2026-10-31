<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import {
    Globe, Code2, FileCode, Box, Lock, Database, Zap,
    RefreshCw, Download, Trash2, Play, Square, RotateCw,
    CheckCircle2, XCircle, Loader2, Settings, Terminal
  } from 'lucide-svelte';
  import ServiceManagerModal from './ServiceManagerModal.svelte';

  const serverId = $derived($page.params.id);

  let server = $state<any>(null);
  let packages = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let actionPending = $state('');
  let actionResult = $state<{ type: 'success' | 'error'; message: string } | null>(null);
  let osInfo = $state('');
  let showOutput = $state(false);
  let lastOutput = $state('');
  let activeManagePkg = $state<any>(null);

  const knownPackages = [
    { name: 'nginx', label: 'Nginx', icon: Globe, desc: 'Web server & reverse proxy', multiVersion: false, availableVersions: ['1.26.0', '1.24.0'] },
    { name: 'php', label: 'PHP', icon: FileCode, desc: 'PHP-FPM runtime + extensions', multiVersion: true, availableVersions: ['8.5', '8.4', '8.3', '8.2', '8.1', '8.0', '7.4'] },
    { name: 'node', label: 'Node.js', icon: Code2, desc: 'JavaScript runtime + npm', multiVersion: true, availableVersions: ['22', '20', '18'] },
    { name: 'python', label: 'Python', icon: Terminal, desc: 'Python 3 + pip + venv', multiVersion: false, availableVersions: ['3.12', '3.11'] },
    { name: 'go', label: 'Go', icon: Box, desc: 'Go programming language', multiVersion: true, availableVersions: ['1.24.0', '1.23.6', '1.22.12'] },
    { name: 'docker', label: 'Docker', icon: Box, desc: 'Container runtime', multiVersion: false },
    { name: 'certbot', label: 'Certbot', icon: Lock, desc: 'SSL/TLS certificates', multiVersion: false },
    { name: 'mariadb', label: 'MariaDB', icon: Database, desc: 'MySQL-compatible database', multiVersion: false },
    { name: 'redis', label: 'Redis', icon: Zap, desc: 'In-memory data store', multiVersion: false },
    { name: 'supervisor', label: 'Supervisor', icon: Settings, desc: 'Process manager', multiVersion: false },
  ];

  function loadServer() {
    if (!auth.user) { window.location.href = '/login'; return; }
    apiFetch(auth.token, 'GET', `servers/${serverId}`)
      .then((d: any) => server = d)
      .catch((e) => { error = e.message; });
  }

  function loadStack() {
    if (!auth.user) return;
    loading = true;
    error = '';
    apiFetch(auth.token, 'GET', `servers/${serverId}/stack`)
      .then((d: any) => {
        // API returns { data: { packages: [...] } } after apiFetch extracts json.data
        const stackData = d?.data ?? d;
        packages = stackData?.packages ?? [];
        osInfo = `${stackData?.os ?? 'unknown'} / ${stackData?.arch ?? 'unknown'}`;
      })
      .catch((e) => { error = e.message; })
      .finally(() => loading = false);
  }

  onMount(() => { loadServer(); loadStack(); });

  function getPackageInfo(name: string) {
    return packages.find(p => p.name === name);
  }

  function statusConfig(status: string) {
    switch (status) {
      case 'running': return { label: 'Running', badge: 'badge-success' };
      case 'installed': return { label: 'Installed', badge: 'badge-info' };
      case 'stopped': return { label: 'Stopped', badge: 'badge-warning' };
      case 'missing': return { label: 'Not installed', badge: '' };
      default: return { label: 'Unknown', badge: '' };
    }
  }

  let uninstallPkg = $state<any>(null);
  let confirmUninstall = $state(false);

  const quickActions = [
    { pkg: 'nginx', label: 'Restart Nginx' },
    { pkg: 'php', label: 'Restart PHP' },
    { pkg: 'redis', label: 'Restart Redis' },
    { pkg: 'docker', label: 'Restart Docker' },
  ];

  async function doAction(pkg: string, action: string, version?: string) {
    if (!auth.user) return;
    actionPending = `${pkg}:${action}`;
    actionResult = null;
    lastOutput = '';
    try {
      const body: any = { package: pkg, action };
      if (version) body.version = version;
      const result = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, body);
      lastOutput = result?.output || '';
      if (result?.ok) {
        actionResult = { type: 'success', message: result?.message || `${action} ${pkg} completed` };
      } else {
        actionResult = { type: 'error', message: result?.message || `${action} ${pkg} failed` };
      }
      setTimeout(loadStack, 1500);
    } catch (e: any) {
      actionResult = { type: 'error', message: e.message };
    } finally {
      actionPending = '';
    }
  }

  function pkgIcon(name: string) {
    const pkg = knownPackages.find(p => p.name === name);
    return pkg?.icon || Box;
  }
</script>

<DashboardLayout active="Stack">
  <PageHeader title={server?.name ?? 'Loading…'} back="/stack" backLabel="Stack" description={server?.hostname ?? ''}>
    {#snippet meta()}
      {#if osInfo}<span class="badge font-mono">{osInfo}</span>{/if}
    {/snippet}
    {#snippet actions()}
      <button onclick={() => loadStack()} disabled={loading} class="btn btn-secondary btn-sm">
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
    {/snippet}
  </PageHeader>

  {#if error}
    <div class="mb-6 flex items-center gap-2 rounded-lg bg-danger/10 px-4 py-3 text-[13px] text-danger">
      <XCircle size={16} class="shrink-0" /> {error}
    </div>
  {/if}

  {#if actionResult}
    <div class="mb-6 flex items-center gap-2 rounded-lg px-4 py-3 text-[13px] {actionResult.type === 'success' ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}">
      {#if actionResult.type === 'success'}<CheckCircle2 size={16} class="shrink-0" />{:else}<XCircle size={16} class="shrink-0" />{/if}
      <span class="min-w-0 flex-1">{actionResult.message}</span>
      {#if lastOutput}
        <button onclick={() => (showOutput = !showOutput)} class="btn btn-ghost btn-xs">{showOutput ? 'Hide' : 'Show'} output</button>
      {/if}
    </div>
    {#if showOutput && lastOutput}
      <pre class="mb-6 max-h-48 overflow-auto rounded-lg border border-line bg-surface-2 p-4 font-mono text-xs whitespace-pre-wrap text-muted">{lastOutput}</pre>
    {/if}
  {/if}

  {#if loading}
    <div class="skeleton mb-6 h-14 rounded-xl"></div>
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each Array(6) as _, i (i)}<div class="skeleton h-44 rounded-xl"></div>{/each}
    </div>
  {:else}
    <div class="card mb-6 flex flex-wrap items-center gap-2 px-4 py-3">
      <span class="mr-1 text-xs font-medium text-subtle">Quick actions</span>
      {#each quickActions as qa (qa.pkg)}
        <button onclick={() => doAction(qa.pkg, 'restart')} disabled={actionPending !== ''} class="btn btn-secondary btn-xs">
          {#if actionPending === `${qa.pkg}:restart`}<Loader2 size={12} class="animate-spin" />{:else}<RotateCw size={12} />{/if}
          {qa.label}
        </button>
      {/each}
    </div>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each knownPackages as pkg (pkg.name)}
        {@const info = getPackageInfo(pkg.name)}
        {@const status = info?.status ?? 'missing'}
        {@const sc = statusConfig(status)}
        {@const Icon = pkg.icon}
        {@const isActive = actionPending.startsWith(pkg.name + ':')}

        <div class="card flex flex-col">
          <div class="flex-1 p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="flex min-w-0 items-center gap-3">
                <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted">
                  <Icon size={16} />
                </div>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-fg">{pkg.label}</h3>
                  <p class="truncate text-xs text-subtle">{pkg.desc}</p>
                </div>
              </div>
              <span class="badge {sc.badge}"><span class="dot"></span>{sc.label}</span>
            </div>
            {#if info?.version}
              <p class="mt-3"><span class="code text-xs">v{info.version}</span></p>
            {/if}
          </div>

          <div class="space-y-2 border-t border-line p-3">
            <button onclick={() => (activeManagePkg = pkg)} class="btn btn-secondary btn-sm w-full">
              <Settings size={14} /> Manage service & versions
            </button>

            {#if status === 'missing'}
              <button onclick={() => doAction(pkg.name, 'install')} disabled={actionPending !== ''} class="btn btn-primary btn-sm w-full">
                {#if isActive}<Loader2 size={14} class="animate-spin" /> Installing…{:else}<Download size={14} /> Install {pkg.label}{/if}
              </button>
            {:else}
              <div class="flex items-center gap-1">
                {#if status === 'stopped'}
                  <button onclick={() => doAction(pkg.name, 'start')} disabled={actionPending !== ''} title="Start" class="btn btn-ghost btn-xs flex-1 text-success">
                    {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<Play size={12} />{/if} Start
                  </button>
                {/if}
                {#if status === 'running'}
                  <button onclick={() => doAction(pkg.name, 'stop')} disabled={actionPending !== ''} title="Stop" class="btn btn-ghost btn-xs flex-1 text-warning">
                    {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<Square size={12} />{/if} Stop
                  </button>
                {/if}
                <button onclick={() => doAction(pkg.name, 'restart')} disabled={actionPending !== ''} title="Restart" class="btn btn-ghost btn-xs flex-1">
                  {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<RotateCw size={12} />{/if} Restart
                </button>
                <button onclick={() => doAction(pkg.name, 'update')} disabled={actionPending !== ''} title="Update" class="btn btn-ghost btn-xs flex-1">
                  {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<RefreshCw size={12} />{/if} Update
                </button>
                <button
                  onclick={() => { uninstallPkg = pkg; confirmUninstall = true; }}
                  disabled={actionPending !== ''}
                  title="Uninstall"
                  aria-label="Uninstall {pkg.label}"
                  class="btn btn-danger-ghost btn-xs btn-icon"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <ConfirmDialog
    bind:open={confirmUninstall}
    title="Uninstall {uninstallPkg?.label ?? 'package'}?"
    message="The package will be removed from this server. Sites that depend on it may stop working."
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => { if (uninstallPkg) doAction(uninstallPkg.name, 'uninstall'); }}
  />
</DashboardLayout>

<ServiceManagerModal
  open={activeManagePkg !== null}
  pkg={activeManagePkg}
  serverId={serverId || ''}
  onClose={() => activeManagePkg = null}
  onChanged={() => loadStack()}
/>
