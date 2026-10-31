<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative, fmtBytesFull } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import {
    Globe, Zap, Server, ArrowRight, Plus, RefreshCw, ShieldAlert, Shield, ShieldCheck, Box, Database,
    Terminal, Archive, Lock, AlertTriangle, CheckCircle2, WifiOff, Activity,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type MetricState =
    | { state: 'loading' }
    | { state: 'ok'; m: any }
    | { state: 'empty' }
    | { state: 'error'; message: string };
  type SecState = 'idle' | 'loading' | 'ok' | 'unreachable' | 'unconfigured' | 'error';

  let servers = $state<any[]>([]);
  let sites = $state<any[]>([]);
  let jobs = $state<any[]>([]);
  let loadingServers = $state(true);
  let loadingSites = $state(true);
  let loadingJobs = $state(true);
  let metrics = $state<Record<number, MetricState>>({});
  let security = $state<any>(null);
  let secState = $state<SecState>('idle');
  let refreshing = $state(false);
  let refreshedAt = $state<Date | null>(null);

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));

  function percent(used: number, total: number): number {
    if (!total || total <= 0) return 0;
    return Math.min(100, Math.max(0, (used / total) * 100));
  }

  function barTone(v: number): string {
    if (v >= 90) return 'bg-danger';
    if (v >= 75) return 'bg-warning';
    return 'bg-accent';
  }

  function isExpired(e: any): boolean {
    return e?.message === 'session expired';
  }

  async function loadMetrics(server: any) {
    if (!metrics[server.id]) metrics[server.id] = { state: 'loading' };
    try {
      const m = await apiFetch(auth.token, 'GET', `servers/${server.id}/metrics/latest`);
      metrics[server.id] = m ? { state: 'ok', m } : { state: 'empty' };
    } catch (e: any) {
      if (isExpired(e)) return;
      // 404 "no metrics yet" is normal for a freshly added server.
      const msg = String(e?.message || '');
      metrics[server.id] = /no metrics|not found/i.test(msg) ? { state: 'empty' } : { state: 'error', message: msg };
    }
  }

  async function loadSecurity(server: any) {
    if (secState === 'idle') secState = 'loading';
    try {
      const sec = await apiFetch(auth.token, 'GET', `servers/${server.id}/security/overview`);
      security = sec?.data || sec || null;
      secState = security ? 'ok' : 'error';
    } catch (e: any) {
      if (isExpired(e)) return;
      const msg = String(e?.message || '');
      security = null;
      secState = /unreachable|unavailable/i.test(msg) ? 'unreachable' : /not configured/i.test(msg) ? 'unconfigured' : 'error';
    }
  }

  // Each source renders as soon as it arrives; per-server metrics and the
  // security overview start right after the server list, in parallel.
  async function loadDashboard() {
    refreshing = true;
    await Promise.allSettled([
      apiFetch(auth.token, 'GET', 'servers')
        .then(async (d) => {
          servers = d?.servers || [];
          loadingServers = false;
          const withId = servers.filter((s) => s.id);
          await Promise.allSettled([
            ...withId.map(loadMetrics),
            isAdmin && withId[0] ? loadSecurity(withId[0]) : Promise.resolve(),
          ]);
        })
        .catch((e) => { if (!isExpired(e)) servers = []; })
        .finally(() => (loadingServers = false)),
      apiFetch(auth.token, 'GET', 'sites?limit=500')
        .then((d) => (sites = d?.sites || []))
        .catch((e) => { if (!isExpired(e)) sites = []; })
        .finally(() => (loadingSites = false)),
      apiFetch(auth.token, 'GET', 'jobs?limit=50')
        .then((d) => (jobs = d?.jobs || []))
        .catch((e) => { if (!isExpired(e)) jobs = []; })
        .finally(() => (loadingJobs = false)),
    ]);
    refreshedAt = new Date();
    refreshing = false;
  }

  onMount(loadDashboard);

  const onlineServers = $derived(servers.filter((s) => s.status === 'active' || s.status === 'online').length);
  const activeSites = $derived(sites.filter((s) => s.status === 'active').length);
  const sslAttention = $derived.by(() => {
    let expiring = 0;
    let expired = 0;
    for (const s of sites) {
      if (!s.ssl_status || s.ssl_status === 'none' || !s.ssl_expires_at) continue;
      const ms = new Date(s.ssl_expires_at).getTime() - Date.now();
      if (ms <= 0) expired++;
      else if (ms < 14 * 864e5) expiring++;
    }
    return { expiring, expired, total: expiring + expired };
  });
  const runningJobs = $derived(jobs.filter((j) => j.status === 'running' || j.status === 'queued').length);
  const failedJobs = $derived(jobs.filter((j) => j.status === 'failed').length);
  const recentJobs = $derived(jobs.slice(0, 6));

  const quickActions = [
    { label: 'New site', href: '/sites', icon: Globe },
    { label: 'Docker', href: '/docker', icon: Box },
    { label: 'Firewall', href: '/firewall', icon: Shield },
    { label: 'Databases', href: '/databases', icon: Database },
    { label: 'Backups', href: '/backups', icon: Archive },
    { label: 'SSL', href: '/ssl', icon: Lock },
    { label: 'Redis', href: '/redis', icon: Zap },
    { label: 'Terminal', href: '/terminal', icon: Terminal },
  ];
</script>

<DashboardLayout active="Overview">
  <PageHeader
    title="Overview"
    description={auth.user ? `Signed in as ${auth.user.username} · ${auth.user.role.replace('_', ' ')}` : ''}
  >
    {#snippet actions()}
      <button onclick={loadDashboard} class="btn btn-secondary btn-sm" disabled={refreshing}>
        <RefreshCw size={14} class={refreshing ? 'animate-spin' : ''} /> Refresh
      </button>
      <a href="/servers" class="btn btn-secondary btn-sm"><Server size={14} /> Add server</a>
      <a href="/sites" class="btn btn-primary btn-sm"><Plus size={14} /> New site</a>
    {/snippet}
  </PageHeader>

  <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
    <StatCard
      label="Servers"
      value={loadingServers ? '–' : servers.length}
      hint={loadingServers ? '' : `${onlineServers} online`}
      icon={Server}
      tone={!loadingServers && servers.length > 0 && onlineServers < servers.length ? 'warning' : ''}
      href="/servers"
    />
    <StatCard
      label="Sites"
      value={loadingSites ? '–' : sites.length}
      hint={loadingSites ? '' : `${activeSites} active`}
      icon={Globe}
      href="/sites"
    />
    <StatCard
      label="SSL needing attention"
      value={loadingSites ? '–' : sslAttention.total}
      hint={loadingSites ? '' : sslAttention.total === 0 ? 'All certificates healthy' : `${sslAttention.expired} expired · ${sslAttention.expiring} expiring`}
      icon={sslAttention.total > 0 ? ShieldAlert : ShieldCheck}
      tone={loadingSites ? '' : sslAttention.expired > 0 ? 'danger' : sslAttention.expiring > 0 ? 'warning' : 'success'}
      href="/ssl"
    />
    <StatCard
      label="Jobs running"
      value={loadingJobs ? '–' : runningJobs}
      hint={loadingJobs ? '' : `${failedJobs} failed in last ${jobs.length}`}
      icon={Zap}
      tone={failedJobs > 0 ? 'danger' : runningJobs > 0 ? 'info' : ''}
      href="/jobs"
    />
  </div>

  <div class="grid gap-6 lg:grid-cols-3">
    <!-- Server health + security -->
    <section class="min-w-0 space-y-3 lg:col-span-2">
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-sm font-semibold text-fg">Server health</h2>
        <a href="/monitoring" class="link inline-flex items-center gap-1 text-[13px]">Monitoring <ArrowRight size={13} /></a>
      </div>

      {#if loadingServers}
        <div class="grid gap-3 md:grid-cols-2">
          {#each Array(2) as _, i (i)}
            <div class="card space-y-4 p-5">
              <div class="flex items-center gap-3"><div class="skeleton size-9"></div><div class="flex-1 space-y-2"><div class="skeleton h-3.5 w-32"></div><div class="skeleton h-3 w-24"></div></div></div>
              {#each Array(3) as _, j (j)}<div class="space-y-1.5"><div class="skeleton h-3 w-full"></div><div class="skeleton h-1.5 w-full"></div></div>{/each}
            </div>
          {/each}
        </div>
      {:else if servers.length === 0}
        <div class="card">
          <EmptyState compact icon={Server} title="No servers yet" description="Connect a server to start deploying sites and collecting metrics.">
            <a href="/servers" class="btn btn-primary btn-sm"><Plus size={14} /> Add server</a>
          </EmptyState>
        </div>
      {:else}
        <div class="grid gap-3 md:grid-cols-2">
          {#each servers as srv (srv.id)}
            {@const st = metrics[srv.id] ?? { state: 'loading' }}
            <article class="card min-w-0 p-5">
              <div class="mb-4 flex items-center gap-3">
                <div class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-subtle">
                  <Server size={16} strokeWidth={1.75} />
                </div>
                <div class="min-w-0 flex-1">
                  <a href="/servers" class="block truncate text-sm font-semibold text-fg hover:text-accent">{srv.name}</a>
                  <p class="truncate font-mono text-xs text-subtle">{srv.hostname || srv.ip_address || '—'}</p>
                </div>
                <StatusPill status={srv.status || 'unknown'} />
              </div>

              {#if st.state === 'loading'}
                <div class="space-y-3">
                  {#each Array(3) as _, j (j)}<div class="space-y-1.5"><div class="skeleton h-3 w-full"></div><div class="skeleton h-1.5 w-full"></div></div>{/each}
                </div>
              {:else if st.state === 'ok'}
                {@const m = st.m}
                {@const rows = [
                  { label: 'CPU', value: Math.min(100, Math.max(0, Number(m.cpu_percent) || 0)), detail: '' },
                  { label: 'Memory', value: percent(m.mem_used, m.mem_total), detail: m.mem_total ? `${fmtBytesFull(m.mem_used || 0)} / ${fmtBytesFull(m.mem_total)}` : '' },
                  { label: 'Disk', value: percent(m.disk_used, m.disk_total), detail: m.disk_total ? `${fmtBytesFull(m.disk_used || 0)} / ${fmtBytesFull(m.disk_total)}` : '' },
                  ...(m.swap_total ? [{ label: 'Swap', value: percent(m.swap_used, m.swap_total), detail: `${fmtBytesFull(m.swap_used || 0)} / ${fmtBytesFull(m.swap_total)}` }] : []),
                ]}
                <div class="space-y-3">
                  {#each rows as row (row.label)}
                    <div>
                      <div class="mb-1.5 flex items-baseline justify-between gap-3 text-xs">
                        <span class="font-medium text-muted">{row.label}</span>
                        <span class="truncate text-subtle tabular-nums">
                          {#if row.detail}<span class="max-sm:hidden">{row.detail} · </span>{/if}<span class="font-medium {row.value >= 90 ? 'text-danger' : row.value >= 75 ? 'text-warning' : 'text-fg'}">{row.value.toFixed(1)}%</span>
                        </span>
                      </div>
                      <div class="progress"><div class="progress-bar {barTone(row.value)}" style="width: {row.value}%"></div></div>
                    </div>
                  {/each}
                </div>
                {#if m.collected_at}<p class="mt-3 text-xs text-subtle">Updated {fmtRelative(m.collected_at)}</p>{/if}
              {:else if st.state === 'empty'}
                <div class="flex items-center gap-2 rounded-lg border border-dashed border-line px-3 py-4 text-[13px] text-muted">
                  <Activity size={14} class="shrink-0 text-subtle" /> No metrics yet — the agent hasn't reported.
                </div>
              {:else}
                <div class="flex items-center gap-2 rounded-lg border border-dashed border-line px-3 py-4 text-[13px] text-muted">
                  <WifiOff size={14} class="shrink-0 text-subtle" /> Metrics unavailable right now.
                </div>
              {/if}
            </article>
          {/each}
        </div>
      {/if}

      {#if isAdmin && secState !== 'idle'}
        <div class="card mt-6">
          <div class="card-header">
            <h3 class="card-title flex min-w-0 items-center gap-2">
              <Shield size={16} class="shrink-0 text-subtle" />
              <span class="truncate">Security{servers[0]?.name ? ` · ${servers[0].name}` : ''}</span>
            </h3>
            <div class="flex shrink-0 items-center gap-3 text-[13px]">
              <a href="/fail2ban" class="link">Fail2ban</a>
              <a href="/threats" class="link">Threats</a>
            </div>
          </div>
          {#if secState === 'loading'}
            <div class="grid gap-3 p-5 sm:grid-cols-2">
              {#each Array(4) as _, i (i)}<div class="skeleton h-8"></div>{/each}
            </div>
          {:else if secState === 'ok' && security}
            {@const integrityOk = !security.db_exposed_public && !(security.threats_detected > 0)}
            <div class="grid sm:grid-cols-2">
              <div class="divide-y divide-line sm:border-r sm:border-line">
                <div class="flex items-center justify-between gap-3 px-5 py-3">
                  <span class="text-[13px] text-muted">Firewall (UFW)</span>
                  <span class="badge {security.firewall_active ? 'badge-success' : 'badge-danger'}"><span class="dot"></span>{security.firewall_active ? 'Active' : 'Disabled'}</span>
                </div>
                <div class="flex items-center justify-between gap-3 px-5 py-3">
                  <span class="text-[13px] text-muted">Anti-DDoS</span>
                  <span class="badge {security.ddos_syn_defense ? 'badge-success' : 'badge-warning'}">{security.ddos_syn_defense ? 'SYN cookies + rate limit' : 'Standard'}</span>
                </div>
              </div>
              <div class="divide-y divide-line border-t border-line sm:border-t-0">
                <div class="flex items-center justify-between gap-3 px-5 py-3">
                  <span class="text-[13px] text-muted">Banned IPs (Fail2ban)</span>
                  <span class="text-sm font-semibold text-fg tabular-nums">{security.banned_ips_count ?? 0}</span>
                </div>
                <div class="flex items-center justify-between gap-3 px-5 py-3">
                  <span class="text-[13px] text-muted">DB & file integrity</span>
                  <span class="badge {integrityOk ? 'badge-success' : 'badge-danger'}">
                    {#if security.db_exposed_public}<AlertTriangle size={12} /> Database exposed
                    {:else if security.threats_detected > 0}<AlertTriangle size={12} /> {security.threats_detected} suspicious file(s)
                    {:else}<CheckCircle2 size={12} /> Clean{/if}
                  </span>
                </div>
              </div>
            </div>
          {:else}
            <div class="flex items-center gap-2 px-5 py-4 text-[13px] text-muted">
              <WifiOff size={14} class="shrink-0 text-subtle" />
              {secState === 'unreachable' ? 'Agent unreachable — security status appears once it responds.' : secState === 'unconfigured' ? 'Agent not configured for this server.' : 'No security data yet.'}
            </div>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Right column -->
    <div class="min-w-0 space-y-6">
      <section class="card">
        <div class="card-header">
          <h2 class="card-title">Recent jobs</h2>
          <a href="/jobs" class="link inline-flex items-center gap-1 text-[13px]">All jobs <ArrowRight size={13} /></a>
        </div>
        {#if loadingJobs}
          <div class="space-y-3 p-4">{#each Array(4) as _, i (i)}<div class="skeleton h-8"></div>{/each}</div>
        {:else if recentJobs.length === 0}
          <EmptyState compact icon={Zap} title="No jobs yet" description="Deploys, backups and SSL requests show up here." />
        {:else}
          <ul class="divide-y divide-line">
            {#each recentJobs as job (job.id)}
              <li class="flex items-center gap-3 px-5 py-3">
                <div class="min-w-0 flex-1">
                  <p class="truncate font-mono text-[13px] text-fg">{job.type || 'unknown'}</p>
                  <p class="text-xs text-subtle">{fmtRelative(job.created_at)}</p>
                </div>
                <StatusPill status={job.status} />
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="card">
        <div class="card-header"><h2 class="card-title">Quick actions</h2></div>
        <div class="grid grid-cols-2 gap-1 p-2">
          {#each quickActions as a (a.label)}
            <a href={a.href} class="flex items-center gap-2.5 rounded-lg px-3 py-2.5 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-fg">
              <a.icon size={16} strokeWidth={1.75} class="shrink-0 text-subtle" /> {a.label}
            </a>
          {/each}
        </div>
      </section>

      {#if refreshedAt && !refreshing}
        <p class="text-xs text-subtle">Updated {refreshedAt.toLocaleTimeString()}</p>
      {/if}
    </div>
  </div>
</DashboardLayout>
