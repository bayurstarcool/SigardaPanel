<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Box, Play, Square, RotateCcw, Trash2, Terminal, RefreshCw, Download, Upload, HardDrive, Globe, Layers, Info, Plus } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let activeTab = $state('containers');
  let containers = $state<any[]>([]);
  let images = $state<any[]>([]);
  let volumes = $state<any[]>([]);
  let networks = $state<any[]>([]);
  let dockerInfo = $state<any>(null);
  let diskUsage = $state<any[]>([]);
  let loading = $state(false);
  let logs = $state('');
  let showLogs = $state(false);
  let logsContainer = $state('');
  let actionPending = $state('');
  let pullImage = $state('');
  let pullProgress = $state<{active: boolean; image: string; layers: {id: string; status: string}[]; done: boolean; error: string}>({active: false, image: '', layers: [], done: false, error: ''});
  let searchResults = $state<any[]>([]);
  let searchOpen = $state(false);
  let searchLoading = $state(false);
  let searchTimer = $state<ReturnType<typeof setTimeout> | null>(null);
  let createVolume = $state('');
  let createNetwork = $state('');
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));
  let showCreateContainer = $state(false);
  let createContainerForm = $state({image: '', name: '', ports: '', volumes: '', env: '', restart: 'no', network: 'bridge', auto_start: true, command: ''});
  let createPending = $state(false);
  let createImageSearch = $state('');
  let createImageResults = $state<any[]>([]);
  let createImageOpen = $state(false);
  let createImageTimer = $state<ReturnType<typeof setTimeout> | null>(null);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadAll();
    }
  });

  function loadAll() {
    loadContainers();
    loadImages();
    loadVolumes();
    loadNetworks();
    loadInfo();
    loadDiskUsage();
  }

  function loadContainers() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/containers`)
      .then((d: any) => { containers = d?.containers ?? []; })
      .catch(() => {})
      .finally(() => loading = false);
  }

  function loadImages() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/images`)
      .then((d: any) => { images = d?.images ?? []; })
      .catch(() => {});
  }

  function loadVolumes() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/volumes`)
      .then((d: any) => { volumes = d?.volumes ?? []; })
      .catch(() => {});
  }

  function loadNetworks() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/networks`)
      .then((d: any) => { networks = d?.networks ?? []; })
      .catch(() => {});
  }

  function loadInfo() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/info`)
      .then((d: any) => { dockerInfo = d?.info ?? null; })
      .catch(() => {});
  }

  function loadDiskUsage() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/disk-usage`)
      .then((d: any) => { diskUsage = d?.usage ?? []; })
      .catch(() => {});
  }

  async function containerAction(id: string, action: string) {
    actionPending = `${id}:${action}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/container/${action}`, { container_id: id });
      loadContainers();
    } catch (e) {}
    actionPending = '';
  }

  async function removeContainer(id: string) {
    if (!confirm(`Remove container ${id}?`)) return;
    actionPending = `${id}:remove`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/container/remove`, { container_id: id, force: true });
      loadContainers();
    } catch (e) {}
    actionPending = '';
  }

  async function createContainerAction() {
    if (!createContainerForm.image) return;
    createPending = true;
    try {
      const ports = createContainerForm.ports.split(',').map((s: string) => s.trim()).filter(Boolean);
      const volumes = createContainerForm.volumes.split(',').map((s: string) => s.trim()).filter(Boolean);
      const envPairs = createContainerForm.env.split('\n').filter(Boolean);
      const env: Record<string, string> = {};
      for (const pair of envPairs) {
        const [k, ...rest] = pair.split('=');
        if (k) env[k.trim()] = rest.join('=').trim();
      }
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/container/create`, {
        image: createContainerForm.image,
        name: createContainerForm.name || undefined,
        command: createContainerForm.command || undefined,
        ports: ports.length ? ports : undefined,
        volumes: volumes.length ? volumes : undefined,
        env: Object.keys(env).length ? env : undefined,
        restart: createContainerForm.restart,
        network: createContainerForm.network,
        auto_start: createContainerForm.auto_start,
      });
      showCreateContainer = false;
      createContainerForm = {image: '', name: '', ports: '', volumes: '', env: '', restart: 'no', network: 'bridge', auto_start: true, command: ''};
      createImageSearch = '';
      loadContainers();
    } catch (e) {}
    createPending = false;
  }

  function onCreateImageInput(value: string) {
    createImageSearch = value;
    createContainerForm.image = value;
    if (createImageTimer) clearTimeout(createImageTimer);
    if (!value) { createImageOpen = false; createImageResults = []; return; }
    // Show matching local images immediately
    const localMatches = images.filter((img: any) => {
      const full = img.tag === 'latest' || !img.tag ? img.repository : `${img.repository}:${img.tag}`;
      return full.toLowerCase().includes(value.toLowerCase());
    }).map((img: any) => ({
      name: img.tag === 'latest' || !img.tag ? img.repository : `${img.repository}:${img.tag}`,
      source: 'local' as const,
      size: img.size,
    }));
    createImageResults = localMatches;
    createImageOpen = true;
    // Also search Docker Hub after debounce
    createImageTimer = setTimeout(async () => {
      try {
        const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/images/search?q=${encodeURIComponent(value)}&page_size=5`);
        const hubResults = (d?.results ?? []).map((r: any) => ({
          name: r.name,
          source: 'hub' as const,
          description: r.description,
          stars: r.stars,
          pulls: r.pulls,
          official: r.official,
        }));
        // Merge: local first, then hub (dedupe by name)
        const seen = new Set(localMatches.map((m: any) => m.name));
        const merged = [...localMatches, ...hubResults.filter((h: any) => !seen.has(h.name))];
        createImageResults = merged;
      } catch {}
    }, 300);
  }

  function selectCreateImage(name: string) {
    createContainerForm.image = name;
    createImageSearch = name;
    createImageOpen = false;
    createImageResults = [];
  }

  async function viewLogs(id: string) {
    logsContainer = id;
    showLogs = true;
    try {
      const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/container/logs?container_id=${id}&lines=200`);
      logs = d?.logs ?? '';
    } catch (e) { logs = 'Failed to load logs'; }
  }

  async function pullImageAction() {
    if (!pullImage) return;
    actionPending = 'pull';
    searchOpen = false;
    pullProgress = { active: true, image: pullImage, layers: [], done: false, error: '' };
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/image/pull`, { image: pullImage });
      const output = d?.output || d?.message || '';
      const lines = output.split('\n').filter((l: string) => l.trim());
      const layers: {id: string; status: string}[] = [];
      for (const line of lines) {
        const m = line.match(/^([a-f0-9]{12}): (.+)$/);
        if (m) {
          const existing = layers.find(l => l.id === m[1]);
          if (existing) existing.status = m[2];
          else layers.push({ id: m[1], status: m[2] });
        }
      }
      if (layers.length === 0 && lines.length > 0) {
        layers.push({ id: 'output', status: lines[lines.length - 1] });
      }
      pullProgress = { active: true, image: pullImage, layers, done: true, error: d?.ok === false ? (d?.message || 'pull failed') : '' };
      pullImage = '';
      loadImages();
    } catch (e: any) {
      pullProgress = { active: true, image: pullImage, layers: [], done: true, error: e.message || 'pull failed' };
    }
    actionPending = '';
  }

  function onImageInput(value: string) {
    pullImage = value;
    if (searchTimer) clearTimeout(searchTimer);
    if (!value || value.length < 2) { searchOpen = false; searchResults = []; return; }
    searchLoading = true;
    searchTimer = setTimeout(async () => {
      try {
        const d: any = await apiFetch(auth.token, 'GET', `servers/${selectedServer}/docker/images/search?q=${encodeURIComponent(value)}&page_size=8`);
        searchResults = d?.results ?? [];
        searchOpen = searchResults.length > 0;
      } catch { searchResults = []; searchOpen = false; }
      searchLoading = false;
    }, 300);
  }

  function selectImage(name: string) {
    pullImage = name;
    searchOpen = false;
    searchResults = [];
  }

  function fmtPulls(n: number): string {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
  }

  async function removeImage(image: string) {
    if (!confirm(`Remove image ${image}?`)) return;
    actionPending = `image:${image}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/image/remove`, { image, force: true });
      loadImages();
    } catch (e) {}
    actionPending = '';
  }

  async function createVolumeAction() {
    if (!createVolume) return;
    actionPending = `vol:${createVolume}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/volume/create`, { name: createVolume });
      createVolume = '';
      loadVolumes();
    } catch (e) {}
    actionPending = '';
  }

  async function removeVolume(name: string) {
    if (!confirm(`Remove volume ${name}?`)) return;
    actionPending = `vol:${name}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/volume/remove`, { name });
      loadVolumes();
    } catch (e) {}
    actionPending = '';
  }

  async function createNetworkAction() {
    if (!createNetwork) return;
    actionPending = `net:${createNetwork}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/network/create`, { name: createNetwork });
      createNetwork = '';
      loadNetworks();
    } catch (e) {}
    actionPending = '';
  }

  async function removeNetwork(name: string) {
    if (!confirm(`Remove network ${name}?`)) return;
    actionPending = `net:${name}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/docker/network/remove`, { name });
      loadNetworks();
    } catch (e) {}
    actionPending = '';
  }

  function stateIcon(state: string) {
    if (state === 'running') return '🟢';
    if (state === 'exited') return '🔴';
    if (state === 'paused') return '🟡';
    return '⚪';
  }

  const tabs = [
    { id: 'containers', label: 'Containers', icon: Box },
    { id: 'images', label: 'Images', icon: Layers },
    { id: 'volumes', label: 'Volumes', icon: HardDrive },
    { id: 'networks', label: 'Networks', icon: Globe },
    { id: 'system', label: 'System', icon: Info },
  ];
</script>

<DashboardLayout active="Docker">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Docker Management</h1>
      <p class="text-sm text-slate-500 mt-1">{dockerInfo ? `v${dockerInfo.version} • ${dockerInfo.running} running` : 'Loading...'}</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => loadAll()}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => loadAll()} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} />
      </button>
    </div>
  </div>

  <!-- Tabs -->
  <div class="flex gap-1 mb-6 bg-white/[0.03] border border-white/[0.07] rounded-xl p-1 overflow-x-auto">
    {#each tabs as tab}
      {@const Icon = tab.icon}
      <button onclick={() => activeTab = tab.id}
        class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition whitespace-nowrap {activeTab === tab.id ? 'bg-white/[0.08] text-white' : 'text-slate-400 hover:text-white hover:bg-white/[0.04]'}">
        <Icon size={16} />
        {tab.label}
      </button>
    {/each}
  </div>

  <!-- Containers Tab -->
  {#if activeTab === 'containers'}
    {#if canManage}
      <div class="flex gap-3 mb-6">
        <button onclick={() => showCreateContainer = true}
          class="px-4 py-2 bg-sky-500 text-white rounded-lg text-sm font-medium hover:bg-sky-600 transition flex items-center gap-2">
          <Plus size={16} /> Create Container
        </button>
        <button onclick={() => loadContainers()} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
          <RefreshCw size={16} />
        </button>
      </div>
    {/if}
    {#if loading}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
        <div class="animate-pulse text-[var(--text-muted)]">Loading containers...</div>
      </div>
    {:else if containers.length === 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
        <Box size={48} class="mb-4 opacity-30" />
        <h2 class="text-lg font-semibold mb-2">No Containers</h2>
        <p class="text-sm text-slate-500">No Docker containers found on this server.</p>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead><tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">ID</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Image</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">State</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Ports</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
            </tr></thead>
            <tbody>
              {#each containers as c}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3 border-b border-white/[0.04] font-mono text-xs text-slate-400">{c.id?.slice(0,12)}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm font-medium text-white">{c.name}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-400">{c.image}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04]">
                    <span class="inline-flex items-center gap-1.5 text-xs">
                      {stateIcon(c.state)} {c.state}
                    </span>
                  </td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-500 max-w-[200px] truncate">{c.ports || '—'}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-right">
                    <div class="flex items-center justify-end gap-1 opacity-60 group-hover:opacity-100 transition">
                      {#if c.state !== 'running'}
                        <button onclick={() => containerAction(c.id, 'start')} class="p-1.5 rounded hover:bg-emerald-500/20 text-emerald-400" title="Start"><Play size={14} /></button>
                      {/if}
                      {#if c.state === 'running'}
                        <button onclick={() => containerAction(c.id, 'stop')} class="p-1.5 rounded hover:bg-amber-500/20 text-amber-400" title="Stop"><Square size={14} /></button>
                        <button onclick={() => containerAction(c.id, 'restart')} class="p-1.5 rounded hover:bg-blue-500/20 text-blue-400" title="Restart"><RotateCcw size={14} /></button>
                      {/if}
                      <button onclick={() => viewLogs(c.id)} class="p-1.5 rounded hover:bg-white/10 text-slate-400" title="Logs"><Terminal size={14} /></button>
                      {#if canManage}
                        <button onclick={() => removeContainer(c.id)} class="p-1.5 rounded hover:bg-red-500/20 text-red-400" title="Remove"><Trash2 size={14} /></button>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Images Tab -->
  {#if activeTab === 'images'}
    {#if canManage}
      <div class="flex gap-3 mb-6">
        <div class="flex gap-2 flex-1 relative">
          <div class="flex-1 relative">
            <input
              value={pullImage}
              oninput={(e) => onImageInput(e.currentTarget.value)}
              onfocus={() => { if (searchResults.length > 0) searchOpen = true; }}
              onblur={() => setTimeout(() => { searchOpen = false; }, 200)}
              placeholder="Search Docker Hub images..."
              class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50"
            />
            {#if searchLoading}
              <div class="absolute right-3 top-1/2 -translate-y-1/2">
                <div class="w-4 h-4 border-2 border-sky-400/30 border-t-sky-400 rounded-full animate-spin"></div>
              </div>
            {/if}
            {#if searchOpen && searchResults.length > 0}
              <div class="absolute z-50 top-full mt-1 w-full bg-[#0c0d0f] border border-white/[0.1] rounded-xl shadow-2xl shadow-black/50 overflow-hidden">
                {#each searchResults as img}
                  <button
                    onmousedown={() => selectImage(img.name)}
                    class="w-full flex items-start gap-3 px-4 py-3 hover:bg-white/[0.05] transition text-left border-b border-white/[0.04] last:border-0"
                  >
                    <Layers size={14} class="text-sky-400 mt-0.5 shrink-0" />
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="text-sm font-medium text-white truncate">{img.name}</span>
                        {#if img.official}
                          <span class="px-1.5 py-0.5 rounded text-[0.6rem] font-bold bg-sky-500/20 text-sky-300">OFFICIAL</span>
                        {/if}
                        {#if img.automated}
                          <span class="px-1.5 py-0.5 rounded text-[0.6rem] font-bold bg-emerald-500/20 text-emerald-300">AUTO</span>
                        {/if}
                      </div>
                      {#if img.description}
                        <p class="text-[0.7rem] text-slate-500 truncate mt-0.5">{img.description}</p>
                      {/if}
                      <div class="flex gap-3 mt-1 text-[0.65rem] text-slate-600">
                        <span>⭐ {fmtPulls(img.stars)}</span>
                        <span>⬇ {fmtPulls(img.pulls)}</span>
                      </div>
                    </div>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
          <button onclick={pullImageAction} disabled={actionPending === 'pull'}
            class="px-4 py-2 bg-sky-500 text-white rounded-lg text-sm font-medium hover:bg-sky-600 transition flex items-center gap-2 disabled:opacity-50 shrink-0">
            <Download size={16} /> Pull
          </button>
        </div>
      </div>
    {/if}
    {#if images.length === 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
        <Layers size={48} class="mb-4 opacity-30" />
        <h2 class="text-lg font-semibold mb-2">No Images</h2>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead><tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Repository</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Tag</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Size</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Created</th>
              {#if canManage}
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
              {/if}
            </tr></thead>
            <tbody>
              {#each images as img}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm font-medium text-white">{img.repository}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04]"><span class="px-2 py-0.5 rounded text-xs bg-white/[0.05] text-slate-400">{img.tag}</span></td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm text-slate-400">{img.size}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-500">{img.created?.slice(0,10)}</td>
                  {#if canManage}
                    <td class="px-5 py-3 border-b border-white/[0.04] text-right">
                      <button onclick={() => removeImage(`${img.repository}:${img.tag}`)} class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-60 group-hover:opacity-100 transition" title="Remove"><Trash2 size={14} /></button>
                    </td>
                  {/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Volumes Tab -->
  {#if activeTab === 'volumes'}
    {#if canManage}
      <div class="flex gap-3 mb-6">
        <div class="flex gap-2 flex-1">
          <input bind:value={createVolume} placeholder="Volume name" class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500" />
          <button onclick={createVolumeAction} class="px-4 py-2 bg-emerald-500 text-white rounded-lg text-sm font-medium hover:bg-emerald-600 transition flex items-center gap-2">
            <Plus size={16} /> Create
          </button>
        </div>
      </div>
    {/if}
    {#if volumes.length === 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
        <HardDrive size={48} class="mb-4 opacity-30" />
        <h2 class="text-lg font-semibold mb-2">No Volumes</h2>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead><tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Driver</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Created</th>
              {#if canManage}
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
              {/if}
            </tr></thead>
            <tbody>
              {#each volumes as v}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm font-medium text-white font-mono">{v.name}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-400">{v.driver}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-500">{v.created?.slice(0,10)}</td>
                  {#if canManage}
                    <td class="px-5 py-3 border-b border-white/[0.04] text-right">
                      <button onclick={() => removeVolume(v.name)} class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-60 group-hover:opacity-100 transition" title="Remove"><Trash2 size={14} /></button>
                    </td>
                  {/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Networks Tab -->
  {#if activeTab === 'networks'}
    {#if canManage}
      <div class="flex gap-3 mb-6">
        <div class="flex gap-2 flex-1">
          <input bind:value={createNetwork} placeholder="Network name" class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500" />
          <button onclick={createNetworkAction} class="px-4 py-2 bg-violet-500 text-white rounded-lg text-sm font-medium hover:bg-violet-600 transition flex items-center gap-2">
            <Plus size={16} /> Create
          </button>
        </div>
      </div>
    {/if}
    {#if networks.length === 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
        <Globe size={48} class="mb-4 opacity-30" />
        <h2 class="text-lg font-semibold mb-2">No Networks</h2>
      </div>
    {:else}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead><tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Driver</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Scope</th>
              {#if canManage}
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
              {/if}
            </tr></thead>
            <tbody>
              {#each networks as n}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm font-medium text-white font-mono">{n.name}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-400">{n.driver}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-xs text-slate-500">{n.scope}</td>
                  {#if canManage}
                    <td class="px-5 py-3 border-b border-white/[0.04] text-right">
                      <button onclick={() => removeNetwork(n.name)} class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-60 group-hover:opacity-100 transition" title="Remove"><Trash2 size={14} /></button>
                    </td>
                  {/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}

  <!-- System Tab -->
  {#if activeTab === 'system'}
    {#if dockerInfo}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">Version</div>
          <div class="text-lg font-bold text-white">{dockerInfo.version}</div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">Driver</div>
          <div class="text-lg font-bold text-white">{dockerInfo.driver}</div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">CPUs</div>
          <div class="text-lg font-bold text-white">{dockerInfo.cpus}</div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">Memory</div>
          <div class="text-lg font-bold text-white">{(parseInt(dockerInfo.memory || '0') / 1073741824).toFixed(1)} GB</div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">Containers</div>
          <div class="text-lg font-bold text-white">{dockerInfo.running} running / {dockerInfo.stopped} stopped</div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
          <div class="text-xs text-slate-500 mb-1">Images</div>
          <div class="text-lg font-bold text-white">{dockerInfo.images}</div>
        </div>
      </div>
    {/if}

    {#if diskUsage.length > 0}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <div class="px-5 py-4 border-b border-white/[0.06]">
          <h3 class="text-sm font-semibold text-white">Disk Usage</h3>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead><tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Type</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Count</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Size</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Reclaimable</th>
            </tr></thead>
            <tbody>
              {#each diskUsage as u}
                <tr class="hover:bg-white/[0.02] transition">
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm font-medium text-white">{u.type}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm text-slate-400">{u.count}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm text-slate-400">{u.size}</td>
                  <td class="px-5 py-3 border-b border-white/[0.04] text-sm text-emerald-400">{u.reclaimable}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Pull Progress Modal -->
  {#if pullProgress.active}
    <div class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4" onclick={() => { if (pullProgress.done) pullProgress = {...pullProgress, active: false}; }}>
      <div class="bg-[#0c0d0f] border border-white/[0.08] rounded-xl w-full max-w-lg" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <div class="flex items-center gap-3">
            {#if !pullProgress.done}
              <div class="w-5 h-5 border-2 border-sky-400/30 border-t-sky-400 rounded-full animate-spin"></div>
            {:else if pullProgress.error}
              <div class="w-5 h-5 rounded-full bg-red-500/20 flex items-center justify-center"><span class="text-red-400 text-xs">✕</span></div>
            {:else}
              <div class="w-5 h-5 rounded-full bg-emerald-500/20 flex items-center justify-center"><span class="text-emerald-400 text-xs">✓</span></div>
            {/if}
            <h3 class="text-sm font-semibold text-white">
              {#if !pullProgress.done}
                Pulling <span class="text-sky-400">{pullProgress.image}</span>...
              {:else if pullProgress.error}
                Pull failed
              {:else}
                <span class="text-emerald-400">{pullProgress.image}</span> pulled
              {/if}
            </h3>
          </div>
          {#if pullProgress.done}
            <button onclick={() => pullProgress = {...pullProgress, active: false}} class="text-slate-500 hover:text-white text-lg">×</button>
          {/if}
        </div>
        <div class="px-5 py-4 max-h-[50vh] overflow-auto">
          {#if pullProgress.error}
            <p class="text-sm text-red-400">{pullProgress.error}</p>
          {:else if pullProgress.layers.length === 0 && !pullProgress.done}
            <div class="flex items-center gap-3 py-4">
              <div class="flex-1 h-2 bg-white/5 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-sky-400 to-indigo-400 rounded-full animate-pulse" style="width: 60%"></div>
              </div>
              <span class="text-xs text-slate-500 shrink-0">Connecting...</span>
            </div>
          {:else}
            <div class="space-y-2">
              {#each pullProgress.layers as layer}
                {@const isComplete = layer.status.includes('complete') || layer.status.includes('already') || layer.status === 'Pull complete' || layer.status.startsWith('Status:')}
                {@const isDownloading = layer.status.includes('Downloading') || layer.status.includes('Pulling')}
                <div class="flex items-start gap-3 text-xs">
                  {#if isComplete}
                    <span class="text-emerald-400 mt-0.5 shrink-0">✓</span>
                  {:else if isDownloading}
                    <div class="w-3.5 h-3.5 mt-0.5 shrink-0 border-2 border-sky-400/30 border-t-sky-400 rounded-full animate-spin"></div>
                  {:else}
                    <span class="text-slate-600 mt-0.5 shrink-0">○</span>
                  {/if}
                  <div class="flex-1 min-w-0">
                    <span class="font-mono text-slate-500">{layer.id === 'output' ? '' : layer.id.slice(0,12)}</span>
                    <span class="text-slate-400 ml-1">{layer.status}</span>
                  </div>
                </div>
              {/each}
              {#if pullProgress.done && !pullProgress.error}
                <div class="flex items-center gap-2 pt-2 mt-2 border-t border-white/[0.04]">
                  <span class="text-emerald-400 text-xs">✓ Image ready</span>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <!-- Create Container Modal -->
  {#if showCreateContainer}
    <div class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4" onclick={() => showCreateContainer = false}>
      <div class="bg-[#0c0d0f] border border-white/[0.08] rounded-xl w-full max-w-lg max-h-[90vh] flex flex-col" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <h3 class="text-sm font-semibold text-white">Create Container</h3>
          <button onclick={() => showCreateContainer = false} class="text-slate-500 hover:text-white text-lg">×</button>
        </div>
        <div class="px-5 py-4 overflow-auto flex-1 space-y-4">
          <!-- Image (required) -->
          <div class="relative">
            <label class="block text-xs font-medium text-slate-400 mb-1">Image <span class="text-red-400">*</span></label>
            <input
              value={createContainerForm.image}
              oninput={(e) => onCreateImageInput(e.currentTarget.value)}
              onfocus={() => { if (createImageResults.length > 0) createImageOpen = true; }}
              onblur={() => setTimeout(() => { createImageOpen = false; }, 200)}
              placeholder="Search local images or Docker Hub..."
              class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50"
            />
            {#if createImageOpen && createImageResults.length > 0}
              <div class="absolute z-50 top-full mt-1 w-full bg-[#0c0d0f] border border-white/[0.1] rounded-xl shadow-2xl shadow-black/50 overflow-hidden max-h-60 overflow-y-auto">
                {#each createImageResults as img}
                  <button
                    onmousedown={() => selectCreateImage(img.name)}
                    class="w-full flex items-center gap-3 px-4 py-2.5 hover:bg-white/[0.05] transition text-left border-b border-white/[0.04] last:border-0"
                  >
                    {#if img.source === 'local'}
                      <div class="w-5 h-5 rounded bg-emerald-500/20 flex items-center justify-center shrink-0">
                        <span class="text-emerald-400 text-[0.6rem] font-bold">L</span>
                      </div>
                      <div class="flex-1 min-w-0">
                        <span class="text-sm font-medium text-white">{img.name}</span>
                        <span class="text-[0.65rem] text-emerald-400 ml-2">local</span>
                        {#if img.size}
                          <span class="text-[0.65rem] text-slate-600 ml-2">{img.size}</span>
                        {/if}
                      </div>
                    {:else}
                      <div class="w-5 h-5 rounded bg-sky-500/20 flex items-center justify-center shrink-0">
                        <span class="text-sky-400 text-[0.6rem] font-bold">H</span>
                      </div>
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-1.5">
                          <span class="text-sm font-medium text-white">{img.name}</span>
                          {#if img.official}
                            <span class="px-1 py-0.5 rounded text-[0.55rem] font-bold bg-sky-500/20 text-sky-300">OFFICIAL</span>
                          {/if}
                        </div>
                        {#if img.description}
                          <p class="text-[0.65rem] text-slate-500 truncate">{img.description}</p>
                        {/if}
                        <div class="flex gap-2 mt-0.5 text-[0.6rem] text-slate-600">
                          {#if img.stars}<span>⭐ {img.stars}</span>{/if}
                          {#if img.pulls}<span>⬇ {img.pulls >= 1e6 ? (img.pulls/1e6).toFixed(1)+'M' : img.pulls}</span>{/if}
                        </div>
                      </div>
                    {/if}
                  </button>
                {/each}
              </div>
            {/if}
          </div>
          <!-- Name -->
          <div>
            <label class="block text-xs font-medium text-slate-400 mb-1">Container Name</label>
            <input bind:value={createContainerForm.name} placeholder="my-nginx (auto-generated if empty)" class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50" />
          </div>
          <!-- Command -->
          <div>
            <label class="block text-xs font-medium text-slate-400 mb-1">Command (optional)</label>
            <input bind:value={createContainerForm.command} placeholder="sh -c 'echo hello'" class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50" />
          </div>
          <!-- Ports -->
          <div>
            <label class="block text-xs font-medium text-slate-400 mb-1">Ports <span class="text-slate-600">(comma separated: host:container)</span></label>
            <input bind:value={createContainerForm.ports} placeholder="8080:80, 443:443" class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50" />
          </div>
          <!-- Volumes -->
          <div>
            <label class="block text-xs font-medium text-slate-400 mb-1">Volumes <span class="text-slate-600">(comma separated: host:container)</span></label>
            <input bind:value={createContainerForm.volumes} placeholder="/data:/var/www/html" class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50" />
          </div>
          <!-- Environment -->
          <div>
            <label class="block text-xs font-medium text-slate-400 mb-1">Environment <span class="text-slate-600">(one KEY=value per line)</span></label>
            <textarea bind:value={createContainerForm.env} rows={3} placeholder="NODE_ENV=production&#10;PORT=3000" class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500/50 font-mono resize-none"></textarea>
          </div>
          <!-- Row: Restart + Network -->
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs font-medium text-slate-400 mb-1">Restart Policy</label>
              <select bind:value={createContainerForm.restart} class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
                <option value="no">No</option>
                <option value="always">Always</option>
                <option value="unless-stopped">Unless Stopped</option>
                <option value="on-failure">On Failure</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-400 mb-1">Network</label>
              <select bind:value={createContainerForm.network} class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
                <option value="bridge">Bridge</option>
                <option value="host">Host</option>
                <option value="none">None</option>
              </select>
            </div>
          </div>
          <!-- Auto start -->
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" bind:checked={createContainerForm.auto_start} class="rounded border-white/20 bg-white/5 text-sky-500 focus:ring-sky-500/50" />
            <span class="text-sm text-slate-400">Start immediately after creation</span>
          </label>
        </div>
        <div class="px-5 py-4 border-t border-white/[0.06] flex justify-end gap-3">
          <button onclick={() => showCreateContainer = false} class="px-4 py-2 text-sm text-slate-400 hover:text-white transition">Cancel</button>
          <button onclick={createContainerAction} disabled={createPending || !createContainerForm.image}
            class="px-4 py-2 bg-sky-500 text-white rounded-lg text-sm font-medium hover:bg-sky-600 transition flex items-center gap-2 disabled:opacity-50">
            {#if createPending}
              <div class="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></div>
              Creating...
            {:else}
              <Plus size={16} /> Create
            {/if}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Logs Modal -->
  {#if showLogs}
    <div class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4" onclick={() => showLogs = false}>
      <div class="bg-[#0c0d0f] border border-white/[0.08] rounded-xl w-full max-w-4xl max-h-[80vh] flex flex-col" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between px-5 py-4 border-b border-white/[0.06]">
          <h3 class="text-sm font-semibold text-white">Container Logs: {logsContainer?.slice(0,12)}</h3>
          <button onclick={() => showLogs = false} class="text-slate-500 hover:text-white text-lg">×</button>
        </div>
        <pre class="flex-1 overflow-auto p-5 text-xs text-slate-300 font-mono whitespace-pre-wrap">{logs}</pre>
      </div>
    </div>
  {/if}
</DashboardLayout>
