<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { fmtBytes } from '$lib/format';
  import { Server, HardDrive, ChevronRight, Clock, Cpu, Trash2, AlertTriangle, Activity, KeyRound, Key } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';

  let version = $state('—');
  let serverInfo = $state<any>(null);
  let diskUsed = $state(0);
  let diskTotal = $state(0);
  let loading = $state(true);
  const isSuperAdmin = $derived(hasRole(auth.user?.role, 'super_admin'));

  let purgeDays = $state('90');
  let confirmPurge = $state(false);
  let purging = $state(false);

  onMount(async () => {
    if (!auth.user) { loading = false; return; }
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
  // "Clear cache" was removed: the API keeps no application cache to clear.
  async function purgeAudit() {
    purging = true;
    try {
      const d = await apiFetch(auth.token, 'DELETE', `audit-logs?older_than_days=${purgeDays}`);
      const n = d?.deleted ?? 0;
      toasts.success(n ? `Deleted ${n} audit entr${n === 1 ? 'y' : 'ies'} older than ${purgeDays} days` : `No audit entries older than ${purgeDays} days`);
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to purge audit logs');
    } finally {
      purging = false;
    }
  }
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
      <a href="/member/api-keys" class="group flex items-center gap-3 p-5 transition-colors duration-150 hover:bg-surface-2">
        <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent"><Key size={16} /></span>
        <span class="min-w-0 flex-1">
          <span class="block text-sm font-semibold text-fg">API keys</span>
          <span class="block text-[13px] text-muted">Personal keys for the CLI and scripts.</span>
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

    <!-- Danger zone (super admin only) -->
    {#if isSuperAdmin}
      <section class="card border-danger/30">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2 text-danger"><AlertTriangle size={16} /> Danger zone</h2>
        </div>
        <div class="card-body">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div class="min-w-0 flex-1 basis-56">
              <p class="text-sm text-fg">Purge old audit logs</p>
              <p class="text-xs text-subtle">Permanently delete audit entries older than the selected age. The purge itself is recorded.</p>
            </div>
            <div class="flex items-center gap-2">
              <label for="purge-days" class="sr-only">Older than</label>
              <select id="purge-days" bind:value={purgeDays} class="input h-8 w-44 text-[13px]" disabled={purging}>
                <option value="30">Older than 30 days</option>
                <option value="90">Older than 90 days</option>
                <option value="180">Older than 180 days</option>
                <option value="365">Older than 1 year</option>
              </select>
              <button class="btn btn-danger-ghost btn-sm" onclick={() => (confirmPurge = true)} disabled={purging}>
                <Trash2 size={14} /> {purging ? 'Purging…' : 'Purge'}
              </button>
            </div>
          </div>
        </div>
      </section>
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmPurge}
    title="Purge audit logs?"
    message={`Audit entries older than ${purgeDays} days will be permanently deleted. This cannot be undone.`}
    confirmLabel="Purge"
    destructive
    onconfirm={purgeAudit} />
</DashboardLayout>
