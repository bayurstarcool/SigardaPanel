<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Server, ChevronRight } from 'lucide-svelte';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let loading = $state(true);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
    } finally {
      loading = false;
    }
  });

  function agentState(s: any): { label: string; cls: string } {
    if (!s.agent_url) return { label: 'No agent', cls: 'text-subtle' };
    if (s.health_status === 'online') return { label: 'Agent online', cls: 'text-success' };
    if (s.health_status === 'degraded') return { label: 'Agent degraded', cls: 'text-warning' };
    return { label: 'Agent offline', cls: 'text-subtle' };
  }
</script>

<DashboardLayout active="Stack">
  <PageHeader
    title="Stack management"
    description="Install, update and manage software on your servers — nginx, PHP, Node.js and more."
  />

  {#if loading}
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each Array(3) as _, i (i)}<div class="skeleton h-[104px] rounded-xl"></div>{/each}
    </div>
  {:else if servers.length === 0}
    <div class="card">
      <EmptyState
        icon={Server as any}
        title="No servers yet"
        description="Add a server first to start managing its stack."
        actionLabel="Go to servers"
        onaction={() => goto('/servers')}
      />
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each servers as s (s.id)}
        {@const a = agentState(s)}
        <a href="/stack/{s.id}" class="card group block p-4 transition-colors duration-150 hover:border-line-strong hover:bg-surface-2/50">
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted">
                <Server size={16} />
              </div>
              <div class="min-w-0">
                <h3 class="truncate text-sm font-semibold text-fg group-hover:text-accent">{s.name}</h3>
                <p class="truncate text-xs text-subtle">{s.hostname}</p>
              </div>
            </div>
            <StatusPill status={s.status} />
          </div>
          <div class="mt-4 flex items-center justify-between border-t border-line pt-3">
            <span class="inline-flex items-center gap-1.5 text-xs {a.cls}"><span class="dot"></span>{a.label}</span>
            <span class="inline-flex items-center gap-0.5 text-xs text-subtle group-hover:text-fg">Manage <ChevronRight size={14} /></span>
          </div>
        </a>
      {/each}
    </div>
  {/if}
</DashboardLayout>
