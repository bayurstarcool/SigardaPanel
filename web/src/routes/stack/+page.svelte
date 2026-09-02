<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Layers, Server, ArrowRight, Activity } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let loading = $state(true);

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
    } finally {
      loading = false;
    }
  });
</script>

<DashboardLayout active="Stack">
  {#if loading}
    <div class="flex items-center justify-center py-32">
      <div class="flex flex-col items-center gap-4">
        <div class="relative">
          <div class="absolute inset-0 rounded-2xl bg-sky-400/20 blur-xl animate-pulse"></div>
          <div class="relative w-12 h-12 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-lg">
            <Layers size={20} class="text-white animate-pulse" />
          </div>
        </div>
        <span class="text-sm text-slate-500">Loading servers...</span>
      </div>
    </div>
  {:else}
    <!-- Hero Header -->
    <header class="relative mb-8 overflow-hidden rounded-2xl border border-white/[0.06] p-8 lg:p-10 bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-secondary)]/80">
      <div class="absolute top-0 right-0 w-96 h-96 rounded-full bg-sky-500/5 blur-3xl -translate-y-1/2 translate-x-1/2"></div>
      <div class="absolute bottom-0 left-0 w-64 h-64 rounded-full bg-indigo-500/5 blur-3xl translate-y-1/2 -translate-x-1/2"></div>
      <div class="absolute inset-0" style="background-image: radial-gradient(rgba(255,255,255,0.15) 1px, transparent 1px); background-size: 24px 24px;"></div>
      <div class="relative">
        <p class="mb-3 text-[0.65rem] font-bold uppercase tracking-[0.25em] text-sky-300/60">INSTANCE MANAGEMENT</p>
        <h1 class="text-3xl lg:text-4xl font-extrabold tracking-tight bg-gradient-to-r from-white via-sky-100 to-indigo-200 bg-clip-text text-transparent">
          Stack Management
        </h1>
        <p class="mt-3 text-sm text-slate-400 max-w-xl">
          Install, update, and manage software on your servers. One-click operations for nginx, PHP, Node.js, and more.
        </p>
      </div>
    </header>

    {#if servers.length === 0}
      <div class="relative overflow-hidden rounded-2xl border border-white/[0.06] p-12 text-center bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-secondary)]/80">
        <div class="absolute inset-0" style="background-image: radial-gradient(rgba(255,255,255,0.05) 1px, transparent 1px); background-size: 24px 24px;"></div>
        <div class="relative">
          <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20 flex items-center justify-center mx-auto mb-6 border border-white/[0.06]">
            <Server size={28} class="text-sky-400" />
          </div>
          <h2 class="text-xl font-bold text-white mb-2">No Servers Yet</h2>
          <p class="text-sm text-slate-400 mb-6 max-w-md mx-auto">
            Add a server first to start managing its stack. You can install nginx, PHP, Node.js, and more with one click.
          </p>
          <a href="/servers" class="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-sky-500 to-indigo-500 text-white text-sm font-medium hover:from-sky-400 hover:to-indigo-400 transition-all shadow-lg shadow-sky-500/20 no-underline">
            <Server size={16} />
            Add Server
          </a>
        </div>
      </div>
    {:else}
      <!-- Server Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {#each servers as s}
          <a href="/stack/{s.id}" class="group relative overflow-hidden rounded-2xl border border-white/[0.06] bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-secondary)]/80 p-6 hover:border-sky-500/30 hover:shadow-lg hover:shadow-sky-500/5 transition-all duration-300 no-underline">
            <!-- Subtle background pattern -->
            <div class="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-300" style="background-image: radial-gradient(rgba(56,189,248,0.08) 1px, transparent 1px); background-size: 16px 16px;"></div>
            
            <div class="relative">
              <!-- Header -->
              <div class="flex items-start justify-between mb-5">
                <div class="flex items-center gap-3">
                  <div class="w-11 h-11 rounded-xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20 flex items-center justify-center border border-white/[0.06] group-hover:from-sky-400/30 group-hover:to-indigo-500/30 transition-all">
                    <Server size={18} class="text-sky-400" />
                  </div>
                  <div>
                    <h3 class="font-semibold text-white group-hover:text-sky-400 transition-colors">{s.name}</h3>
                    <p class="text-xs text-slate-500 mt-0.5">{s.hostname}</p>
                  </div>
                </div>
                <StatusPill status={s.status} />
              </div>

              <!-- Agent Status -->
              <div class="flex items-center gap-2 mb-4 pb-4 border-b border-white/[0.04]">
                {#if s.agent_url}
                  {#if s.health_status === 'online'}
                    <span class="inline-flex items-center gap-1.5 text-xs text-emerald-400">
                      <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                      Agent Online
                    </span>
                  {:else if s.health_status === 'degraded'}
                    <span class="inline-flex items-center gap-1.5 text-xs text-yellow-400">
                      <span class="w-1.5 h-1.5 rounded-full bg-yellow-400"></span>
                      Agent Degraded
                    </span>
                  {:else}
                    <span class="inline-flex items-center gap-1.5 text-xs text-slate-500">
                      <span class="w-1.5 h-1.5 rounded-full bg-slate-500"></span>
                      Agent Offline
                    </span>
                  {/if}
                {:else}
                  <span class="inline-flex items-center gap-1.5 text-xs text-slate-500">
                    <span class="w-1.5 h-1.5 rounded-full bg-slate-500"></span>
                    No Agent
                  </span>
                {/if}
              </div>

              <!-- CTA -->
              <div class="flex items-center justify-between">
                <span class="text-xs text-slate-500">Manage Stack</span>
                <div class="w-8 h-8 rounded-lg bg-white/[0.03] flex items-center justify-center group-hover:bg-sky-500/20 transition-all">
                  <ArrowRight size={14} class="text-slate-500 group-hover:text-sky-400 transition-colors" />
                </div>
              </div>
            </div>
          </a>
        {/each}
      </div>
    {/if}
  {/if}
</DashboardLayout>
