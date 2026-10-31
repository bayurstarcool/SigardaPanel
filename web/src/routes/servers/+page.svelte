<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import AddServerButton from '$components/AddServerButton.svelte';
  import EditServerModal from '$components/EditServerModal.svelte';
  import { Server as ServerIcon, Layers, Cpu, Pencil, RefreshCw } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let loading = $state(true);
  let editServer = $state<any>(null);
  let editOpen = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  function loadServers() {
    if (!auth.user) { window.location.href = '/login'; return; }
    loading = true;
    apiFetch(auth.token, 'GET', 'servers')
      .then((d: any) => servers = d?.servers ?? [])
      .catch((e) => { if (e.message !== 'session expired') {} })
      .finally(() => loading = false);
  }

  onMount(() => loadServers());

  const health: Record<string, { label: string; cls: string }> = {
    online: { label: 'online', cls: 'text-success' },
    degraded: { label: 'degraded', cls: 'text-warning' },
    offline: { label: 'offline', cls: 'text-danger' },
  };

  function openEdit(s: any) {
    editServer = s;
    editOpen = true;
  }
</script>

<DashboardLayout active="Servers">
  <PageHeader title="Servers" description={loading ? 'Loading…' : `${servers.length} server${servers.length === 1 ? '' : 's'} registered`}>
    {#snippet actions()}
      <button onclick={loadServers} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      <AddServerButton oncreated={() => loadServers()} />
    {/snippet}
  </PageHeader>

  <div class="card">
    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}
      </div>
    {:else if servers.length === 0}
      <EmptyState icon={ServerIcon as any} title="No servers yet" description="Add your first server to start managing sites.">
        <AddServerButton oncreated={() => loadServers()} />
      </EmptyState>
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>Name</th>
              <th class="max-sm:hidden">Hostname</th>
              <th>Status</th>
              <th class="max-md:hidden">IP</th>
              <th>Agent</th>
              <th class="max-lg:hidden">Added</th>
              {#if canManage}<th class="text-right">Actions</th>{/if}
            </tr>
          </thead>
          <tbody>
            {#each servers as s (s.id)}
              {@const h = health[s.health_status] ?? { label: 'unknown', cls: 'text-subtle' }}
              <tr>
                <td class="font-medium">{s.name}</td>
                <td class="text-muted max-sm:hidden">{s.hostname}</td>
                <td><StatusPill status={s.status} /></td>
                <td class="font-mono text-[13px] text-muted max-md:hidden">{s.ip_address || '—'}</td>
                <td class="text-xs">
                  {#if s.agent_url}
                    <span class="inline-flex items-center gap-1.5 {h.cls}"><span class="dot"></span>{h.label}</span>
                  {:else}
                    <span class="text-subtle">—</span>
                  {/if}
                </td>
                <td class="text-xs text-subtle tabular-nums max-lg:hidden">{fmtDate(s.created_at)}</td>
                {#if canManage}
                  <td class="text-right">
                    <div class="inline-flex gap-1">
                      <a href="/stack/{s.id}" class="btn btn-ghost btn-xs"><Layers size={12} /> Stack</a>
                      <a href="/server/{s.id}/specs" class="btn btn-ghost btn-xs"><Cpu size={12} /> Specs</a>
                      <button onclick={() => openEdit(s)} class="btn btn-ghost btn-xs"><Pencil size={12} /> Edit</button>
                    </div>
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
  <EditServerModal bind:open={editOpen} server={editServer} onsaved={() => loadServers()} />
</DashboardLayout>
