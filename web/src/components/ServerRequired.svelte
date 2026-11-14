<script lang="ts">
  // Shown by single-server pages while "All servers" is selected: the user
  // picks the server here (or in the header), never the first by default.
  import { serverCtx } from '$lib/serverContext.svelte';
  import { Server } from 'lucide-svelte';

  let { feature = 'this page' }: { feature?: string } = $props();
  const dot: Record<string, string> = { online: 'bg-success', degraded: 'bg-warning', offline: 'bg-danger', unauthorized: 'bg-danger' };
</script>

<div class="card mx-auto max-w-xl animate-fade-in">
  <div class="card-body">
    <div class="mb-4 flex items-start gap-3">
      <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent"><Server size={17} /></span>
      <div>
        <h2 class="text-[15px] font-semibold text-fg">Choose a server</h2>
        <p class="mt-0.5 text-[13px] text-muted">{feature} works on one server at a time. Pick it here or with the server switcher at the top.</p>
      </div>
    </div>
    <div class="space-y-1.5">
      {#each serverCtx.servers as s (s.id)}
        <button onclick={() => serverCtx.select(s.id)} class="flex w-full cursor-pointer items-center gap-3 rounded-lg border border-line px-3 py-2.5 text-left transition-colors hover:border-accent/40 hover:bg-accent/5">
          <span class="size-2 shrink-0 rounded-full {dot[s.health_status] ?? 'bg-subtle'}"></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[13.5px] font-medium text-fg">{s.name}</span>
            <span class="block truncate font-mono text-[11.5px] text-subtle">{s.public_ip || s.ip_address || s.hostname}</span>
          </span>
          <span class="text-[12px] text-subtle">{s.health_status || ''}</span>
        </button>
      {/each}
    </div>
  </div>
</div>
