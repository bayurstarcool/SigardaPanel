<script lang="ts">
  import { auth, hasRole, hasPermission } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import {
    Globe, Code2, FileCode, Box, Lock, Database, Zap, HardDrive,
    RefreshCw, Download, Trash2, Play, Square, RotateCw,
    XCircle, Loader2, Settings, Terminal, ArrowUpCircle, Info
  } from 'lucide-svelte';
  import ServiceManagerModal from './ServiceManagerModal.svelte';
  import PhpManager from './PhpManager.svelte';
  import OpProgress from './OpProgress.svelte';
  import { runStackAction, pollStackOp, statusBadge, describeAction, type StackActionBody, type StackOp } from './stackOps';

  const serverId = $derived($page.params.id ?? '');
  const isAdmin = $derived(hasPermission('server.stack'));
  const isSuper = $derived(hasPermission('server.panel_host'));

  let server = $state<any>(null);
  let packages = $state<any[]>([]);
  let phpVersions = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let distro = $state('');
  let isLocal = $state(false);
  let packageManager = $state('');

  // The running / last package operation and what it was.
  let op = $state<StackOp | null>(null);
  let opTitle = $state('');
  let busy = $state(false);

  let managePkg = $state<any>(null);
  let phpOpen = $state(false);

  type Pkg = { name: string; label: string; icon: any; desc: string; service: boolean; versions?: boolean; uninstallNote: string };
  const knownPackages: Pkg[] = [
    { name: 'nginx', label: 'Nginx', icon: Globe, desc: 'Web server & reverse proxy', service: true, uninstallNote: 'Every site on this server stops being served. The panel refuses while sites exist.' },
    { name: 'php', label: 'PHP', icon: FileCode, desc: 'PHP-FPM, several versions side by side', service: true, versions: true, uninstallNote: '' },
    { name: 'node', label: 'Node.js', icon: Code2, desc: 'JavaScript runtime + npm (NodeSource)', service: false, versions: true, uninstallNote: 'Node.js apps on this server stop working. Refused while Node sites exist.' },
    { name: 'python', label: 'Python', icon: Terminal, desc: 'Python 3 + pip + venv', service: false, uninstallNote: 'Removes pip and venv only; python3 belongs to the operating system and stays.' },
    { name: 'go', label: 'Go', icon: Box, desc: 'Go toolchain in /usr/local/go', service: false, versions: true, uninstallNote: 'Removes /usr/local/go. Compiled Go apps keep running.' },
    { name: 'docker', label: 'Docker', icon: Box, desc: 'Container runtime', service: true, uninstallNote: 'Refused while containers are running or Docker sites exist. Images and volumes in /var/lib/docker are kept.' },
    { name: 'certbot', label: 'Certbot', icon: Lock, desc: "Let's Encrypt certificates", service: false, uninstallNote: 'Certificates stop renewing. Refused while sites use SSL.' },
    { name: 'mariadb', label: 'MariaDB', icon: Database, desc: 'MySQL-compatible database server', service: true, uninstallNote: 'Database server is removed; the data directory /var/lib/mysql is kept. Refused while panel databases exist.' },
    { name: 'redis', label: 'Redis', icon: Zap, desc: 'In-memory data store', service: true, uninstallNote: 'Apps using Redis lose their cache/queues. /var/lib/redis is kept.' },
    { name: 'memcached', label: 'Memcached', icon: HardDrive, desc: 'Memory object cache', service: true, uninstallNote: 'Apps using Memcached lose their cache.' },
    { name: 'supervisor', label: 'Supervisor', icon: Settings, desc: 'Process manager for workers', service: true, uninstallNote: 'Programs managed by Supervisor stop.' },
  ];

  function loadServer() {
    if (!auth.user) { window.location.href = '/login'; return; }
    apiFetch(auth.token, 'GET', `servers/${serverId}`)
      .then((d: any) => (server = d))
      .catch((e) => { error = e.message; });
  }

  async function loadStack(silent = false) {
    if (!auth.user) return;
    if (!silent) loading = true;
    error = '';
    try {
      const d: any = await apiFetch(auth.token, 'GET', `servers/${serverId}/stack`);
      const data = d?.packages ? d : (d?.data ?? d);
      packages = data?.packages ?? [];
      phpVersions = data?.php_versions ?? [];
      distro = data?.distro || `${data?.os ?? 'unknown'} / ${data?.arch ?? ''}`;
      isLocal = !!data?.is_local;
      packageManager = data?.package_manager ?? '';
      // An operation started earlier (another tab, before a reload) is still running: follow it.
      if (data?.operation && !data.operation.done && !busy && isAdmin) {
        follow(data.operation);
      }
    } catch (e: any) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function follow(running: StackOp) {
    busy = true;
    op = running;
    opTitle = describeAction(running as StackActionBody) + ' (started earlier)';
    try {
      op = await pollStackOp(auth.token, serverId, running.op_id!, (p) => (op = p));
    } catch (e: any) {
      op = { ...(op as StackOp), done: true, ok: false, status: 'failed', message: e.message };
    } finally {
      busy = false;
      loadStack(true);
    }
  }

  onMount(() => { loadServer(); loadStack(); });

  /** Runs an action and reports its verified result; returns it to modals too. */
  async function run(body: StackActionBody): Promise<StackOp | null> {
    if (busy) return null;
    busy = true;
    opTitle = describeAction(body);
    op = { package: body.package, action: body.action, version: body.version, status: 'running', ok: false, done: false, message: 'Sending to the server…' };
    try {
      op = await runStackAction(auth.token, serverId, body, (p) => (op = p));
    } catch (e: any) {
      op = { package: body.package, action: body.action, version: body.version, status: 'failed', ok: false, done: true, message: e.message };
    } finally {
      busy = false;
    }
    await loadStack(true);
    return op;
  }

  function info(name: string) {
    return packages.find((p) => p.name === name);
  }

  let confirmPkg = $state<Pkg | null>(null);
  let confirmOpen = $state(false);

  function protectedOnLocal(name: string, action: string) {
    return isLocal && (name === 'nginx' || (name === 'certbot' && action === 'uninstall'));
  }

  const quickActions = $derived(
    [
      { pkg: 'nginx', label: 'Restart Nginx' },
      { pkg: 'php', label: 'Restart PHP-FPM (all)' },
      { pkg: 'redis', label: 'Restart Redis' },
      { pkg: 'mariadb', label: 'Restart MariaDB' },
      { pkg: 'docker', label: 'Restart Docker' },
    ].filter((q) => {
      const st = info(q.pkg)?.status;
      return st && st !== 'missing' && st !== 'installed';
    }),
  );
</script>

<DashboardLayout active="Stack">
  <PageHeader title={server?.name ?? 'Loading…'} back="/stack" backLabel="Stack" description={server?.hostname ?? ''}>
    {#snippet meta()}
      {#if distro}<span class="badge font-mono">{distro}</span>{/if}
      {#if isLocal}<span class="badge badge-accent">Panel host</span>{/if}
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

  {#if !isAdmin}
    <div class="mb-6 flex items-center gap-2 rounded-lg bg-surface-2 px-4 py-3 text-[13px] text-muted">
      <Info size={16} class="shrink-0" /> Read-only: only admins can install, remove or restart software.
    </div>
  {:else if packageManager && packageManager !== 'apt-get'}
    <div class="mb-6 flex items-center gap-2 rounded-lg bg-warning/10 px-4 py-3 text-[13px] text-warning">
      <Info size={16} class="shrink-0" /> This server uses {packageManager}; package installs are only supported on Debian/Ubuntu. Service actions still work.
    </div>
  {/if}

  <OpProgress {op} title={opTitle} ondismiss={() => (op = null)} />

  {#if loading}
    <div class="skeleton mb-6 h-14 rounded-xl"></div>
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each Array(6) as _, i (i)}<div class="skeleton h-44 rounded-xl"></div>{/each}
    </div>
  {:else}
    {#if isAdmin && quickActions.length > 0}
      <div class="card mb-6 flex flex-wrap items-center gap-2 px-4 py-3">
        <span class="mr-1 text-xs font-medium text-subtle">Quick actions</span>
        {#each quickActions as qa (qa.pkg)}
          <button onclick={() => run({ package: qa.pkg, action: 'restart' })} disabled={busy} class="btn btn-secondary btn-xs">
            {#if busy && op?.package === qa.pkg && op?.action === 'restart'}<Loader2 size={12} class="animate-spin" />{:else}<RotateCw size={12} />{/if}
            {qa.label}
          </button>
        {/each}
      </div>
    {/if}

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each knownPackages as pkg (pkg.name)}
        {@const pi = info(pkg.name)}
        {@const status = pi?.status ?? 'missing'}
        {@const sc = statusBadge(status)}
        {@const Icon = pkg.icon}
        {@const mine = busy && op?.package === pkg.name}
        {@const hasService = pkg.service && status !== 'installed' && (pkg.name !== 'mariadb' || !!pi?.service)}

        <div class="card flex flex-col" data-testid="stack-card-{pkg.name}" data-status={status}>
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
            {#if status !== 'missing'}
              <div class="mt-3 flex flex-wrap items-center gap-1.5">
                {#if pi?.version}<span class="code text-xs" data-testid="stack-version-{pkg.name}">{pi.version}</span>{/if}
                {#if pkg.name === 'php' && pi?.versions?.length}
                  {#each pi.versions as v (v)}<span class="badge font-mono">{v}</span>{/each}
                {:else if pi?.detail}
                  <span class="text-xs text-subtle">{pi.detail}</span>
                {/if}
              </div>
            {/if}
          </div>

          {#if isAdmin}
            <div class="space-y-2 border-t border-line p-3">
              {#if pkg.versions}
                <button
                  onclick={() => (pkg.name === 'php' ? (phpOpen = true) : (managePkg = pkg))}
                  class="btn btn-secondary btn-sm w-full"
                  data-testid="stack-manage-{pkg.name}"
                >
                  <Settings size={14} /> {pkg.name === 'php' ? 'Manage versions & extensions' : 'Manage versions'}
                </button>
              {/if}

              {#if status === 'missing'}
                {#if !pkg.versions}
                  <button onclick={() => run({ package: pkg.name, action: 'install' })} disabled={busy} class="btn btn-primary btn-sm w-full" data-testid="stack-install-{pkg.name}">
                    {#if mine && op?.action === 'install'}<Loader2 size={14} class="animate-spin" /> Installing…{:else}<Download size={14} /> Install {pkg.label}{/if}
                  </button>
                {/if}
              {:else}
                <div class="flex flex-wrap items-center gap-1">
                  <!-- PHP versions are started/stopped one by one in the PHP manager. -->
                  {#if hasService && pkg.name !== 'php' && (status === 'stopped' || status === 'failed')}
                    <button onclick={() => run({ package: pkg.name, action: 'start' })} disabled={busy} class="btn btn-ghost btn-xs flex-1 text-success">
                      {#if mine && op?.action === 'start'}<Loader2 size={12} class="animate-spin" />{:else}<Play size={12} />{/if} Start
                    </button>
                  {/if}
                  {#if hasService && pkg.name !== 'php' && (status === 'running' || status === 'starting') && !protectedOnLocal(pkg.name, 'stop')}
                    <button onclick={() => run({ package: pkg.name, action: 'stop' })} disabled={busy || (isLocal && !isSuper)} class="btn btn-ghost btn-xs flex-1 text-warning">
                      {#if mine && op?.action === 'stop'}<Loader2 size={12} class="animate-spin" />{:else}<Square size={12} />{/if} Stop
                    </button>
                  {/if}
                  {#if hasService}
                    <button onclick={() => run({ package: pkg.name, action: 'restart' })} disabled={busy} class="btn btn-ghost btn-xs flex-1" data-testid="stack-restart-{pkg.name}">
                      {#if mine && op?.action === 'restart'}<Loader2 size={12} class="animate-spin" />{:else}<RotateCw size={12} />{/if} Restart
                    </button>
                  {/if}
                  {#if pkg.name !== 'go'}
                    <button onclick={() => run({ package: pkg.name, action: 'update' })} disabled={busy} class="btn btn-ghost btn-xs flex-1" title="Upgrade to the newest packaged version">
                      {#if mine && op?.action === 'update'}<Loader2 size={12} class="animate-spin" />{:else}<ArrowUpCircle size={12} />{/if} Update
                    </button>
                  {/if}
                  {#if pkg.name !== 'php' && !protectedOnLocal(pkg.name, 'uninstall')}
                    <button
                      onclick={() => { confirmPkg = pkg; confirmOpen = true; }}
                      disabled={busy || (isLocal && !isSuper)}
                      title={isLocal && !isSuper ? 'Only a super_admin can uninstall software on the panel host' : 'Uninstall'}
                      aria-label="Uninstall {pkg.label}"
                      class="btn btn-danger-ghost btn-xs btn-icon"
                      data-testid="stack-uninstall-{pkg.name}"
                    >
                      {#if mine && op?.action === 'uninstall'}<Loader2 size={12} class="animate-spin" />{:else}<Trash2 size={12} />{/if}
                    </button>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  <ConfirmDialog
    bind:open={confirmOpen}
    title="Uninstall {confirmPkg?.label ?? 'package'}?"
    message={confirmPkg?.uninstallNote ?? ''}
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => { if (confirmPkg) run({ package: confirmPkg.name, action: 'uninstall' }); }}
  />
</DashboardLayout>

<ServiceManagerModal
  open={managePkg !== null}
  pkg={managePkg}
  info={managePkg ? info(managePkg.name) : null}
  {serverId}
  {busy}
  {op}
  {run}
  onClose={() => (managePkg = null)}
/>

<PhpManager
  open={phpOpen}
  {serverId}
  versions={phpVersions}
  {busy}
  {op}
  {run}
  {isLocal}
  {isSuper}
  onClose={() => (phpOpen = false)}
/>
