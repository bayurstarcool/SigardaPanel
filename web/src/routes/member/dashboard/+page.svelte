<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { Shield, Copy, Check, ExternalLink, Server, Globe, Zap, Activity } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let token = $state('');
  let copied = $state(false);
  let servers = $state(0);
  let sites = $state(0);

  onMount(() => {
    token = auth.token || '';
    // Fetch real stats — the HttpOnly session cookie authenticates via the
    // proxy; an explicit Bearer header is only needed for impersonation.
    if (auth.user) {
      fetch('/api/proxy/servers')
        .then(r => r.json())
        .then(d => { if (d.data) servers = d.data.count || (d.data.servers || []).length || 0; })
        .catch(() => {});
      fetch('/api/proxy/sites?limit=100')
        .then(r => r.json())
        .then(d => { if (d.data) sites = d.data.count || d.data.total_count || (d.data.sites || []).length || 0; })
        .catch(() => {});
    }
  });

  function copyToken() {
    navigator.clipboard.writeText(token);
    copied = true;
    setTimeout(() => copied = false, 2000);
  }
</script>

<svelte:head>
  <title>Member Dashboard — SigardaPanel</title>
</svelte:head>

<div class="space-y-8">
  <!-- Welcome -->
  <div>
    <h1 class="text-2xl font-bold text-white mb-1">Welcome back, {auth.user?.username}</h1>
    <p class="text-sm text-slate-500">Manage your account and API access.</p>
  </div>

  <!-- Plan Card -->
  <div class="relative p-6 rounded-2xl border border-sky-400/20 bg-gradient-to-br from-sky-400/[0.06] to-indigo-400/[0.04] overflow-hidden">
    <div class="absolute top-0 right-0 w-32 h-32 bg-sky-400/10 rounded-full blur-3xl -translate-y-1/2 translate-x-1/2"></div>
    <div class="relative">
      <div class="flex items-center gap-2 mb-3">
        <Shield size={18} class="text-sky-400" />
        <span class="text-sm font-semibold text-sky-400">Community Plan</span>
        <span class="px-2 py-0.5 rounded-full text-[0.6rem] font-bold bg-emerald-500/10 text-emerald-400">Active</span>
      </div>
      <p class="text-sm text-slate-400 mb-4">You're using the free Community plan. Upgrade for multi-server management and priority support.</p>
      <a href="/pricing" class="inline-flex items-center gap-2 px-4 py-2 rounded-xl text-xs font-semibold bg-white/[0.08] border border-white/[0.12] hover:bg-white/[0.15] text-white transition-all">
        View Plans <ExternalLink size={12} />
      </a>
    </div>
  </div>

  <!-- Stats Grid -->
  <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
    {#each [
      { label: 'Servers', value: servers, icon: Server, color: 'sky' },
      { label: 'Sites', value: sites, icon: Globe, color: 'emerald' },
      { label: 'Plan', value: 'Free', icon: Shield, color: 'violet' },
      { label: 'Status', value: 'Active', icon: Activity, color: 'green' },
    ] as stat}
      <div class="p-4 rounded-xl border border-white/[0.06] bg-white/[0.02]">
        <div class="flex items-center gap-2 mb-2">
          <stat.icon size={14} class="text-{stat.color}-400" />
          <span class="text-[0.65rem] text-slate-500 uppercase tracking-wider">{stat.label}</span>
        </div>
        <div class="text-xl font-bold text-white">{stat.value}</div>
      </div>
    {/each}
  </div>

  <!-- API Token -->
  <div class="p-6 rounded-2xl border border-white/[0.06] bg-white/[0.02]">
    <h2 class="text-sm font-semibold text-white mb-4">API Token</h2>
    <p class="text-xs text-slate-500 mb-4">Use this token to authenticate API requests and CLI commands.</p>
    <div class="flex items-center gap-2">
      <div class="flex-1 font-mono text-xs text-slate-400 bg-black/20 rounded-xl px-4 py-3 border border-white/[0.06] truncate">
        {token || 'No token available'}
      </div>
      <button onclick={copyToken} class="px-4 py-3 rounded-xl bg-white/[0.06] border border-white/[0.08] hover:bg-white/[0.1] transition-all">
        {#if copied}
          <Check size={14} class="text-emerald-400" />
        {:else}
          <Copy size={14} class="text-slate-400" />
        {/if}
      </button>
    </div>
    <div class="mt-4 p-3 rounded-xl bg-white/[0.02] border border-white/[0.04]">
      <p class="text-[0.65rem] text-slate-500 mb-2">Quick start with CLI:</p>
      <code class="text-xs font-mono text-emerald-400/70">sigardapanel login --api-url https://panel.sigarda.dev</code>
    </div>
  </div>

  <!-- Quick Links -->
  <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
    <a href="/member/api-keys" class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] hover:bg-white/[0.04] transition-all group">
      <div class="w-10 h-10 rounded-xl bg-sky-500/10 flex items-center justify-center mb-3 group-hover:scale-110 transition-transform">
        <svg class="w-5 h-5 text-sky-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
      </div>
      <h3 class="text-sm font-semibold text-white mb-1">API Keys</h3>
      <p class="text-xs text-slate-500">Manage your API tokens and access.</p>
    </a>
    <a href="/member/billing" class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] hover:bg-white/[0.04] transition-all group">
      <div class="w-10 h-10 rounded-xl bg-violet-500/10 flex items-center justify-center mb-3 group-hover:scale-110 transition-transform">
        <svg class="w-5 h-5 text-violet-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z"/></svg>
      </div>
      <h3 class="text-sm font-semibold text-white mb-1">Billing</h3>
      <p class="text-xs text-slate-500">View plans and manage subscription.</p>
    </a>
    <a href="/dashboard" class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] hover:bg-white/[0.04] transition-all group">
      <div class="w-10 h-10 rounded-xl bg-emerald-500/10 flex items-center justify-center mb-3 group-hover:scale-110 transition-transform">
        <svg class="w-5 h-5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
      </div>
      <h3 class="text-sm font-semibold text-white mb-1">Panel Dashboard</h3>
      <p class="text-xs text-slate-500">Go to VPS management panel.</p>
    </a>
  </div>
</div>
