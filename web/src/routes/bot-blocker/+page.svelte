<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { Bot, Shield, ShieldCheck, Plus, Trash2, Loader2, Ban, X, RefreshCw, Globe } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let enabled = $state(false);
  let loading = $state(true);
  let busy = $state('');
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  let blockedAgents = $state<string[]>([]);
  let blockedIPs = $state<string[]>([]);
  let rawConfig = $state('');

  let showAddAgent = $state(false);
  let showAddIP = $state(false);
  let newAgent = $state('');
  let newIP = $state('');

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadRules();
    } else {
      loading = false;
    }
  });

  function loadRules() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'list' })
      .then((d: any) => {
        enabled = d?.enabled ?? false;
        rawConfig = d?.config ?? '';
        parseConfig(rawConfig);
      })
      .catch((e: any) => toasts.error('Failed to load: ' + e.message))
      .finally(() => loading = false);
  }

  function parseConfig(config: string) {
    const agentRegex = /~\*\(([^)]+)\)\s+1;/g;
    const a: string[] = [];
    let m;
    while ((m = agentRegex.exec(config)) !== null) a.push(m[1]);
    blockedAgents = a;

    const ipRegex = /deny\s+([^\s;]+);/g;
    const i: string[] = [];
    while ((m = ipRegex.exec(config)) !== null) i.push(m[1]);
    blockedIPs = i;
  }

  async function toggle(enable: boolean) {
    busy = 'toggle';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: enable ? 'enable' : 'disable' });
      toasts.success(enable ? 'Bot blocker enabled' : 'Bot blocker disabled');
      loadRules();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = ''; }
  }

  async function addAgent() {
    if (!newAgent.trim()) return;
    busy = 'add-agent';
    try {
      const list = newAgent.split(',').map(s => s.trim()).filter(Boolean);
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'add-agent', agents: list });
      toasts.success(`Added ${list.length} pattern(s)`);
      newAgent = ''; showAddAgent = false;
      loadRules();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = ''; }
  }

  async function removeAgent(agent: string) {
    busy = `ra-${agent}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'remove-agent', agents: [agent] });
      toasts.success('Removed');
      loadRules();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = ''; }
  }

  async function addIP() {
    if (!newIP.trim()) return;
    busy = 'add-ip';
    try {
      const list = newIP.split(',').map(s => s.trim()).filter(Boolean);
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'add-ip', ips: list });
      toasts.success(`Added ${list.length} IP(s)`);
      newIP = ''; showAddIP = false;
      loadRules();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = ''; }
  }

  async function removeIP(ip: string) {
    busy = `ri-${ip}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/bot-blocker/manage`, { action: 'remove-ip', ips: [ip] });
      toasts.success('Removed');
      loadRules();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = ''; }
  }
</script>

<DashboardLayout active="Bot Blocker">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-orange-100 bg-clip-text text-transparent">Bot Blocker</h1>
      <p class="text-sm text-slate-500 mt-1">Block malicious bots, scrapers, and unwanted traffic via nginx</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => loadRules()}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => loadRules()} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} />
      </button>
    </div>
  </div>

  <!-- Status + Toggle -->
  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <div class="text-xs text-slate-500 mb-2">Bot Blocker Status</div>
      <div class="flex items-center gap-3">
        <div class="w-3 h-3 rounded-full {enabled ? 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.5)]' : 'bg-red-400'}"></div>
        <span class="text-lg font-semibold {enabled ? 'text-emerald-400' : 'text-red-400'}">
          {enabled ? 'Active' : 'Inactive'}
        </span>
      </div>
    </div>
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 flex items-center justify-between">
      <div>
        <div class="text-xs text-slate-500 mb-2">Quick Actions</div>
        {#if enabled}
          <button onclick={() => toggle(false)} disabled={busy === 'toggle'}
            class="px-3 py-1.5 bg-red-500/20 text-red-400 rounded-lg text-xs font-medium hover:bg-red-500/30 transition disabled:opacity-50 flex items-center gap-1.5">
            {#if busy === 'toggle'}<Loader2 size={12} class="animate-spin" />{/if} Disable
          </button>
        {:else}
          <button onclick={() => toggle(true)} disabled={busy === 'toggle'}
            class="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-medium hover:bg-emerald-500/30 transition disabled:opacity-50 flex items-center gap-1.5">
            {#if busy === 'toggle'}<Loader2 size={12} class="animate-spin" />{/if} Enable
          </button>
        {/if}
      </div>
      <div class="text-right">
        <div class="text-xs text-slate-500">Rules</div>
        <div class="text-2xl font-bold text-white">{blockedAgents.length + blockedIPs.length}</div>
      </div>
    </div>
  </div>

  <!-- Action Buttons -->
  {#if canManage}
    <div class="flex flex-wrap items-center gap-3 mb-6">
      <button onclick={() => { showAddAgent = !showAddAgent; showAddIP = false; }}
        class="px-4 py-2 bg-indigo-500/20 text-indigo-400 rounded-lg text-sm font-medium hover:bg-indigo-500/30 transition flex items-center gap-2">
        <Bot size={16} /> Block User-Agent
      </button>
      <button onclick={() => { showAddIP = !showAddIP; showAddAgent = false; }}
        class="px-4 py-2 bg-red-500/20 text-red-400 rounded-lg text-sm font-medium hover:bg-red-500/30 transition flex items-center gap-2">
        <Ban size={16} /> Block IP
      </button>
    </div>

    {#if showAddAgent}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 mb-6">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-white">Block User-Agent Pattern</h3>
          <button onclick={() => showAddAgent = false} class="text-slate-500 hover:text-white transition"><X size={16} /></button>
        </div>
        <p class="text-xs text-slate-500 mb-3">Regex patterns to match User-Agent strings. Comma-separated for multiple.</p>
        <div class="flex gap-3">
          <input bind:value={newAgent} placeholder="e.g. semrushbot, ahrefsbot, dotbot"
            class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 font-mono"
            onkeydown={(e) => e.key === 'Enter' && addAgent()} />
          <button onclick={addAgent} disabled={busy === 'add-agent' || !newAgent.trim()}
            class="px-4 py-2 bg-indigo-500 text-white rounded-lg text-sm font-medium hover:bg-indigo-600 transition disabled:opacity-50 flex items-center gap-2">
            {#if busy === 'add-agent'}<Loader2 size={14} class="animate-spin" />{/if} Add
          </button>
        </div>
      </div>
    {/if}

    {#if showAddIP}
      <div class="bg-red-500/5 border border-red-500/20 rounded-xl p-5 mb-6">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-red-300">Block IP Address</h3>
          <button onclick={() => showAddIP = false} class="text-slate-500 hover:text-white transition"><X size={16} /></button>
        </div>
        <p class="text-xs text-slate-500 mb-3">IP addresses to deny. Comma-separated for multiple.</p>
        <div class="flex gap-3">
          <input bind:value={newIP} placeholder="e.g. 1.2.3.4, 5.6.7.8"
            class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 font-mono"
            onkeydown={(e) => e.key === 'Enter' && addIP()} />
          <button onclick={addIP} disabled={busy === 'add-ip' || !newIP.trim()}
            class="px-4 py-2 bg-red-500 text-white rounded-lg text-sm font-medium hover:bg-red-600 transition disabled:opacity-50 flex items-center gap-2">
            {#if busy === 'add-ip'}<Loader2 size={14} class="animate-spin" />{/if} Block
          </button>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Content -->
  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-slate-500">Loading bot blocker rules...</div>
    </div>
  {:else if !enabled && blockedAgents.length === 0 && blockedIPs.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Shield size={48} class="mx-auto mb-4 opacity-30 text-slate-600" />
      <h2 class="text-lg font-semibold text-white mb-2">Bot Blocker Not Configured</h2>
      <p class="text-sm text-slate-500 mb-4">Enable bot blocker to start blocking malicious traffic.</p>
      {#if canManage}
        <button onclick={() => toggle(true)} disabled={busy === 'toggle'}
          class="px-4 py-2 bg-emerald-500/20 text-emerald-400 rounded-lg text-sm font-medium hover:bg-emerald-500/30 transition disabled:opacity-50 inline-flex items-center gap-2">
          {#if busy === 'toggle'}<Loader2 size={14} class="animate-spin" />{/if}
          <ShieldCheck size={14} /> Enable Bot Blocker
        </button>
      {/if}
    </div>
  {:else}
    <!-- Blocked User-Agents -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden mb-6">
      <div class="px-5 py-4 border-b border-white/[0.06] flex items-center gap-2">
        <Bot size={16} class="text-sky-400" />
        <h3 class="text-sm font-semibold text-white">Blocked User-Agent Patterns</h3>
        <span class="px-1.5 py-0.5 rounded text-[0.65rem] font-medium bg-sky-500/20 text-sky-400">{blockedAgents.length}</span>
      </div>
      {#if blockedAgents.length === 0}
        <div class="px-5 py-8 text-center text-sm text-slate-500">No User-Agent patterns configured.</div>
      {:else}
        <div class="divide-y divide-white/[0.04]">
          {#each blockedAgents as agent}
            <div class="px-5 py-3 flex items-center justify-between hover:bg-white/[0.02] transition group">
              <code class="text-sm text-slate-300 font-mono bg-white/[0.04] px-2 py-0.5 rounded">{agent}</code>
              {#if canManage}
                <button onclick={() => removeAgent(agent)} disabled={busy === `ra-${agent}`}
                  class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-0 group-hover:opacity-100 transition disabled:opacity-50" title="Remove">
                  {#if busy === `ra-${agent}`}<Loader2 size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                </button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Blocked IPs -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden mb-6">
      <div class="px-5 py-4 border-b border-white/[0.06] flex items-center gap-2">
        <Ban size={16} class="text-red-400" />
        <h3 class="text-sm font-semibold text-white">Blocked IP Addresses</h3>
        <span class="px-1.5 py-0.5 rounded text-[0.65rem] font-medium bg-red-500/20 text-red-400">{blockedIPs.length}</span>
      </div>
      {#if blockedIPs.length === 0}
        <div class="px-5 py-8 text-center text-sm text-slate-500">No IPs blocked.</div>
      {:else}
        <div class="divide-y divide-white/[0.04]">
          {#each blockedIPs as ip}
            <div class="px-5 py-3 flex items-center justify-between hover:bg-white/[0.02] transition group">
              <code class="text-sm text-slate-300 font-mono bg-white/[0.04] px-2 py-0.5 rounded">{ip}</code>
              {#if canManage}
                <button onclick={() => removeIP(ip)} disabled={busy === `ri-${ip}`}
                  class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-0 group-hover:opacity-100 transition disabled:opacity-50" title="Remove">
                  {#if busy === `ri-${ip}`}<Loader2 size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                </button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Raw Config -->
    {#if rawConfig}
      <details class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
        <summary class="px-5 py-4 cursor-pointer text-sm text-slate-400 hover:text-white transition flex items-center gap-2">
          <Globe size={14} /> Raw nginx config
        </summary>
        <div class="px-5 pb-5">
          <pre class="bg-black/30 border border-white/[0.06] rounded-lg p-4 text-xs text-slate-400 font-mono overflow-x-auto whitespace-pre-wrap">{rawConfig}</pre>
        </div>
      </details>
    {/if}
  {/if}
</DashboardLayout>
