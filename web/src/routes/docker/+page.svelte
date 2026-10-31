<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import CreateContainerModal from './CreateContainerModal.svelte';
  import ImagePuller from './ImagePuller.svelte';
  import { Box, Play, Square, RotateCcw, Trash2, ScrollText, RefreshCw, HardDrive, Network, Layers, Info, Plus, Search, LoaderCircle, Server } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type Tab = 'containers' | 'images' | 'volumes' | 'networks' | 'system';
  type Pending = { kind: 'container' | 'image' | 'volume' | 'network'; id: string; label: string };

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let activeTab = $state<Tab>('containers');
  let containers = $state<any[]>([]);
  let images = $state<any[]>([]);
  let volumes = $state<any[]>([]);
  let networks = $state<any[]>([]);
  let dockerInfo = $state<any>(null);
  let diskUsage = $state<any[]>([]);
  let loading = $state<Record<string, boolean>>({});
  let errors = $state<Record<string, string>>({});
  let actionPending = $state('');
  let query = $state('');
  let stateFilter = $state<'all' | 'running' | 'stopped'>('all');

  let showLogs = $state(false);
  let logs = $state('');
  let logsLoading = $state(false);
  let logsContainer = $state<{ id: string; name: string } | null>(null);

  let showCreate = $state(false);
  let createVolume = $state('');
  let createNetwork = $state('');

  let confirmOpen = $state(false);
  let confirmTarget = $state<Pending | null>(null);

  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load servers');
    }
    serversLoaded = true;
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadAll();
    }
  });

  // Generic loader: tracks loading + error per resource so each tab can render its own state.
  async function load(key: string, path: string, apply: (d: any) => void) {
    if (!selectedServer) return;
    const sid = selectedServer;
    loading[key] = true;
    errors[key] = '';
    try {
      const d = await apiFetch(auth.token, 'GET', `servers/${sid}/docker/${path}`);
      if (sid === selectedServer) apply(d);
    } catch (e: any) {
      if (e?.message !== 'session expired' && sid === selectedServer) errors[key] = e?.message || 'Request failed';
    } finally {
      loading[key] = false;
    }
  }

  const loadContainers = () => load('containers', 'containers', (d) => (containers = d?.containers ?? []));
  const loadImages = () => load('images', 'images', (d) => (images = d?.images ?? []));
  const loadVolumes = () => load('volumes', 'volumes', (d) => (volumes = d?.volumes ?? []));
  const loadNetworks = () => load('networks', 'networks', (d) => (networks = d?.networks ?? []));
  const loadInfo = () => load('info', 'info', (d) => (dockerInfo = d?.info ?? null));
  const loadDiskUsage = () => load('disk', 'disk-usage', (d) => (diskUsage = d?.usage ?? []));

  function loadAll() {
    loadContainers();
    // Images, volumes, networks, info and disk usage are admin-only endpoints.
    if (canManage) {
      loadImages();
      loadVolumes();
      loadNetworks();
      loadInfo();
      loadDiskUsage();
    }
  }

  function changeServer() {
    containers = []; images = []; volumes = []; networks = []; dockerInfo = null; diskUsage = [];
    loadAll();
  }

  async function run(key: string, path: string, body: any, success: string, after: () => void) {
    actionPending = key;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/${path}`, body);
      toasts.success(success);
      after();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Action failed');
    } finally {
      actionPending = '';
    }
  }

  function containerAction(c: any, action: 'start' | 'stop' | 'restart') {
    const verb = { start: 'Started', stop: 'Stopped', restart: 'Restarted' }[action];
    run(`${c.id}:${action}`, `container/${action}`, { container_id: c.id }, `${verb} ${c.name || c.id.slice(0, 12)}`, loadContainers);
  }

  function askRemove(p: Pending) {
    confirmTarget = p;
    confirmOpen = true;
  }

  function doRemove() {
    const t = confirmTarget;
    if (!t) return;
    if (t.kind === 'container') run(`${t.id}:remove`, 'container/remove', { container_id: t.id, force: true }, `Removed ${t.label}`, loadContainers);
    else if (t.kind === 'image') run(`image:${t.id}`, 'image/remove', { image: t.id, force: true }, `Removed ${t.label}`, loadImages);
    else if (t.kind === 'volume') run(`vol:${t.id}`, 'volume/remove', { name: t.id }, `Removed volume ${t.label}`, loadVolumes);
    else run(`net:${t.id}`, 'network/remove', { name: t.id }, `Removed network ${t.label}`, loadNetworks);
  }

  function createVolumeAction() {
    if (!createVolume) return;
    const name = createVolume;
    run(`vol:${name}`, 'volume/create', { name }, `Volume ${name} created`, () => { createVolume = ''; loadVolumes(); });
  }

  function createNetworkAction() {
    if (!createNetwork) return;
    const name = createNetwork;
    run(`net:${name}`, 'network/create', { name }, `Network ${name} created`, () => { createNetwork = ''; loadNetworks(); });
  }

  async function viewLogs(c: any) {
    logsContainer = { id: c.id, name: c.name || c.id.slice(0, 12) };
    logs = '';
    logsLoading = true;
    showLogs = true;
    try {
      const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/container/logs?container_id=${encodeURIComponent(c.id)}&lines=200`);
      logs = d?.logs ?? '';
    } catch {
      logs = 'Failed to load logs';
    }
    logsLoading = false;
  }

  function stateBadge(state: string): string {
    if (state === 'running') return 'badge-success';
    if (state === 'paused' || state === 'restarting') return 'badge-warning';
    if (state === 'exited' || state === 'dead') return 'badge-danger';
    return '';
  }

  const tabs = $derived(
    [
      { id: 'containers' as Tab, label: 'Containers', icon: Box, count: containers.length, admin: false },
      { id: 'images' as Tab, label: 'Images', icon: Layers, count: images.length, admin: true },
      { id: 'volumes' as Tab, label: 'Volumes', icon: HardDrive, count: volumes.length, admin: true },
      { id: 'networks' as Tab, label: 'Networks', icon: Network, count: networks.length, admin: true },
      { id: 'system' as Tab, label: 'System', icon: Info, count: null, admin: true },
    ].filter((t) => !t.admin || canManage),
  );

  const runningCount = $derived(containers.filter((c) => c.state === 'running').length);
  const visibleContainers = $derived(
    containers
      .filter((c) => stateFilter === 'all' || (stateFilter === 'running' ? c.state === 'running' : c.state !== 'running'))
      .filter((c) => !query || `${c.name} ${c.image} ${c.id}`.toLowerCase().includes(query.toLowerCase())),
  );
  const description = $derived(
    dockerInfo ? `Docker ${dockerInfo.version} · ${dockerInfo.running} running` : loading.containers ? 'Loading…' : `${containers.length} containers · ${runningCount} running`,
  );
  const confirmMessage = $derived(
    confirmTarget
      ? confirmTarget.kind === 'container'
        ? `Container ${confirmTarget.label} will be force-removed. Data not stored in a volume is lost.`
        : confirmTarget.kind === 'image'
          ? `Image ${confirmTarget.label} will be force-removed from this server.`
          : `The ${confirmTarget.kind} ${confirmTarget.label} will be removed.`
      : '',
  );
</script>

<DashboardLayout active="Docker">
  <PageHeader title="Docker" description={servers.length ? description : ''}>
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={changeServer} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      {#if servers.length > 0}
        <button onclick={loadAll} class="btn btn-secondary btn-sm" disabled={loading.containers}>
          <RefreshCw size={14} class={loading.containers ? 'animate-spin' : ''} /> Refresh
        </button>
      {/if}
      {#if canManage && servers.length > 0}
        <button onclick={() => (showCreate = true)} class="btn btn-primary btn-sm"><Plus size={14} /> Create container</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — Docker is managed through its agent.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else}
    <div class="tabs mb-6">
      {#each tabs as tab (tab.id)}
        <button class="tab {activeTab === tab.id ? 'tab-active' : ''}" onclick={() => (activeTab = tab.id)}>
          <tab.icon size={15} />
          {tab.label}
          {#if tab.count !== null && tab.count > 0}<span class="text-xs text-subtle tabular-nums">{tab.count}</span>{/if}
        </button>
      {/each}
    </div>

    {#if activeTab === 'containers'}
      <div class="card">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div class="segmented">
            {#each [['all', 'All'], ['running', 'Running'], ['stopped', 'Stopped']] as [key, label] (key)}
              <button class="segment {stateFilter === key ? 'segment-active' : ''}" onclick={() => (stateFilter = key as any)}>{label}</button>
            {/each}
          </div>
          <div class="relative w-full sm:w-64">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input bind:value={query} class="input h-8 pl-8" placeholder="Filter containers…" aria-label="Filter containers" />
          </div>
        </div>

        {#if loading.containers && containers.length === 0}
          <div class="space-y-3 p-4">{#each Array(5) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
        {:else if errors.containers && containers.length === 0}
          <EmptyState compact icon={Box} title="Couldn't load containers" description={errors.containers} actionLabel="Retry" onaction={loadContainers} />
        {:else if visibleContainers.length === 0}
          <EmptyState compact icon={Box} title={containers.length ? 'No containers match' : 'No containers'} description={containers.length ? 'Try another filter.' : 'No Docker containers found on this server.'} />
        {:else}
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th class="max-md:hidden">Image</th>
                  <th>State</th>
                  <th class="max-lg:hidden">Ports</th>
                  <th class="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {#each visibleContainers as c (c.id)}
                  <tr>
                    <td>
                      <div class="font-medium text-fg">{c.name}</div>
                      <div class="font-mono text-xs text-subtle">{c.id?.slice(0, 12)}</div>
                    </td>
                    <td class="max-w-[240px] truncate font-mono text-[13px] text-muted max-md:hidden">{c.image}</td>
                    <td><span class="badge {stateBadge(c.state)}"><span class="dot"></span>{c.state}</span></td>
                    <td class="max-w-[220px] truncate font-mono text-xs text-subtle max-lg:hidden" title={c.ports}>{c.ports || '—'}</td>
                    <td>
                      <div class="flex items-center justify-end gap-0.5">
                        {#if canManage}
                          {#if c.state !== 'running'}
                            <button onclick={() => containerAction(c, 'start')} disabled={!!actionPending} class="btn btn-ghost btn-icon btn-xs text-success" aria-label="Start" title="Start">
                              {#if actionPending === `${c.id}:start`}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if}
                            </button>
                          {:else}
                            <button onclick={() => containerAction(c, 'stop')} disabled={!!actionPending} class="btn btn-ghost btn-icon btn-xs" aria-label="Stop" title="Stop">
                              {#if actionPending === `${c.id}:stop`}<LoaderCircle size={14} class="animate-spin" />{:else}<Square size={14} />{/if}
                            </button>
                            <button onclick={() => containerAction(c, 'restart')} disabled={!!actionPending} class="btn btn-ghost btn-icon btn-xs" aria-label="Restart" title="Restart">
                              {#if actionPending === `${c.id}:restart`}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCcw size={14} />{/if}
                            </button>
                          {/if}
                        {/if}
                        <button onclick={() => viewLogs(c)} class="btn btn-ghost btn-icon btn-xs" aria-label="Logs" title="Logs"><ScrollText size={14} /></button>
                        {#if canManage}
                          <button onclick={() => askRemove({ kind: 'container', id: c.id, label: c.name || c.id.slice(0, 12) })} disabled={!!actionPending} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove" title="Remove">
                            {#if actionPending === `${c.id}:remove`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                          </button>
                        {/if}
                      </div>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}

    {#if activeTab === 'images' && canManage}
      <div class="card">
        <div class="border-b border-line px-4 py-3">
          <ImagePuller serverId={selectedServer} onpulled={loadImages} />
        </div>
        {#if loading.images && images.length === 0}
          <div class="space-y-3 p-4">{#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
        {:else if errors.images && images.length === 0}
          <EmptyState compact icon={Layers} title="Couldn't load images" description={errors.images} actionLabel="Retry" onaction={loadImages} />
        {:else if images.length === 0}
          <EmptyState compact icon={Layers} title="No images" description="Pull an image from Docker Hub to get started." />
        {:else}
          <div class="table-wrap">
            <table class="table">
              <thead><tr><th>Repository</th><th>Tag</th><th>Size</th><th class="max-sm:hidden">Created</th><th class="text-right">Actions</th></tr></thead>
              <tbody>
                {#each images as img, i (img.id ?? `${img.repository}:${img.tag}:${i}`)}
                  {@const ref = `${img.repository}:${img.tag}`}
                  <tr>
                    <td class="font-mono text-[13px] text-fg">{img.repository}</td>
                    <td><span class="code">{img.tag}</span></td>
                    <td class="text-muted tabular-nums">{img.size}</td>
                    <td class="text-xs text-subtle tabular-nums max-sm:hidden">{img.created?.slice(0, 10)}</td>
                    <td class="text-right">
                      <button onclick={() => askRemove({ kind: 'image', id: ref, label: ref })} disabled={actionPending === `image:${ref}`} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove image" title="Remove">
                        {#if actionPending === `image:${ref}`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}

    {#if activeTab === 'volumes' && canManage}
      <div class="card">
        <form class="flex gap-2 border-b border-line px-4 py-3" onsubmit={(e) => { e.preventDefault(); createVolumeAction(); }}>
          <input bind:value={createVolume} placeholder="New volume name" aria-label="Volume name" class="input h-8 flex-1 font-mono text-[13px]" />
          <button type="submit" disabled={!createVolume || actionPending === `vol:${createVolume}`} class="btn btn-secondary btn-sm"><Plus size={14} /> Create</button>
        </form>
        {#if loading.volumes && volumes.length === 0}
          <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
        {:else if errors.volumes && volumes.length === 0}
          <EmptyState compact icon={HardDrive} title="Couldn't load volumes" description={errors.volumes} actionLabel="Retry" onaction={loadVolumes} />
        {:else if volumes.length === 0}
          <EmptyState compact icon={HardDrive} title="No volumes" description="Volumes persist container data across restarts." />
        {:else}
          <div class="table-wrap">
            <table class="table">
              <thead><tr><th>Name</th><th>Driver</th><th class="max-sm:hidden">Created</th><th class="text-right">Actions</th></tr></thead>
              <tbody>
                {#each volumes as v (v.name)}
                  <tr>
                    <td class="max-w-[320px] truncate font-mono text-[13px] text-fg">{v.name}</td>
                    <td class="text-muted">{v.driver}</td>
                    <td class="text-xs text-subtle tabular-nums max-sm:hidden">{v.created?.slice(0, 10)}</td>
                    <td class="text-right">
                      <button onclick={() => askRemove({ kind: 'volume', id: v.name, label: v.name })} disabled={actionPending === `vol:${v.name}`} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove volume" title="Remove">
                        {#if actionPending === `vol:${v.name}`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}

    {#if activeTab === 'networks' && canManage}
      <div class="card">
        <form class="flex gap-2 border-b border-line px-4 py-3" onsubmit={(e) => { e.preventDefault(); createNetworkAction(); }}>
          <input bind:value={createNetwork} placeholder="New network name" aria-label="Network name" class="input h-8 flex-1 font-mono text-[13px]" />
          <button type="submit" disabled={!createNetwork || actionPending === `net:${createNetwork}`} class="btn btn-secondary btn-sm"><Plus size={14} /> Create</button>
        </form>
        {#if loading.networks && networks.length === 0}
          <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
        {:else if errors.networks && networks.length === 0}
          <EmptyState compact icon={Network} title="Couldn't load networks" description={errors.networks} actionLabel="Retry" onaction={loadNetworks} />
        {:else if networks.length === 0}
          <EmptyState compact icon={Network} title="No networks" />
        {:else}
          <div class="table-wrap">
            <table class="table">
              <thead><tr><th>Name</th><th>Driver</th><th>Scope</th><th class="text-right">Actions</th></tr></thead>
              <tbody>
                {#each networks as n (n.id ?? n.name)}
                  <tr>
                    <td class="font-mono text-[13px] text-fg">{n.name}</td>
                    <td class="text-muted">{n.driver}</td>
                    <td class="text-muted">{n.scope}</td>
                    <td class="text-right">
                      <button onclick={() => askRemove({ kind: 'network', id: n.name, label: n.name })} disabled={actionPending === `net:${n.name}`} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove network" title="Remove">
                        {#if actionPending === `net:${n.name}`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    {/if}

    {#if activeTab === 'system' && canManage}
      {#if loading.info && !dockerInfo}
        <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-3">{#each Array(6) as _, i (i)}<div class="skeleton h-20"></div>{/each}</div>
      {:else if dockerInfo}
        <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-3">
          {#each [
            ['Version', dockerInfo.version],
            ['Storage driver', dockerInfo.driver],
            ['CPUs', dockerInfo.cpus],
            ['Memory', `${(parseInt(dockerInfo.memory || '0') / 1073741824).toFixed(1)} GB`],
            ['Containers', `${dockerInfo.running} running · ${dockerInfo.stopped} stopped`],
            ['Images', dockerInfo.images],
          ] as [label, value] (label)}
            <div class="card p-4">
              <div class="stat-label">{label}</div>
              <div class="mt-1 truncate text-base font-semibold text-fg tabular-nums">{value ?? '—'}</div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="card mb-6">
          <EmptyState compact icon={Info} title="Docker info unavailable" description={errors.info || 'The agent did not return Docker system information.'} actionLabel="Retry" onaction={loadInfo} />
        </div>
      {/if}

      {#if diskUsage.length > 0}
        <div class="card">
          <div class="card-header"><h3 class="card-title">Disk usage</h3></div>
          <div class="table-wrap">
            <table class="table">
              <thead><tr><th>Type</th><th>Count</th><th>Size</th><th>Reclaimable</th></tr></thead>
              <tbody>
                {#each diskUsage as u (u.type)}
                  <tr>
                    <td class="font-medium text-fg">{u.type}</td>
                    <td class="text-muted tabular-nums">{u.count}</td>
                    <td class="text-muted tabular-nums">{u.size}</td>
                    <td class="text-success tabular-nums">{u.reclaimable}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      {/if}
    {/if}
  {/if}

  {#if canManage && selectedServer}
    <CreateContainerModal bind:open={showCreate} serverId={selectedServer} {images} oncreated={loadContainers} />
  {/if}

  <Modal bind:open={showLogs} title={logsContainer ? `Logs · ${logsContainer.name}` : 'Logs'} maxWidth="4xl">
    {#if logsLoading}
      <div class="space-y-2">{#each Array(8) as _, i (i)}<div class="skeleton h-3.5" style="width: {[92, 78, 86, 64, 90, 72, 84, 58][i]}%"></div>{/each}</div>
    {:else}
      <pre class="max-h-[60vh] overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{logs || 'No log output.'}</pre>
    {/if}
    {#snippet footer()}
      <span class="mr-auto text-xs text-subtle">Last 200 lines</span>
      {#if logsContainer}
        <button class="btn btn-secondary btn-sm" onclick={() => logsContainer && viewLogs(logsContainer)} disabled={logsLoading}>
          <RefreshCw size={14} class={logsLoading ? 'animate-spin' : ''} /> Reload
        </button>
      {/if}
      <button class="btn btn-secondary btn-sm" onclick={() => (showLogs = false)}>Close</button>
    {/snippet}
  </Modal>

  <ConfirmDialog
    bind:open={confirmOpen}
    title={confirmTarget ? `Remove ${confirmTarget.kind}?` : 'Remove?'}
    message={confirmMessage}
    confirmLabel="Remove"
    destructive
    onconfirm={doRemove}
  />
</DashboardLayout>
