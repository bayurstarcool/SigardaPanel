<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { fmtBytes } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import { Server, HardDrive, Database, ExternalLink, Clock, Cpu, Trash2, AlertTriangle, Activity } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let version = $state('—');
  let serverInfo = $state<any>(null);
  let diskUsed = $state(0);
  let diskTotal = $state(0);
  let loading = $state(true);
  let clearingCache = $state(false);
  let clearingAudit = $state(false);
  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));

  onMount(async () => {
    if (!auth.user) return;
    try {
      const [ver, metrics] = await Promise.allSettled([
        apiFetch(auth.token, 'GET', 'version'),
        (async () => {
          const srvs = await apiFetch(auth.token, 'GET', 'servers');
          const id = srvs?.servers?.[0]?.id;
          return id ? apiFetch(auth.token, 'GET', `servers/${id}/metrics/latest`) : null;
        })(),
      ]);
      if (ver.status === 'fulfilled' && ver.value) version = ver.value.version || '—';
      if (metrics.status === 'fulfilled' && metrics.value) {
        serverInfo = metrics.value;
        diskUsed = metrics.value.disk_used || 0;
        diskTotal = metrics.value.disk_total || 0;
      }
    } catch { /* ignore */ }
    loading = false;
  });

  function diskPct(): number { return diskTotal ? Math.round((diskUsed / diskTotal) * 100) : 0; }
  function uptimeStr(): string {
    if (!serverInfo?.uptime_seconds) return '—';
    const s = Math.floor(serverInfo.uptime_seconds);
    const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
    return d > 0 ? `${d}d ${h}h ${m}m` : h > 0 ? `${h}h ${m}m` : `${m}m`;
  }
  async function clearCache() {
    if (!auth.user) return; clearingCache = true;
    try { await apiFetch(auth.token, 'POST', 'settings/clear-cache'); toasts.success('Cache cleared'); }
    catch (e: any) { toasts.error(e.message || 'Failed to clear cache'); }
    finally { clearingCache = false; }
  }
  async function resetAuditLogs() {
    if (!auth.user) return; clearingAudit = true;
    try { await apiFetch(auth.token, 'DELETE', 'audit-logs'); toasts.success('Audit logs cleared'); }
    catch (e: any) { toasts.error(e.message || 'Failed to clear audit logs'); }
    finally { clearingAudit = false; }
  }
</script>

<DashboardLayout active="Settings">
  <div class="mb-8">
    <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Settings</h1>
    <p class="text-sm text-slate-500 mt-1">System configuration and management</p>
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    <!-- Backup Settings -->
    <a href="/backups" class="group bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 hover:border-sky-500/30 hover:bg-sky-500/[0.03] transition-all">
      <div class="flex items-start justify-between">
        <div>
          <h3 class="text-sm font-semibold text-slate-300 mb-2 flex items-center gap-2">
            <span class="w-6 h-6 rounded-lg bg-sky-500/10 flex items-center justify-center"><HardDrive size={14} /></span>
            Backup Settings
          </h3>
          <p class="text-xs text-slate-500 leading-relaxed">Configure retention, providers, schedules, and backup history.</p>
        </div>
        <ExternalLink size={14} class="text-slate-600 group-hover:text-sky-400 transition mt-1" />
      </div>
      <div class="mt-4 pt-3 border-t border-white/[0.05]">
        <span class="text-xs text-sky-400 group-hover:text-sky-300 transition">Manage Backups →</span>
      </div>
    </a>

    <!-- System Info -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
      <h3 class="text-sm font-semibold text-slate-300 mb-4 flex items-center gap-2">
        <span class="w-6 h-6 rounded-lg bg-purple-500/10 flex items-center justify-center"><Server size={14} /></span>
        System Info
      </h3>
      {#if loading}
        <div class="space-y-2.5">{#each [1,2,3] as _}<div class="h-7 bg-white/[0.03] rounded-lg animate-pulse"></div>{/each}</div>
      {:else}
        <div class="space-y-2 text-sm">
          <div class="flex justify-between py-1.5 border-b border-white/[0.03]">
            <span class="text-slate-500 flex items-center gap-1.5"><Activity size={12} /> Version</span>
            <code class="text-sky-300 text-xs font-mono">{version}</code>
          </div>
          <div class="flex justify-between py-1.5 border-b border-white/[0.03]">
            <span class="text-slate-500 flex items-center gap-1.5"><Clock size={12} /> Uptime</span>
            <span class="text-slate-300 text-xs">{uptimeStr()}</span>
          </div>
          <div class="flex justify-between py-1.5 last:border-0">
            <span class="text-slate-500 flex items-center gap-1.5"><Cpu size={12} /> Backend</span>
            <span class="text-slate-300 text-xs">Go + Echo + SQLite</span>
          </div>
        </div>
        {#if diskTotal > 0}
          <div class="mt-3 pt-3 border-t border-white/[0.05]">
            <div class="flex items-center justify-between text-xs mb-1.5">
              <span class="text-slate-500">Disk Usage</span>
              <span class="text-slate-400">{fmtBytes(diskUsed)} / {fmtBytes(diskTotal)} ({diskPct()}%)</span>
            </div>
            <div class="w-full h-1.5 bg-white/[0.05] rounded-full overflow-hidden">
              <div class="h-full rounded-full transition-all" class:bg-emerald-500={diskPct() < 70} class:bg-amber-500={diskPct() >= 70 && diskPct() < 90} class:bg-red-500={diskPct() >= 90} style="width: {diskPct()}%"></div>
            </div>
          </div>
        {/if}
      {/if}
    </div>

    <!-- Danger Zone (admin only) -->
    {#if isAdmin}
      <div class="bg-red-500/[0.03] border border-red-500/20 rounded-xl p-6">
        <h3 class="text-sm font-semibold text-red-400 mb-4 flex items-center gap-2">
          <span class="w-6 h-6 rounded-lg bg-red-500/10 flex items-center justify-center"><AlertTriangle size={14} /></span>
          Danger Zone
        </h3>
        <div class="space-y-3">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm text-slate-300">Clear Cache</p>
              <p class="text-xs text-slate-500">Flush all cached data and rebuild.</p>
            </div>
            <button onclick={clearCache} disabled={clearingCache} class="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-red-500/10 text-red-400 border border-red-400/20 hover:bg-red-500/20 disabled:opacity-50 transition">
              {#if clearingCache}<span class="w-3 h-3 border border-red-400/40 border-t-red-400 rounded-full animate-spin"></span>{:else}<Trash2 size={12} />{/if}
              Clear
            </button>
          </div>
          <div class="border-t border-red-500/10"></div>
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm text-slate-300">Reset Audit Logs</p>
              <p class="text-xs text-slate-500">Permanently delete all audit log entries.</p>
            </div>
            <button onclick={resetAuditLogs} disabled={clearingAudit} class="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-red-500/10 text-red-400 border border-red-400/20 hover:bg-red-500/20 disabled:opacity-50 transition">
              {#if clearingAudit}<span class="w-3 h-3 border border-red-400/40 border-t-red-400 rounded-full animate-spin"></span>{:else}<Database size={12} />{/if}
              Clear
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</DashboardLayout>
