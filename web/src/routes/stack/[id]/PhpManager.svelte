<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { CheckCircle2, XCircle, Loader2, Download, Check, Plus, FileCode, Play, Square, RotateCw, Trash2, RefreshCw } from 'lucide-svelte';
  import { onMount } from 'svelte';

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
      case 'running': return { label: 'Running', bg: 'bg-emerald-500/10', text: 'text-emerald-400', border: 'border-emerald-500/20', dot: 'bg-emerald-400' };
      case 'stopped': return { label: 'Stopped', bg: 'bg-yellow-500/10', text: 'text-yellow-400', border: 'border-yellow-500/20', dot: 'bg-yellow-400' };
      default: return { label: 'Installed', bg: 'bg-sky-500/10', text: 'text-sky-400', border: 'border-sky-500/20', dot: 'bg-sky-400' };
    }
  }

  function isInstalled(version: string) {
    return installedVersions.some(v => v.version === version);
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-start justify-center pt-10 pb-10 overflow-auto bg-black/60 backdrop-blur-sm" onclick={onClose} role="presentation">
    <div class="w-full max-w-3xl bg-[var(--bg-secondary)] border border-white/[0.06] rounded-2xl shadow-2xl overflow-hidden" onclick={(e) => e.stopPropagation()} role="dialog">
      <!-- Header -->
      <div class="p-6 border-b border-white/[0.06] flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-400/20 to-purple-500/20 flex items-center justify-center border border-white/[0.06]">
            <FileCode size={18} class="text-indigo-400" />
          </div>
          <div>
            <h2 class="font-semibold text-white">PHP Version Manager</h2>
            <p class="text-xs text-slate-500">Install, manage, and switch between PHP versions</p>
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
      <div class="p-6 max-h-[65vh] overflow-y-auto">
        {#if actionResult}
          <div class="mb-4 p-3 rounded-xl flex items-center gap-2 text-xs {actionResult.type === 'success' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-400' : 'bg-red-500/10 border border-red-500/20 text-red-400'}">
            {#if actionResult.type === 'success'}<CheckCircle2 size={14} />{:else}<XCircle size={14} />{/if}
            {actionResult.message}
          </div>
        {/if}

        {#if loading}
          <div class="flex items-center justify-center py-16">
            <Loader2 size={20} class="text-indigo-400 animate-spin" />
          </div>
        {:else}
          <!-- Installed -->
          {#if installedVersions.length > 0}
            <h3 class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3 flex items-center gap-2">
              <Check size={14} class="text-emerald-400" />
              Installed
            </h3>
            <div class="space-y-2 mb-6">
              {#each installedVersions as ver}
                {@const sc = statusColor(ver.status)}
                {@const isPending = actionPending.startsWith(ver.version + ':')}
                <div class="flex items-center justify-between p-3 rounded-xl bg-white/[0.02] border border-white/[0.06]">
                  <div class="flex items-center gap-3">
                    <div class="w-9 h-9 rounded-lg bg-indigo-500/10 flex items-center justify-center border border-white/[0.06]">
                      <span class="text-sm font-bold text-indigo-400">{ver.version}</span>
                    </div>
                    <div>
                      <div class="text-sm font-medium text-white">PHP {ver.version}</div>
                      <div class="text-[0.6rem] text-slate-500 font-mono">{ver.path}</div>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5">
                    <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[0.6rem] font-medium {sc.bg} {sc.text} border {sc.border}">
                      <span class="w-1 h-1 rounded-full {sc.dot} {ver.status === 'running' ? 'animate-pulse' : ''}"></span>
                      {sc.label}
                    </span>
                    {#if ver.status === 'stopped'}
                      <button onclick={() => doAction(ver.version, 'start')} disabled={isPending}
                        class="p-1.5 rounded-lg bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/20 transition disabled:opacity-40 cursor-pointer">
                        {#if isPending}<Loader2 size={11} class="animate-spin" />{:else}<Play size={11} />{/if}
                      </button>
                    {/if}
                    {#if ver.status === 'running'}
                      <button onclick={() => doAction(ver.version, 'stop')} disabled={isPending}
                        class="p-1.5 rounded-lg bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20 border border-yellow-500/20 transition disabled:opacity-40 cursor-pointer">
                        {#if isPending}<Loader2 size={11} class="animate-spin" />{:else}<Square size={11} />{/if}
                      </button>
                    {/if}
                    <button onclick={() => doAction(ver.version, 'restart')} disabled={isPending}
                      class="p-1.5 rounded-lg bg-white/[0.03] text-slate-400 hover:text-white hover:bg-white/[0.06] border border-white/[0.06] transition disabled:opacity-40 cursor-pointer">
                      <RotateCw size={11} />
                    </button>
                    <button onclick={() => { if (confirm(`Uninstall PHP ${ver.version}?`)) doAction(ver.version, 'uninstall'); }} disabled={isPending}
                      class="p-1.5 rounded-lg bg-red-500/5 text-red-400/60 hover:bg-red-500/10 hover:text-red-400 border border-red-500/10 transition disabled:opacity-40 cursor-pointer">
                      <Trash2 size={11} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <div class="p-6 rounded-xl bg-white/[0.02] border border-white/[0.06] text-center mb-6">
              <FileCode size={24} class="mx-auto mb-2 text-slate-500" />
              <p class="text-sm text-slate-400">No PHP versions installed</p>
            </div>
          {/if}

          <!-- Available -->
          <h3 class="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3 flex items-center gap-2">
            <Plus size={14} class="text-sky-400" />
            Available versions
          </h3>
          <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
            {#each availableVersions as ver}
              {@const installed = ver.installed}
              {@const isPending = actionPending.startsWith(ver.version + ':install')}
              <button
                onclick={() => { if (!installed) doAction(ver.version, 'install'); }}
                disabled={installed || isPending}
                class="rounded-xl border p-3 text-center transition-all duration-200 {installed
                  ? 'border-emerald-500/15 bg-emerald-500/5 cursor-default'
                  : 'border-white/[0.06] bg-white/[0.02] hover:border-indigo-500/30 hover:bg-indigo-500/5 cursor-pointer'} disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {#if isPending}
                  <Loader2 size={16} class="mx-auto mb-1 text-indigo-400 animate-spin" />
                {:else if installed}
                  <CheckCircle2 size={16} class="mx-auto mb-1 text-emerald-400" />
                {:else}
                  <Download size={16} class="mx-auto mb-1 text-slate-500 group-hover:text-indigo-400" />
                {/if}
                <div class="text-xs font-semibold {installed ? 'text-emerald-400' : 'text-white'}">PHP {ver.version}</div>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="p-4 border-t border-white/[0.06] flex justify-end">
        <button onclick={onClose}
          class="px-4 py-2 rounded-xl bg-white/[0.05] border border-white/[0.08] text-sm text-slate-300 hover:bg-white/[0.08] hover:text-white transition cursor-pointer">
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
