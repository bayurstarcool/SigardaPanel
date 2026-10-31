<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import { Download, Layers, LoaderCircle, Search, CheckCircle2, XCircle, Circle } from 'lucide-svelte';
  import { onDestroy } from 'svelte';

  let { serverId, onpulled }: { serverId: number; onpulled?: () => void } = $props();

  type Layer = { id: string; status: string };

  let image = $state('');
  let results = $state<any[]>([]);
  let resultsOpen = $state(false);
  let searching = $state(false);
  let pulling = $state(false);
  let progressOpen = $state(false);
  let progress = $state<{ image: string; layers: Layer[]; done: boolean; error: string }>({ image: '', layers: [], done: false, error: '' });
  let timer: ReturnType<typeof setTimeout> | null = null;

  onDestroy(() => { if (timer) clearTimeout(timer); });

  function fmtCount(n: number): string {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n ?? 0);
  }

  function onInput(value: string) {
    image = value;
    if (timer) clearTimeout(timer);
    if (!value || value.length < 2) { resultsOpen = false; results = []; searching = false; return; }
    searching = true;
    timer = setTimeout(async () => {
      try {
        const d: any = await apiFetch(auth.token, 'GET', `servers/${serverId}/docker/images/search?q=${encodeURIComponent(value)}&page_size=8`);
        results = d?.results ?? [];
        resultsOpen = results.length > 0;
      } catch {
        results = [];
        resultsOpen = false;
      }
      searching = false;
    }, 300);
  }

  function select(name: string) {
    image = name;
    resultsOpen = false;
    results = [];
  }

  async function pull() {
    if (!image) return;
    const target = image;
    pulling = true;
    resultsOpen = false;
    progress = { image: target, layers: [], done: false, error: '' };
    progressOpen = true;
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${serverId}/docker/image/pull`, { image: target });
      const output: string = d?.output || d?.message || '';
      const lines = output.split('\n').filter((l) => l.trim());
      const layers: Layer[] = [];
      for (const line of lines) {
        const m = line.match(/^([a-f0-9]{12}): (.+)$/);
        if (m) {
          const existing = layers.find((l) => l.id === m[1]);
          if (existing) existing.status = m[2];
          else layers.push({ id: m[1], status: m[2] });
        }
      }
      if (layers.length === 0 && lines.length > 0) layers.push({ id: 'output', status: lines[lines.length - 1] });
      progress = { image: target, layers, done: true, error: d?.ok === false ? d?.message || 'pull failed' : '' };
      image = '';
      onpulled?.();
    } catch (e: any) {
      progress = { image: target, layers: [], done: true, error: e?.message || 'pull failed' };
    }
    pulling = false;
  }

  function layerState(status: string): 'done' | 'active' | 'idle' {
    if (status.includes('complete') || status.includes('already') || status.startsWith('Status:')) return 'done';
    if (status.includes('Downloading') || status.includes('Pulling')) return 'active';
    return 'idle';
  }
</script>

<div class="flex gap-2">
  <div class="relative flex-1">
    <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
    <input
      value={image}
      oninput={(e) => onInput(e.currentTarget.value)}
      onfocus={() => { if (results.length > 0) resultsOpen = true; }}
      onblur={() => setTimeout(() => (resultsOpen = false), 200)}
      onkeydown={(e) => { if (e.key === 'Enter') pull(); }}
      placeholder="Search Docker Hub, e.g. nginx:alpine"
      aria-label="Image to pull"
      autocomplete="off"
      class="input h-8 pr-8 pl-8 font-mono text-[13px]"
    />
    {#if searching}
      <LoaderCircle size={14} class="absolute top-1/2 right-3 -translate-y-1/2 animate-spin text-subtle" />
    {/if}
    {#if resultsOpen && results.length > 0}
      <div class="popover absolute top-full z-20 mt-1 max-h-80 w-full overflow-y-auto py-1">
        {#each results as img (img.name)}
          <button type="button" onmousedown={() => select(img.name)} class="flex w-full items-start gap-2.5 px-3 py-2 text-left transition-colors hover:bg-surface-2">
            <Layers size={14} class="mt-0.5 shrink-0 text-subtle" />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5">
                <span class="truncate font-mono text-[13px] text-fg">{img.name}</span>
                {#if img.official}<span class="badge badge-accent">official</span>{/if}
                {#if img.automated}<span class="badge">auto</span>{/if}
              </div>
              {#if img.description}<p class="mt-0.5 truncate text-xs text-subtle">{img.description}</p>{/if}
              <p class="mt-0.5 text-xs text-subtle tabular-nums">{fmtCount(img.stars)} stars · {fmtCount(img.pulls)} pulls</p>
            </div>
          </button>
        {/each}
      </div>
    {/if}
  </div>
  <button onclick={pull} disabled={pulling || !image} class="btn btn-primary btn-sm">
    {#if pulling}<LoaderCircle size={14} class="animate-spin" />{:else}<Download size={14} />{/if} Pull
  </button>
</div>

<Modal bind:open={progressOpen} title={!progress.done ? `Pulling ${progress.image}` : progress.error ? 'Pull failed' : `${progress.image} pulled`} maxWidth="lg">
  {#if progress.error}
    <p class="text-sm text-danger">{progress.error}</p>
  {:else if !progress.done}
    <div class="flex items-center gap-3 py-2 text-[13px] text-muted">
      <LoaderCircle size={16} class="animate-spin text-accent" /> Downloading layers — this can take a minute…
    </div>
  {:else}
    <ul class="space-y-1.5">
      {#each progress.layers as layer (layer.id)}
        {@const st = layerState(layer.status)}
        <li class="flex items-start gap-2.5 text-xs">
          {#if st === 'done'}<CheckCircle2 size={14} class="mt-px shrink-0 text-success" />
          {:else if st === 'active'}<LoaderCircle size={14} class="mt-px shrink-0 animate-spin text-accent" />
          {:else}<Circle size={14} class="mt-px shrink-0 text-subtle" />{/if}
          <span class="min-w-0">
            {#if layer.id !== 'output'}<span class="font-mono text-subtle">{layer.id.slice(0, 12)}</span>{/if}
            <span class="text-muted">{layer.status}</span>
          </span>
        </li>
      {/each}
    </ul>
    <p class="mt-3 flex items-center gap-1.5 border-t border-line pt-3 text-xs text-success"><CheckCircle2 size={13} /> Image ready</p>
  {/if}
  {#if progress.error}
    <p class="mt-2 flex items-center gap-1.5 text-xs text-subtle"><XCircle size={13} /> Check the image name and that the server can reach the registry.</p>
  {/if}
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (progressOpen = false)} disabled={!progress.done}>Close</button>
  {/snippet}
</Modal>
