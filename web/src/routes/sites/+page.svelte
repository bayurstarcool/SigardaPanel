<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Globe, Search, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let sites = $state<any[]>([]);
  let totalCount = $state(0);
  let loading = $state(true);
  let showCreate = $state(false);
  let form = $state({ server_id: 0, name: '', domain: '', runtime: 'static', php_version: '8.3', root_path: '', proxy_pass_url: '', docker_image: '', docker_language: '', docker_name: '', docker_ports: '', docker_env: '', docker_volume: '', docker_restart: 'unless-stopped' });
  let phpVersions = ['8.5', '8.4', '8.3', '8.2', '8.1', '8.0', '7.4'];

  // Docker language presets
  const dockerPresets = [
    { lang: 'static', label: 'Static HTML', icon: '🌐', image: 'nginx:alpine', desc: 'Plain HTML/CSS/JS' },
    { lang: 'node', label: 'Node.js', icon: '🟢', image: 'node:20-alpine', desc: 'Express, Fastify, etc.' },
    { lang: 'python', label: 'Python', icon: '🐍', image: 'python:3.12-slim', desc: 'Flask, Django, FastAPI' },
    { lang: 'php', label: 'PHP', icon: '🐘', image: 'php:8.3-apache', desc: 'Laravel, WordPress' },
    { lang: 'go', label: 'Go', icon: '🐹', image: 'golang:1.22-alpine', desc: 'Echo, Gin, Chi' },
    { lang: 'ruby', label: 'Ruby', icon: '💎', image: 'ruby:3.3-alpine', desc: 'Rails, Sinatra' },
    { lang: 'java', label: 'Java', icon: '☕', image: 'eclipse-temurin:21-jre', desc: 'Spring Boot, Quarkus' },
    { lang: 'rust', label: 'Rust', icon: '🦀', image: 'rust:1.78-slim', desc: 'Actix, Axum' },
  ];
  let selectedPreset = $state('');

  // Manually tracked — Svelte 5 $derived/$state unreliable in nested {#if}
  let showDockerConfig = $state(false);

  // Docker Hub image search
  let imageSearch = $state('');
  let imageSearchResults = $state<any[]>([]);
  let imageSearchOpen = $state(false);
  let imageSearchTimer: ReturnType<typeof setTimeout> | null = null;

  function onImageSearchInput(value: string) {
    form.docker_image = value;
    imageSearch = value;
    if (imageSearchTimer) clearTimeout(imageSearchTimer);
    if (!value || value.length < 2) { imageSearchOpen = false; imageSearchResults = []; return; }
    imageSearchTimer = setTimeout(async () => {
      try {
        const sid = form.server_id || servers[0]?.id;
        if (!sid) return;
        const d: any = await apiFetch(auth.token, 'GET', `servers/${sid}/docker/images/search?q=${encodeURIComponent(value)}&page_size=8`);
        imageSearchResults = d?.results ?? [];
        imageSearchOpen = imageSearchResults.length > 0;
      } catch { imageSearchResults = []; imageSearchOpen = false; }
    }, 300);
  }

  function selectImageResult(img: any) {
    form.docker_image = img.name;
    imageSearch = img.name;
    imageSearchOpen = false;
    imageSearchResults = [];
    // Auto-detect language from selected image
    const detected = detectLangFromImage(img.name);
    if (detected) {
      selectedPreset = detected;
      form.docker_language = detected;
    }
  }

  function detectLangFromImage(image: string): string {
    const img = image.toLowerCase();
    if (img.includes('nginx')) return 'static';
    if (img.includes('node') || img.includes('bun')) return 'node';
    if (img.includes('python')) return 'python';
    if (img.includes('php')) return 'php';
    if (img.includes('golang') || img.includes('go:')) return 'go';
    if (img.includes('ruby')) return 'ruby';
    if (img.includes('java') || img.includes('temurin')) return 'java';
    if (img.includes('rust')) return 'rust';
    return '';
  }

  function fmtPulls(n: number): string {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
  }

  function selectPreset(preset: typeof dockerPresets[0]) {
    selectedPreset = preset.lang;
    form.docker_language = preset.lang;
    form.docker_image = preset.image;
    imageSearch = preset.image;
    imageSearchOpen = false;
    showDockerConfig = true;
  }
  // When runtime changes, reset docker fields if switching away from docker
  function onRuntimeChange() {
    if (form.runtime !== 'docker') {
      selectedPreset = '';
      form.docker_language = '';
      form.docker_image = '';
      form.docker_name = '';
      form.docker_ports = '';
      form.docker_env = '';
      form.docker_volume = '';
      form.docker_restart = 'unless-stopped';
      showDockerConfig = false;
    }
  }
  let busy = $state(false);
  let cfEnabled = $state(true);
  let servers = $state<any[]>([]);
  let cfResult = $state<{provisioned: boolean; zone: string; error: string} | null>(null);

  let searchInput = $state('');
  let runtimeFilter = $state('');
  let statusFilter = $state('');
  let page = $state(1);
  let pageSize = $state(10);
  let sortKey = $state<'domain' | 'runtime' | 'status' | 'created_at'>('domain');
  let sortDir = $state<'asc' | 'desc'>('asc');
  const PAGE_SIZES = [10, 25, 50, 100];

  function buildQuery(params: Record<string, string | number | null>) {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === null || v === '' || v === undefined) continue;
      p.set(k, String(v));
    }
    return p.toString();
  }

  async function fetchSites() {
    if (!auth.user) { window.location.href = '/login'; return; }
    loading = true;
    const params: Record<string, string | number> = {
      limit: pageSize,
      offset: (page - 1) * pageSize,
    };
    if (searchInput) params.search = searchInput;
    if (runtimeFilter) params.runtime = runtimeFilter;
    if (statusFilter) params.status = statusFilter;
    try {
      const d = await apiFetch(auth.token, 'GET', `sites?${buildQuery(params)}`);
      sites = d?.sites || [];
      totalCount = d?.total_count ?? d?.sites?.length ?? 0;
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    }
    finally { loading = false; }
  }

  async function fetchServers() {
    if (!auth.user) return;
    try {
      const d = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers || [];
    } catch {}
  }

  onMount(() => { fetchSites(); fetchServers().then(() => {
    if (servers.length === 1 && form.server_id === 0) form.server_id = servers[0].id;
  }); });
  $effect(() => { page; pageSize; runtimeFilter; statusFilter; fetchSites(); });
  $effect(() => { if (servers.length === 1 && form.server_id === 0) form.server_id = servers[0].id; });

  const totalPages = $derived(Math.max(1, Math.ceil(totalCount / pageSize)));
  const sortedSites = $derived((() => {
    const arr = [...sites];
    arr.sort((a: any, b: any) => {
      const va = a[sortKey] || '';
      const vb = b[sortKey] || '';
      return sortDir === 'asc' ? String(va).localeCompare(String(vb)) : String(vb).localeCompare(String(va));
    });
    return arr;})())

  function toggleSort(key: string) {
    if (sortKey === key) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortKey = key as any; sortDir = 'asc'; }
  }

  function pages(): (number | string)[] {
    const p: (number | string)[] = [];
    const tp = totalPages;
    if (tp <= 7) { for (let i = 1; i <= tp; i++) p.push(i); }
    else {
      p.push(1);
      if (page > 3) p.push('...');
      for (let i = Math.max(2, page - 1); i <= Math.min(tp - 1, page + 1); i++) p.push(i);
      if (page < tp - 2) p.push('...');
      p.push(tp);
    }
    return p;
  }

  async function createSite() {
    if (!form.name || !form.domain || form.server_id <= 0) return;
    if (form.runtime === 'proxy' && !form.proxy_pass_url.trim()) {
      toasts.error('Proxy Pass URL is required for proxy runtime');
      return;
    }
    busy = true;
    try {
      await apiFetch(auth.token, 'POST', 'sites', { ...form, cloudflare_provision: cfEnabled });
      showCreate = false;
      form = { server_id: 0, name: '', domain: '', runtime: 'static', php_version: '8.3', root_path: '', proxy_pass_url: '', docker_image: '', docker_language: '', docker_name: '', docker_ports: '', docker_env: '', docker_volume: '', docker_restart: 'unless-stopped' };
      selectedPreset = '';
      showDockerConfig = false;
      fetchSites();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = false; }
  }

  let searchTimeout: ReturnType<typeof setTimeout>;
  function onSearch(e: Event) {
    searchInput = (e.target as HTMLInputElement).value;
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => { page = 1; fetchSites(); }, 300);
  }

  function SortIcon({ col }: { col: string }) {
    return sortKey === col ? (sortDir === 'asc' ? '↑' : '↓') : '';
  }
</script>

<DashboardLayout active="Sites">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Sites</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${totalCount} site(s) total`}</p>
    </div>
    <div class="flex items-center gap-3">
      <div class="relative">
        <Search size={14} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
        <input type="text" placeholder="Search domains..." value={searchInput} oninput={onSearch}
          class="bg-white/[0.04] border border-white/[0.07] rounded-xl pl-9 pr-3 py-2 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition w-48" />
      </div>
      <button onclick={() => showCreate = !showCreate}
        class="inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer border-0 whitespace-nowrap">
        + Create Site
      </button>
    </div>
  </div>

  <!-- Create form -->
  {#if showCreate}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
      <h3 class="text-sm font-semibold text-slate-200 mb-4">Create New Site</h3>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label class="block text-xs text-slate-500 mb-1.5">Server</label>
          <select bind:value={form.server_id} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40">
            <option value={0}>Select server...</option>
            {#each servers as s}<option value={s.id}>{s.name} ({s.hostname})</option>{/each}
          </select>
        </div>
        <div>
          <label for="site-name" class="block text-xs text-slate-500 mb-1.5">Site Name</label>
          <input id="site-name" bind:value={form.name} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="my-site" />
        </div>
        <div>
          <label for="site-domain" class="block text-xs text-slate-500 mb-1.5">Domain</label>
          <input id="site-domain" bind:value={form.domain} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="example.com" />
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1.5">Runtime</label>
          <select bind:value={form.runtime} onchange={onRuntimeChange} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40">
            {#each ['static', 'php', 'node', 'python', 'go', 'proxy', 'docker'] as r}<option value={r}>{r}</option>{/each}
          </select>
        </div>
        {#if form.runtime === 'php'}
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">PHP Version</label>
            <select bind:value={form.php_version} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 font-mono">
              {#each phpVersions as v}<option value={v}>PHP {v}</option>{/each}
            </select>
          </div>
        {/if}
        <div>
          <label class="block text-xs text-slate-500 mb-1.5">Root Path</label>
          <input bind:value={form.root_path} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="/home/user/htdocs/example.com (auto if empty)" />
        </div>
        <div class="flex items-end">
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" bind:checked={cfEnabled} class="rounded border-white/20 bg-white/[0.04] text-sky-400 focus:ring-sky-400/20" />
            <span class="text-xs text-slate-400">Cloudflare DNS</span>
          </label>
        </div>
      </div>
      {#if form.runtime === 'docker'}
        <div class="mt-4 pt-4 border-t border-white/[0.05]">
          <div class="flex items-center gap-2 mb-3">
            <span class="text-xs font-semibold text-sky-400 bg-sky-500/10 px-2 py-0.5 rounded-full">1</span>
            <label class="block text-xs text-slate-400 font-medium">Pilih Bahasa / Stack</label>
          </div>
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
            {#each dockerPresets as p}
              <button type="button" onclick={() => selectPreset(p)}
                class="flex flex-col items-center gap-1 px-3 py-3 rounded-xl text-xs transition-all duration-150 cursor-pointer border {form.docker_language === p.lang ? 'bg-sky-500/15 border-sky-400/40 text-sky-300 shadow-[0_0_12px_rgba(56,189,248,0.1)]' : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:bg-white/[0.06] hover:text-slate-200'}">
                <span class="text-xl">{p.icon}</span>
                <span class="font-medium">{p.label}</span>
                <span class="text-[0.65rem] opacity-60 leading-tight text-center">{p.desc}</span>
              </button>
            {/each}
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-4 pt-4 border-t border-white/[0.05]">
          <div class="flex items-center gap-2 mb-1 sm:col-span-2">
            <span class="text-xs font-semibold text-sky-400 bg-sky-500/10 px-2 py-0.5 rounded-full">2</span>
            <label class="block text-xs text-slate-400 font-medium">Konfigurasi Container</label>
          </div>
          <div class="sm:col-span-2">
            <label class="block text-xs text-slate-500 mb-1.5">Docker Image</label>
            <div class="relative">
              <input
                value={imageSearch}
                oninput={(e) => onImageSearchInput(e.currentTarget.value)}
                onfocus={() => { if (imageSearchResults.length > 0) imageSearchOpen = true; }}
                onblur={() => setTimeout(() => { imageSearchOpen = false; }, 200)}
                class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 font-mono"
                placeholder="Search Docker Hub or type image:tag..."
              />
              {#if imageSearchOpen && imageSearchResults.length > 0}
                <div class="absolute z-50 top-full mt-1 w-full bg-[#0c0d0f] border border-white/[0.1] rounded-xl shadow-2xl shadow-black/50 overflow-hidden max-h-64 overflow-y-auto">
                  {#each imageSearchResults as img}
                    <button
                      onmousedown={() => selectImageResult(img)}
                      class="w-full flex items-start gap-3 px-4 py-2.5 hover:bg-white/[0.05] transition text-left border-b border-white/[0.04] last:border-0 cursor-pointer"
                    >
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2">
                          <span class="text-sm font-medium text-white truncate">{img.name}</span>
                          {#if img.official}
                            <span class="px-1.5 py-0.5 rounded text-[0.6rem] font-bold bg-sky-500/20 text-sky-300">OFFICIAL</span>
                          {/if}
                        </div>
                        {#if img.description}
                          <p class="text-[0.7rem] text-slate-500 truncate mt-0.5">{img.description}</p>
                        {/if}
                        <div class="flex gap-3 mt-0.5 text-[0.65rem] text-slate-600">
                          <span>⭐ {fmtPulls(img.stars)}</span>
                          <span>⬇ {fmtPulls(img.pulls)}</span>
                        </div>
                      </div>
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
            <p class="text-[0.65rem] text-slate-600 mt-1">Pilih dari preset di atas, search, atau ketik manual.</p>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">Container Name</label>
            <input bind:value={form.docker_name} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="auto: site-name" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">Port Mapping</label>
            <input bind:value={form.docker_ports} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="auto" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">Volumes</label>
            <input bind:value={form.docker_volume} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="/host/path:/container/path" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">Restart Policy</label>
            <select bind:value={form.docker_restart} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40">
              <option value="unless-stopped">Unless Stopped</option>
              <option value="always">Always</option>
              <option value="no">No</option>
            </select>
          </div>
          <div class="sm:col-span-2">
            <label class="block text-xs text-slate-500 mb-1.5">Environment (one per line: KEY=value)</label>
            <textarea bind:value={form.docker_env} rows={2} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 font-mono resize-none" placeholder="APP_ENV=production&#10;PORT=3000"></textarea>
          </div>
        </div>
      {/if}
      {#if form.runtime === 'proxy'}
        <div class="grid grid-cols-1 gap-4 mt-4 pt-4 border-t border-white/[0.05]">
          <div>
            <label class="block text-xs text-slate-500 mb-1.5">Proxy Pass URL <span class="text-red-400">*</span></label>
            <input bind:value={form.proxy_pass_url} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40" placeholder="http://127.0.0.1:3000" />
          </div>
        </div>
      {/if}
      <!-- Create/Cancel buttons -->
      <div class="flex gap-3 mt-4">
        {#if form.runtime === 'docker'}
          <!-- Docker: require preset selection before enabling create -->
          <button onclick={createSite} disabled={busy || !form.name || !form.domain || form.server_id <= 0 || !showDockerConfig}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
            {busy ? 'Creating...' : showDockerConfig ? 'Create' : 'Pilih bahasa dulu ↓'}
          </button>
        {:else}
          <button onclick={createSite} disabled={busy || !form.name || !form.domain || form.server_id <= 0 || (form.runtime === 'proxy' && !form.proxy_pass_url.trim())}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
            {busy ? 'Creating...' : 'Create'}
          </button>
        {/if}
        <button onclick={() => showCreate = false} class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
      </div>
      {#if cfResult}
        <div class="mt-3 text-xs {cfResult.provisioned ? 'text-emerald-400' : 'text-amber-400'}">
          {cfResult.provisioned ? `DNS provisioned in zone ${cfResult.zone}` : cfResult.error}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Filters -->
  <div class="flex items-center gap-3 mb-4">
    <select bind:value={runtimeFilter} onchange={() => { page = 1; fetchSites(); }}
      class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-300 focus:outline-none focus:border-sky-400/40">
      <option value="">All runtimes</option>
      {#each ['static', 'php', 'node', 'python', 'go', 'docker', 'proxy'] as r}<option value={r}>{r}</option>{/each}
    </select>
    <select bind:value={statusFilter} onchange={() => { page = 1; fetchSites(); }}
      class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-300 focus:outline-none focus:border-sky-400/40">
      <option value="">All statuses</option>
      {#each ['active', 'pending', 'deploying', 'disabled'] as s}<option value={s}>{s}</option>{/each}
    </select>
  </div>

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20"><div class="animate-pulse text-[var(--text-muted)]">Loading sites...</div></div>
  {:else if sites.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Globe size={48} class="mb-4 opacity-30 mx-auto" />
      <h2 class="text-lg font-semibold mb-2">No Sites</h2>
      <p class="text-sm text-slate-500 mb-6">Create your first site to get started.</p>
      <button onclick={() => showCreate = true} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition cursor-pointer border-0">+ Create Site</button>
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead><tr class="text-left">
            {#each [
              { key: 'domain', label: 'Domain' },
              { key: 'runtime', label: 'Runtime' },
              { key: 'managed_by', label: 'Managed' },
              { key: null, label: 'System User' },
              { key: null, label: 'Root / Proxy' },
              { key: 'status', label: 'Status' },
              { key: null, label: 'SSL' },
              { key: 'created_at', label: 'Created' },
              { key: null, label: '' },
            ] as col}
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap {col.key ? 'cursor-pointer hover:text-slate-300' : ''}"
                onclick={() => col.key && toggleSort(col.key)}>
                {col.label} {col.key && sortKey === col.key ? (sortDir === 'asc' ? '↑' : '↓') : ''}
              </th>
            {/each}
          </tr></thead>
          <tbody>
            {#each sortedSites as s}
              <tr class="hover:bg-white/[0.02] transition">
                <td class="px-5 py-3.5 border-b border-white/[0.03] font-medium align-middle">
                  <a href="/sites/{s.id}" class="text-sky-400 hover:text-sky-300 hover:underline transition-colors">{s.domain}</a>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs align-middle">
                  <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[0.72rem] font-medium bg-white/[0.05] text-slate-300">{s.runtime}</span>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[0.72rem] font-medium bg-sky-500/10 text-sky-300">
                    <span class="w-1.5 h-1.5 rounded-full bg-sky-400"></span>{s.managed_by || 'sigardapanel'}
                  </span>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-400 align-middle">{s.system_user || '—'}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-500 align-middle max-w-[18rem] truncate" title={s.proxy_pass_url || s.root_path || s.conf_file || ''}>{s.proxy_pass_url || s.root_path || '—'}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <div class="flex flex-col gap-1">
                    <StatusPill status={s.status} />
                    {#if s.status === 'pending'}
                      <span class="text-[0.6rem] text-amber-400/70 leading-tight">
                        {#if !s.conf_file}
                          ⚠ nginx config belum terdaftar
                        {:else}
                          ⚠ menunggu provisioning
                        {/if}
                      </span>
                    {/if}
                  </div>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs align-middle">
                  {#if s.ssl_status && s.ssl_status !== 'none'}
                    <span class="inline-flex items-center gap-1.5 px-3 py-0.5 rounded-full text-[0.72rem] font-medium bg-emerald-500/10 text-emerald-400"><span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>{s.ssl_status}</span>
                  {:else}
                    <span class="text-slate-600">—</span>
                  {/if}
                  {#if s.app_status}
                    <span class="inline-flex items-center gap-1 ml-1.5 {s.app_status === 'running' ? 'text-emerald-400' : 'text-slate-500'}">
                      <span class="w-1 h-1 rounded-full {s.app_status === 'running' ? 'bg-emerald-400' : 'bg-slate-600'}"></span>
                      <span class="text-[0.65rem]">{s.app_status}</span>
                    </span>
                  {/if}
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-500 align-middle whitespace-nowrap">
                  {new Date(s.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })}
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <a href="/sites/{s.id}" class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition-all duration-150 cursor-pointer whitespace-nowrap">Manage →</a>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <!-- Pagination -->
    <div class="flex flex-col sm:flex-row items-center justify-between gap-4 mt-6">
      <div class="flex items-center gap-3 text-xs text-slate-500">
        <span>Showing {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, totalCount)} of {totalCount}</span>
        <span class="text-slate-700">&middot;</span>
        <div class="flex items-center gap-1.5">
          <span>Per page</span>
          <select bind:value={pageSize} onchange={() => { page = 1; fetchSites(); }}
            class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-2 py-1 text-xs text-slate-300 focus:outline-none focus:border-sky-400/40">
            {#each PAGE_SIZES as ps}<option value={ps}>{ps}</option>{/each}
          </select>
        </div>
      </div>
      <div class="flex items-center gap-1">
        <button disabled={page <= 1} onclick={() => page--}
          class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"><ChevronLeft size={14} /></button>
        {#each pages() as p}
          {#if p === '...'}
            <span class="w-8 h-8 flex items-center justify-center text-xs text-slate-600">…</span>
          {:else}
            <button onclick={() => page = p as number}
              class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-xs font-medium transition-all duration-150 {p === page ? 'bg-sky-500/20 text-sky-300 border border-sky-400/30 shadow-[0_0_12px_rgba(56,189,248,0.1)]' : 'text-slate-400 hover:bg-white/[0.05] hover:text-slate-200'}">{p}</button>
          {/if}
        {/each}
        <button disabled={page >= totalPages} onclick={() => page++}
          class="inline-flex items-center justify-center w-8 h-8 rounded-lg text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"><ChevronRight size={14} /></button>
      </div>
    </div>
  {/if}
</DashboardLayout>
