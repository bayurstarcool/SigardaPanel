<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { fmtBytes } from '$lib/format';
  import { Server, HardDrive, Database, ChevronRight, Clock, Cpu, Trash2, AlertTriangle, Activity, KeyRound } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let version = $state('—');
  let serverInfo = $state<any>(null);
  let diskUsed = $state(0);
  let diskTotal = $state(0);
  let loading = $state(true);
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
  // "Clear cache" (POST settings/clear-cache) and "Reset audit logs" (DELETE audit-logs)
  // used to live here, but the API exposes neither endpoint — the controls are shown
  // disabled until the backend implements them.
</script>

<DashboardLayout active="Settings">
  <PageHeader title="Settings" description="System configuration and management" />

  <div class="grid gap-4 md:grid-cols-2">
    <!-- Shortcuts -->
    <div class="card divide-y divide-line">
      <a href="/backups" class="group flex items-center gap-3 p-5 transition-colors duration-150 hover:bg-surface-2">
        <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent"><HardDrive size={16} /></span>
        <span class="min-w-0 flex-1">
          <span class="block text-sm font-semibold text-fg">Backup settings</span>
          <span class="block text-[13px] text-muted">Configure retention, providers, schedules, and backup history.</span>
        </span>
        <ChevronRight size={16} class="shrink-0 text-subtle transition-colors group-hover:text-fg" />
      </a>
      <a href="/settings/twofa" class="group flex items-center gap-3 p-5 transition-colors duration-150 hover:bg-surface-2">
        <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent"><KeyRound size={16} /></span>
        <span class="min-w-0 flex-1">
          <span class="block text-sm font-semibold text-fg">Two-factor authentication</span>
          <span class="block text-[13px] text-muted">Protect your account with an authenticator app.</span>
        </span>
        <ChevronRight size={16} class="shrink-0 text-subtle transition-colors group-hover:text-fg" />
      </a>
    </div>

    <!-- System info -->
    <section class="card">
      <div class="card-header">
        <h2 class="card-title flex items-center gap-2"><Server size={16} class="text-subtle" /> System info</h2>
      </div>
      <div class="card-body">
        {#if loading}
          <div class="space-y-2.5">{#each [1, 2, 3] as i (i)}<div class="skeleton h-7"></div>{/each}</div>
        {:else}
          <dl class="divide-y divide-line text-sm">
            <div class="flex items-center justify-between py-2 first:pt-0">
              <dt class="flex items-center gap-1.5 text-muted"><Activity size={14} /> Version</dt>
              <dd><code class="code">{version}</code></dd>
            </div>
            <div class="flex items-center justify-between py-2">
              <dt class="flex items-center gap-1.5 text-muted"><Clock size={14} /> Uptime</dt>
              <dd class="text-fg tabular-nums">{uptimeStr()}</dd>
            </div>
            <div class="flex items-center justify-between py-2 last:pb-0">
              <dt class="flex items-center gap-1.5 text-muted"><Cpu size={14} /> Backend</dt>
              <dd class="text-fg">Go + Echo + SQLite</dd>
            </div>
          </dl>
          {#if diskTotal > 0}
            {@const pct = diskPct()}
            <div class="mt-4 border-t border-line pt-4">
              <div class="mb-1.5 flex items-center justify-between text-xs">
                <span class="text-subtle">Disk usage</span>
                <span class="text-muted tabular-nums">{fmtBytes(diskUsed)} / {fmtBytes(diskTotal)} ({pct}%)</span>
              </div>
              <div class="progress">
                <div class="progress-bar {pct >= 90 ? 'bg-danger' : pct >= 70 ? 'bg-warning' : 'bg-success'}" style="width: {pct}%"></div>
              </div>
            </div>
          {/if}
        {/if}
      </div>
    </section>

    <!-- Danger zone (admin only) -->
    {#if isAdmin}
      <section class="card border-danger/30">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2 text-danger"><AlertTriangle size={16} /> Danger zone</h2>
        </div>
        <div class="card-body divide-y divide-line py-0">
          <div class="flex items-center justify-between gap-4 py-4">
            <div>
              <p class="text-sm text-fg">Clear cache</p>
              <p class="text-xs text-subtle">Flush all cached data and rebuild. Not available yet — the API has no endpoint for this.</p>
            </div>
            <button disabled class="btn btn-danger-ghost btn-sm" title="Not available"><Trash2 size={14} /> Clear</button>
          </div>
          <div class="flex items-center justify-between gap-4 py-4">
            <div>
              <p class="text-sm text-fg">Reset audit logs</p>
              <p class="text-xs text-subtle">Permanently delete all audit log entries. Not available yet — the API has no endpoint for this.</p>
            </div>
            <button disabled class="btn btn-danger-ghost btn-sm" title="Not available"><Database size={14} /> Clear</button>
          </div>
        </div>
      </section>
    {/if}
  </div>
</DashboardLayout>
