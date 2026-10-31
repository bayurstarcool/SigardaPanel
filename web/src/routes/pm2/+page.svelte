<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Play, Square, RotateCcw, Trash2, ScrollText, RefreshCw, Save, Zap, Plus, Server, Layers, Activity, RotateCw, LoaderCircle, Search } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let processes = $state<any[]>([]);
  let loading = $state(false);
  let loadError = $state('');
  let actionPending = $state('');
  let query = $state('');
  let showLogs = $state(false);
  let logsProcess = $state('');
  let logs = $state('');
  let logsLoading = $state(false);
  let showStartForm = $state(false);
  let confirmOpen = $state(false);
  let confirmName = $state('');
  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

  // Start form fields
  let formAppName = $state('');
  let formRootPath = $state('');
  let formScript = $state('app.js');
  let formEnv = $state('production');
  let formInstances = $state(1);

  const statusOf = (p: any): string => p.pm2_env?.status ?? p.status ?? 'unknown';
  const restartsOf = (p: any): number => p.pm2_env?.restart_time ?? p.restarts ?? 0;

  const totalProcesses = $derived(processes.length);
  const runningProcesses = $derived(processes.filter((p) => statusOf(p) === 'online').length);
  const erroredProcesses = $derived(processes.filter((p) => statusOf(p) === 'errored').length);
  const totalRestarts = $derived(processes.reduce((sum: number, p: any) => sum + (Number(restartsOf(p)) || 0), 0));
  const visible = $derived(processes.filter((p) => !query || String(p.name).toLowerCase().includes(query.toLowerCase())));

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load servers');
    }
    serversLoaded = true;
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadProcesses();
    }
  });

  function loadProcesses() {
    if (!selectedServer) return;
    loading = true;
    loadError = '';
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'list' })
      .then((d: any) => {
        const raw = d?.data?.output ?? d?.output ?? '';
        try {
          const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw;
          processes = Array.isArray(parsed) ? parsed : [];
        } catch {
          // Fall back to parsing PM2's table output
          processes = parsePM2Table(raw);
        }
      })
      .catch((e: any) => {
        processes = [];
        if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load processes';
      })
      .finally(() => (loading = false));
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
          _raw: { uptime: parts[6] || '', mem: parts[10] || '' },
        });
      }
    }
    return procs;
  }

  async function pm2Action(appName: string, action: string) {
    actionPending = `${appName}:${action}`;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action, app_name: appName });
      toasts.success(`${appName}: ${action} done`);
      loadProcesses();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || `Failed to ${action} ${appName}`);
    }
    actionPending = '';
  }

  function askDelete(name: string) {
    confirmName = name;
    confirmOpen = true;
  }

  async function viewLogs(appName: string) {
    logsProcess = appName;
    showLogs = true;
    logsLoading = true;
    logs = '';
    try {
      const d: any = await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action: 'logs', app_name: appName });
      logs = d?.data?.output ?? d?.output ?? 'No logs available';
    } catch {
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
        instances: formInstances,
      });
      toasts.success(`Started ${formAppName}`);
      showStartForm = false;
      formAppName = '';
      formRootPath = '';
      formScript = 'app.js';
      formEnv = 'production';
      formInstances = 1;
      loadProcesses();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to start app');
    }
    actionPending = '';
  }

  async function simpleAction(action: 'save' | 'startup', okMsg: string) {
    actionPending = action;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/pm2/manage`, { action });
      toasts.success(okMsg);
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Action failed');
    }
    actionPending = '';
  }

  function statusBadge(status: string) {
    if (status === 'online') return 'badge-success';
    if (status === 'errored') return 'badge-danger';
    if (status === 'stopped') return 'badge-danger';
    if (status === 'stopping' || status === 'launching') return 'badge-warning';
    return '';
  }

  function formatMemory(bytes: number) {
    if (!bytes) return '—';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
    return (bytes / 1073741824).toFixed(2) + ' GB';
  }
</script>

<DashboardLayout active="PM2">
  <PageHeader title="PM2 processes" description={loading ? 'Loading…' : `${totalProcesses} process(es) · ${runningProcesses} online`}>
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={loadProcesses} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      <button onclick={loadProcesses} class="btn btn-secondary btn-sm" disabled={loading || !selectedServer}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if canManage}
        <button onclick={() => simpleAction('save', 'Process list saved')} disabled={!!actionPending || !selectedServer} class="btn btn-secondary btn-sm" title="pm2 save">
          {#if actionPending === 'save'}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save
        </button>
        <button onclick={() => simpleAction('startup', 'Startup script configured')} disabled={!!actionPending || !selectedServer} class="btn btn-secondary btn-sm" title="pm2 startup">
          {#if actionPending === 'startup'}<LoaderCircle size={14} class="animate-spin" />{:else}<Server size={14} />{/if} Startup
        </button>
        <button onclick={() => (showStartForm = true)} class="btn btn-primary btn-sm" disabled={!selectedServer}><Plus size={14} /> Start app</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — PM2 is managed through its agent.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="Processes" value={loading ? '–' : totalProcesses} icon={Layers} />
      <StatCard label="Online" value={loading ? '–' : runningProcesses} icon={Activity} tone="success" />
      <StatCard label="Errored" value={loading ? '–' : erroredProcesses} icon={Zap} tone={erroredProcesses > 0 ? 'danger' : ''} />
      <StatCard label="Restarts" value={loading ? '–' : totalRestarts} icon={RotateCw} tone={totalRestarts > 0 ? 'warning' : ''} />
    </div>

    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 class="text-sm font-semibold text-fg">Processes</h2>
        <div class="relative w-full sm:w-64">
          <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
          <input bind:value={query} class="input h-8 pl-8" placeholder="Filter processes…" aria-label="Filter processes" />
        </div>
      </div>

      {#if loading && processes.length === 0}
        <div class="space-y-3 p-4">{#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
      {:else if loadError}
        <EmptyState compact icon={Layers} title="Couldn't load PM2 processes" description={loadError} actionLabel="Retry" onaction={loadProcesses} />
      {:else if visible.length === 0}
        <EmptyState compact icon={Layers} title={processes.length ? 'No processes match' : 'No PM2 processes'} description={processes.length ? 'Try another filter.' : 'Start an application to see it here.'} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Process</th>
                <th>Status</th>
                <th class="text-right max-sm:hidden">CPU</th>
                <th class="text-right max-sm:hidden">Memory</th>
                <th class="text-right max-md:hidden">Uptime</th>
                <th class="text-right max-md:hidden">Restarts</th>
                <th class="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each visible as proc, i (proc.pm_id ?? `${proc.name}:${i}`)}
                {@const status = statusOf(proc)}
                {@const restarts = restartsOf(proc)}
                <tr>
                  <td>
                    <div class="font-medium text-fg">{proc.name}</div>
                    <div class="text-xs text-subtle tabular-nums">id {proc.pm_id} · pid {proc.pid || '—'}</div>
                  </td>
                  <td><span class="badge {statusBadge(status)}"><span class="dot"></span>{status}</span></td>
                  <td class="text-right text-muted tabular-nums max-sm:hidden">{proc.monit?.cpu ?? 0}%</td>
                  <td class="text-right text-muted tabular-nums max-sm:hidden">{proc._raw?.mem || formatMemory(proc.monit?.memory)}</td>
                  <td class="text-right text-muted tabular-nums max-md:hidden">{proc._raw?.uptime || '—'}</td>
                  <td class="text-right tabular-nums max-md:hidden {restarts > 0 ? 'text-warning' : 'text-muted'}">{restarts}</td>
                  <td>
                    <div class="flex items-center justify-end gap-0.5">
                      {#if canManage}
                        {#if status !== 'online'}
                          <button onclick={() => pm2Action(proc.name, 'start')} disabled={actionPending === `${proc.name}:start`} class="btn btn-ghost btn-icon btn-xs text-success" aria-label="Start" title="Start">
                            {#if actionPending === `${proc.name}:start`}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if}
                          </button>
                        {:else}
                          <button onclick={() => pm2Action(proc.name, 'stop')} disabled={actionPending === `${proc.name}:stop`} class="btn btn-ghost btn-icon btn-xs" aria-label="Stop" title="Stop">
                            {#if actionPending === `${proc.name}:stop`}<LoaderCircle size={14} class="animate-spin" />{:else}<Square size={14} />{/if}
                          </button>
                        {/if}
                        <button onclick={() => pm2Action(proc.name, 'restart')} disabled={actionPending === `${proc.name}:restart`} class="btn btn-ghost btn-icon btn-xs" aria-label="Restart" title="Restart">
                          {#if actionPending === `${proc.name}:restart`}<LoaderCircle size={14} class="animate-spin" />{:else}<RotateCcw size={14} />{/if}
                        </button>
                      {/if}
                      <button onclick={() => viewLogs(proc.name)} class="btn btn-ghost btn-icon btn-xs" aria-label="Logs" title="Logs"><ScrollText size={14} /></button>
                      {#if canManage}
                        <button onclick={() => askDelete(proc.name)} disabled={actionPending === `${proc.name}:delete`} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete" title="Delete">
                          {#if actionPending === `${proc.name}:delete`}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                        </button>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <Modal bind:open={showStartForm} title="Start application" maxWidth="lg">
    <form id="pm2-start" class="grid gap-4 sm:grid-cols-2" onsubmit={(e) => { e.preventDefault(); startApp(); }}>
      <div>
        <label class="label" for="pm2-name">App name <span class="text-danger">*</span></label>
        <input id="pm2-name" bind:value={formAppName} placeholder="my-app" class="input" />
      </div>
      <div>
        <label class="label" for="pm2-script">Script <span class="text-danger">*</span></label>
        <input id="pm2-script" bind:value={formScript} placeholder="app.js" class="input font-mono text-[13px]" />
      </div>
      <div class="sm:col-span-2">
        <label class="label" for="pm2-root">Root path <span class="text-danger">*</span></label>
        <input id="pm2-root" bind:value={formRootPath} placeholder="/var/www/my-app" class="input font-mono text-[13px]" />
      </div>
      <div>
        <label class="label" for="pm2-env">Environment</label>
        <select id="pm2-env" bind:value={formEnv} class="input">
          <option value="production">production</option>
          <option value="development">development</option>
          <option value="staging">staging</option>
        </select>
      </div>
      <div>
        <label class="label" for="pm2-inst">Instances</label>
        <input id="pm2-inst" bind:value={formInstances} type="number" min="1" max="16" class="input tabular-nums" />
      </div>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showStartForm = false)}>Cancel</button>
      <button type="submit" form="pm2-start" class="btn btn-primary btn-sm" disabled={!formAppName || !formRootPath || !formScript || actionPending === 'start'}>
        {#if actionPending === 'start'}<LoaderCircle size={14} class="animate-spin" /> Starting…{:else}<Play size={14} /> Start{/if}
      </button>
    {/snippet}
  </Modal>

  <Modal bind:open={showLogs} title="Logs · {logsProcess}" maxWidth="4xl">
    {#if logsLoading}
      <div class="space-y-2">{#each Array(8) as _, i (i)}<div class="skeleton h-3.5" style="width: {[92, 78, 86, 64, 90, 72, 84, 58][i]}%"></div>{/each}</div>
    {:else}
      <pre class="max-h-[60vh] overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-muted">{logs}</pre>
    {/if}
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => viewLogs(logsProcess)} disabled={logsLoading}>
        <RefreshCw size={14} class={logsLoading ? 'animate-spin' : ''} /> Reload
      </button>
      <button class="btn btn-secondary btn-sm" onclick={() => (showLogs = false)}>Close</button>
    {/snippet}
  </Modal>

  <ConfirmDialog
    bind:open={confirmOpen}
    title="Delete process?"
    message={`Process "${confirmName}" will be removed from PM2. This cannot be undone.`}
    confirmLabel="Delete"
    destructive
    onconfirm={() => pm2Action(confirmName, 'delete')}
  />
</DashboardLayout>
