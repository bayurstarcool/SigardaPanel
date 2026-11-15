<script lang="ts">
  // Header control choosing the server the panel works on ("All servers" or
  // one). Hidden when there is only one server.
  import { serverCtx } from '$lib/serverContext.svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { Check, ChevronsUpDown, Layers } from 'lucide-svelte';

  let open = $state(false);
  let el = $state<HTMLDivElement>();

  onMount(() => {
    void serverCtx.ensure();
    const onDown = (e: MouseEvent) => { if (el && !el.contains(e.target as Node)) open = false; };
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') open = false; };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => { document.removeEventListener('mousedown', onDown); document.removeEventListener('keydown', onKey); };
  });

  const dot: Record<string, string> = { online: 'bg-success', degraded: 'bg-warning', offline: 'bg-danger', unauthorized: 'bg-danger' };

  function choose(id: number) {
    open = false;
    serverCtx.select(id);
    // Pages addressed by server (/server/<id>/…, /stack/<id>) follow the choice.
    // "All servers" goes back to the page's overview, which asks for one.
    const m = page.url.pathname.match(/^\/(server|stack)\/(\d+)(\/.*)?$/);
    if (!m || Number(m[2]) === id) return;
    if (id > 0) goto(`/${m[1]}/${id}${m[3] ?? ''}`);
    else goto(m[1] === 'stack' ? '/stack' : ({ '/fail2ban': '/fail2ban', '/threats': '/threats', '/terminal': '/terminal' } as Record<string, string>)[m[3] ?? ''] ?? '/servers');
  }
</script>

{#if serverCtx.servers.length > 1}
  <div class="relative" bind:this={el}>
    <button
      onclick={() => (open = !open)}
      class="flex h-8 max-w-52 cursor-pointer items-center gap-2 rounded-lg border border-line bg-surface px-2.5 text-[13px] transition-colors hover:border-line-strong"
      style="box-shadow: var(--shadow-card)"
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-label="Server"
      title="The server you are working on"
    >
      {#if serverCtx.selected}
        <span class="size-2 shrink-0 rounded-full {dot[serverCtx.selected.health_status] ?? 'bg-subtle'}"></span>
        <span class="truncate font-medium text-fg">{serverCtx.selected.name}</span>
      {:else}
        <Layers size={14} class="shrink-0 text-subtle" />
        <span class="truncate text-muted max-sm:hidden">All servers</span>
      {/if}
      <ChevronsUpDown size={13} class="ml-auto shrink-0 text-subtle" />
    </button>

    {#if open}
      <div class="popover animate-pop-in absolute top-full left-0 z-30 mt-1.5 w-72 max-w-[calc(100vw-2rem)] overflow-hidden p-1" role="listbox">
        <button onclick={() => choose(0)} class="flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] hover:bg-surface-2" role="option" aria-selected={serverCtx.selectedId === 0}>
          <Layers size={14} class="text-subtle" />
          <span class="flex-1 text-fg">All servers</span>
          {#if serverCtx.selectedId === 0}<Check size={14} class="text-accent" />{/if}
        </button>
        <div class="my-1 border-t border-line"></div>
        <div class="max-h-80 overflow-y-auto">
          {#each serverCtx.servers as s (s.id)}
            <button onclick={() => choose(s.id)} class="flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] hover:bg-surface-2" role="option" aria-selected={serverCtx.selectedId === s.id}>
              <span class="size-2 shrink-0 rounded-full {dot[s.health_status] ?? 'bg-subtle'}" title={s.health_status || 'unknown'}></span>
              <span class="min-w-0 flex-1">
                <span class="block truncate font-medium text-fg">{s.name}</span>
                <span class="block truncate font-mono text-[11.5px] text-subtle">{s.public_ip || s.ip_address || s.hostname}</span>
              </span>
              {#if serverCtx.selectedId === s.id}<Check size={14} class="text-accent" />{/if}
            </button>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
