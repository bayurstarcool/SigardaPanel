<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import AddServerButton from '$components/AddServerButton.svelte';
  import EditServerModal from '$components/EditServerModal.svelte';
  import { Server as ServerIcon } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let loading = $state(true);
  let editServer = $state<any>(null);
  let editOpen = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  function loadServers() {
    if (!auth.token) { window.location.href = '/login'; return; }
    loading = true;
    apiFetch(auth.token, 'GET', 'servers')
      .then((d: any) => servers = d?.servers ?? [])
      .catch((e) => { if (e.message !== 'session expired') {} })
      .finally(() => loading = false);
  }

  onMount(() => loadServers());

  function openEdit(s: any) {
    editServer = s;
    editOpen = true;
  }
</script>

<DashboardLayout active="Servers">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Servers</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${servers.length} server(s) registered`}</p>
    </div>
    <AddServerButton oncreated={() => loadServers()} />
  </div>

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading servers...</div>
    </div>
  {:else if servers.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <ServerIcon size={48} class="mb-4 opacity-30" />
      <h2 class="text-lg font-semibold mb-2">No Servers</h2>
      <p class="text-sm text-slate-500 mb-6">Add your first server to start managing sites.</p>
      <AddServerButton oncreated={() => loadServers()} />
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead><tr class="text-left">
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Name</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Hostname</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Status</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">IP</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Agent</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Added</th>
            {#if canManage}<th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap"></th>{/if}
          </tr></thead>
          <tbody>
            {#each servers as s}
              <tr>
                <td class="px-5 py-3.5 border-b border-white/[0.03] font-medium align-middle">{s.name}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-400 align-middle">{s.hostname}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle"><StatusPill status={s.status} /></td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-400 text-xs align-middle">{s.ip_address || '—'}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-500 text-xs align-middle">
                  {#if s.agent_url}
                    {#if s.health_status === 'online'}
                      <span class="inline-flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span><span class="text-emerald-400">online</span></span>
                    {:else if s.health_status === 'degraded'}
                      <span class="inline-flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-yellow-400"></span><span class="text-yellow-400">degraded</span></span>
                    {:else if s.health_status === 'offline'}
                      <span class="inline-flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-red-400"></span><span class="text-red-400">offline</span></span>
                    {:else}
                      <span class="inline-flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-gray-500"></span><span class="text-gray-400">unknown</span></span>
                    {/if}
                  {:else}—{/if}
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-500 text-xs align-middle">{fmtDate(s.created_at)}</td>
                {#if canManage}
                  <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs align-middle">
                    <div class="flex gap-1.5">
                      <a href="/stack/{s.id}" class="px-2.5 py-1 rounded-lg text-xs bg-sky-600/20 text-sky-400 hover:bg-sky-600/30 transition border border-sky-500/20 no-underline">Stack</a>
                      <a href="/server/{s.id}/specs" class="px-2.5 py-1 rounded-lg text-xs bg-emerald-600/20 text-emerald-400 hover:bg-emerald-600/30 transition border border-emerald-500/20 no-underline">Specs</a>
                      <button onclick={() => openEdit(s)} class="px-2.5 py-1 rounded-lg text-xs bg-gray-700/50 text-gray-300 hover:bg-gray-700 hover:text-white transition cursor-pointer border-0">Edit</button>
                    </div>
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
  <EditServerModal bind:open={editOpen} server={editServer} onsaved={() => loadServers()} />
</DashboardLayout>
