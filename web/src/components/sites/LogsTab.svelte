<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { ScrollText, RefreshCw, AlertTriangle, Globe, Server, Box } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();

  let site = $state<any>(null);
  let logType = $state('error');
  let logEntries = $state<string[]>([]);
  let logLoading = $state(false);
  let logLoaded = $state(false);
  let logPath = $state('');
  let logExists = $state(false);

  $effect(() => { if (!logLoading && logLoaded === false) { logLoaded = true; loadLogs(); } });

  async function loadLogs() {
    logLoading = true;
    try {
      if (!site) {
        const data = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
        site = data?.site || data;
      }

      if (site?.runtime === 'docker' && site?.server_id) {
        // Docker container logs
        const containerName = `site-${site.name}`;
        const domainBase = site.domain?.split(".")[0] || "";
        const containers = await apiFetch(auth.token, 'GET', `servers/${site.server_id}/docker/containers`);
        const list = containers?.containers || [];
        const container = list.find((c: any) => c.name === containerName)
          || list.find((c: any) => c.name?.includes(site.name) || c.name?.includes(domainBase));
        if (container) {
          const d = await apiFetch(auth.token, 'GET', `servers/${site.server_id}/docker/container/logs?container_id=${container.id}&lines=200`);
          const raw = d?.logs || '';
          logEntries = raw.split('\n').filter((l: string) => l.trim());
          logExists = true;
          logPath = `docker:${container.name}`;
        } else {
          logEntries = ['Container not found'];
          logExists = true;
        }
      } else {
        // Standard site logs
        const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/logs?type=${logType}&limit=300`);
        logEntries = d?.logs || [];
        logPath = d?.path || '';
        logExists = d?.exists || false;
      }
    } catch (e: any) { toasts.error(e.message); } finally { logLoading = false; }
  }

  const isDocker = $derived(site?.runtime === 'docker');
  const logOptions = $derived(isDocker
    ? [{ key: 'container', label: 'Container Logs', icon: Box }]
    : [{ key: 'error', label: 'Nginx Error', icon: AlertTriangle }, { key: 'access', label: 'Nginx Access', icon: Globe }, { key: 'php', label: 'PHP-FPM', icon: Server }]
  );
</script>

<div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
    <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><ScrollText size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">{isDocker ? 'Docker' : 'Site'} Logs</span>{#if logEntries.length > 0}<span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">{logEntries.length} lines</span>{/if}</div>
    <button onclick={() => { logLoaded = false; loadLogs(); }} class="inline-flex items-center gap-1 px-3 py-1.5 rounded-xl text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition"><RefreshCw size={12} class={logLoading ? 'animate-spin' : ''} /> Refresh</button>
  </div>
  <div class="flex gap-1 px-5 pt-3 pb-0 overflow-x-auto scrollbar-hide">
    {#each logOptions as logOpt}
      <button onclick={() => { logType = logOpt.key; logLoaded = false; loadLogs(); }} class="flex items-center gap-2 px-3.5 py-2.5 rounded-t-xl text-xs font-medium transition-all duration-200 whitespace-nowrap border border-b-0 {logType === logOpt.key ? 'bg-white/[0.05] text-slate-200 border-white/[0.08]' : 'text-slate-500 hover:text-slate-400 hover:bg-white/[0.02] border-transparent'}"><svelte:component this={logOpt.icon} size={13} /><span>{logOpt.label}</span></button>
    {/each}
  </div>
  <div class="px-5 py-2 bg-white/[0.01] border-b border-white/[0.04] flex items-center justify-between">
    <span class="text-[0.65rem] text-slate-600">{#if isDocker}Docker container logs{:else if logType === 'error'}Nginx error log{:else if logType === 'access'}Nginx access log{:else if logType === 'php'}PHP-FPM log{/if}</span>
    {#if logPath}<span class="text-[0.6rem] text-slate-600 font-mono truncate max-w-[300px]" title={logPath}>{logPath}</span>{/if}
  </div>
  {#if logLoading}
    <div class="p-6 space-y-2">{#each Array(12) as _, i}<div class="flex items-center gap-3"><div class="h-2 w-16 bg-white/[0.03] rounded animate-pulse flex-shrink-0"></div><div class="h-2.5 bg-gradient-to-r from-white/[0.03] via-white/[0.06] to-white/[0.03] rounded animate-pulse" style="width: {40 + (i * 7) % 50}%"></div></div>{/each}</div>
  {:else if !logExists}
    <div class="flex flex-col items-center justify-center py-14"><ScrollText size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">{isDocker ? 'Container not found' : 'Log file not found'}</p></div>
  {:else if logEntries.length === 0}
    <div class="flex flex-col items-center justify-center py-14"><ScrollText size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">Log is empty</p></div>
  {:else}
    <div class="p-1 max-h-[60vh] overflow-y-auto">
      <pre class="bg-[#0d1117] text-emerald-300 font-mono text-[11px] leading-relaxed p-4 rounded-b-2xl overflow-x-auto whitespace-pre-wrap border border-white/[0.04]">{#each logEntries as entry}{@const isErr = entry.toLowerCase().includes('error') || entry.toLowerCase().includes('fatal') || entry.includes('[crit]')}{@const isWarn = entry.toLowerCase().includes('warn')}{#if isErr}<span class="text-red-400">{entry}</span>{:else if isWarn}<span class="text-amber-400">{entry}</span>{:else}<span class="text-emerald-300/80">{entry}</span>{/if}
{/each}</pre>
    </div>
    <div class="px-5 py-2 border-t border-white/[0.04] bg-white/[0.01] flex items-center justify-between">
      <span class="text-[0.65rem] text-slate-600">Showing last {logEntries.length} lines</span>
    </div>
  {/if}
</div>
