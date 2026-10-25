<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { Search, Server, Globe, Database, ArrowRight, X, Loader2 } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';

  let { open, onClose }: { open: boolean; onClose: () => void } = $props();
  let query = $state('');
  let results = $state<any[]>([]);
  let loading = $state(false);
  let selectedIndex = $state(0);
  let inputEl = $state<HTMLInputElement>();

  let debounceTimer: ReturnType<typeof setTimeout>;

  interface SearchResult {
    type: 'server' | 'site' | 'database';
    id: number;
    title: string;
    subtitle: string;
    href: string;
  }

  onMount(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        if (open) {
          onClose();
        } else {
          // Dispatch event to open
          window.dispatchEvent(new CustomEvent('open-search'));
        }
      }
      if (e.key === 'Escape' && open) {
        onClose();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  });

  $effect(() => {
    if (open && inputEl) {
      setTimeout(() => inputEl?.focus(), 100);
      query = '';
      results = [];
      selectedIndex = 0;
    }
  });

  function onInput() {
    clearTimeout(debounceTimer);
    if (query.length < 2) {
      results = [];
      return;
    }
    loading = true;
    debounceTimer = setTimeout(() => search(query), 200);
  }

  async function search(q: string) {
    if (!auth.user) return;
    const all: SearchResult[] = [];

    try {
      // Search servers
      const servers: any = await apiFetch(auth.token, 'GET', 'servers');
      for (const s of servers?.servers ?? []) {
        if (match(`${s.name} ${s.hostname} ${s.status}`, q)) {
          all.push({ type: 'server', id: s.id, title: s.name, subtitle: `${s.hostname} • ${s.status}`, href: '/servers' });
        }
      }

      // Search sites
      const sites: any = await apiFetch(auth.token, 'GET', 'sites');
      for (const s of sites?.sites ?? []) {
        if (match(`${s.name} ${s.domain} ${s.runtime} ${s.status}`, q)) {
          all.push({ type: 'site', id: s.id, title: s.domain || s.name, subtitle: `${s.runtime} • ${s.status}`, href: `/sites/${s.id}` });
        }
      }

      // Search databases
      const dbs: any = await apiFetch(auth.token, 'GET', 'databases');
      for (const d of dbs?.databases ?? []) {
        if (match(`${d.name} ${d.type}`, q)) {
          all.push({ type: 'database', id: d.id, title: d.name, subtitle: `${d.type} • ${d.charset || ''}`, href: '/databases' });
        }
      }
    } catch {}

    results = all.slice(0, 10);
    loading = false;
    selectedIndex = 0;
  }

  function match(text: string, q: string): boolean {
    return text.toLowerCase().includes(q.toLowerCase());
  }

  function navigate(href: string) {
    window.location.href = href;
    onClose();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, results.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, 0);
    } else if (e.key === 'Enter' && results[selectedIndex]) {
      navigate(results[selectedIndex].href);
    }
  }

  function typeIcon(type: string) {
    if (type === 'server') return Server;
    if (type === 'site') return Globe;
    return Database;
  }

  function typeColor(type: string) {
    if (type === 'server') return 'text-sky-400';
    if (type === 'site') return 'text-emerald-400';
    return 'text-purple-400';
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-[100] bg-black/60 backdrop-blur-sm flex items-start justify-center pt-[15vh] p-4"
    onclick={onClose} onkeydown={(e) => e.key === 'Escape' && onClose()}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="bg-[#0c0d0f] border border-white/[0.08] rounded-2xl w-full max-w-lg overflow-hidden shadow-2xl shadow-black/50"
      onclick={(e) => e.stopPropagation()} onkeydown={onKeydown}>

      <!-- Search Input -->
      <div class="flex items-center gap-3 px-5 py-4 border-b border-white/[0.06]">
        {#if loading}
          <Loader2 size={18} class="text-sky-400 animate-spin" />
        {:else}
          <Search size={18} class="text-slate-500" />
        {/if}
        <input bind:this={inputEl} bind:value={query} oninput={onInput}
          placeholder="Search servers, sites, databases..."
          class="flex-1 bg-transparent border-0 outline-none text-sm text-white placeholder-slate-500" />
        <kbd class="hidden sm:inline-flex items-center px-1.5 py-0.5 text-[0.6rem] font-mono text-slate-500 bg-white/[0.05] border border-white/[0.08] rounded">ESC</kbd>
      </div>

      <!-- Results -->
      <div class="max-h-80 overflow-auto">
        {#if query.length < 2}
          <div class="px-5 py-8 text-center">
            <p class="text-xs text-slate-500">Type at least 2 characters to search</p>
            <p class="text-[0.65rem] text-slate-600 mt-2">Search across servers, sites, and databases</p>
          </div>
        {:else if results.length === 0 && !loading}
          <div class="px-5 py-8 text-center">
            <p class="text-xs text-slate-500">No results for "{query}"</p>
          </div>
        {:else}
          <div class="py-1">
            {#each results as r, i}
              {@const Icon = typeIcon(r.type)}
              <button onclick={() => navigate(r.href)}
                class="w-full flex items-center gap-3 px-5 py-3 text-left transition-colors {i === selectedIndex ? 'bg-white/[0.06]' : 'hover:bg-white/[0.03]'}">
                <div class="w-8 h-8 rounded-lg bg-white/[0.05] flex items-center justify-center">
                  <Icon size={14} class={typeColor(r.type)} />
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-white truncate">{r.title}</div>
                  <div class="text-[0.7rem] text-slate-500 truncate">{r.subtitle}</div>
                </div>
                <ArrowRight size={14} class="text-slate-600" />
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-2.5 border-t border-white/[0.06] flex items-center gap-4 text-[0.6rem] text-slate-600">
        <span><kbd class="px-1 py-0.5 bg-white/[0.05] border border-white/[0.08] rounded font-mono">↑↓</kbd> navigate</span>
        <span><kbd class="px-1 py-0.5 bg-white/[0.05] border border-white/[0.08] rounded font-mono">↵</kbd> select</span>
        <span><kbd class="px-1 py-0.5 bg-white/[0.05] border border-white/[0.08] rounded font-mono">esc</kbd> close</span>
      </div>
    </div>
  </div>
{/if}
