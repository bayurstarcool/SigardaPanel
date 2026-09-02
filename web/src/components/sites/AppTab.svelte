<script lang="ts">
  import { apiFetch } from '$lib/api';
  import { auth } from '$lib/auth.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { onMount } from 'svelte';
  import {
    Zap, Terminal, Activity, Box, FileCode, RefreshCw,
    Play, Square, RotateCcw, Settings, Loader2, Save, X, Trash2,
    Search, ChevronRight, Plus, FolderOpen
  } from 'lucide-svelte';

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

  // ─── File manager ───
  let fmPath = $state('/');
  let fmBreadcrumbs = $state<{ name: string; path: string }[]>([]);

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


  // ─── onMount ───
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
        // Set file manager to site root path (mounted into container)
        fmPath = site.root_path || '/';
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

  // ─── File manager: exec helper ───
  async function execInContainer(command: string): Promise<string> {
    const d: any = await apiFetch(auth.token, 'POST', `servers/${site.server_id}/docker/container/exec`, {
      container_id: dockerContainer.id,
      command,
    });
    return d?.output || '';
  }



  // Convert absolute path to relative (strip root_path prefix)




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

</script>

<!-- ═══════════════════════════════════════════════════════════════
     TEMPLATE
     ═══════════════════════════════════════════════════════════════ -->
{#if loading}
  <div class="space-y-4">
    {#each Array(3) as _}
      <div class="h-20 bg-white/[0.03] rounded-xl animate-pulse"></div>
    {/each}
  </div>
{:else if !site}
  <div class="text-center py-12 text-slate-500">Site not found</div>
{:else if !isAppRuntime}
  <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-8 text-center">
    <Settings size={32} class="text-slate-600 mx-auto mb-3" />
    <p class="text-sm text-slate-500">
      App process management is not available for
      <span class="font-semibold text-slate-400">{site.runtime}</span> sites.
    </p>
  </div>
{:else}
  <div class="space-y-4">
    <!-- ═══ Docker Container Management ═══ -->
    {#if isDocker}
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4 sm:p-5">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20 flex items-center justify-center">
              <Box size={18} class="text-sky-400" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-slate-200">Docker Container</h3>
              <p class="text-xs text-slate-500">{site.domain}</p>
            </div>
          </div>
          {#if dockerLoading}
            <Loader2 size={16} class="animate-spin text-slate-500" />
          {:else if dockerContainer}
            <span
              class="text-xs px-2 py-0.5 rounded-full font-medium {isRunning
                ? 'bg-emerald-500/10 text-emerald-400'
                : 'bg-slate-500/10 text-slate-400'}"
            >
              {dockerContainer.state}
            </span>
          {:else}
            <span class="text-xs px-2 py-0.5 rounded-full font-medium bg-red-500/10 text-red-400">
              not found
            </span>
          {/if}
        </div>

        {#if dockerContainer}
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4 text-xs">
            <div class="bg-white/[0.03] rounded-lg p-3">
              <span class="text-slate-500 block mb-1">Container</span>
              <span class="text-slate-200 font-mono">{dockerContainer.name}</span>
            </div>
            <div class="bg-white/[0.03] rounded-lg p-3">
              <span class="text-slate-500 block mb-1">Image</span>
              <span class="text-slate-200 font-mono truncate block">{dockerContainer.image}</span>
            </div>
            <div class="bg-white/[0.03] rounded-lg p-3">
              <span class="text-slate-500 block mb-1">ID</span>
              <span class="text-slate-200 font-mono">{dockerContainer.id?.slice(0, 12)}</span>
            </div>
            <div class="bg-white/[0.03] rounded-lg p-3">
              <span class="text-slate-500 block mb-1">Ports</span>
              <span class="text-slate-200 font-mono truncate block">{dockerContainer.ports || '—'}</span>
            </div>
          </div>

          <div class="flex flex-wrap gap-2">
            {#if !isRunning}
              <button
                onclick={() => dockerAction('start')}
                disabled={actionBusy}
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-green-500 to-emerald-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
              >
                {#if actionBusy}<Loader2 size={12} class="animate-spin" />{/if}
                <Play size={12} /> Start
              </button>
            {/if}
            {#if isRunning}
              <button
                onclick={() => dockerAction('stop')}
                disabled={actionBusy}
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-red-500 to-rose-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
              >
                {#if actionBusy}<Loader2 size={12} class="animate-spin" />{/if}
                <Square size={12} /> Stop
              </button>
              <button
                onclick={() => dockerAction('restart')}
                disabled={actionBusy}
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-500 to-orange-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
              >
                {#if actionBusy}<Loader2 size={12} class="animate-spin" />{/if}
                <RotateCcw size={12} /> Restart
              </button>
            {/if}
            <button
              onclick={() => dockerAction('remove')}
              disabled={actionBusy}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium text-red-400 bg-red-500/10 hover:bg-red-500/20 transition cursor-pointer disabled:opacity-40 border border-red-500/20"
            >
              <Trash2 size={12} /> Remove
            </button>
            <button
              onclick={loadDockerContainer}
              disabled={dockerLoading}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium text-slate-300 hover:text-slate-100 bg-white/[0.04] hover:bg-white/[0.08] transition border border-white/[0.06] cursor-pointer"
            >
              <RefreshCw size={12} class={dockerLoading ? 'animate-spin' : ''} /> Refresh
            </button>
          </div>
        {:else if !dockerLoading}
          <div class="text-center py-6">
            <Box size={24} class="text-slate-600 mx-auto mb-2" />
            <p class="text-xs text-slate-500">Container not found.</p>
          </div>
        {/if}
      </div>

      <!-- ═══ Docker Sub-tabs ═══ -->
      {#if dockerContainer}
        <div class="flex gap-1 bg-white/[0.03] border border-white/[0.06] rounded-xl p-1">
          {#each [
            { key: 'manage', label: 'Manage', icon: Box },
            { key: 'terminal', label: 'Terminal', icon: Terminal },
            { key: 'config', label: 'Config', icon: Settings },
          ] as tab}
            {@const Icon = tab.icon}
            <button
              onclick={() => (dockerTab = tab.key as typeof dockerTab)}
              class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition cursor-pointer {dockerTab === tab.key
                ? 'bg-white/[0.08] text-sky-300'
                : 'text-slate-500 hover:text-slate-300'}"
            >
              <Icon size={13} /> {tab.label}
            </button>
          {/each}
        </div>

        <!-- ─── Manage Tab (default: show container card details again) ─── -->
        {#if dockerTab === 'manage'}
          <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4 sm:p-5">
            <div class="flex items-center gap-2 mb-3">
              <Activity size={13} class="text-sky-400" />
              <span class="text-xs font-semibold text-slate-200">Container Overview</span>
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
              <div class="bg-white/[0.03] rounded-lg p-3">
                <span class="text-slate-500 block mb-1">Runtime</span>
                <span class="text-slate-200">{site.runtime}</span>
              </div>
              <div class="bg-white/[0.03] rounded-lg p-3">
                <span class="text-slate-500 block mb-1">Domain</span>
                <span class="text-slate-200 font-mono">{site.domain}</span>
              </div>
              <div class="bg-white/[0.03] rounded-lg p-3">
                <span class="text-slate-500 block mb-1">Status</span>
                <span class="text-slate-200">{isRunning ? 'Running' : 'Stopped'}</span>
              </div>
              <div class="bg-white/[0.03] rounded-lg p-3">
                <span class="text-slate-500 block mb-1">Created</span>
                <span class="text-slate-200">{dockerContainer.created ? new Date(dockerContainer.created * 1000).toLocaleDateString() : '—'}</span>
              </div>
            </div>
          </div>
        {/if}

        <!-- ─── Terminal Tab ─── -->
        {#if dockerTab === 'terminal'}
          <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl overflow-hidden">
            <div class="px-4 py-2.5 border-b border-white/[0.05] flex items-center gap-2">
              <Terminal size={13} class="text-sky-400" />
              <span class="text-xs font-semibold text-slate-200">Container Exec</span>
              <span class="text-[0.6rem] text-slate-600 ml-auto">{dockerContainer.name}</span>
            </div>
            <div class="p-1">
              <pre
                class="bg-[#0d1117] text-emerald-300 font-mono text-[11px] leading-relaxed p-4 rounded-lg max-h-[40vh] overflow-y-auto whitespace-pre-wrap">{execOutput || '$ '}</pre>
            </div>
            <div class="px-4 py-3 border-t border-white/[0.05] flex gap-2">
              <input
                bind:value={execCmd}
                onkeydown={(e) => {
                  if (e.key === 'Enter') execCommand();
                }}
                placeholder="Enter command..."
                class="flex-1 bg-[#0d1117] border border-white/[0.08] rounded-lg px-3 py-2 text-xs text-emerald-300 font-mono placeholder-slate-600 outline-none focus:border-sky-400/50"
              />
              <button
                onclick={execCommand}
                disabled={execRunning || !execCmd.trim()}
                class="px-4 py-2 bg-sky-500/20 text-sky-300 rounded-lg text-xs font-medium hover:bg-sky-500/30 transition disabled:opacity-40 flex items-center gap-1.5 cursor-pointer"
              >
                {#if execRunning}<Loader2 size={12} class="animate-spin" />{/if} Run
              </button>
            </div>
          </div>
        {/if}

        <!-- ─── Config Tab ─── -->
        {#if dockerTab === 'config'}
          <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl overflow-hidden">
            <div class="px-4 py-2.5 border-b border-white/[0.05] flex items-center gap-2">
              <Settings size={13} class="text-sky-400" />
              <span class="text-xs font-semibold text-slate-200">Container Configuration</span>
            </div>
            <div class="p-4 space-y-4">
              <div>
                <label class="block text-xs text-slate-500 mb-1.5">Docker Image</label>
                <input
                  bind:value={configImage}
                  class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 font-mono outline-none focus:border-sky-400/50"
                />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1.5">
                  Environment Variables <span class="text-slate-600">(one per line: KEY=value)</span>
                </label>
                <textarea
                  bind:value={configEnv}
                  rows={4}
                  class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 font-mono resize-none outline-none focus:border-sky-400/50"
                  placeholder="APP_ENV=production&#10;PORT=3000"
                ></textarea>
              </div>
              <div class="flex gap-2">
                <button
                  onclick={rebuildContainer}
                  disabled={configBusy || !configImage}
                  class="px-4 py-2 bg-sky-500/20 text-sky-300 rounded-lg text-xs font-medium hover:bg-sky-500/30 transition flex items-center gap-1.5 disabled:opacity-40 cursor-pointer border-0"
                >
                  {#if configBusy}<Loader2 size={12} class="animate-spin" />{/if} Rebuild Container
                </button>
              </div>
              <p class="text-[0.65rem] text-slate-600">
                Rebuild will: Pull image → Stop → Remove → Create → Start
              </p>
            </div>
          </div>
        {/if}
      {/if}
    {/if}

    <!-- ═══ Non-Docker App Process (Node/Go/Python) ═══ -->
    {#if !isDocker}
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4 sm:p-5">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-emerald-400/20 to-teal-500/20 flex items-center justify-center">
              <Zap size={18} class="text-emerald-400" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-slate-200">{site.runtime} Application</h3>
              <p class="text-xs text-slate-500">{site.domain}</p>
            </div>
          </div>
          <span
            class="text-xs px-2 py-0.5 rounded-full font-medium {isRunning
              ? 'bg-emerald-500/10 text-emerald-400'
              : 'bg-slate-500/10 text-slate-400'}"
          >
            {site.app_status || 'stopped'}
          </span>
        </div>

        <div class="flex flex-wrap gap-2">
          {#if !isRunning}
            <button
              onclick={() => appAction('start')}
              disabled={appBusy}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-green-500 to-emerald-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
            >
              {#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}
              <Play size={12} /> Start
            </button>
          {/if}
          {#if isRunning}
            <button
              onclick={() => appAction('stop')}
              disabled={appBusy}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-red-500 to-rose-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
            >
              {#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}
              <Square size={12} /> Stop
            </button>
            <button
              onclick={() => appAction('restart')}
              disabled={appBusy}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-500 to-orange-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"
            >
              {#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}
              <RotateCcw size={12} /> Restart
            </button>
          {/if}
        </div>

        <!-- App info -->
        <div class="mt-4 grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
          <div class="bg-white/[0.03] rounded-lg p-3">
            <span class="text-slate-500 block mb-1">Runtime</span>
            <span class="text-slate-200">{site.runtime}</span>
          </div>
          <div class="bg-white/[0.03] rounded-lg p-3">
            <span class="text-slate-500 block mb-1">Domain</span>
            <span class="text-slate-200 font-mono truncate block">{site.domain}</span>
          </div>
          <div class="bg-white/[0.03] rounded-lg p-3">
            <span class="text-slate-500 block mb-1">Port</span>
            <span class="text-slate-200 font-mono">{site.port || '—'}</span>
          </div>
          <div class="bg-white/[0.03] rounded-lg p-3">
            <span class="text-slate-500 block mb-1">Status</span>
            <span class="text-slate-200">{isRunning ? 'Running' : 'Stopped'}</span>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}
