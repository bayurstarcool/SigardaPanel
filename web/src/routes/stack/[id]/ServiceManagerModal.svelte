<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { Loader2, Download, Check, Box, Trash2, RefreshCw, X } from 'lucide-svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import OpProgress from './OpProgress.svelte';
  import { statusBadge, type StackActionBody, type StackOp } from './stackOps';

  // Version manager for single-install runtimes chosen by version: Node.js
  // (NodeSource major line) and Go (official tarball). Installing another
  // version replaces the current one.
  let {
    open,
    pkg,
    info = null,
    serverId = '',
    busy = false,
    op = null,
    run,
    onClose,
  }: {
    open: boolean;
    pkg: { name: string; label: string; desc: string; icon: any; uninstallNote?: string } | null;
    info?: any;
    serverId: string;
    busy?: boolean;
    op?: StackOp | null;
    run: (body: StackActionBody) => Promise<StackOp | null>;
    onClose: () => void;
  } = $props();

  let available = $state<any[]>([]);
  let customVersion = $state('');
  let loading = $state(false);
  let loadError = $state('');
  let confirmUninstall = $state(false);

  const installed = $derived(info && info.status !== 'missing' ? info : null);
  const mine = $derived(op && pkg && op.package === pkg.name ? op : null);

  $effect(() => {
    if (open && pkg) loadAvailable();
  });

  async function loadAvailable() {
    if (!auth.user || !pkg) return;
    loading = true;
    loadError = '';
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, { package: pkg.name, action: 'list-available' });
      available = d?.versions ?? d?.data?.versions ?? [];
    } catch (e: any) {
      loadError = e.message;
      available = [];
    } finally {
      loading = false;
    }
  }

  async function act(action: string, version?: string) {
    if (!pkg) return;
    const res = await run({ package: pkg.name, action, version });
    if (res?.done) loadAvailable();
  }

  function isCurrent(v: string) {
    if (!installed?.version) return false;
    return pkg?.name === 'node' ? installed.version.startsWith(v + '.') : installed.version === v;
  }
</script>

{#if open && pkg}
  {@const Icon = pkg.icon || Box}
  {@const sc = statusBadge(installed?.status ?? 'missing')}
  <div class="overlay animate-fade-in flex items-end justify-center overflow-auto sm:items-start sm:px-4 sm:py-10" onclick={onClose} role="presentation">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      class="popover animate-pop-in flex max-h-[90dvh] w-full max-w-2xl flex-col max-sm:rounded-b-none sm:max-h-none"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label="{pkg.label} versions"
      tabindex="-1"
    >
      <div class="flex shrink-0 items-center justify-between gap-3 border-b border-line px-5 py-3.5">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted">
            <Icon size={16} />
          </div>
          <div class="min-w-0">
            <h2 class="truncate text-[15px] font-semibold text-fg">{pkg.label} versions</h2>
            <p class="truncate text-xs text-subtle">{pkg.desc} — one version at a time; installing another replaces it</p>
          </div>
        </div>
        <div class="flex items-center gap-1">
          <button onclick={() => loadAvailable()} disabled={loading} class="btn btn-ghost btn-icon btn-sm" aria-label="Refresh versions">
            <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
          </button>
          <button onclick={onClose} class="btn btn-ghost btn-icon btn-sm" aria-label="Close"><X size={16} /></button>
        </div>
      </div>

      <div class="overflow-y-auto px-5 py-4">
        <OpProgress op={mine} />

        <h3 class="mb-2 text-sm font-semibold text-fg">Installed</h3>
        {#if installed}
          <div class="mb-6 flex items-center justify-between gap-3 rounded-lg border border-line px-3 py-2.5">
            <div class="flex min-w-0 items-center gap-2.5">
              <span class="font-mono text-[13px] font-semibold text-fg">{installed.version}</span>
              <span class="badge {sc.badge}"><span class="dot"></span>{sc.label}</span>
              {#if installed.detail}<span class="text-xs text-subtle">{installed.detail}</span>{/if}
              {#if installed.path}<span class="truncate font-mono text-xs text-subtle max-sm:hidden">{installed.path}</span>{/if}
            </div>
            <div class="flex shrink-0 items-center gap-1">
              {#if pkg.name === 'node'}
                <button onclick={() => act('update')} disabled={busy} class="btn btn-ghost btn-xs" title="Upgrade within the installed major line">Update</button>
              {/if}
              <button
                onclick={() => (confirmUninstall = true)}
                disabled={busy}
                class="btn btn-danger-ghost btn-icon btn-xs"
                aria-label="Uninstall {pkg.label}"
                title="Uninstall"
              >
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        {:else}
          <div class="mb-6 rounded-lg border border-dashed border-line px-4 py-5 text-center text-[13px] text-muted">
            {pkg.label} is not installed
          </div>
        {/if}

        <h3 class="mb-2 text-sm font-semibold text-fg">{installed ? 'Switch to another version' : 'Install'}</h3>
        {#if loadError}<p class="mb-2 text-xs text-danger">{loadError}</p>{/if}
        {#if loading}
          <div class="grid grid-cols-3 gap-2">{#each Array(3) as _, i (i)}<div class="skeleton h-14"></div>{/each}</div>
        {:else}
          <div class="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
            {#each available as ver (ver.version)}
              {@const current = isCurrent(ver.version)}
              {@const pending = busy && mine?.version === ver.version}
              <button
                onclick={() => { if (!current) act('install', ver.version); }}
                disabled={current || busy}
                data-testid="stack-version-{pkg.name}-{ver.version}"
                class="rounded-lg border px-3 py-2.5 text-center transition-colors duration-150 disabled:cursor-not-allowed {current
                  ? 'border-success/20 bg-success/5'
                  : 'cursor-pointer border-line bg-surface hover:border-accent/40 hover:bg-accent/5'} {busy && !current ? 'opacity-60' : ''}"
              >
                <div class="mb-0.5 font-mono text-[13px] font-semibold text-fg">{pkg.name === 'node' ? `${ver.version}.x` : ver.version}</div>
                {#if current}
                  <span class="inline-flex items-center gap-1 text-[11px] font-medium text-success"><Check size={10} /> Installed</span>
                {:else if pending}
                  <span class="inline-flex items-center gap-1 text-[11px] font-medium text-accent"><Loader2 size={10} class="animate-spin" /> Installing…</span>
                {:else}
                  <span class="inline-flex items-center gap-1 text-[11px] font-medium text-subtle"><Download size={10} /> {installed ? 'Switch' : 'Install'}</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}

        {#if pkg.name === 'go'}
          <div class="mt-4 rounded-lg border border-line bg-surface-2/50 p-3">
            <label for="custom-ver-input" class="label">Install a specific Go release</label>
            <div class="flex items-center gap-2">
              <input id="custom-ver-input" type="text" placeholder="e.g. 1.24.10" bind:value={customVersion} class="input flex-1 font-mono text-[13px]" />
              <button
                onclick={() => { const v = customVersion.trim(); if (v) { act('install', v); customVersion = ''; } }}
                disabled={!/^\d+\.\d+(\.\d+)?$/.test(customVersion.trim()) || busy}
                class="btn btn-primary"
              >
                <Download size={14} /> Install
              </button>
            </div>
            <p class="mt-1.5 text-xs text-subtle">Downloaded from go.dev and checked against its published SHA-256.</p>
          </div>
        {/if}
      </div>
    </div>
  </div>

  <ConfirmDialog
    bind:open={confirmUninstall}
    title="Uninstall {pkg.label}?"
    message={pkg.uninstallNote ?? 'It will be removed from the server.'}
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => act('uninstall')}
  />
{/if}
