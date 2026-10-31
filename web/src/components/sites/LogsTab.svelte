<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import { ScrollText, RefreshCw, TriangleAlert, Globe, Server, Box } from 'lucide-svelte';
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
    ? [{ key: 'container', label: 'Container logs', icon: Box }]
    : [{ key: 'error', label: 'Nginx error', icon: TriangleAlert }, { key: 'access', label: 'Nginx access', icon: Globe }, { key: 'php', label: 'PHP-FPM', icon: Server }]
  );

  function lineClass(entry: string): string {
    const l = entry.toLowerCase();
    if (l.includes('error') || l.includes('fatal') || entry.includes('[crit]')) return 'text-danger';
    if (l.includes('warn')) return 'text-warning';
    return 'text-muted';
  }
</script>

<div class="card overflow-hidden">
  <div class="card-header">
    <div class="flex items-center gap-2">
      <ScrollText size={16} class="text-subtle" />
      <h3 class="card-title">{isDocker ? 'Docker' : 'Site'} logs</h3>
      {#if logEntries.length > 0}<span class="badge tabular-nums">{logEntries.length} lines</span>{/if}
    </div>
    <button onclick={() => { logLoaded = false; loadLogs(); }} class="btn btn-ghost btn-sm"><RefreshCw size={14} class={logLoading ? 'animate-spin' : ''} /> Refresh</button>
  </div>
  <div class="flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-2.5">
    <div class="segmented">
      {#each logOptions as logOpt (logOpt.key)}
        <button onclick={() => { logType = logOpt.key; logLoaded = false; loadLogs(); }} class="segment {logType === logOpt.key || isDocker ? 'segment-active' : ''}">
          <logOpt.icon size={12} /> {logOpt.label}
        </button>
      {/each}
    </div>
    {#if logPath}<span class="max-w-[300px] truncate font-mono text-xs text-subtle" title={logPath}>{logPath}</span>{/if}
  </div>
  {#if logLoading}
    <div class="space-y-2 p-5">{#each Array(12) as _, i (i)}<div class="skeleton h-2.5" style="width: {40 + (i * 7) % 50}%"></div>{/each}</div>
  {:else if !logExists}
    <EmptyState compact icon={ScrollText} title={isDocker ? 'Container not found' : 'Log file not found'} />
  {:else if logEntries.length === 0}
    <EmptyState compact icon={ScrollText} title="Log is empty" />
  {:else}
    <div class="max-h-[60vh] overflow-y-auto bg-surface-2/40">
      <pre class="overflow-x-auto p-4 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">{#each logEntries as entry, i (i)}<span class={lineClass(entry)}>{entry}</span>
{/each}</pre>
    </div>
    <div class="border-t border-line px-5 py-2 text-xs text-subtle tabular-nums">Showing last {logEntries.length} lines</div>
  {/if}
</div>
