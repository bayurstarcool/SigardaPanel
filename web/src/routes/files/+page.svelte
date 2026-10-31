<script lang="ts">
  // Global file manager: pick a site, then browse it with the per-site file
  // manager (`/api/v1/sites/:id/files…`). The selected site lives in `?site=<id>`
  // so the view can be linked and survives reloads/back navigation.
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Folder, Globe, Search, RefreshCw, ArrowLeftRight, ExternalLink, ChevronRight, TriangleAlert } from 'lucide-svelte';
  import { onMount, tick } from 'svelte';

  let sites = $state<any[]>([]);
  let loading = $state(true);
  let query = $state('');
  let pickerOpen = $state(false);
  let pickerInput = $state<HTMLInputElement | null>(null);

  const siteId = $derived(Number($page.url.searchParams.get('site')) || 0);
  const selected = $derived(siteId ? sites.find((s) => s.id === siteId) ?? null : null);
  const matches = $derived(
    query ? sites.filter((s) => s.domain?.toLowerCase().includes(query.toLowerCase()) || s.name?.toLowerCase().includes(query.toLowerCase())) : sites,
  );

  onMount(() => {
    if (!auth.user) { window.location.href = '/login'; return; }
    loadSites();
  });

  async function loadSites() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'sites?limit=500');
      sites = (d?.sites || []).slice().sort((a: any, b: any) => String(a.domain).localeCompare(String(b.domain)));
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  function selectSite(id: number) {
    pickerOpen = false;
    query = '';
    if (id === siteId) return;
    goto(`?site=${id}`, { keepFocus: true, noScroll: true });
  }

  function clearSite() {
    pickerOpen = false;
    goto($page.url.pathname, { keepFocus: true, noScroll: true });
  }

  async function openPicker() {
    pickerOpen = !pickerOpen;
    query = '';
    if (pickerOpen) { await tick(); pickerInput?.focus(); }
  }

  function onPickerKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') pickerOpen = false;
    if (e.key === 'Enter' && matches.length > 0) selectSite(matches[0].id);
  }
</script>

{#snippet siteList(list: any[])}
  {#if list.length === 0}
    <EmptyState compact icon={Globe} title={sites.length === 0 ? 'No sites yet' : 'No sites match'} description={sites.length === 0 ? 'Create a site first, then manage its files here.' : 'Try another search.'} />
  {:else}
    <ul class="max-h-[60vh] divide-y divide-line overflow-y-auto" role="listbox" aria-label="Sites">
      {#each list as s (s.id)}
        <li role="option" aria-selected={s.id === siteId}>
          <button onclick={() => selectSite(s.id)} class="flex w-full cursor-pointer items-center gap-3 px-4 py-2.5 text-left transition-colors duration-100 hover:bg-surface-2/60 {s.id === siteId ? 'bg-accent/10' : ''}">
            <Globe size={15} class="shrink-0 {s.id === siteId ? 'text-accent' : 'text-subtle'}" />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium text-fg">{s.domain}</span>
              {#if s.root_path}<span class="block truncate font-mono text-xs text-subtle">{s.root_path}</span>{/if}
            </span>
            <span class="badge max-sm:hidden">{s.runtime}</span>
            <ChevronRight size={14} class="shrink-0 text-subtle" />
          </button>
        </li>
      {/each}
    </ul>
  {/if}
{/snippet}

<DashboardLayout active="Files">
  <PageHeader title="Files" description="Browse, edit and upload files for any of your sites.">
    {#snippet actions()}
      <button onclick={loadSites} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh sites
      </button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="card space-y-3 p-4">
      <div class="skeleton h-8 w-full sm:w-72"></div>
      {#each Array(6) as _, i (i)}<div class="skeleton h-10"></div>{/each}
    </div>
  {:else if !siteId}
    <!-- Step 1: pick a site -->
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 class="card-title">Choose a site</h2>
        <div class="relative w-full sm:w-72">
          <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
          <input bind:value={query} aria-label="Search sites" class="input h-8 pl-8" placeholder="Search sites…" onkeydown={(e) => { if (e.key === 'Enter' && matches.length > 0) selectSite(matches[0].id); }} />
        </div>
      </div>
      {@render siteList(matches)}
    </div>
  {:else if !selected}
    <div class="card">
      <EmptyState icon={TriangleAlert} title="Site not available" description="This site does not exist or you don't have access to it." actionLabel="Choose another site" onaction={clearSite} />
    </div>
  {:else}
    <!-- Step 2: selected site + file manager -->
    <div class="relative mb-4">
      <div class="card flex flex-wrap items-center gap-3 px-4 py-3">
        <Globe size={16} class="shrink-0 text-subtle" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate text-sm font-semibold text-fg">{selected.domain}</span>
            <span class="badge">{selected.runtime}</span>
          </div>
          {#if selected.root_path}<div class="truncate font-mono text-xs text-subtle">{selected.root_path}</div>{/if}
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <a href="/sites/{selected.id}" class="btn btn-ghost btn-sm"><ExternalLink size={14} /> <span class="max-sm:hidden">Site settings</span></a>
          <button onclick={openPicker} class="btn btn-secondary btn-sm" aria-expanded={pickerOpen} aria-haspopup="listbox"><ArrowLeftRight size={14} /> Change site</button>
        </div>
      </div>

      {#if pickerOpen}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="fixed inset-0 z-40" onclick={() => (pickerOpen = false)}></div>
        <div class="popover animate-pop-in absolute top-full right-0 z-50 mt-1 w-full overflow-hidden sm:w-96">
          <div class="border-b border-line p-2">
            <div class="relative">
              <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
              <input bind:this={pickerInput} bind:value={query} onkeydown={onPickerKeydown} aria-label="Search sites" class="input h-8 pl-8" placeholder="Search sites…" />
            </div>
          </div>
          {@render siteList(matches)}
        </div>
      {/if}
    </div>

    {#key selected.id}
      {#await import('$components/sites/FileManagerTab.svelte')}
        <div class="card space-y-3 p-4">
          <div class="skeleton h-8"></div>
          {#each Array(6) as _, i (i)}<div class="skeleton h-9"></div>{/each}
        </div>
      {:then m}
        <m.default siteId={selected.id} />
      {:catch}
        <div class="card"><EmptyState icon={Folder} title="The file manager failed to load" description="Check your connection and reload the page." /></div>
      {/await}
    {/key}
  {/if}
</DashboardLayout>
