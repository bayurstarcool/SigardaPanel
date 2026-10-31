<script lang="ts">
  import { apiFetch } from '$lib/api';
  import { auth } from '$lib/auth.svelte';
  import { toasts } from '$lib/toast.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { onMount } from 'svelte';
  import { Zap, Terminal, Activity, Box, RefreshCw, Play, Square, RotateCcw, Settings, LoaderCircle, Trash2, Globe } from 'lucide-svelte';

  let { siteId }: { siteId: number } = $props();

  // ─── Core state ───
  let site = $state<any>(null);
  let loading = $state(true);
  let actionBusy = $state(false);

  // ─── Docker state ───
  let dockerContainer = $state<any>(null);
  let dockerLoading = $state(false);
  let dockerTab = $state<'manage' | 'terminal' | 'config'>('manage');

  // ─── Exec terminal ───
  let execCmd = $state('');
  let execOutput = $state('');
  let execRunning = $state(false);

  // ─── Config editor (docker) ───
  let configImage = $state('');
  let configEnv = $state('');
  let configBusy = $state(false);

  // ─── Non-docker app state ───
  let appBusy = $state(false);

  // ─── Derived ───
  const isDocker = $derived(site?.runtime === 'docker');
  const isAppRuntime = $derived(site && !['static', 'php'].includes(site.runtime));
  const isRunning = $derived(
    isDocker ? dockerContainer?.state === 'running' : site?.app_status === 'running'
  );

  onMount(async () => {
    try {
      const data = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
      site = data?.site || data;
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      loading = false;
    }
    if (site?.runtime === 'docker' && site?.server_id) {
      await loadDockerContainer();
    }
  });

  // ─── Docker container ───
  async function loadDockerContainer() {
    dockerLoading = true;
    try {
      const containers = await apiFetch(auth.token, 'GET', `servers/${site.server_id}/docker/containers`);
      const list = containers?.containers || [];
      const containerName = `site-${site.name}`;
      const domainBase = site.domain?.split('.')[0] || '';
      dockerContainer =
        list.find((c: any) => c.name === containerName) ||
        list.find((c: any) => c.name?.includes(site.name) || c.name?.includes(domainBase));
      if (dockerContainer) {
        configImage = dockerContainer.image || '';
      }
    } catch {
      /* silent */
    }
    dockerLoading = false;
  }

  async function dockerAction(action: string) {
    if (!dockerContainer) return;
    const msgs: Record<string, string> = {
      start: `Start container ${dockerContainer.name}?`,
      stop: `Stop container ${dockerContainer.name}?`,
      restart: `Restart container ${dockerContainer.name}?`,
      remove: `DELETE container ${dockerContainer.name}? This cannot be undone!`,
    };
    if (!confirm(msgs[action] || `${action} container?`)) return;
    actionBusy = true;
    try {
      if (action === 'remove') {
        await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/remove`, {
          container_id: dockerContainer.id,
          force: true,
        });
        dockerContainer = null;
        toasts.success('Container removed');
      } else {
        await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/${action}`, {
          container_id: dockerContainer.id,
        });
        toasts.success(`Container ${action}ed`);
        setTimeout(loadDockerContainer, 2000);
      }
    } catch (e: any) {
      toasts.error(e.message);
    }
    actionBusy = false;
  }

  // ─── Exec terminal ───
  async function execCommand() {
    if (!execCmd.trim() || !dockerContainer) return;
    execRunning = true;
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/exec`, {
        container_id: dockerContainer.id,
        command: execCmd,
      });
      execOutput += `\n$ ${execCmd}\n${d?.output || '(no output)'}\n`;
      execCmd = '';
    } catch (e: any) {
      execOutput += `\n$ ${execCmd}\nERROR: ${e.message}\n`;
      execCmd = '';
    }
    execRunning = false;
  }

  // ─── Rebuild Container ───
  async function rebuildContainer() {
    if (!confirm(`Rebuild container with image ${configImage}? This will stop, remove, and recreate the container.`)) return;
    configBusy = true;
    try {
      toasts.success('Pulling image...');
      await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/image/pull`, { image: configImage });

      if (dockerContainer) {
        await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/stop`, { container_id: dockerContainer.id }).catch(() => {});
        await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/remove`, { container_id: dockerContainer.id, force: true }).catch(() => {});
      }

      const ports = site.proxy_pass_url ? [site.proxy_pass_url.replace('http://127.0.0.1:', '') + ':80'] : [];
      await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/create`, {
        image: configImage,
        name: `site-${site.name}`,
        restart: 'unless-stopped',
        ports: ports.length ? ports : undefined,
        env: configEnv
          ? Object.fromEntries(
              configEnv
                .split('\n')
                .filter(Boolean)
                .map((l: string) => {
                  const [k, ...v] = l.split('=');
                  return [k.trim(), v.join('=').trim()];
                })
            )
          : undefined,
        auto_start: true,
      });
      toasts.success('Container rebuilt');
      setTimeout(loadDockerContainer, 3000);
    } catch (e: any) {
      toasts.error(e.message);
    }
    configBusy = false;
  }

  // ─── Non-docker app actions ───
  async function appAction(action: 'start' | 'stop' | 'restart') {
    if (!site) return;
    const msgs: Record<string, string> = {
      start: 'Start this app?',
      stop: 'Stop this app?',
      restart: 'Restart this app?',
    };
    if (!confirm(msgs[action] || `${action} app?`)) return;
    appBusy = true;
    try {
      await apiFetch(auth.token, 'POST', `sites/${siteId}/app/${action}`);
      toasts.success(`App ${action}ed`);
      setTimeout(async () => {
        try {
          const d: any = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
          site = d?.site || d;
        } catch {}
      }, 2000);
    } catch (e: any) {
      toasts.error(e.message);
    }
    appBusy = false;
  }

  const dockerTabs = [
    { key: 'manage', label: 'Manage', icon: Box },
    { key: 'terminal', label: 'Terminal', icon: Terminal },
    { key: 'config', label: 'Config', icon: Settings },
  ] as const;
</script>

{#snippet field(label: string, value: string, mono = false)}
  <div class="min-w-0 rounded-lg border border-line bg-surface-2/50 p-3">
    <div class="stat-label">{label}</div>
    <div class="mt-1 truncate text-[13px] text-fg {mono ? 'font-mono' : ''}" title={value}>{value}</div>
  </div>
{/snippet}

{#if loading}
  <div class="space-y-3">
    {#each Array(3) as _, i (i)}<div class="skeleton h-20 rounded-xl"></div>{/each}
  </div>
{:else if !site}
  <div class="card"><EmptyState icon={Globe} title="Site not found" /></div>
{:else if !isAppRuntime}
  <div class="card">
    <EmptyState icon={Settings} title="Not available" description="App process management is not available for {site.runtime} sites." />
  </div>
{:else}
  <div class="space-y-4">
    {#if isDocker}
      <!-- Docker container -->
      <div class="card">
        <div class="card-header">
          <div class="flex min-w-0 items-center gap-2">
            <Box size={16} class="shrink-0 text-subtle" />
            <div class="min-w-0">
              <h3 class="card-title">Docker container</h3>
              <p class="truncate text-xs text-subtle">{site.domain}</p>
            </div>
          </div>
          {#if dockerLoading}
            <LoaderCircle size={16} class="animate-spin text-subtle" />
          {:else if dockerContainer}
            <span class="badge {isRunning ? 'badge-success' : ''}"><span class="dot"></span>{dockerContainer.state}</span>
          {:else}
            <span class="badge badge-danger">not found</span>
          {/if}
        </div>

        {#if dockerContainer}
          <div class="card-body space-y-4">
            <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
              {@render field('Container', dockerContainer.name, true)}
              {@render field('Image', dockerContainer.image, true)}
              {@render field('ID', dockerContainer.id?.slice(0, 12) ?? '—', true)}
              {@render field('Ports', dockerContainer.ports || '—', true)}
            </div>
            <div class="flex flex-wrap gap-2">
              {#if !isRunning}
                <button onclick={() => dockerAction('start')} disabled={actionBusy} class="btn btn-primary btn-sm">
                  {#if actionBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if} Start
                </button>
              {:else}
                <button onclick={() => dockerAction('stop')} disabled={actionBusy} class="btn btn-secondary btn-sm">
                  {#if actionBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Square size={14} />{/if} Stop
                </button>
                <button onclick={() => dockerAction('restart')} disabled={actionBusy} class="btn btn-secondary btn-sm">
                  {#if actionBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCcw size={14} />{/if} Restart
                </button>
              {/if}
              <button onclick={() => dockerAction('remove')} disabled={actionBusy} class="btn btn-danger-ghost btn-sm"><Trash2 size={14} /> Remove</button>
              <button onclick={loadDockerContainer} disabled={dockerLoading} class="btn btn-ghost btn-sm">
                <RefreshCw size={14} class={dockerLoading ? 'animate-spin' : ''} /> Refresh
              </button>
            </div>
          </div>
        {:else if !dockerLoading}
          <EmptyState compact icon={Box} title="Container not found" />
        {/if}
      </div>

      {#if dockerContainer}
        <div class="segmented">
          {#each dockerTabs as tab (tab.key)}
            <button onclick={() => (dockerTab = tab.key)} class="segment {dockerTab === tab.key ? 'segment-active' : ''}">
              <tab.icon size={12} /> {tab.label}
            </button>
          {/each}
        </div>

        {#if dockerTab === 'manage'}
          <div class="card">
            <div class="card-header">
              <h3 class="card-title">Container overview</h3>
              <Activity size={16} class="text-subtle" />
            </div>
            <div class="card-body grid grid-cols-1 gap-3 sm:grid-cols-2">
              {@render field('Runtime', site.runtime)}
              {@render field('Domain', site.domain, true)}
              {@render field('Status', isRunning ? 'Running' : 'Stopped')}
              {@render field('Created', dockerContainer.created ? new Date(dockerContainer.created * 1000).toLocaleDateString() : '—')}
            </div>
          </div>
        {:else if dockerTab === 'terminal'}
          <div class="card overflow-hidden">
            <div class="card-header">
              <div class="flex items-center gap-2">
                <Terminal size={16} class="text-subtle" />
                <h3 class="card-title">Container exec</h3>
              </div>
              <span class="truncate font-mono text-xs text-subtle">{dockerContainer.name}</span>
            </div>
            <pre class="max-h-[40vh] overflow-y-auto bg-surface-2/50 p-4 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-fg">{execOutput || '$ '}</pre>
            <div class="flex gap-2 border-t border-line px-4 py-3">
              <label for="exec-cmd" class="sr-only">Command</label>
              <input
                id="exec-cmd"
                bind:value={execCmd}
                onkeydown={(e) => { if (e.key === 'Enter') execCommand(); }}
                placeholder="Enter command…"
                class="input font-mono text-[13px]"
                autocomplete="off"
                spellcheck="false"
              />
              <button onclick={execCommand} disabled={execRunning || !execCmd.trim()} class="btn btn-primary">
                {#if execRunning}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if} Run
              </button>
            </div>
          </div>
        {:else if dockerTab === 'config'}
          <div class="card">
            <div class="card-header">
              <h3 class="card-title">Container configuration</h3>
              <Settings size={16} class="text-subtle" />
            </div>
            <div class="card-body space-y-4">
              <div>
                <label for="cfg-image" class="label">Docker image</label>
                <input id="cfg-image" bind:value={configImage} class="input font-mono text-[13px]" />
              </div>
              <div>
                <label for="cfg-env" class="label">Environment variables <span class="text-subtle">(one per line: KEY=value)</span></label>
                <textarea id="cfg-env" bind:value={configEnv} rows={4} class="input resize-none font-mono text-[13px]" placeholder="APP_ENV=production&#10;PORT=3000"></textarea>
              </div>
              <div class="flex flex-wrap items-center gap-3">
                <button onclick={rebuildContainer} disabled={configBusy || !configImage} class="btn btn-primary btn-sm">
                  {#if configBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<RefreshCw size={14} />{/if} Rebuild container
                </button>
                <span class="text-xs text-subtle">Pull image → Stop → Remove → Create → Start</span>
              </div>
            </div>
          </div>
        {/if}
      {/if}
    {:else}
      <!-- Node / Go / Python app process -->
      <div class="card">
        <div class="card-header">
          <div class="flex min-w-0 items-center gap-2">
            <Zap size={16} class="shrink-0 text-subtle" />
            <div class="min-w-0">
              <h3 class="card-title"><span class="capitalize">{site.runtime}</span> application</h3>
              <p class="truncate text-xs text-subtle">{site.domain}</p>
            </div>
          </div>
          <span class="badge {isRunning ? 'badge-success' : ''}"><span class="dot"></span>{site.app_status || 'stopped'}</span>
        </div>
        <div class="card-body space-y-4">
          <div class="flex flex-wrap gap-2">
            {#if !isRunning}
              <button onclick={() => appAction('start')} disabled={appBusy} class="btn btn-primary btn-sm">
                {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if} Start
              </button>
            {:else}
              <button onclick={() => appAction('stop')} disabled={appBusy} class="btn btn-secondary btn-sm">
                {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Square size={14} />{/if} Stop
              </button>
              <button onclick={() => appAction('restart')} disabled={appBusy} class="btn btn-secondary btn-sm">
                {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCcw size={14} />{/if} Restart
              </button>
            {/if}
          </div>
          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {@render field('Runtime', site.runtime)}
            {@render field('Domain', site.domain, true)}
            {@render field('Port', String(site.port || '—'), true)}
            {@render field('Status', isRunning ? 'Running' : 'Stopped')}
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}
