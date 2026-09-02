<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { Lock, Ban, Plus, Trash2, Save, Loader2 } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();

  let secEnabled = $state(false);
  let secUsername = $state('');
  let secPassword = $state('');
  let secBlockedIPs = $state<string[]>([]);
  let secLoading = $state(false);
  let secSaving = $state(false);
  let newBlockedIP = $state('');
  let secLoaded = $state(false);

  $effect(() => { if (!secLoaded && !secLoading) { secLoaded = true; loadSecurity(); } });

  async function loadSecurity() {
    secLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/security`); secEnabled = d?.basic_auth_enabled || false; secUsername = d?.basic_auth_username || ''; secBlockedIPs = d?.blocked_ips || []; }
    catch {} finally { secLoading = false; secLoaded = true; }
  }
  async function saveSecurity() {
    secSaving = true;
    try { await apiFetch(auth.token, 'PUT', `sites/${siteId}/security`, { basic_auth_enabled: secEnabled, basic_auth_username: secUsername, basic_auth_password: secPassword, blocked_ips: secBlockedIPs }); toasts.success('Security updated'); secPassword = ''; }
    catch (e: any) { toasts.error(e.message); } finally { secSaving = false; }
  }
  function addBlockedIP() { if (!newBlockedIP) return; if (!secBlockedIPs.includes(newBlockedIP)) secBlockedIPs = [...secBlockedIPs, newBlockedIP]; newBlockedIP = ''; }
  function removeBlockedIP(ip: string) { secBlockedIPs = secBlockedIPs.filter(i => i !== ip); }
</script>

<div class="grid grid-cols-1 md:grid-cols-2 gap-4 lg:gap-5 mb-5">
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
    <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Lock size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Basic Authentication</span></div>
    <div class="p-3.5 sm:p-5">
      <p class="text-xs text-slate-500 mb-4">Protect this site with HTTP basic auth.</p>
      <div class="space-y-3">
        <label class="flex items-center gap-3 cursor-pointer"><input type="checkbox" bind:checked={secEnabled} class="w-4 h-4 rounded border-white/20 bg-white/[0.04] text-sky-500 focus:ring-sky-400" /><span class="text-sm text-slate-200">{secEnabled ? 'Enabled' : 'Disabled'}</span></label>
        {#if secEnabled}
          <div><label class="block text-xs text-slate-500 mb-1">Username</label><input bind:value={secUsername} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="admin" /></div>
          <div><label class="block text-xs text-slate-500 mb-1">Password</label><input bind:value={secPassword} type="password" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="Leave empty to keep current" /></div>
        {/if}
      </div>
    </div>
  </div>
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
    <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Ban size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">IP Blocking</span></div>
    <div class="p-3.5 sm:p-5">
      <p class="text-xs text-slate-500 mb-4">Block specific IP addresses from accessing this site.</p>
      <div class="flex gap-2 mb-3">
        <input bind:value={newBlockedIP} onkeydown={(e) => e.key === 'Enter' && addBlockedIP()} class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="192.168.1.1" />
        <button onclick={addBlockedIP} class="px-3 py-2 rounded-xl text-sm font-medium text-sky-400 hover:bg-sky-400/10 transition border border-sky-500/20"><Plus size={14} /></button>
      </div>
      {#if secBlockedIPs.length === 0}
        <div class="flex flex-col items-center justify-center py-8"><Ban size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">No blocked IPs</p><p class="text-xs text-slate-600">Add an IP above to block it</p></div>
      {:else}
        <div class="space-y-1">{#each secBlockedIPs as ip}<div class="flex items-center justify-between px-3 py-2 rounded-xl bg-white/[0.02] border border-white/[0.03]"><span class="text-sm text-slate-300 font-mono">{ip}</span><button onclick={() => removeBlockedIP(ip)} class="text-red-400/60 hover:text-red-400 transition p-1 rounded-lg hover:bg-red-400/10"><Trash2 size={12} /></button></div>{/each}</div>
      {/if}
    </div>
  </div>
</div>
<button onclick={saveSecurity} disabled={secSaving} class="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0">
  {#if secSaving}<Loader2 size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save Security
</button>
