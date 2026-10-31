<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { goto } from '$app/navigation';
  import { Search, Server, Globe, Database, CornerDownLeft, LoaderCircle } from 'lucide-svelte';
  import { tick } from 'svelte';

  // ⌘K / Ctrl+K toggling is owned by DashboardLayout; this component handles
  // everything once open (Escape, arrows, Enter).
  let { open, onClose }: { open: boolean; onClose: () => void } = $props();

  interface SearchResult {
    type: 'server' | 'site' | 'database';
    id: number;
    title: string;
    subtitle: string;
    href: string;
  }

  let query = $state('');
  let selectedIndex = $state(0);
  let inputEl = $state<HTMLInputElement>();
  let listEl = $state<HTMLDivElement>();

  // The three lists are fetched once per opening (in parallel) and filtered locally.
  let index = $state<SearchResult[] | null>(null);
  let indexing = $state(false);
  let indexPromise: Promise<void> | null = null;

  $effect(() => {
    if (open) {
      query = '';
      selectedIndex = 0;
      index = null;
      indexPromise = null;
      tick().then(() => inputEl?.focus());
    }
  });

  function buildIndex(): Promise<void> {
    if (indexPromise) return indexPromise;
    if (!auth.user) return Promise.resolve();
    indexing = true;
    indexPromise = (async () => {
      const [servers, sites, dbs] = await Promise.allSettled([
        apiFetch(auth.token, 'GET', 'servers'),
        apiFetch(auth.token, 'GET', 'sites'),
        apiFetch(auth.token, 'GET', 'databases'),
      ]);
      const all: SearchResult[] = [];
      if (servers.status === 'fulfilled') {
        for (const s of servers.value?.servers ?? []) {
          all.push({ type: 'server', id: s.id, title: s.name, subtitle: [s.hostname, s.status].filter(Boolean).join(' · '), href: '/servers' });
        }
      }
      if (sites.status === 'fulfilled') {
        for (const s of sites.value?.sites ?? []) {
          all.push({ type: 'site', id: s.id, title: s.domain || s.name, subtitle: [s.runtime, s.status].filter(Boolean).join(' · '), href: `/sites/${s.id}` });
        }
      }
      if (dbs.status === 'fulfilled') {
        for (const d of dbs.value?.databases ?? []) {
          all.push({ type: 'database', id: d.id, title: d.name, subtitle: [d.type, d.charset].filter(Boolean).join(' · '), href: '/databases' });
        }
      }
      index = all;
      indexing = false;
    })();
    return indexPromise;
  }

  function haystack(r: SearchResult): string {
    return `${r.title} ${r.subtitle}`.toLowerCase();
  }

  const q = $derived(query.trim().toLowerCase());
  const results = $derived(q.length < 2 || !index ? [] : index.filter((r) => haystack(r).includes(q)).slice(0, 10));
  const loading = $derived(q.length >= 2 && indexing);

  function onInput() {
    selectedIndex = 0;
    if (query.trim().length >= 2) buildIndex();
  }

  function close() {
    onClose();
  }

  async function scrollSelected() {
    await tick();
    listEl?.querySelector<HTMLElement>(`[data-index="${selectedIndex}"]`)?.scrollIntoView({ block: 'nearest' });
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (results.length) selectedIndex = (selectedIndex + 1) % results.length;
      scrollSelected();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (results.length) selectedIndex = (selectedIndex - 1 + results.length) % results.length;
      scrollSelected();
    } else if (e.key === 'Enter' && results[selectedIndex]) {
      e.preventDefault();
      const href = results[selectedIndex].href;
      close();
      goto(href);
    }
  }

  const typeMeta = {
    server: { icon: Server, label: 'Server' },
    site: { icon: Globe, label: 'Site' },
    database: { icon: Database, label: 'Database' },
  } as const;
</script>

<svelte:window onkeydown={(e) => { if (open && e.key === 'Escape') close(); }} />

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="overlay animate-fade-in flex items-start justify-center p-4 pt-[12vh]" onclick={close}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      class="popover animate-pop-in flex w-full max-w-lg flex-col overflow-hidden"
      role="dialog"
      aria-modal="true"
      aria-label="Search"
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={onKeydown}
    >
      <div class="flex items-center gap-3 border-b border-line px-4">
        {#if loading}
          <LoaderCircle size={16} class="shrink-0 animate-spin text-subtle" />
        {:else}
          <Search size={16} class="shrink-0 text-subtle" />
        {/if}
        <input
          bind:this={inputEl}
          bind:value={query}
          oninput={onInput}
          placeholder="Search servers, sites, databases…"
          aria-label="Search"
          role="combobox"
          aria-expanded={results.length > 0}
          aria-controls="global-search-results"
          aria-activedescendant={results[selectedIndex] ? `gs-${selectedIndex}` : undefined}
          autocomplete="off"
          spellcheck="false"
          class="h-12 flex-1 border-0 bg-transparent text-sm text-fg outline-none placeholder:text-subtle"
        />
        <kbd class="kbd max-sm:hidden">esc</kbd>
      </div>

      <div bind:this={listEl} id="global-search-results" role="listbox" class="max-h-80 overflow-y-auto">
        {#if q.length < 2}
          <p class="px-4 py-8 text-center text-[13px] text-muted">Type at least 2 characters to search servers, sites and databases.</p>
        {:else if loading}
          <div class="space-y-1 p-2">
            {#each Array(3) as _, i (i)}<div class="skeleton h-11"></div>{/each}
          </div>
        {:else if results.length === 0}
          <p class="px-4 py-8 text-center text-[13px] text-muted">No results for “{query.trim()}”</p>
        {:else}
          <div class="p-1.5">
            {#each results as r, i (`${r.type}:${r.id}`)}
              {@const meta = typeMeta[r.type]}
              <a
                href={r.href}
                id="gs-{i}"
                data-index={i}
                role="option"
                aria-selected={i === selectedIndex}
                onclick={close}
                onmousemove={() => (selectedIndex = i)}
                class="flex items-center gap-3 rounded-lg px-3 py-2.5 transition-colors {i === selectedIndex ? 'bg-surface-2' : ''}"
              >
                <div class="flex size-8 shrink-0 items-center justify-center rounded-md border border-line bg-surface text-subtle">
                  <meta.icon size={15} strokeWidth={1.75} />
                </div>
                <div class="min-w-0 flex-1">
                  <div class="truncate text-sm font-medium text-fg">{r.title}</div>
                  <div class="truncate text-xs text-subtle">{meta.label}{r.subtitle ? ` · ${r.subtitle}` : ''}</div>
                </div>
                {#if i === selectedIndex}<CornerDownLeft size={14} class="shrink-0 text-subtle" />{/if}
              </a>
            {/each}
          </div>
        {/if}
      </div>

      <div class="flex items-center gap-4 border-t border-line px-4 py-2 text-xs text-subtle max-sm:hidden">
        <span class="flex items-center gap-1"><kbd class="kbd">↑</kbd><kbd class="kbd">↓</kbd> navigate</span>
        <span class="flex items-center gap-1"><kbd class="kbd">↵</kbd> open</span>
        <span class="flex items-center gap-1"><kbd class="kbd">esc</kbd> close</span>
      </div>
    </div>
  </div>
{/if}
