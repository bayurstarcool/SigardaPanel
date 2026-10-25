<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Shield, ShieldCheck, ShieldOff, Plus, Trash2, RefreshCw, Zap, Globe, Server, Database, Ban } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let status = $state<any>(null);
  let rules = $state<any[]>([]);
  let loading = $state(false);
  let showAddRule = $state(false);
  let showQuickBlock = $state(false);
  let showPresets = $state(false);
  let newPort = $state('');
  let newProtocol = $state('tcp');
  let newFrom = $state('');
  let newAction = $state('allow');
  let blockIP = $state('');
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  const presets = [
    { name: 'Web Server', icon: Globe, ports: ['80/tcp', '443/tcp'], desc: 'HTTP + HTTPS' },
    { name: 'SSH', icon: Server, ports: ['22/tcp'], desc: 'Secure Shell' },
    { name: 'Database', icon: Database, ports: ['3306/tcp', '5432/tcp'], desc: 'MySQL + PostgreSQL' },
    { name: 'Redis', icon: Zap, ports: ['6379/tcp'], desc: 'Redis cache' },
    { name: 'Node.js', icon: Server, ports: ['3000/tcp', '8080/tcp'], desc: 'Common Node ports' },
  ];

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadStatus();
      loadRules();
    }
  });

  function loadStatus() {
    if (!selectedServer) return;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/firewall/status`)
      .then((d: any) => { status = d; })
      .catch(() => {});
  }

  function loadRules() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'GET', `servers/${selectedServer}/firewall/rules`)
      .then((d: any) => { rules = d?.rules ?? []; })
      .catch(() => {})
      .finally(() => loading = false);
  }

  async function toggleFirewall(enable: boolean) {
    const action = enable ? 'enable' : 'disable';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/${action}`, {});
      loadStatus();
    } catch (e) {}
  }

  async function addRule() {
    if (!newPort) return;
    const endpoint = newAction === 'allow' ? 'allow' : 'deny';
    const body: any = { port: newPort, protocol: newProtocol };
    if (newFrom) body.from = newFrom;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/${endpoint}`, body);
      showAddRule = false;
      newPort = '';
      newFrom = '';
      loadRules();
    } catch (e) {}
  }

  async function applyPreset(preset: any) {
    for (const port of preset.ports) {
      const [p, proto] = port.split('/');
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/allow`, {
        port: p, protocol: proto || 'tcp', comment: preset.name
      });
    }
    showPresets = false;
    loadRules();
  }

  async function blockIPAddr() {
    if (!blockIP) return;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/deny`, {
        port: 'any', from: blockIP, comment: 'Blocked via panel'
      });
      showQuickBlock = false;
      blockIP = '';
      loadRules();
    } catch (e) {}
  }

  async function deleteRule(num: number) {
    if (!confirm(`Delete rule #${num}?`)) return;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/delete`, { number: num });
      loadRules();
    } catch (e) {}
  }

  async function resetFirewall() {
    if (!confirm('Reset firewall to defaults? This will remove ALL rules.')) return;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/firewall/reset`, {});
      loadStatus();
      loadRules();
    } catch (e) {}
  }
</script>

<DashboardLayout active="Firewall">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-orange-100 bg-clip-text text-transparent">Firewall (UFW)</h1>
      <p class="text-sm text-slate-500 mt-1">{status?.active ? '🟢 Active' : '🔴 Inactive'} • {rules.length} rule(s)</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => { loadStatus(); loadRules(); }}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => { loadStatus(); loadRules(); }} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} />
      </button>
    </div>
  </div>

  <!-- Status + Actions -->
  <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <div class="text-xs text-slate-500 mb-2">Firewall Status</div>
      <div class="flex items-center gap-3">
        <div class="w-3 h-3 rounded-full {status?.active ? 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.5)]' : 'bg-red-400'}"></div>
        <span class="text-lg font-semibold {status?.active ? 'text-emerald-400' : 'text-red-400'}">
          {status?.active ? 'Active' : 'Inactive'}
        </span>
      </div>
    </div>
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 flex items-center justify-between">
      <div>
        <div class="text-xs text-slate-500 mb-2">Quick Actions</div>
        <div class="flex gap-2">
          {#if status?.active}
            <button onclick={() => toggleFirewall(false)} class="px-3 py-1.5 bg-red-500/20 text-red-400 rounded-lg text-xs font-medium hover:bg-red-500/30 transition">Disable</button>
          {:else}
            <button onclick={() => toggleFirewall(true)} class="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-medium hover:bg-emerald-500/30 transition">Enable</button>
          {/if}
          <button onclick={() => resetFirewall()} class="px-3 py-1.5 bg-white/[0.05] text-slate-400 rounded-lg text-xs font-medium hover:bg-white/[0.1] transition">Reset</button>
        </div>
      </div>
    </div>
  </div>

  <!-- Action Buttons -->
  {#if canManage}
    <div class="flex flex-wrap items-center gap-3 mb-6">
      <button onclick={() => { showAddRule = !showAddRule; showPresets = false; showQuickBlock = false; }}
        class="px-4 py-2 bg-indigo-500/20 text-indigo-400 rounded-lg text-sm font-medium hover:bg-indigo-500/30 transition flex items-center gap-2">
        <Plus size={16} /> Add Rule
      </button>
      <button onclick={() => { showPresets = !showPresets; showAddRule = false; showQuickBlock = false; }}
        class="px-4 py-2 bg-sky-500/20 text-sky-400 rounded-lg text-sm font-medium hover:bg-sky-500/30 transition flex items-center gap-2">
        <Zap size={16} /> Presets
      </button>
      <button onclick={() => { showQuickBlock = !showQuickBlock; showAddRule = false; showPresets = false; }}
        class="px-4 py-2 bg-red-500/20 text-red-400 rounded-lg text-sm font-medium hover:bg-red-500/30 transition flex items-center gap-2">
        <Ban size={16} /> Block IP
      </button>
    </div>

    {#if showAddRule}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 mb-6">
        <h3 class="text-sm font-semibold text-white mb-3">Add Firewall Rule</h3>
        <div class="grid grid-cols-1 sm:grid-cols-4 gap-3">
          <input bind:value={newPort} placeholder="Port (e.g. 443)" class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500" />
          <select bind:value={newProtocol} class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
          <input bind:value={newFrom} placeholder="From IP (optional)" class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500" />
          <div class="flex gap-2">
            <select bind:value={newAction} class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white flex-1">
              <option value="allow">Allow</option>
              <option value="deny">Deny</option>
            </select>
            <button onclick={addRule} class="px-4 py-2 bg-indigo-500 text-white rounded-lg text-sm font-medium hover:bg-indigo-600 transition">Save</button>
          </div>
        </div>
      </div>
    {/if}

    {#if showPresets}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 mb-6">
        <h3 class="text-sm font-semibold text-white mb-3">Quick Presets</h3>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {#each presets as preset}
            {@const Icon = preset.icon}
            <button onclick={() => applyPreset(preset)}
              class="flex items-center gap-3 p-3 bg-white/[0.03] border border-white/[0.06] rounded-lg hover:bg-white/[0.06] hover:border-white/[0.12] transition text-left">
              <div class="w-10 h-10 rounded-lg bg-sky-500/10 flex items-center justify-center">
                <Icon size={18} class="text-sky-400" />
              </div>
              <div>
                <div class="text-sm font-medium text-white">{preset.name}</div>
                <div class="text-[0.65rem] text-slate-500">{preset.ports.join(', ')}</div>
              </div>
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#if showQuickBlock}
      <div class="bg-red-500/5 border border-red-500/20 rounded-xl p-5 mb-6">
        <h3 class="text-sm font-semibold text-red-300 mb-3">Block IP Address</h3>
        <div class="flex gap-3">
          <input bind:value={blockIP} placeholder="IP address (e.g. 1.2.3.4)" class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500" />
          <button onclick={blockIPAddr} class="px-4 py-2 bg-red-500 text-white rounded-lg text-sm font-medium hover:bg-red-600 transition flex items-center gap-2">
            <Ban size={14} /> Block
          </button>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Rules Table -->
  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-[var(--text-muted)]">Loading rules...</div>
    </div>
  {:else if rules.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <Shield size={48} class="mb-4 opacity-30" />
      <h2 class="text-lg font-semibold mb-2">No Rules</h2>
      <p class="text-sm text-slate-500">No firewall rules configured.</p>
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="px-5 py-4 border-b border-white/[0.06] flex items-center justify-between">
        <h3 class="text-sm font-semibold text-white">Firewall Rules ({rules.length})</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead><tr class="text-left">
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">#</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Port</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Proto</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Action</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Direction</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">From</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">To</th>
            <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Comment</th>
            {#if canManage}
              <th class="px-4 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
            {/if}
          </tr></thead>
          <tbody>
            {#each rules as r}
              <tr class="hover:bg-white/[0.02] transition group">
                <td class="px-4 py-3 border-b border-white/[0.04] text-sm text-slate-400 font-mono">{r.number}</td>
                <td class="px-4 py-3 border-b border-white/[0.04] text-sm text-white font-mono">{r.port || '—'}</td>
                <td class="px-4 py-3 border-b border-white/[0.04]">
                  <span class="px-1.5 py-0.5 rounded text-[0.65rem] font-medium bg-white/[0.05] text-slate-400">{r.protocol || 'any'}</span>
                </td>
                <td class="px-4 py-3 border-b border-white/[0.04]">
                  <span class="px-2 py-0.5 rounded text-xs font-medium {r.action === 'ALLOW' ? 'bg-emerald-500/20 text-emerald-400' : r.action === 'DENY' ? 'bg-red-500/20 text-red-400' : 'bg-amber-500/20 text-amber-400'}">
                    {r.action}
                  </span>
                </td>
                <td class="px-4 py-3 border-b border-white/[0.04]">
                  <span class="text-xs {r.direction === 'IN' ? 'text-sky-400' : r.direction === 'OUT' ? 'text-violet-400' : 'text-slate-500'}">
                    {r.direction || '—'}
                  </span>
                </td>
                <td class="px-4 py-3 border-b border-white/[0.04] text-sm text-slate-300">{r.from || 'Anywhere'}</td>
                <td class="px-4 py-3 border-b border-white/[0.04] text-sm text-slate-300">{r.to || 'Anywhere'}</td>
                <td class="px-4 py-3 border-b border-white/[0.04] text-xs text-slate-500 max-w-[150px] truncate">{r.comment || '—'}</td>
                {#if canManage}
                  <td class="px-4 py-3 border-b border-white/[0.04] text-right">
                    <button onclick={() => deleteRule(r.number)} class="p-1.5 rounded hover:bg-red-500/20 text-red-400 opacity-60 group-hover:opacity-100 transition" title="Delete">
                      <Trash2 size={14} />
                    </button>
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</DashboardLayout>
