<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import Modal from '$components/Modal.svelte';
  import { Plus, LoaderCircle, HardDrive, Cloud } from 'lucide-svelte';
  import { onDestroy } from 'svelte';

  let {
    open = $bindable(false),
    serverId,
    images = [],
    oncreated,
  }: { open: boolean; serverId: number; images?: any[]; oncreated?: () => void } = $props();

  const emptyForm = () => ({ image: '', name: '', ports: '', volumes: '', env: '', restart: 'no', network: 'bridge', auto_start: true, command: '' });

  let form = $state(emptyForm());
  let pending = $state(false);
  let results = $state<any[]>([]);
  let resultsOpen = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  onDestroy(() => { if (timer) clearTimeout(timer); });

  function fmtCount(n: number): string {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
  }

  function onImageInput(value: string) {
    form.image = value;
    if (timer) clearTimeout(timer);
    if (!value) { resultsOpen = false; results = []; return; }
    // Matching local images show immediately; Docker Hub results follow after a debounce.
    const localMatches = images
      .map((img: any) => ({ name: img.tag === 'latest' || !img.tag ? img.repository : `${img.repository}:${img.tag}`, size: img.size }))
      .filter((img) => img.name.toLowerCase().includes(value.toLowerCase()))
      .map((img) => ({ ...img, source: 'local' as const }));
    results = localMatches;
    resultsOpen = true;
    timer = setTimeout(async () => {
      try {
        const d: any = await apiFetch(auth.token, 'GET', `servers/${serverId}/docker/images/search?q=${encodeURIComponent(value)}&page_size=5`);
        const hub = (d?.results ?? []).map((r: any) => ({
          name: r.name, source: 'hub' as const, description: r.description, stars: r.stars, pulls: r.pulls, official: r.official,
        }));
        const seen = new Set(localMatches.map((m) => m.name));
        results = [...localMatches, ...hub.filter((h: any) => !seen.has(h.name))];
      } catch {}
    }, 300);
  }

  function selectImage(name: string) {
    form.image = name;
    resultsOpen = false;
    results = [];
  }

  async function create() {
    if (!form.image) return;
    pending = true;
    try {
      const ports = form.ports.split(',').map((s) => s.trim()).filter(Boolean);
      const volumes = form.volumes.split(',').map((s) => s.trim()).filter(Boolean);
      const env: Record<string, string> = {};
      for (const pair of form.env.split('\n').filter(Boolean)) {
        const [k, ...rest] = pair.split('=');
        if (k) env[k.trim()] = rest.join('=').trim();
      }
      await apiFetch(auth.token, 'POST', `servers/${serverId}/docker/container/create`, {
        image: form.image,
        name: form.name || undefined,
        command: form.command || undefined,
        ports: ports.length ? ports : undefined,
        volumes: volumes.length ? volumes : undefined,
        env: Object.keys(env).length ? env : undefined,
        restart: form.restart,
        network: form.network,
        auto_start: form.auto_start,
      });
      toasts.success(`Container created from ${form.image}`);
      open = false;
      form = emptyForm();
      oncreated?.();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to create container');
    } finally {
      pending = false;
    }
  }
</script>

<Modal bind:open title="Create container" maxWidth="lg">
  <div class="space-y-4">
    <div class="relative">
      <label class="label" for="cc-image">Image <span class="text-danger">*</span></label>
      <input
        id="cc-image"
        value={form.image}
        oninput={(e) => onImageInput(e.currentTarget.value)}
        onfocus={() => { if (results.length > 0) resultsOpen = true; }}
        onblur={() => setTimeout(() => (resultsOpen = false), 200)}
        placeholder="Search local images or Docker Hub…"
        autocomplete="off"
        class="input font-mono text-[13px]"
      />
      {#if resultsOpen && results.length > 0}
        <div class="popover absolute top-full z-10 mt-1 max-h-60 w-full overflow-y-auto py-1">
          {#each results as img (img.source + img.name)}
            <button
              type="button"
              onmousedown={() => selectImage(img.name)}
              class="flex w-full items-start gap-2.5 px-3 py-2 text-left transition-colors hover:bg-surface-2"
            >
              {#if img.source === 'local'}
                <HardDrive size={14} class="mt-0.5 shrink-0 text-success" />
              {:else}
                <Cloud size={14} class="mt-0.5 shrink-0 text-subtle" />
              {/if}
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="truncate font-mono text-[13px] text-fg">{img.name}</span>
                  {#if img.source === 'local'}<span class="badge badge-success">local</span>{/if}
                  {#if img.official}<span class="badge badge-accent">official</span>{/if}
                </div>
                {#if img.description}<p class="truncate text-xs text-subtle">{img.description}</p>{/if}
                {#if img.source === 'local' && img.size}
                  <p class="text-xs text-subtle">{img.size}</p>
                {:else if img.stars || img.pulls}
                  <p class="text-xs text-subtle tabular-nums">{fmtCount(img.stars || 0)} stars · {fmtCount(img.pulls || 0)} pulls</p>
                {/if}
              </div>
            </button>
          {/each}
        </div>
      {/if}
    </div>

    <div>
      <label class="label" for="cc-name">Container name</label>
      <input id="cc-name" bind:value={form.name} placeholder="my-nginx (auto-generated if empty)" class="input" />
    </div>
    <div>
      <label class="label" for="cc-cmd">Command</label>
      <input id="cc-cmd" bind:value={form.command} placeholder="sh -c 'echo hello'" class="input font-mono text-[13px]" />
      <p class="hint">Optional — overrides the image's default command.</p>
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="label" for="cc-ports">Ports</label>
        <input id="cc-ports" bind:value={form.ports} placeholder="8080:80, 443:443" class="input font-mono text-[13px]" />
        <p class="hint">Comma separated host:container</p>
      </div>
      <div>
        <label class="label" for="cc-vols">Volumes</label>
        <input id="cc-vols" bind:value={form.volumes} placeholder="/data:/var/www/html" class="input font-mono text-[13px]" />
        <p class="hint">Comma separated host:container</p>
      </div>
    </div>
    <div>
      <label class="label" for="cc-env">Environment</label>
      <textarea id="cc-env" bind:value={form.env} rows={3} placeholder={'NODE_ENV=production\nPORT=3000'} class="input resize-none font-mono text-[13px]"></textarea>
      <p class="hint">One KEY=value per line</p>
    </div>
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label class="label" for="cc-restart">Restart policy</label>
        <select id="cc-restart" bind:value={form.restart} class="input">
          <option value="no">No</option>
          <option value="always">Always</option>
          <option value="unless-stopped">Unless stopped</option>
          <option value="on-failure">On failure</option>
        </select>
      </div>
      <div>
        <label class="label" for="cc-net">Network</label>
        <select id="cc-net" bind:value={form.network} class="input">
          <option value="bridge">Bridge</option>
          <option value="host">Host</option>
          <option value="none">None</option>
        </select>
      </div>
    </div>
    <label class="flex cursor-pointer items-center gap-2 text-sm text-muted">
      <input type="checkbox" bind:checked={form.auto_start} class="checkbox" />
      Start immediately after creation
    </label>
  </div>

  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (open = false)}>Cancel</button>
    <button class="btn btn-primary btn-sm" onclick={create} disabled={pending || !form.image}>
      {#if pending}<LoaderCircle size={14} class="animate-spin" /> Creating…{:else}<Plus size={14} /> Create{/if}
    </button>
  {/snippet}
</Modal>
