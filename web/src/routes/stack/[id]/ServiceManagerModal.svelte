<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import {
    CheckCircle2, XCircle, Loader2, Download, Check, Plus, Box, Play, Square,
    RotateCw, Trash2, RefreshCw, ArrowUpCircle, X
  } from 'lucide-svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';

  let {
    open,
    pkg,
    serverId = '',
    onClose,
    onChanged
  }: {
    open: boolean;
    pkg: { name: string; label: string; desc: string; icon: any; color?: string; multiVersion?: boolean; versionSupport?: boolean; availableVersions?: string[] } | null;
    serverId: string;
    onClose: () => void;
    onChanged: () => void;
  } = $props();

  let installedVersions = $state<any[]>([]);
  let availableVersions = $state<any[]>([]);
  let customVersion = $state('');
  let loading = $state(true);
  let actionPending = $state('');
  let actionResult = $state<{ type: 'success' | 'error'; message: string } | null>(null);

  $effect(() => {
    if (open && pkg) {
      loadVersions();
    }
  });

  function loadVersions() {
    if (!auth.user || !pkg) return;
    loading = true;
    actionResult = null;

    apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
      package: pkg.name,
      action: 'list-versions'
    }).then((d: any) => {
      installedVersions = d?.data?.versions ?? d?.versions ?? [];
    }).catch(() => {
      installedVersions = [];
    });

    apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
      package: pkg.name,
      action: 'list-available'
    }).then((d: any) => {
      let av = d?.data?.versions ?? d?.versions ?? [];
      if (av.length === 0 && pkg?.availableVersions) {
        av = pkg.availableVersions.map(v => ({
          version: v,
          installed: installedVersions.some(iv => iv.version === v)
        }));
      }
      availableVersions = av;
    }).catch(() => {
      if (pkg?.availableVersions) {
        availableVersions = pkg.availableVersions.map(v => ({
          version: v,
          installed: installedVersions.some(iv => iv.version === v)
        }));
      } else {
        availableVersions = [];
      }
    }).finally(() => {
      loading = false;
    });
  }

  async function doAction(version: string, action: string) {
    if (!auth.user || !pkg) return;
    actionPending = `${version}:${action}`;
    actionResult = null;
    try {
      const result = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
        package: pkg.name,
        action,
        version
      });
      if (result?.ok) {
        actionResult = { type: 'success', message: result?.message || `${action} ${pkg.label} ${version} completed` };
        onChanged();
      } else {
        actionResult = { type: 'error', message: result?.message || `${action} ${pkg.label} ${version} failed` };
      }
      setTimeout(loadVersions, 1500);
    } catch (e: any) {
      actionResult = { type: 'error', message: e.message };
    } finally {
      actionPending = '';
    }
  }

  function statusColor(status: string) {
    switch (status) {
      case 'running': return { label: 'Running', badge: 'badge-success' };
      case 'stopped': return { label: 'Stopped', badge: 'badge-warning' };
      default: return { label: 'Installed', badge: 'badge-info' };
    }
  }

  let uninstallVersion = $state('');
  let confirmUninstall = $state(false);
</script>

{#if open && pkg}
  {@const Icon = pkg.icon || Box}
  <div class="overlay animate-fade-in flex items-end justify-center overflow-auto sm:items-start sm:px-4 sm:py-10" onclick={onClose} role="presentation">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      class="popover animate-pop-in flex max-h-[90dvh] w-full max-w-3xl flex-col max-sm:rounded-b-none sm:max-h-none"
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label="{pkg.label} version manager"
      tabindex="-1"
    >
      <div class="flex shrink-0 items-center justify-between gap-3 border-b border-line px-5 py-3.5">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted">
            <Icon size={16} />
          </div>
          <div class="min-w-0">
            <h2 class="truncate text-[15px] font-semibold text-fg">{pkg.label} versions & service</h2>
            <p class="truncate text-xs text-subtle">{pkg.desc} — install, switch or manage versions</p>
          </div>
        </div>
        <div class="flex items-center gap-1">
          <button onclick={() => loadVersions()} disabled={loading} class="btn btn-ghost btn-icon btn-sm" aria-label="Refresh versions">
            <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
          </button>
          <button onclick={onClose} class="btn btn-ghost btn-icon btn-sm" aria-label="Close">
            <X size={16} />
          </button>
        </div>
      </div>

      <div class="overflow-y-auto px-5 py-4">
        {#if actionResult}
          <div class="mb-4 flex items-center gap-2 rounded-lg px-3 py-2 text-[13px] {actionResult.type === 'success' ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}">
            {#if actionResult.type === 'success'}<CheckCircle2 size={14} class="shrink-0" />{:else}<XCircle size={14} class="shrink-0" />{/if}
            {actionResult.message}
          </div>
        {/if}

        {#if loading}
          <div class="space-y-2">
            {#each Array(3) as _, i (i)}<div class="skeleton h-12"></div>{/each}
          </div>
        {:else}
          <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
            <Check size={14} class="text-success" />
            Installed versions <span class="font-normal text-subtle tabular-nums">({installedVersions.length})</span>
          </h3>

          {#if installedVersions.length > 0}
            <div class="mb-6 divide-y divide-line rounded-lg border border-line">
              {#each installedVersions as ver, i (i)}
                {@const sc = statusColor(ver.status)}
                {@const isPending = actionPending.startsWith(ver.version + ':')}
                <div class="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div class="flex min-w-0 items-center gap-2.5">
                    <span class="font-mono text-[13px] font-semibold text-fg">v{ver.version}</span>
                    <span class="badge {sc.badge}"><span class="dot"></span>{sc.label}</span>
                    {#if ver.path}
                      <span class="truncate font-mono text-xs text-subtle max-sm:hidden">{ver.path}</span>
                    {/if}
                  </div>
                  <div class="flex shrink-0 items-center gap-0.5">
                    {#if ver.status === 'stopped'}
                      <button onclick={() => doAction(ver.version, 'start')} disabled={isPending} title="Start" aria-label="Start" class="btn btn-ghost btn-icon btn-xs text-success">
                        {#if isPending}<Loader2 size={12} class="animate-spin" />{:else}<Play size={12} />{/if}
                      </button>
                    {/if}
                    {#if ver.status === 'running'}
                      <button onclick={() => doAction(ver.version, 'stop')} disabled={isPending} title="Stop" aria-label="Stop" class="btn btn-ghost btn-icon btn-xs text-warning">
                        {#if isPending}<Loader2 size={12} class="animate-spin" />{:else}<Square size={12} />{/if}
                      </button>
                    {/if}
                    <button onclick={() => doAction(ver.version, 'restart')} disabled={isPending} title="Restart" aria-label="Restart" class="btn btn-ghost btn-icon btn-xs">
                      <RotateCw size={12} />
                    </button>
                    <button onclick={() => doAction(ver.version, 'update')} disabled={isPending} title="Update/Rebuild" aria-label="Update" class="btn btn-ghost btn-icon btn-xs">
                      <ArrowUpCircle size={12} />
                    </button>
                    <button
                      onclick={() => { uninstallVersion = ver.version; confirmUninstall = true; }}
                      disabled={isPending}
                      title="Uninstall"
                      aria-label="Uninstall"
                      class="btn btn-danger-ghost btn-icon btn-xs"
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <div class="mb-6 rounded-lg border border-dashed border-line px-4 py-6 text-center">
              <Box size={18} class="mx-auto mb-2 text-subtle" />
              <p class="text-[13px] text-muted">No {pkg.label} versions currently installed</p>
            </div>
          {/if}

          <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
            <Plus size={14} class="text-accent" />
            Install other versions
          </h3>

          {#if availableVersions.length > 0}
            <div class="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
              {#each availableVersions as ver, i (i)}
                {@const installed = ver.installed || installedVersions.some(iv => iv.version === ver.version)}
                {@const isPending = actionPending.startsWith(ver.version + ':install')}
                <button
                  onclick={() => { if (!installed) doAction(ver.version, 'install'); }}
                  disabled={installed || isPending}
                  class="rounded-lg border px-3 py-2.5 text-center transition-colors duration-150 disabled:cursor-not-allowed {installed
                    ? 'border-success/20 bg-success/5'
                    : 'cursor-pointer border-line bg-surface hover:border-accent/40 hover:bg-accent/5'} {isPending ? 'opacity-60' : ''}"
                >
                  <div class="mb-0.5 font-mono text-[13px] font-semibold text-fg">v{ver.version}</div>
                  {#if installed}
                    <span class="inline-flex items-center gap-1 text-[11px] font-medium text-success"><Check size={10} /> Installed</span>
                  {:else if isPending}
                    <span class="inline-flex items-center gap-1 text-[11px] font-medium text-accent"><Loader2 size={10} class="animate-spin" /> Installing…</span>
                  {:else}
                    <span class="inline-flex items-center gap-1 text-[11px] font-medium text-subtle"><Download size={10} /> Install</span>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}

          <div class="mt-4 rounded-lg border border-line bg-surface-2/50 p-3">
            <label for="custom-ver-input" class="label">Install a specific version (or custom tag)</label>
            <div class="flex items-center gap-2">
              <input
                id="custom-ver-input"
                type="text"
                placeholder={pkg.name === 'go' ? 'e.g. 1.24.0' : pkg.name === 'node' ? 'e.g. 20' : 'e.g. 8.4'}
                bind:value={customVersion}
                class="input flex-1 font-mono text-[13px]"
              />
              <button
                onclick={() => {
                  if (customVersion.trim()) {
                    doAction(customVersion.trim(), 'install');
                    customVersion = '';
                  }
                }}
                disabled={!customVersion.trim() || actionPending !== ''}
                class="btn btn-primary"
              >
                <Download size={14} /> Install
              </button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>

  <ConfirmDialog
    bind:open={confirmUninstall}
    title="Uninstall {pkg.label} {uninstallVersion}?"
    message="This version will be removed from the server."
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => { if (uninstallVersion) doAction(uninstallVersion, 'uninstall'); }}
  />
{/if}
