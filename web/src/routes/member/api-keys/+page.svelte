<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { Key, Copy, Check, Plus, Trash2, Shield } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let token = $state('');
  let copied = $state(false);
  let keys = $state<any[]>([]);

  onMount(() => {
    token = auth.token || '';
    keys = [
      { id: 1, name: 'Default', token: token, created: '2026-07-12', lastUsed: 'Just now', active: true },
    ];
  });

  function copyToken(t: string) {
    navigator.clipboard.writeText(t);
    copied = true;
    setTimeout(() => copied = false, 2000);
  }

  function maskToken(t: string) {
    if (!t) return '••••••••••••••••';
    return t.slice(0, 8) + '...' + t.slice(-4);
  }
</script>

<svelte:head>
  <title>API Keys — SigardaPanel</title>
</svelte:head>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold text-white mb-1">API Keys</h1>
    <p class="text-sm text-slate-500">Manage your API tokens for CLI and integrations.</p>
  </div>

  <!-- Primary Token -->
  <div class="p-6 rounded-2xl border border-sky-400/20 bg-sky-400/[0.04]">
    <div class="flex items-center gap-2 mb-3">
      <Key size={16} class="text-sky-400" />
      <span class="text-sm font-semibold text-sky-400">Primary Token</span>
    </div>
    <p class="text-xs text-slate-400 mb-4">This is your main API token for CLI login and API access.</p>
    <div class="flex items-center gap-2">
      <div class="flex-1 font-mono text-xs text-slate-400 bg-black/20 rounded-xl px-4 py-3 border border-white/[0.06]">
        {maskToken(token)}
      </div>
      <button onclick={() => copyToken(token)} class="px-4 py-3 rounded-xl bg-white/[0.06] border border-white/[0.08] hover:bg-white/[0.1] transition-all">
        {#if copied}
          <Check size={14} class="text-emerald-400" />
        {:else}
          <Copy size={14} class="text-slate-400" />
        {/if}
      </button>
    </div>
  </div>

  <!-- Keys List -->
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-sm font-semibold text-white">All Keys</h2>
    </div>
    <div class="space-y-3">
      {#each keys as key}
        <div class="flex items-center gap-4 p-4 rounded-xl border border-white/[0.06] bg-white/[0.02]">
          <div class="w-8 h-8 rounded-lg bg-sky-500/10 flex items-center justify-center">
            <Key size={14} class="text-sky-400" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium text-white">{key.name}</div>
            <div class="text-[0.65rem] text-slate-500">Created {key.created} &middot; Last used {key.lastUsed}</div>
          </div>
          <div class="flex items-center gap-2">
            <span class="px-2 py-0.5 rounded-full text-[0.6rem] font-semibold {key.active ? 'bg-emerald-500/10 text-emerald-400' : 'bg-slate-500/10 text-slate-400'}">
              {key.active ? 'Active' : 'Revoked'}
            </span>
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Usage Info -->
  <div class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02]">
    <h3 class="text-sm font-semibold text-white mb-3">How to use</h3>
    <div class="space-y-3">
      <div class="p-3 rounded-lg bg-black/20 border border-white/[0.04]">
        <p class="text-[0.65rem] text-slate-500 mb-1">CLI Login</p>
        <code class="text-xs font-mono text-emerald-400/70">sigardapanel login --api-url https://panel.sigarda.dev</code>
      </div>
      <div class="p-3 rounded-lg bg-black/20 border border-white/[0.04]">
        <p class="text-[0.65rem] text-slate-500 mb-1">API Request</p>
        <code class="text-xs font-mono text-emerald-400/70">curl -H "Authorization: Bearer YOUR_TOKEN" https://panel.sigarda.dev/api/v1/servers</code>
      </div>
      <div class="p-3 rounded-lg bg-black/20 border border-white/[0.04]">
        <p class="text-[0.65rem] text-slate-500 mb-1">Environment Variable</p>
        <code class="text-xs font-mono text-emerald-400/70">export SIGARDAPANEL_TOKEN=YOUR_TOKEN</code>
      </div>
    </div>
  </div>
</div>
