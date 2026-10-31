<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Pagination from '$components/Pagination.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Globe, Search, Plus, ArrowUp, ArrowDown, ChevronRight, Star, Download, TriangleAlert, X } from 'lucide-svelte';
  import { onMount, untrack } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let sites = $state<any[]>([]);
  let totalCount = $state(0);
  let loading = $state(true);
  let showCreate = $state(false);
  let form = $state({ server_id: 0, name: '', domain: '', runtime: 'static', php_version: '8.3', root_path: '', proxy_pass_url: '', docker_image: '', docker_language: '', docker_name: '', docker_ports: '', docker_env: '', docker_volume: '', docker_restart: 'unless-stopped' });
  let phpVersions = ['8.5', '8.4', '8.3', '8.2', '8.1', '8.0', '7.4'];

  // Docker language presets
  const dockerPresets = [
    { lang: 'static', label: 'Static HTML', image: 'nginx:alpine', desc: 'Plain HTML/CSS/JS' },
    { lang: 'node', label: 'Node.js', image: 'node:20-alpine', desc: 'Express, Fastify, etc.' },
    { lang: 'python', label: 'Python', image: 'python:3.12-slim', desc: 'Flask, Django, FastAPI' },
    { lang: 'php', label: 'PHP', image: 'php:8.3-apache', desc: 'Laravel, WordPress' },
    { lang: 'go', label: 'Go', image: 'golang:1.22-alpine', desc: 'Echo, Gin, Chi' },
    { lang: 'ruby', label: 'Ruby', image: 'ruby:3.3-alpine', desc: 'Rails, Sinatra' },
    { lang: 'java', label: 'Java', image: 'eclipse-temurin:21-jre', desc: 'Spring Boot, Quarkus' },
    { lang: 'rust', label: 'Rust', image: 'rust:1.78-slim', desc: 'Actix, Axum' },
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
  let sortKey = $state<'domain' | 'runtime' | 'status' | 'created_at' | 'managed_by'>('domain');
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

  onMount(() => { fetchServers(); });
  // Refetch whenever paging or filters change (also runs once on mount).
  $effect(() => { page; pageSize; runtimeFilter; statusFilter; untrack(fetchSites); });
  $effect(() => { if (servers.length === 1 && form.server_id === 0) form.server_id = servers[0].id; });

  const sortedSites = $derived((() => {
    const arr = [...sites];
    arr.sort((a: any, b: any) => {
      const va = a[sortKey] || '';
      const vb = b[sortKey] || '';
      return sortDir === 'asc' ? String(va).localeCompare(String(vb)) : String(vb).localeCompare(String(va));
    });
    return arr;})());

  function toggleSort(key: string) {
    if (sortKey === key) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    else { sortKey = key as any; sortDir = 'asc'; }
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
    // Resetting the page refetches via the effect; otherwise fetch directly.
    searchTimeout = setTimeout(() => { if (page !== 1) page = 1; else fetchSites(); }, 300);
  }

  const columns: { key: string | null; label: string; cls?: string }[] = [
    { key: 'domain', label: 'Domain' },
    { key: 'runtime', label: 'Runtime' },
    { key: 'managed_by', label: 'Managed', cls: 'max-2xl:hidden' },
    { key: null, label: 'System user', cls: 'max-2xl:hidden' },
    { key: null, label: 'Root / proxy', cls: 'max-md:hidden' },
    { key: 'status', label: 'Status' },
    { key: null, label: 'SSL', cls: 'max-sm:hidden' },
    { key: 'created_at', label: 'Created', cls: 'max-md:hidden' },
  ];

  const createDisabled = $derived(
    busy || !form.name || !form.domain || form.server_id <= 0 ||
    (form.runtime === 'docker' ? !showDockerConfig : form.runtime === 'proxy' && !form.proxy_pass_url.trim()),
  );
</script>

<DashboardLayout active="Sites">
  <PageHeader title="Sites" description={loading ? 'Loading…' : `${totalCount} site${totalCount === 1 ? '' : 's'} total`}>
    {#snippet actions()}
      <button onclick={() => (showCreate = !showCreate)} class="btn btn-primary btn-sm">
        {#if showCreate}<X size={14} /> Close{:else}<Plus size={14} /> Create site{/if}
      </button>
    {/snippet}
  </PageHeader>

  <!-- Create form -->
  {#if showCreate}
    <div class="card mb-6 animate-fade-in">
      <div class="card-header"><h2 class="card-title">Create new site</h2></div>
      <div class="card-body">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label for="site-server" class="label">Server</label>
            <select id="site-server" bind:value={form.server_id} class="input">
              <option value={0}>Select server…</option>
              {#each servers as s (s.id)}<option value={s.id}>{s.name} ({s.hostname})</option>{/each}
            </select>
          </div>
          <div>
            <label for="site-name" class="label">Site name</label>
            <input id="site-name" bind:value={form.name} class="input" placeholder="my-site" />
          </div>
          <div>
            <label for="site-domain" class="label">Domain</label>
            <input id="site-domain" bind:value={form.domain} class="input" placeholder="example.com" />
          </div>
          <div>
            <label for="site-runtime" class="label">Runtime</label>
            <select id="site-runtime" bind:value={form.runtime} onchange={onRuntimeChange} class="input">
              {#each ['static', 'php', 'node', 'python', 'go', 'proxy', 'docker'] as r (r)}<option value={r}>{r}</option>{/each}
            </select>
          </div>
          {#if form.runtime === 'php'}
            <div>
              <label for="site-php" class="label">PHP version</label>
              <select id="site-php" bind:value={form.php_version} class="input font-mono">
                {#each phpVersions as v (v)}<option value={v}>PHP {v}</option>{/each}
              </select>
            </div>
          {/if}
          <div>
            <label for="site-root" class="label">Root path</label>
            <input id="site-root" bind:value={form.root_path} class="input font-mono text-[13px]" placeholder="/home/user/htdocs/example.com (auto if empty)" />
          </div>
          <div class="flex items-end">
            <label class="flex h-9 cursor-pointer items-center gap-2 text-sm text-muted">
              <input type="checkbox" bind:checked={cfEnabled} class="checkbox" />
              Cloudflare DNS
            </label>
          </div>
        </div>

        {#if form.runtime === 'docker'}
          <div class="mt-5 border-t border-line pt-5">
            <p class="mb-3 flex items-center gap-2 text-sm font-semibold text-fg">
              <span class="badge badge-accent">1</span> Pilih bahasa / stack
            </p>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {#each dockerPresets as p (p.lang)}
                <button type="button" onclick={() => selectPreset(p)}
                  class="flex cursor-pointer flex-col items-start gap-0.5 rounded-lg border px-3 py-2.5 text-left transition-colors duration-150 {form.docker_language === p.lang ? 'border-accent bg-accent/10' : 'border-line bg-surface hover:border-line-strong hover:bg-surface-2'}">
                  <span class="text-sm font-medium {form.docker_language === p.lang ? 'text-accent' : 'text-fg'}">{p.label}</span>
                  <span class="text-xs leading-tight text-subtle">{p.desc}</span>
                </button>
              {/each}
            </div>
          </div>
          <div class="mt-5 grid grid-cols-1 gap-4 border-t border-line pt-5 sm:grid-cols-2">
            <p class="flex items-center gap-2 text-sm font-semibold text-fg sm:col-span-2">
              <span class="badge badge-accent">2</span> Konfigurasi container
            </p>
            <div class="sm:col-span-2">
              <label for="site-docker-image" class="label">Docker image</label>
              <div class="relative">
                <input
                  id="site-docker-image"
                  value={imageSearch}
                  oninput={(e) => onImageSearchInput(e.currentTarget.value)}
                  onfocus={() => { if (imageSearchResults.length > 0) imageSearchOpen = true; }}
                  onblur={() => setTimeout(() => { imageSearchOpen = false; }, 200)}
                  class="input font-mono text-[13px]"
                  placeholder="Search Docker Hub or type image:tag…"
                  autocomplete="off"
                />
                {#if imageSearchOpen && imageSearchResults.length > 0}
                  <div class="popover absolute top-full z-50 mt-1 max-h-64 w-full overflow-y-auto py-1">
                    {#each imageSearchResults as img (img.name)}
                      <button
                        onmousedown={() => selectImageResult(img)}
                        class="flex w-full cursor-pointer flex-col px-3 py-2 text-left transition-colors hover:bg-surface-2"
                      >
                        <span class="flex items-center gap-2">
                          <span class="truncate font-mono text-[13px] text-fg">{img.name}</span>
                          {#if img.official}<span class="badge badge-accent">Official</span>{/if}
                        </span>
                        {#if img.description}
                          <span class="mt-0.5 truncate text-xs text-muted">{img.description}</span>
                        {/if}
                        <span class="mt-0.5 flex gap-3 text-xs text-subtle tabular-nums">
                          <span class="inline-flex items-center gap-1"><Star size={11} /> {fmtPulls(img.stars)}</span>
                          <span class="inline-flex items-center gap-1"><Download size={11} /> {fmtPulls(img.pulls)}</span>
                        </span>
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
              <p class="hint">Pilih dari preset di atas, search, atau ketik manual.</p>
            </div>
            <div>
              <label for="site-docker-name" class="label">Container name</label>
              <input id="site-docker-name" bind:value={form.docker_name} class="input" placeholder="auto: site-name" />
            </div>
            <div>
              <label for="site-docker-ports" class="label">Port mapping</label>
              <input id="site-docker-ports" bind:value={form.docker_ports} class="input font-mono text-[13px]" placeholder="auto" />
            </div>
            <div>
              <label for="site-docker-volume" class="label">Volumes</label>
              <input id="site-docker-volume" bind:value={form.docker_volume} class="input font-mono text-[13px]" placeholder="/host/path:/container/path" />
            </div>
            <div>
              <label for="site-docker-restart" class="label">Restart policy</label>
              <select id="site-docker-restart" bind:value={form.docker_restart} class="input">
                <option value="unless-stopped">Unless stopped</option>
                <option value="always">Always</option>
                <option value="no">No</option>
              </select>
            </div>
            <div class="sm:col-span-2">
              <label for="site-docker-env" class="label">Environment (one per line: KEY=value)</label>
              <textarea id="site-docker-env" bind:value={form.docker_env} rows={2} class="input resize-none font-mono text-[13px]" placeholder="APP_ENV=production&#10;PORT=3000"></textarea>
            </div>
          </div>
        {/if}

        {#if form.runtime === 'proxy'}
          <div class="mt-5 border-t border-line pt-5">
            <label for="site-proxy" class="label">Proxy pass URL <span class="text-danger">*</span></label>
            <input id="site-proxy" bind:value={form.proxy_pass_url} class="input font-mono text-[13px]" placeholder="http://127.0.0.1:3000" />
          </div>
        {/if}

        <div class="mt-5 flex flex-wrap items-center gap-2">
          <button onclick={createSite} disabled={createDisabled} class="btn btn-primary btn-sm">
            {busy ? 'Creating…' : form.runtime === 'docker' && !showDockerConfig ? 'Pilih bahasa dulu' : 'Create site'}
          </button>
          <button onclick={() => (showCreate = false)} class="btn btn-ghost btn-sm">Cancel</button>
        </div>
        {#if cfResult}
          <p class="mt-3 text-xs {cfResult.provisioned ? 'text-success' : 'text-warning'}">
            {cfResult.provisioned ? `DNS provisioned in zone ${cfResult.zone}` : cfResult.error}
          </p>
        {/if}
      </div>
    </div>
  {/if}

  <div class="card">
    <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3">
      <div class="relative w-full sm:w-64">
        <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
        <input type="search" aria-label="Search domains" placeholder="Search domains…" value={searchInput} oninput={onSearch} class="input h-8 pl-8" />
      </div>
      <select aria-label="Runtime filter" bind:value={runtimeFilter} onchange={() => (page = 1)} class="input h-8 w-auto text-[13px]">
        <option value="">All runtimes</option>
        {#each ['static', 'php', 'node', 'python', 'go', 'docker', 'proxy'] as r (r)}<option value={r}>{r}</option>{/each}
      </select>
      <select aria-label="Status filter" bind:value={statusFilter} onchange={() => (page = 1)} class="input h-8 w-auto text-[13px]">
        <option value="">All statuses</option>
        {#each ['active', 'pending', 'deploying', 'disabled'] as s (s)}<option value={s}>{s}</option>{/each}
      </select>
      <label class="ml-auto flex items-center gap-2 text-xs text-subtle">
        Per page
        <select bind:value={pageSize} onchange={() => (page = 1)} class="input h-8 w-auto text-[13px]">
          {#each PAGE_SIZES as ps (ps)}<option value={ps}>{ps}</option>{/each}
        </select>
      </label>
    </div>

    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(5) as _, i (i)}<div class="skeleton h-9"></div>{/each}
      </div>
    {:else if sites.length === 0}
      <EmptyState icon={Globe} title="No sites" description="Create your first site to get started." actionLabel="Create site" onaction={() => (showCreate = true)} />
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              {#each columns as col (col.label)}
                <th class={col.cls ?? ''}>
                  {#if col.key}
                    <button class="inline-flex cursor-pointer items-center gap-1 hover:text-fg" onclick={() => toggleSort(col.key!)}>
                      {col.label}
                      {#if sortKey === col.key}
                        {#if sortDir === 'asc'}<ArrowUp size={12} />{:else}<ArrowDown size={12} />{/if}
                      {/if}
                    </button>
                  {:else}
                    {col.label}
                  {/if}
                </th>
              {/each}
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each sortedSites as s (s.id)}
              <tr>
                <td class="font-medium">
                  <a href="/sites/{s.id}" class="text-fg hover:text-accent">{s.domain}</a>
                </td>
                <td><span class="badge">{s.runtime}</span></td>
                <td class="max-2xl:hidden"><span class="badge badge-accent">{s.managed_by || 'sigardapanel'}</span></td>
                <td class="font-mono text-[13px] text-muted max-2xl:hidden">{s.system_user || '—'}</td>
                <td class="max-w-[18rem] truncate font-mono text-[13px] text-muted max-md:hidden" title={s.proxy_pass_url || s.root_path || s.conf_file || ''}>{s.proxy_pass_url || s.root_path || '—'}</td>
                <td>
                  <div class="flex flex-col items-start gap-1">
                    <StatusPill status={s.status} />
                    {#if s.status === 'pending'}
                      <span class="inline-flex items-center gap-1 text-xs leading-tight text-warning">
                        <TriangleAlert size={11} />
                        {!s.conf_file ? 'nginx config belum terdaftar' : 'menunggu provisioning'}
                      </span>
                    {/if}
                  </div>
                </td>
                <td class="max-sm:hidden">
                  <div class="flex flex-wrap items-center gap-1.5">
                    {#if s.ssl_status && s.ssl_status !== 'none'}
                      <span class="badge badge-success"><span class="dot"></span>{s.ssl_status}</span>
                    {:else}
                      <span class="text-subtle">—</span>
                    {/if}
                    {#if s.app_status}
                      <span class="inline-flex items-center gap-1 text-xs {s.app_status === 'running' ? 'text-success' : 'text-subtle'}">
                        <span class="dot"></span>{s.app_status}
                      </span>
                    {/if}
                  </div>
                </td>
                <td class="whitespace-nowrap text-muted tabular-nums max-md:hidden">
                  {new Date(s.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })}
                </td>
                <td class="text-right">
                  <a href="/sites/{s.id}" class="btn btn-ghost btn-xs">Manage <ChevronRight size={12} /></a>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  {#if !loading && sites.length > 0}
    <Pagination totalItems={totalCount} {pageSize} currentPage={page} onPageChange={(p) => (page = p)} />
  {/if}
</DashboardLayout>
