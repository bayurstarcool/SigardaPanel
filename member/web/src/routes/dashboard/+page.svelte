<script>
  import { Shield, Copy, Check, ExternalLink, CreditCard, Key, Server, Activity } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  let member = $state(null);
  let token = $state('');
  let copied = $state(false);

  onMount(() => {
    token = localStorage.getItem('member_token');
    if (!token) { goto('/login'); return; }
    const stored = localStorage.getItem('member');
    if (stored) member = JSON.parse(stored);
    fetchMember();
  });

  async function fetchMember() {
    try {
      const res = await fetch('/api/me', { headers: { Authorization: `Bearer ${token}` } });
      const data = await res.json();
      member = data.data;
      localStorage.setItem('member', JSON.stringify(data.data));
    } catch {}
  }

  function copyToken() {
    navigator.clipboard.writeText(token);
    copied = true;
    setTimeout(() => copied = false, 2000);
  }

  function logout() {
    localStorage.removeItem('member_token');
    localStorage.removeItem('member');
    goto('/login');
  }
</script>

<div class="min-h-screen bg-[#060d1f] text-white">
  <nav class="border-b border-white/[0.06] px-6 py-4">
    <div class="max-w-6xl mx-auto flex items-center justify-between">
      <a href="/" class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center">
          <Shield size={16} class="text-white" />
        </div>
        <span class="font-bold text-sm">Member Portal</span>
      </a>
      <div class="flex items-center gap-4">
        <a href="/dashboard" class="text-sm text-white">Dashboard</a>
        <button onclick={logout} class="text-sm text-slate-500 hover:text-white">Logout</button>
      </div>
    </div>
  </nav>

  <main class="max-w-6xl mx-auto p-6 lg:p-10">
    {#if member}
      <h1 class="text-2xl font-bold mb-1">Welcome, {member.name}</h1>
      <p class="text-sm text-slate-500 mb-8">{member.email}</p>

      <!-- Plan -->
      <div class="p-6 rounded-2xl border border-sky-400/20 bg-sky-400/[0.04] mb-8">
        <div class="flex items-center gap-2 mb-2">
          <CreditCard size={16} class="text-sky-400" />
          <span class="text-sm font-semibold text-sky-400">Plan: {member.plan}</span>
        </div>
        {#if member.license_key}
          <div class="mt-3 p-3 rounded-xl bg-black/20 border border-white/[0.06] flex items-center gap-2">
            <code class="flex-1 text-xs font-mono text-sky-300">{member.license_key}</code>
            <button onclick={() => { navigator.clipboard.writeText(member.license_key); copied = true; setTimeout(() => copied = false, 2000); }} class="p-1.5 rounded-lg hover:bg-white/[0.05]">
              {#if copied}<Check size={14} class="text-emerald-400" />{:else}<Copy size={14} class="text-slate-400" />{/if}
            </button>
          </div>
        {/if}
        <div class="mt-3 text-xs text-slate-500">Use this license key in SigardaPanel to activate your plan.</div>
      </div>

      <!-- API Token -->
      <div class="p-6 rounded-2xl border border-white/[0.06] bg-white/[0.02] mb-8">
        <h2 class="text-sm font-semibold text-white mb-3">API Token</h2>
        <div class="flex items-center gap-2">
          <code class="flex-1 text-xs font-mono text-slate-400 bg-black/20 rounded-xl px-4 py-3 truncate">{token}</code>
          <button onclick={copyToken} class="px-3 py-3 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] transition-all">
            {#if copied}<Check size={14} class="text-emerald-400" />{:else}<Copy size={14} class="text-slate-400" />{/if}
          </button>
        </div>
      </div>

      <!-- Quick Links -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <a href="/pricing" class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] transition-all">
          <CreditCard size={20} class="text-violet-400 mb-2" />
          <h3 class="text-sm font-semibold text-white">Upgrade Plan</h3>
          <p class="text-xs text-slate-500">View plans and upgrade your account.</p>
        </a>
        <a href="https://panel.sigarda.dev" target="_blank" class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] transition-all">
          <Server size={20} class="text-sky-400 mb-2" />
          <h3 class="text-sm font-semibold text-white">Open Panel</h3>
          <p class="text-xs text-slate-500">Access SigardaPanel dashboard.</p>
        </a>
      </div>
    {:else}
      <div class="flex items-center justify-center py-20">
        <div class="w-8 h-8 border-2 border-sky-400 border-t-transparent rounded-full animate-spin"></div>
      </div>
    {/if}
  </main>
</div>
