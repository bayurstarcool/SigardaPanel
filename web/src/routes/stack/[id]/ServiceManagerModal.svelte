<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import {
    CheckCircle2, XCircle, Loader2, Download, Check, Plus, Box, Play, Square,
    RotateCw, Trash2, RefreshCw, ChevronRight, Layers, ArrowUpCircle, Info
  } from 'lucide-svelte';

  let {
    open,
    pkg,
    serverId = '',
    onClose,
    onChanged
  }: {
    open: boolean;
    pkg: { name: string; label: string; desc: string; icon: any; color: string; multiVersion?: boolean; versionSupport?: boolean; availableVersions?: string[] } | null;
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
    if (!auth.token || !pkg) return;
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
    if (!auth.token || !pkg) return;
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
      case 'running': return { label: 'Running', bg: 'bg-emerald-500/10', text: 'text-emerald-400', border: 'border-emerald-500/20', dot: 'bg-emerald-400' };
      case 'stopped': return { label: 'Stopped', bg: 'bg-yellow-500/10', text: 'text-yellow-400', border: 'border-yellow-500/20', dot: 'bg-yellow-400' };
      default: return { label: 'Installed', bg: 'bg-sky-500/10', text: 'text-sky-400', border: 'border-sky-500/20', dot: 'bg-sky-400' };
    }
  }
</script>

{#if open && pkg}
  {@const Icon = pkg.icon || Box}
  <div class="fixed inset-0 z-50 flex items-start justify-center pt-10 pb-10 overflow-auto bg-black/60 backdrop-blur-sm" onclick={onClose} role="presentation">
    <div class="w-full max-w-3xl bg-[var(--bg-secondary)] border border-white/[0.06] rounded-2xl shadow-2xl overflow-hidden" onclick={(e) => e.stopPropagation()} role="dialog">
      <!-- Header -->
      <div class="p-6 border-b border-white/[0.06] flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-{pkg.color}-500/10 flex items-center justify-center border border-white/[0.06]">
            <Icon size={18} class="text-{pkg.color}-400" />
          </div>
          <div>
            <h2 class="font-semibold text-white">{pkg.label} Version & Service Manager</h2>
            <p class="text-xs text-slate-500">{pkg.desc} — install, switch, or manage versions</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button onclick={() => loadVersions()} disabled={loading}
            class="p-2 rounded-xl bg-white/[0.03] border border-white/[0.06] text-slate-400 hover:text-white hover:bg-white/[0.06] transition disabled:opacity-50 cursor-pointer">
            <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
          </button>
          <button onclick={onClose}
            class="p-2 rounded-xl bg-white/[0.03] border border-white/[0.06] text-slate-400 hover:text-white hover:bg-white/[0.06] transition cursor-pointer">
            <XCircle size={14} />
          </button>
        </div>
      </div>

      <!-- Body -->
      <div class="p-6">
        <!-- Toast -->
        {#if actionResult}
          <div class="mb-5 p-4 rounded-xl flex items-center gap-2 text-sm {actionResult.type === 'success' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-400' : 'bg-red-500/10 border border-red-500/20 text-red-400'}">
            {#if actionResult.type === 'success'}<CheckCircle2 size={16} />{:else}<XCircle size={16} />{/if}
            {actionResult.message}
          </div>
        {/if}

        {#if loading}
          <div class="flex items-center justify-center py-12">
            <Loader2 size={20} class="text-sky-400 animate-spin" />
            <span class="ml-2 text-sm text-slate-400">Loading version details...</span>
          </div>
        {:else}
          <!-- Installed versions -->
          <h3 class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3 flex items-center gap-2">
            <Check size={14} class="text-emerald-400" />
            Installed Versions ({installedVersions.length})
          </h3>

          {#if installedVersions.length > 0}
            <div class="space-y-2 mb-6">
              {#each installedVersions as ver}
                {@const sc = statusColor(ver.status)}
                {@const isPending = actionPending.startsWith(ver.version + ':')}
                <div class="flex items-center justify-between p-3.5 rounded-xl bg-white/[0.02] border border-white/[0.06] hover:border-white/[0.1] transition">
                  <div class="flex items-center gap-3">
                    <span class="font-mono text-sm font-semibold text-white">v{ver.version}</span>
                    <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[0.65rem] font-medium {sc.bg} {sc.text} border {sc.border}">
                      <span class="w-1.5 h-1.5 rounded-full {sc.dot}"></span>
                      {sc.label}
                    </span>
                    {#if ver.path}
                      <span class="font-mono text-xs text-slate-500 hidden sm:inline">{ver.path}</span>
                    {/if}
                  </div>

                  <div class="flex items-center gap-1.5">
                    {#if ver.status === 'stopped'}
                      <button onclick={() => doAction(ver.version, 'start')} disabled={isPending}
                        title="Start"
                        class="p-1.5 rounded-lg bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/20 transition disabled:opacity-40 cursor-pointer">
                        {#if isPending}<Loader2 size={11} class="animate-spin" />{:else}<Play size={11} />{/if}
                      </button>
                    {/if}
                    {#if ver.status === 'running'}
                      <button onclick={() => doAction(ver.version, 'stop')} disabled={isPending}
                        title="Stop"
                        class="p-1.5 rounded-lg bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20 border border-yellow-500/20 transition disabled:opacity-40 cursor-pointer">
                        {#if isPending}<Loader2 size={11} class="animate-spin" />{:else}<Square size={11} />{/if}
                      </button>
                    {/if}
                    <button onclick={() => doAction(ver.version, 'restart')} disabled={isPending}
                      title="Restart"
                      class="p-1.5 rounded-lg bg-white/[0.03] text-slate-400 hover:text-white hover:bg-white/[0.06] border border-white/[0.06] transition disabled:opacity-40 cursor-pointer">
                      <RotateCw size={11} />
                    </button>
                    <button onclick={() => doAction(ver.version, 'update')} disabled={isPending}
                      title="Update/Rebuild"
                      class="p-1.5 rounded-lg bg-sky-500/10 text-sky-400 hover:bg-sky-500/20 border border-sky-500/20 transition disabled:opacity-40 cursor-pointer">
                      <ArrowUpCircle size={11} />
                    </button>
                    <button onclick={() => { if (confirm(`Uninstall ${pkg.label} ${ver.version}?`)) doAction(ver.version, 'uninstall'); }} disabled={isPending}
                      title="Uninstall"
                      class="p-1.5 rounded-lg bg-red-500/5 text-red-400/60 hover:bg-red-500/10 hover:text-red-400 border border-red-500/10 transition disabled:opacity-40 cursor-pointer">
                      <Trash2 size={11} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <div class="p-6 rounded-xl bg-white/[0.02] border border-white/[0.06] text-center mb-6">
              <Box size={24} class="mx-auto mb-2 text-slate-500" />
              <p class="text-sm text-slate-400">No {pkg.label} versions currently installed</p>
            </div>
          {/if}

          <!-- Available / Install Other Versions -->
          <h3 class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3 flex items-center gap-2">
            <Plus size={14} class="text-sky-400" />
            Install Other / Newer Versions
          </h3>

          {#if availableVersions.length > 0}
            <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2 mb-4">
              {#each availableVersions as ver}
                {@const installed = ver.installed || installedVersions.some(iv => iv.version === ver.version)}
                {@const isPending = actionPending.startsWith(ver.version + ':install')}
                <button
                  onclick={() => { if (!installed) doAction(ver.version, 'install'); }}
                  disabled={installed || isPending}
                  class="rounded-xl border p-3 text-center transition-all duration-200 {installed
                    ? 'border-emerald-500/15 bg-emerald-500/5 cursor-default'
                    : 'border-white/[0.06] bg-white/[0.02] hover:border-indigo-500/30 hover:bg-indigo-500/5 cursor-pointer'} disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <div class="font-mono text-sm font-semibold text-white mb-1">
                    v{ver.version}
                  </div>
                  {#if installed}
                    <span class="text-[0.65rem] text-emerald-400 font-medium inline-flex items-center gap-1">
                      <Check size={10} /> Installed
                    </span>
                  {:else if isPending}
                    <span class="text-[0.65rem] text-sky-400 font-medium inline-flex items-center gap-1">
                      <Loader2 size={10} class="animate-spin" /> Installing...
                    </span>
                  {:else}
                    <span class="text-[0.65rem] text-slate-400 hover:text-sky-300 font-medium inline-flex items-center gap-1">
                      <Download size={10} /> Click to Install
                    </span>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}

          <!-- Custom version input -->
          <div class="mt-4 p-4 rounded-xl bg-white/[0.02] border border-white/[0.06]">
            <label for="custom-ver-input" class="block text-xs font-semibold text-slate-400 mb-2">
              Install Specific Version (or custom tag)
            </label>
            <div class="flex items-center gap-2">
              <input
                id="custom-ver-input"
                type="text"
                placeholder={pkg.name === 'go' ? 'e.g. 1.24.0' : pkg.name === 'node' ? 'e.g. 20' : 'e.g. 8.4'}
                bind:value={customVersion}
                class="flex-1 bg-white/[0.05] border border-white/[0.08] rounded-xl px-3 py-2 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50"
              />
              <button
                onclick={() => {
                  if (customVersion.trim()) {
                    doAction(customVersion.trim(), 'install');
                    customVersion = '';
                  }
                }}
                disabled={!customVersion.trim() || actionPending !== ''}
                class="px-4 py-2 rounded-xl bg-sky-500/20 text-sky-300 hover:bg-sky-500/30 border border-sky-500/30 text-xs font-medium transition cursor-pointer disabled:opacity-40"
              >
                Install
              </button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
