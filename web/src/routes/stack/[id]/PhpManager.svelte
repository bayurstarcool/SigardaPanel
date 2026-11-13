<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import {
    Loader2, Download, Check, Plus, FileCode, Play, Square, RotateCw, Trash2,
    RefreshCw, X, ArrowUpCircle, Puzzle, Terminal
  } from 'lucide-svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import OpProgress from './OpProgress.svelte';
  import { statusBadge, type StackActionBody, type StackOp } from './stackOps';

  let {
    open,
    serverId = '',
    versions = [],
    busy = false,
    op = null,
    run,
    isLocal = false,
    isSuper = false,
    onClose,
  }: {
    open: boolean;
    serverId: string;
    versions: any[];
    busy?: boolean;
    op?: StackOp | null;
    run: (body: StackActionBody) => Promise<StackOp | null>;
    isLocal?: boolean;
    isSuper?: boolean;
    onClose: () => void;
  } = $props();

  let available = $state<any[]>([]);
  let loadingAvail = $state(false);
  let sitesByVersion = $state<Record<string, string[]>>({});

  // Extensions panel
  let extVersion = $state('');
  let ext = $state<{ installed: string[]; available: string[]; modules: string[] } | null>(null);
  let extLoading = $state(false);
  let extError = $state('');
  let extChoice = $state('');

  let uninstallVersion = $state('');
  let confirmUninstall = $state(false);

  const mine = $derived(op && op.package === 'php' ? op : null);
  const installedSet = $derived(new Set(versions.map((v) => v.version)));
  const blockedBy = $derived(uninstallVersion ? (sitesByVersion[uninstallVersion] ?? []) : []);
  const canDestroy = $derived(!isLocal || isSuper);

  $effect(() => {
    if (open) {
      loadAvailable();
      loadSites();
    }
  });

  async function loadAvailable() {
    if (!auth.user) return;
    loadingAvail = true;
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, { package: 'php', action: 'list-available' });
      available = d?.versions ?? d?.data?.versions ?? [];
    } catch {
      available = [];
    } finally {
      loadingAvail = false;
    }
  }

  // Sites on this server per PHP version — named in the uninstall dialog.
  async function loadSites() {
    try {
      const d: any = await apiFetch(auth.token, 'GET', `sites?server_id=${serverId}&runtime=php&limit=500`);
      const map: Record<string, string[]> = {};
      for (const s of d?.sites ?? []) {
        const v = s.php_version || '';
        if (!v) continue;
        (map[v] ||= []).push(s.domain);
      }
      sitesByVersion = map;
    } catch {
      sitesByVersion = {};
    }
  }

  async function act(action: string, version: string, extension?: string) {
    const res = await run({ package: 'php', action, version, extension });
    if (res?.done) {
      loadAvailable();
      if (extVersion === version && (action === 'install-extension' || action === 'uninstall-extension')) loadExtensions(version);
      if (action === 'uninstall' && extVersion === version) extVersion = '';
    }
  }

  async function loadExtensions(version: string) {
    extVersion = version;
    extLoading = true;
    extError = '';
    ext = null;
    extChoice = '';
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, { package: 'php', action: 'list-extensions', version });
      ext = { installed: d?.installed ?? [], available: d?.available ?? [], modules: d?.modules ?? [] };
    } catch (e: any) {
      extError = e.message;
    } finally {
      extLoading = false;
    }
  }
</script>

{#if open}
  <div class="overlay animate-fade-in flex items-end justify-center overflow-auto sm:items-start sm:px-4 sm:py-10" onclick={onClose} role="presentation">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      class="popover animate-pop-in flex max-h-[90dvh] w-full max-w-3xl flex-col max-sm:rounded-b-none sm:max-h-none"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label="PHP version manager"
      tabindex="-1"
    >
      <div class="flex shrink-0 items-center justify-between gap-3 border-b border-line px-5 py-3.5">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted">
            <FileCode size={16} />
          </div>
          <div class="min-w-0">
            <h2 class="text-[15px] font-semibold text-fg">PHP versions</h2>
            <p class="truncate text-xs text-subtle">Each version runs its own PHP-FPM; every site picks its version in its settings</p>
          </div>
        </div>
        <div class="flex items-center gap-1">
          <button onclick={() => { loadAvailable(); loadSites(); }} disabled={loadingAvail} class="btn btn-ghost btn-icon btn-sm" aria-label="Refresh versions">
            <RefreshCw size={14} class={loadingAvail ? 'animate-spin' : ''} />
          </button>
          <button onclick={onClose} class="btn btn-ghost btn-icon btn-sm" aria-label="Close"><X size={16} /></button>
        </div>
      </div>

      <div class="overflow-y-auto px-5 py-4 sm:max-h-[70vh]">
        <OpProgress op={mine} />

        <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
          <Check size={14} class="text-success" /> Installed <span class="font-normal text-subtle tabular-nums">({versions.length})</span>
        </h3>
        {#if versions.length > 0}
          <div class="mb-6 divide-y divide-line rounded-lg border border-line">
            {#each versions as ver (ver.version)}
              {@const sc = statusBadge(ver.status)}
              {@const users = sitesByVersion[ver.version] ?? []}
              <div class="px-3 py-2.5" data-testid="php-row-{ver.version}" data-status={ver.status}>
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="flex min-w-0 flex-wrap items-center gap-2">
                    <span class="font-mono text-[13px] font-semibold text-fg">PHP {ver.version}</span>
                    {#if ver.full}<span class="text-xs text-subtle">{ver.full}</span>{/if}
                    <span class="badge {sc.badge}"><span class="dot"></span>{ver.fpm ? sc.label : 'CLI only'}</span>
                    {#if ver.default_cli}<span class="badge badge-accent" title="/usr/bin/php runs this version">CLI default</span>{/if}
                    {#if ver.fpm && !ver.socket && ver.status === 'running'}<span class="badge badge-warning" title="No /run/php/php{ver.version}-fpm.sock">no socket</span>{/if}
                    {#if users.length}<span class="text-xs text-subtle">{users.length} site{users.length > 1 ? 's' : ''}</span>{/if}
                  </div>
                  <div class="flex shrink-0 items-center gap-0.5">
                    {#if ver.fpm && (ver.status === 'stopped' || ver.status === 'failed')}
                      <button onclick={() => act('start', ver.version)} disabled={busy} aria-label="Start PHP {ver.version}-FPM" title="Start FPM" class="btn btn-ghost btn-icon btn-xs text-success"><Play size={12} /></button>
                    {/if}
                    {#if ver.fpm && ver.status === 'running'}
                      <button onclick={() => act('stop', ver.version)} disabled={busy || !canDestroy} aria-label="Stop PHP {ver.version}-FPM" title="Stop FPM" class="btn btn-ghost btn-icon btn-xs text-warning"><Square size={12} /></button>
                    {/if}
                    {#if ver.fpm}
                      <button onclick={() => act('restart', ver.version)} disabled={busy} aria-label="Restart PHP {ver.version}-FPM" title="Restart FPM" class="btn btn-ghost btn-icon btn-xs" data-testid="php-restart-{ver.version}"><RotateCw size={12} /></button>
                    {/if}
                    <button onclick={() => act('update', ver.version)} disabled={busy} aria-label="Update PHP {ver.version}" title="Update packages" class="btn btn-ghost btn-icon btn-xs"><ArrowUpCircle size={12} /></button>
                    {#if !ver.default_cli}
                      <button onclick={() => act('set-default', ver.version)} disabled={busy} aria-label="Use PHP {ver.version} as CLI default" title="Make /usr/bin/php run this version" class="btn btn-ghost btn-icon btn-xs" data-testid="php-default-{ver.version}"><Terminal size={12} /></button>
                    {/if}
                    <button
                      onclick={() => (extVersion === ver.version ? (extVersion = '') : loadExtensions(ver.version))}
                      aria-label="Extensions of PHP {ver.version}"
                      title="Extensions"
                      class="btn btn-ghost btn-icon btn-xs {extVersion === ver.version ? 'text-accent' : ''}"
                      data-testid="php-ext-{ver.version}"
                    ><Puzzle size={12} /></button>
                    <button
                      onclick={() => { uninstallVersion = ver.version; confirmUninstall = true; }}
                      disabled={busy || !canDestroy}
                      aria-label="Uninstall PHP {ver.version}"
                      title={canDestroy ? 'Uninstall' : 'Only a super_admin can uninstall software on the panel host'}
                      class="btn btn-danger-ghost btn-icon btn-xs"
                      data-testid="php-uninstall-{ver.version}"
                    ><Trash2 size={12} /></button>
                  </div>
                </div>

                {#if extVersion === ver.version}
                  <div class="mt-3 rounded-lg border border-line bg-surface-2/50 p-3" data-testid="php-ext-panel">
                    {#if extLoading}
                      <div class="flex items-center gap-2 text-xs text-subtle"><Loader2 size={12} class="animate-spin" /> Loading extensions…</div>
                    {:else if extError}
                      <p class="text-xs text-danger">{extError}</p>
                    {:else if ext}
                      <div class="mb-2 text-xs font-medium text-muted">Extension packages (php{ver.version}-*)</div>
                      <div class="mb-3 flex flex-wrap gap-1.5">
                        {#each ext.installed as e (e)}
                          <span class="badge font-mono">
                            {e}
                            <button
                              onclick={() => act('uninstall-extension', ver.version, e)}
                              disabled={busy || !canDestroy}
                              class="ml-1 opacity-60 hover:opacity-100 disabled:cursor-not-allowed"
                              aria-label="Remove {e}"
                              title="Remove php{ver.version}-{e}"
                            ><X size={10} /></button>
                          </span>
                        {:else}
                          <span class="text-xs text-subtle">none</span>
                        {/each}
                      </div>
                      <div class="flex items-center gap-2">
                        <select bind:value={extChoice} class="input flex-1 font-mono text-[13px]" aria-label="Extension to install" data-testid="php-ext-select">
                          <option value="">{ext.available.length ? 'Choose an extension…' : 'No other extensions in the repository'}</option>
                          {#each ext.available as a (a)}<option value={a}>{a}</option>{/each}
                        </select>
                        <button onclick={() => extChoice && act('install-extension', ver.version, extChoice)} disabled={!extChoice || busy} class="btn btn-primary btn-sm" data-testid="php-ext-install">
                          <Download size={14} /> Install
                        </button>
                      </div>
                      <p class="mt-2 text-xs text-subtle">{ext.modules.length} modules loaded. Installing or removing an extension restarts php{ver.version}-fpm.</p>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {:else}
          <div class="mb-6 rounded-lg border border-dashed border-line px-4 py-6 text-center">
            <FileCode size={18} class="mx-auto mb-2 text-subtle" />
            <p class="text-[13px] text-muted">No PHP version is installed — PHP sites cannot be created on this server yet</p>
          </div>
        {/if}

        <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
          <Plus size={14} class="text-accent" /> Install a version
        </h3>
        <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {#each available as ver (ver.version)}
            {@const installed = ver.installed || installedSet.has(ver.version)}
            {@const pending = busy && mine?.action === 'install' && mine?.version === ver.version}
            <button
              onclick={() => { if (!installed) act('install', ver.version); }}
              disabled={installed || busy}
              data-testid="php-install-{ver.version}"
              class="rounded-lg border px-3 py-2.5 text-center transition-colors duration-150 disabled:cursor-not-allowed {installed
                ? 'border-success/20 bg-success/5'
                : 'cursor-pointer border-line bg-surface hover:border-accent/40 hover:bg-accent/5'} {busy && !installed && !pending ? 'opacity-60' : ''}"
            >
              {#if pending}
                <Loader2 size={14} class="mx-auto mb-1 animate-spin text-accent" />
              {:else if installed}
                <Check size={14} class="mx-auto mb-1 text-success" />
              {:else}
                <Download size={14} class="mx-auto mb-1 text-subtle" />
              {/if}
              <div class="text-xs font-semibold {installed ? 'text-success' : 'text-fg'}">PHP {ver.version}</div>
            </button>
          {/each}
        </div>
        <p class="mt-2 text-xs text-subtle">Versions the distribution does not ship come from ppa:ondrej/php (Ubuntu) or packages.sury.org (Debian), added automatically.</p>
      </div>

      <div class="flex shrink-0 justify-end border-t border-line px-5 py-3">
        <button onclick={onClose} class="btn btn-secondary btn-sm">Close</button>
      </div>
    </div>
  </div>

  <ConfirmDialog
    bind:open={confirmUninstall}
    title="Uninstall PHP {uninstallVersion}?"
    message={blockedBy.length
      ? `PHP ${uninstallVersion} is used by ${blockedBy.join(', ')}. The panel will refuse until those sites use another PHP version (site Settings → PHP).`
      : `All php${uninstallVersion}-* packages and their configuration are purged and php${uninstallVersion}-fpm stops. The panel refuses if any nginx vhost still uses this version.`}
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => { if (uninstallVersion) act('uninstall', uninstallVersion); }}
  />
{/if}
