<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import { Shield, ShieldOff, ShieldCheck, Ban, Unlock, RefreshCw, Loader2, AlertTriangle, Check } from 'lucide-svelte';

  const serverId = $page.params.id;
  let status = $state<any>(null);
  let jails = $state<any[]>([]);
  let selectedJail = $state<string>('');
  let jailDetail = $state<any>(null);
  let loading = $state(true);
  let jailLoading = $state(false);
  let busy = $state(false);
  let unbanIp = $state('');
  let banIp = $state('');
  let banJail = $state('sshd');
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null);

  onMount(() => { loadStatus(); });

  async function loadStatus() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `servers/${serverId}/fail2ban/status`);
      status = d;
      jails = (d?.jails ?? []).map((name: string) => ({ name, loading: false }));
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { loading = false; }
  }

  async function loadJail(name: string) {
    selectedJail = name;
    jailLoading = true;
    jailDetail = null;
    try {
      const d = await apiFetch(auth.token, 'GET', `servers/${serverId}/fail2ban/jail?name=${name}`);
      jailDetail = d;
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { jailLoading = false; }
  }

  async function unbanIP() {
    if (!unbanIp.trim()) return;
    busy = true;
    message = null;
    try {
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/unban`, { jail: selectedJail || 'sshd', ip: unbanIp.trim() });
      message = { type: 'success', text: `Unbanned ${unbanIp}` };
      unbanIp = '';
      if (selectedJail) loadJail(selectedJail);
      loadStatus();
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }

  async function banIP() {
    if (!banIp.trim()) return;
    busy = true;
    message = null;
    try {
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/ban`, { jail: banJail, ip: banIp.trim() });
      message = { type: 'success', text: `Banned ${banIp} in ${banJail}` };
      banIp = '';
      if (selectedJail) loadJail(selectedJail);
      loadStatus();
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }

  async function toggleJail(name: string, disable: boolean) {
    busy = true;
    message = null;
    try {
      const action = disable ? 'disable' : 'enable';
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/${action}`, { jail: name });
      message = { type: 'success', text: `${disable ? 'Disabled' : 'Enabled'} jail ${name}` };
      loadStatus();
      if (selectedJail === name) loadJail(name);
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }
</script>

<div class="max-w-4xl mx-auto">
  <div class="flex items-center justify-between mb-6">
    <div>
      <div class="flex items-center gap-2 mb-1">
        <a href="/server/{serverId}/terminal" class="text-xs text-slate-500 hover:text-sky-400 transition">← Terminal</a>
        <span class="text-slate-700">·</span>
        <a href="/servers" class="text-xs text-slate-500 hover:text-sky-400 transition">Servers</a>
      </div>
      <h1 class="text-xl font-bold text-white flex items-center gap-2">
        <Shield size={20} class="text-sky-400" /> Fail2ban
      </h1>
      <p class="text-xs text-slate-500 mt-0.5">Server #{serverId} — Intrusion Prevention</p>
    </div>
    <button onclick={loadStatus} class="p-2 rounded-lg text-slate-500 hover:text-white hover:bg-white/5 transition">
      <RefreshCw size={16} class={loading ? 'animate-spin' : ''} />
    </button>
  </div>

  {#if message}
    <div class="mb-4 px-4 py-3 rounded-xl text-sm flex items-center gap-2 {message.type === 'success' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-400' : 'bg-red-500/10 border border-red-500/20 text-red-400'}">
      {#if message.type === 'success'}<Check size={14} />{:else}<AlertTriangle size={14} />{/if}
      {message.text}
    </div>
  {/if}

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-8 text-center">
      <Loader2 size={24} class="animate-spin text-slate-500 mx-auto mb-2" />
      <p class="text-sm text-slate-500">Loading fail2ban status...</p>
    </div>
  {:else if !status?.installed}
    <div class="bg-red-500/5 border border-red-500/20 rounded-xl p-6 text-center">
      <Ban size={32} class="text-red-400 mx-auto mb-3" />
      <h3 class="text-sm font-semibold text-red-400 mb-1">fail2ban not installed</h3>
      <p class="text-xs text-slate-500">Install fail2ban on this server to enable intrusion prevention.</p>
    </div>
  {:else}
    <!-- Status Overview -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4">
        <div class="flex items-center gap-2 mb-2">
          <div class="w-8 h-8 rounded-lg bg-sky-400/10 flex items-center justify-center"><Shield size={14} class="text-sky-400" /></div>
          <span class="text-[0.65rem] text-slate-500 uppercase">Status</span>
        </div>
        <p class="text-sm font-semibold {status?.active ? 'text-emerald-400' : 'text-slate-500'}">{status?.active ? 'Active' : 'Inactive'}</p>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4">
        <div class="flex items-center gap-2 mb-2">
          <div class="w-8 h-8 rounded-lg bg-violet-400/10 flex items-center justify-center"><ShieldCheck size={14} class="text-violet-400" /></div>
          <span class="text-[0.65rem] text-slate-500 uppercase">Jails</span>
        </div>
        <p class="text-sm font-semibold text-white">{jails.length}</p>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4">
        <div class="flex items-center gap-2 mb-2">
          <div class="w-8 h-8 rounded-lg bg-amber-400/10 flex items-center justify-center"><Ban size={14} class="text-amber-400" /></div>
          <span class="text-[0.65rem] text-slate-500 uppercase">Banned IPs</span>
        </div>
        <p class="text-sm font-semibold text-amber-400">{jailDetail?.currently_banned ?? '—'}</p>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl p-4">
        <div class="flex items-center gap-2 mb-2">
          <div class="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center"><Shield size={14} class="text-emerald-400" /></div>
          <span class="text-[0.65rem] text-slate-500 uppercase">Version</span>
        </div>
        <p class="text-sm font-semibold text-white">{status?.version?.split('\n')[0] ?? '—'}</p>
      </div>
    </div>

    <!-- Jails List -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden mb-6">
      <div class="px-5 py-3 border-b border-white/[0.05] bg-white/[0.02]">
        <h3 class="text-sm font-semibold text-white">Jails</h3>
      </div>
      <div class="divide-y divide-white/[0.03]">
        {#each jails as jail}
          <div class="px-5 py-3 flex items-center justify-between hover:bg-white/[0.02] transition cursor-pointer"
               onclick={() => loadJail(jail.name)}>
            <div class="flex items-center gap-3">
              <div class="w-2 h-2 rounded-full bg-emerald-400"></div>
              <span class="text-sm text-white font-medium">{jail.name}</span>
            </div>
            <button onclick={(e) => { e.stopPropagation(); toggleJail(jail.name, true); }}
              class="text-xs text-red-400/60 hover:text-red-400 transition px-2 py-1 rounded hover:bg-red-400/10">
              Disable
            </button>
          </div>
        {/each}
        {#if jails.length === 0}
          <div class="px-5 py-8 text-center text-sm text-slate-500">No jails active</div>
        {/if}
      </div>
    </div>

    <!-- Jail Detail -->
    {#if selectedJail}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden mb-6">
        <div class="px-5 py-3 border-b border-white/[0.05] bg-white/[0.02] flex items-center justify-between">
          <h3 class="text-sm font-semibold text-white">Jail: {selectedJail}</h3>
          <button onclick={() => { selectedJail = ''; jailDetail = null; }} class="text-xs text-slate-500 hover:text-white">✕ Close</button>
        </div>
        {#if jailLoading}
          <div class="p-6 text-center"><Loader2 size={20} class="animate-spin text-slate-500 mx-auto" /></div>
        {:else if jailDetail}
          <div class="p-5">
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-4">
              <div><span class="text-[0.65rem] text-slate-500 uppercase block">Currently Banned</span><span class="text-lg font-bold text-amber-400">{jailDetail.currently_banned ?? 0}</span></div>
              <div><span class="text-[0.65rem] text-slate-500 uppercase block">Total Banned</span><span class="text-lg font-bold text-white">{jailDetail.total_banned ?? 0}</span></div>
              <div><span class="text-[0.65rem] text-slate-500 uppercase block">Currently Failed</span><span class="text-lg font-bold text-sky-400">{jailDetail.currently_failed ?? 0}</span></div>
              <div><span class="text-[0.65rem] text-slate-500 uppercase block">Total Failed</span><span class="text-lg font-bold text-white">{jailDetail.failed ?? 0}</span></div>
            </div>

            {#if jailDetail.banned?.length > 0}
              <div class="mt-4">
                <h4 class="text-xs font-semibold text-slate-400 uppercase mb-2">Banned IPs</h4>
                <div class="space-y-1">
                  {#each jailDetail.banned as ip}
                    <div class="flex items-center justify-between px-3 py-2 bg-white/[0.03] rounded-lg">
                      <span class="text-sm text-white font-mono">{ip}</span>
                      <button onclick={() => { unbanIp = ip; unbanIP(); }}
                        class="text-xs text-emerald-400 hover:text-emerald-300 px-2 py-1 rounded hover:bg-emerald-400/10 transition">
                        Unban
                      </button>
                    </div>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <!-- Manual Ban/Unban -->
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <h3 class="text-sm font-semibold text-white mb-3 flex items-center gap-2"><Unlock size={14} class="text-emerald-400" /> Unban IP</h3>
        <div class="flex gap-2">
          <input bind:value={unbanIp} placeholder="IP address"
            class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-sm text-white font-mono outline-none focus:border-emerald-400/50" />
          <button onclick={unbanIP} disabled={busy || !unbanIp.trim()}
            class="px-4 py-2 rounded-lg text-xs font-semibold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/30 transition disabled:opacity-40">
            {busy ? '...' : 'Unban'}
          </button>
        </div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <h3 class="text-sm font-semibold text-white mb-3 flex items-center gap-2"><Ban size={14} class="text-red-400" /> Ban IP</h3>
        <div class="flex gap-2">
          <select bind:value={banJail} class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-sm text-white outline-none">
            {#each jails as j}<option value={j.name}>{j.name}</option>{/each}
          </select>
          <input bind:value={banIp} placeholder="IP address"
            class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-sm text-white font-mono outline-none focus:border-red-400/50" />
          <button onclick={banIP} disabled={busy || !banIp.trim()}
            class="px-4 py-2 rounded-lg text-xs font-semibold bg-red-500/20 text-red-400 border border-red-500/30 hover:bg-red-500/30 transition disabled:opacity-40">
            {busy ? '...' : 'Ban'}
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
