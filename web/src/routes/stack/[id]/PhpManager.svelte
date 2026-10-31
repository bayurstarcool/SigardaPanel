<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { CheckCircle2, XCircle, Loader2, Download, Check, Plus, FileCode, Play, Square, RotateCw, Trash2, RefreshCw, X } from 'lucide-svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';

  let { open, serverId = '', onClose, onInstall }: { open: boolean; serverId: string; onClose: () => void; onInstall: (version: string) => void } = $props();

  let installedVersions = $state<any[]>([]);
  let availableVersions = $state<any[]>([]);
  let loading = $state(true);
  let actionPending = $state('');
  let actionResult = $state<{ type: 'success' | 'error'; message: string } | null>(null);

  $effect(() => {
    if (open) loadVersions();
  });

  function loadVersions() {
    if (!auth.user) return;
    loading = true;
    actionResult = null;

    apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
      package: 'php', action: 'list-versions'
    }).then((d: any) => {
      installedVersions = d?.data?.versions ?? d?.versions ?? [];
    }).catch(() => {});

    apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
      package: 'php', action: 'list-available'
    }).then((d: any) => {
      availableVersions = d?.data?.versions ?? d?.versions ?? [];
    }).catch(() => {})
      .finally(() => loading = false);
  }

  async function doAction(version: string, action: string) {
    if (!auth.user) return;
    actionPending = `${version}:${action}`;
    actionResult = null;
    try {
      const result = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, {
        package: 'php', action, version
      });
      if (result?.ok) {
        actionResult = { type: 'success', message: result?.message || `${action} PHP ${version} completed` };
        if (action === 'install') onInstall(version);
      } else {
        actionResult = { type: 'error', message: result?.message || `${action} PHP ${version} failed` };
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

  function isInstalled(version: string) {
    return installedVersions.some(v => v.version === version);
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
            <h2 class="text-[15px] font-semibold text-fg">PHP version manager</h2>
            <p class="truncate text-xs text-subtle">Install, manage and switch between PHP versions</p>
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

      <div class="overflow-y-auto px-5 py-4 sm:max-h-[65vh]">
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
          {#if installedVersions.length > 0}
            <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
              <Check size={14} class="text-success" /> Installed
            </h3>
            <div class="mb-6 divide-y divide-line rounded-lg border border-line">
              {#each installedVersions as ver, i (i)}
                {@const sc = statusColor(ver.status)}
                {@const isPending = actionPending.startsWith(ver.version + ':')}
                <div class="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div class="min-w-0">
                    <div class="text-sm font-medium text-fg">PHP {ver.version}</div>
                    <div class="truncate font-mono text-xs text-subtle">{ver.path}</div>
                  </div>
                  <div class="flex shrink-0 items-center gap-0.5">
                    <span class="badge {sc.badge} mr-1.5"><span class="dot"></span>{sc.label}</span>
                    {#if ver.status === 'stopped'}
                      <button onclick={() => doAction(ver.version, 'start')} disabled={isPending} aria-label="Start" title="Start" class="btn btn-ghost btn-icon btn-xs text-success">
                        {#if isPending}<Loader2 size={12} class="animate-spin" />{:else}<Play size={12} />{/if}
                      </button>
                    {/if}
                    {#if ver.status === 'running'}
                      <button onclick={() => doAction(ver.version, 'stop')} disabled={isPending} aria-label="Stop" title="Stop" class="btn btn-ghost btn-icon btn-xs text-warning">
                        {#if isPending}<Loader2 size={12} class="animate-spin" />{:else}<Square size={12} />{/if}
                      </button>
                    {/if}
                    <button onclick={() => doAction(ver.version, 'restart')} disabled={isPending} aria-label="Restart" title="Restart" class="btn btn-ghost btn-icon btn-xs">
                      <RotateCw size={12} />
                    </button>
                    <button
                      onclick={() => { uninstallVersion = ver.version; confirmUninstall = true; }}
                      disabled={isPending}
                      aria-label="Uninstall"
                      title="Uninstall"
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
              <FileCode size={18} class="mx-auto mb-2 text-subtle" />
              <p class="text-[13px] text-muted">No PHP versions installed</p>
            </div>
          {/if}

          <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
            <Plus size={14} class="text-accent" /> Available versions
          </h3>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
            {#each availableVersions as ver, i (i)}
              {@const installed = ver.installed}
              {@const isPending = actionPending.startsWith(ver.version + ':install')}
              <button
                onclick={() => { if (!installed) doAction(ver.version, 'install'); }}
                disabled={installed || isPending}
                class="rounded-lg border px-3 py-2.5 text-center transition-colors duration-150 disabled:cursor-not-allowed {installed
                  ? 'border-success/20 bg-success/5'
                  : 'cursor-pointer border-line bg-surface hover:border-accent/40 hover:bg-accent/5'} {isPending ? 'opacity-60' : ''}"
              >
                {#if isPending}
                  <Loader2 size={14} class="mx-auto mb-1 animate-spin text-accent" />
                {:else if installed}
                  <CheckCircle2 size={14} class="mx-auto mb-1 text-success" />
                {:else}
                  <Download size={14} class="mx-auto mb-1 text-subtle" />
                {/if}
                <div class="text-xs font-semibold {installed ? 'text-success' : 'text-fg'}">PHP {ver.version}</div>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <div class="flex shrink-0 justify-end border-t border-line px-5 py-3">
        <button onclick={onClose} class="btn btn-secondary btn-sm">Close</button>
      </div>
    </div>
  </div>

  <ConfirmDialog
    bind:open={confirmUninstall}
    title="Uninstall PHP {uninstallVersion}?"
    message="This PHP version will be removed from the server. Sites using it will stop working."
    confirmLabel="Uninstall"
    destructive
    onconfirm={() => { if (uninstallVersion) doAction(uninstallVersion, 'uninstall'); }}
  />
{/if}
