<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Cpu, Play, Square, RotateCcw, Trash2, Terminal, RefreshCw, Save, Zap, Plus, X, Server, Layers, Activity, RotateCw, FileText, Loader2 } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let processes = $state<any[]>([]);
  let loading = $state(false);
  let actionPending = $state('');
  let showLogs = $state(false);
  let logsProcess = $state('');
  let logs = $state('');
  let logsLoading = $state(false);
  let showStartForm = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  // Start form fields
  let formAppName = $state('');
  let formRootPath = $state('');
  let formScript = $state('app.js');
  let formEnv = $state('production');
  let formInstances = $state(1);

  // Summary stats
  let totalProcesses = $derived(processes.length);
  let runningProcesses = $derived(processes.filter((p: any) => p.status === 'online').length);
  let erroredProcesses = $derived(processes.filter((p: any) => p.status === 'errored').length);
  let totalRestarts = $derived(processes.reduce((sum: number, p: any) => sum + (p.restarts || 0), 0));

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadProcesses();
    }
  });

  function loadProcesses() {
    if (!selectedServer) return;
    loading = true;
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'list' })
      .then((d: any) => {
        const raw = d?.data?.output ?? d?.output ?? '';
        try {
          const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw;
          processes = Array.isArray(parsed) ? parsed : [];
        } catch {
          // Try parsing PM2 table output
          processes = parsePM2Table(raw);
        }
      })
      .catch(() => { processes = []; })
      .finally(() => loading = false);
  }

  function parsePM2Table(output: string): any[] {
    const lines = output.split('\n').filter((l: string) => l.includes('│'));
    const procs: any[] = [];
    for (const line of lines) {
      const parts = line.split('│').map((p: string) => p.trim()).filter(Boolean);
      if (parts.length >= 6 && parts[0] !== 'id' && parts[0] !== '─') {
        procs.push({
          name: parts[1] || '',
          pm_id: parts[0],
          pid: parts[5] || '',
          pm2_env: { status: parts[8]?.toLowerCase() || 'unknown', restart_time: parseInt(parts[7]) || 0, pm_uptime: 0 },
          monit: { cpu: parseFloat(parts[9]) || 0, memory: 0 },
          _raw: { uptime: parts[6] || '', mem: parts[10] || '' }
        });
      }
    }
    return procs;
  }

  async function pm2Action(appName: string, action: string) {
    if (action === 'delete' && !confirm(`Delete process "${appName}"? This cannot be undone.`)) return;
    actionPending = `${appName}:${action}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action, app_name: appName });
      loadProcesses();
    } catch (e) {}
    actionPending = '';
  }

  async function viewLogs(appName: string) {
    logsProcess = appName;
    showLogs = true;
    logsLoading = true;
    logs = '';
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'logs', app_name: appName });
      logs = d?.data?.output ?? d?.output ?? 'No logs available';
    } catch (e) {
      logs = 'Failed to load logs';
    }
    logsLoading = false;
  }

  async function startApp() {
    if (!formAppName || !formRootPath || !formScript) return;
    actionPending = 'start';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, {
        action: 'start',
        app_name: formAppName,
        root_path: formRootPath,
        script: formScript,
        env: formEnv,
        instances: formInstances
      });
      showStartForm = false;
      formAppName = '';
      formRootPath = '';
      formScript = 'app.js';
      formEnv = 'production';
      formInstances = 1;
      loadProcesses();
    } catch (e) {}
    actionPending = '';
  }

  async function saveProcessList() {
    actionPending = 'save';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'save' });
    } catch (e) {}
    actionPending = '';
  }

  async function setupStartup() {
    actionPending = 'startup';
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'startup' });
    } catch (e) {}
    actionPending = '';
  }

  function statusColor(status: string) {
    if (status === 'online') return 'bg-emerald-500/20 text-emerald-400';
    if (status === 'stopped') return 'bg-red-500/20 text-red-400';
    if (status === 'errored') return 'bg-red-500/20 text-red-400';
    if (status === 'stopping') return 'bg-yellow-500/20 text-yellow-400';
    if (status === 'launching') return 'bg-sky-500/20 text-sky-400';
    return 'bg-white/[0.05] text-slate-400';
  }

  function statusDot(status: string) {
    if (status === 'online') return 'bg-emerald-400 shadow-[0_0_6px_rgba(74,222,128,0.4)]';
    if (status === 'errored') return 'bg-red-400 shadow-[0_0_6px_rgba(248,113,113,0.4)]';
    if (status === 'stopped') return 'bg-red-400';
    return 'bg-yellow-400';
  }

  function formatMemory(bytes: number) {
    if (!bytes) return '-';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
    return (bytes / 1073741824).toFixed(2) + ' GB';
  }
</script>

<DashboardLayout active="PM2">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-emerald-100 bg-clip-text text-transparent">PM2 Process Manager</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${totalProcesses} process(es) • ${runningProcesses} online`}</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => loadProcesses()}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => loadProcesses()} class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition">
        <RefreshCw size={16} class={loading ? 'animate-spin' : ''} />
      </button>
    </div>
  </div>

  <!-- Summary Cards -->
  <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
      <div class="flex items-center gap-2 mb-2">
        <div class="w-8 h-8 rounded-lg bg-sky-500/20 flex items-center justify-center">
          <Layers size={14} class="text-sky-400" />
        </div>
        <span class="text-xs text-slate-500">Total</span>
      </div>
      <div class="text-2xl font-bold text-white">{totalProcesses}</div>
    </div>
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
      <div class="flex items-center gap-2 mb-2">
        <div class="w-8 h-8 rounded-lg bg-emerald-500/20 flex items-center justify-center">
          <Activity size={14} class="text-emerald-400" />
        </div>
        <span class="text-xs text-slate-500">Online</span>
      </div>
      <div class="text-2xl font-bold text-emerald-400">{runningProcesses}</div>
    </div>
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
      <div class="flex items-center gap-2 mb-2">
        <div class="w-8 h-8 rounded-lg bg-red-500/20 flex items-center justify-center">
          <Zap size={14} class="text-red-400" />
        </div>
        <span class="text-xs text-slate-500">Errored</span>
      </div>
      <div class="text-2xl font-bold {erroredProcesses > 0 ? 'text-red-400' : 'text-slate-400'}">{erroredProcesses}</div>
    </div>
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4">
      <div class="flex items-center gap-2 mb-2">
        <div class="w-8 h-8 rounded-lg bg-yellow-500/20 flex items-center justify-center">
          <RotateCw size={14} class="text-yellow-400" />
        </div>
        <span class="text-xs text-slate-500">Restarts</span>
      </div>
      <div class="text-2xl font-bold text-yellow-400">{totalRestarts}</div>
    </div>
  </div>

  <!-- Action Buttons -->
  {#if canManage}
    <div class="flex flex-wrap items-center gap-3 mb-6">
      <button onclick={() => { showStartForm = !showStartForm; }}
        class="px-4 py-2 bg-emerald-500/20 text-emerald-400 rounded-lg text-sm font-medium hover:bg-emerald-500/30 transition flex items-center gap-2">
        <Plus size={16} /> Start App
      </button>
      <button onclick={saveProcessList} disabled={actionPending === 'save'}
        class="px-4 py-2 bg-sky-500/20 text-sky-400 rounded-lg text-sm font-medium hover:bg-sky-500/30 transition flex items-center gap-2 disabled:opacity-50">
        <Save size={16} /> {actionPending === 'save' ? 'Saving...' : 'Save'}
      </button>
      <button onclick={setupStartup} disabled={actionPending === 'startup'}
        class="px-4 py-2 bg-indigo-500/20 text-indigo-400 rounded-lg text-sm font-medium hover:bg-indigo-500/30 transition flex items-center gap-2 disabled:opacity-50">
        <Server size={16} /> {actionPending === 'startup' ? 'Setting up...' : 'Startup'}
      </button>
    </div>
  {/if}

  <!-- Start App Form -->
  {#if showStartForm}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5 mb-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-sm font-semibold text-white">Start New Application</h3>
        <button onclick={() => showStartForm = false} class="text-slate-500 hover:text-white transition">
          <X size={16} />
        </button>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <div>
          <label class="text-xs text-slate-400 mb-1 block">App Name *</label>
          <input bind:value={formAppName} placeholder="my-app"
            class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:border-sky-500/50 focus:outline-none transition" />
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1 block">Root Path *</label>
          <input bind:value={formRootPath} placeholder="/var/www/my-app"
            class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:border-sky-500/50 focus:outline-none transition" />
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1 block">Script *</label>
          <input bind:value={formScript} placeholder="app.js"
            class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:border-sky-500/50 focus:outline-none transition" />
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1 block">Environment</label>
          <select bind:value={formEnv}
            class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white focus:border-sky-500/50 focus:outline-none transition">
            <option value="production">production</option>
            <option value="development">development</option>
            <option value="staging">staging</option>
          </select>
        </div>
        <div>
          <label class="text-xs text-slate-400 mb-1 block">Instances</label>
          <input bind:value={formInstances} type="number" min="1" max="16"
            class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-500 focus:border-sky-500/50 focus:outline-none transition" />
        </div>
        <div class="flex items-end">
          <button onclick={startApp} disabled={!formAppName || !formRootPath || !formScript || actionPending === 'start'}
            class="w-full px-4 py-2 bg-emerald-500 text-white rounded-lg text-sm font-medium hover:bg-emerald-600 transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2">
            <Play size={16} />
            {actionPending === 'start' ? 'Starting...' : 'Start Application'}
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Process Table -->
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
    {#if loading}
      <div class="flex items-center justify-center py-16">
        <RefreshCw size={24} class="animate-spin text-sky-400" />
        <span class="ml-3 text-sm text-slate-400">Loading processes...</span>
      </div>
    {:else if processes.length === 0}
      <div class="flex flex-col items-center justify-center py-16">
        <div class="w-16 h-16 rounded-2xl bg-white/[0.03] flex items-center justify-center mb-4">
          <Layers size={28} class="text-slate-600" />
        </div>
        <p class="text-sm text-slate-400 mb-1">No PM2 processes found</p>
        <p class="text-xs text-slate-600">Start a new application to get started</p>
      </div>
    {:else}
      <!-- Desktop Table -->
      <div class="hidden lg:block overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="border-b border-white/[0.07]">
              <th class="text-left text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Process</th>
              <th class="text-left text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Status</th>
              <th class="text-right text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">CPU</th>
              <th class="text-right text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Memory</th>
              <th class="text-right text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Uptime</th>
              <th class="text-right text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Restarts</th>
              <th class="text-right text-xs font-medium text-slate-500 uppercase tracking-wider px-5 py-3">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-white/[0.05]">
            {#each processes as proc}
              <tr class="hover:bg-white/[0.02] transition">
                <td class="px-5 py-3.5">
                  <div class="flex items-center gap-3">
                    <div class="w-2 h-2 rounded-full {statusDot(proc.pm2_env?.status)}"></div>
                    <div>
                      <div class="text-sm font-medium text-white">{proc.name}</div>
                      <div class="text-xs text-slate-500">ID: {proc.pm_id} • PID: {proc.pid}</div>
                    </div>
                  </div>
                </td>
                <td class="px-5 py-3.5">
                  <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium {statusColor(proc.pm2_env?.status)}">
                    {proc.pm2_env?.status || 'unknown'}
                  </span>
                </td>
                <td class="px-5 py-3.5 text-right">
                  <span class="text-sm text-slate-300">{proc.monit?.cpu ?? 0}%</span>
                </td>
                <td class="px-5 py-3.5 text-right">
                  <span class="text-sm text-slate-300">{proc._raw?.mem || formatMemory(proc.monit?.memory)}</span>
                </td>
                <td class="px-5 py-3.5 text-right">
                  <span class="text-sm text-slate-300">{proc._raw?.uptime || '-'}</span>
                </td>
                <td class="px-5 py-3.5 text-right">
                  <span class="text-sm {(proc.pm2_env?.restart_time ?? 0) > 0 ? 'text-yellow-400' : 'text-slate-400'}">{proc.pm2_env?.restart_time ?? 0}</span>
                </td>
                <td class="px-5 py-3.5">
                  <div class="flex items-center justify-end gap-1.5">
                    {#if proc.pm2_env?.status !== 'online'}
                      <button onclick={() => pm2Action(proc.name, 'start')} disabled={actionPending === `${proc.name}:start`}
                        class="p-1.5 rounded-lg text-emerald-400 hover:bg-emerald-500/20 transition disabled:opacity-50" title="Start">
                        <Play size={14} />
                      </button>
                    {:else}
                      <button onclick={() => pm2Action(proc.name, 'stop')} disabled={actionPending === `${proc.name}:stop`}
                        class="p-1.5 rounded-lg text-red-400 hover:bg-red-500/20 transition disabled:opacity-50" title="Stop">
                        <Square size={14} />
                      </button>
                    {/if}
                    <button onclick={() => pm2Action(proc.name, 'restart')} disabled={actionPending === `${proc.name}:restart`}
                      class="p-1.5 rounded-lg text-sky-400 hover:bg-sky-500/20 transition disabled:opacity-50" title="Restart">
                      <RotateCcw size={14} />
                    </button>
                    <button onclick={() => viewLogs(proc.name)}
                      class="p-1.5 rounded-lg text-slate-400 hover:bg-white/[0.1] transition" title="Logs">
                      <Terminal size={14} />
                    </button>
                    <button onclick={() => pm2Action(proc.name, 'delete')} disabled={actionPending === `${proc.name}:delete`}
                      class="p-1.5 rounded-lg text-red-400 hover:bg-red-500/20 transition disabled:opacity-50" title="Delete">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Mobile Cards -->
      <div class="lg:hidden divide-y divide-white/[0.05]">
        {#each processes as proc}
          <div class="p-4">
            <div class="flex items-center justify-between mb-3">
              <div class="flex items-center gap-2">
                <div class="w-2 h-2 rounded-full {statusDot(proc.pm2_env?.status)}"></div>
                <span class="text-sm font-medium text-white">{proc.name}</span>
                <span class="text-xs text-slate-500">#{proc.pm_id}</span>
              </div>
              <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {statusColor(proc.pm2_env?.status)}">
                {proc.pm2_env?.status || 'unknown'}
              </span>
            </div>
            <div class="grid grid-cols-4 gap-2 mb-3">
              <div class="text-center">
                <div class="text-xs text-slate-500">CPU</div>
                <div class="text-sm text-white">{proc.monit?.cpu ?? 0}%</div>
              </div>
              <div class="text-center">
                <div class="text-xs text-slate-500">RAM</div>
                <div class="text-sm text-white">{proc._raw?.mem || formatMemory(proc.monit?.memory)}</div>
              </div>
              <div class="text-center">
                <div class="text-xs text-slate-500">Uptime</div>
                <div class="text-sm text-white">{proc._raw?.uptime || '-'}</div>
              </div>
              <div class="text-center">
                <div class="text-xs text-slate-500">Restarts</div>
                <div class="text-sm {(proc.pm2_env?.restart_time ?? 0) > 0 ? 'text-yellow-400' : 'text-white'}">{proc.pm2_env?.restart_time ?? 0}</div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              {#if proc.pm2_env?.status !== 'online'}
                <button onclick={() => pm2Action(proc.name, 'start')} class="flex-1 px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-medium hover:bg-emerald-500/30 transition flex items-center justify-center gap-1">
                  <Play size={12} /> Start
                </button>
              {:else}
                <button onclick={() => pm2Action(proc.name, 'stop')} class="flex-1 px-3 py-1.5 bg-red-500/20 text-red-400 rounded-lg text-xs font-medium hover:bg-red-500/30 transition flex items-center justify-center gap-1">
                  <Square size={12} /> Stop
                </button>
              {/if}
              <button onclick={() => pm2Action(proc.name, 'restart')} class="flex-1 px-3 py-1.5 bg-sky-500/20 text-sky-400 rounded-lg text-xs font-medium hover:bg-sky-500/30 transition flex items-center justify-center gap-1">
                <RotateCcw size={12} /> Restart
              </button>
              <button onclick={() => viewLogs(proc.name)} class="px-3 py-1.5 bg-white/[0.05] text-slate-400 rounded-lg text-xs font-medium hover:bg-white/[0.1] transition">
                <Terminal size={12} />
              </button>
              <button onclick={() => pm2Action(proc.name, 'delete')} class="px-3 py-1.5 bg-red-500/20 text-red-400 rounded-lg text-xs font-medium hover:bg-red-500/30 transition">
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Logs Modal -->
  {#if showLogs}
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" onclick={() => showLogs = false}>
      <div class="w-full max-w-3xl max-h-[80vh] bg-[#0a1628] border border-white/[0.1] rounded-xl shadow-2xl flex flex-col" onclick={(e) => e.stopPropagation()}>
        <div class="flex items-center justify-between px-5 py-4 border-b border-white/[0.07]">
          <div class="flex items-center gap-2">
            <Terminal size={16} class="text-sky-400" />
            <span class="text-sm font-semibold text-white">Logs: {logsProcess}</span>
          </div>
          <button onclick={() => showLogs = false} class="text-slate-500 hover:text-white transition">
            <X size={18} />
          </button>
        </div>
        <div class="flex-1 overflow-auto p-4">
          {#if logsLoading}
            <div class="flex items-center justify-center py-8">
              <RefreshCw size={20} class="animate-spin text-sky-400" />
              <span class="ml-2 text-sm text-slate-400">Loading logs...</span>
            </div>
          {:else}
            <pre class="text-xs text-slate-300 font-mono whitespace-pre-wrap break-all leading-relaxed">{logs}</pre>
          {/if}
        </div>
        <div class="px-5 py-3 border-t border-white/[0.07] flex justify-end">
          <button onclick={() => viewLogs(logsProcess)} class="px-3 py-1.5 bg-white/[0.05] text-slate-400 rounded-lg text-xs font-medium hover:bg-white/[0.1] transition flex items-center gap-1">
            <RefreshCw size={12} /> Refresh
          </button>
        </div>
      </div>
    </div>
  {/if}
</DashboardLayout>
