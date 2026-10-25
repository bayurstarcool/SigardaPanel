<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Globe, Zap, Server, ArrowRight, Plus, Activity, Cpu, HardDrive, MemoryStick, Shield, Box, Database, GitBranch, Terminal, CheckCircle2, AlertTriangle } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let sites = $state<any[]>([]);
  let jobs = $state<any[]>([]);
  let securityStatus = $state<any>(null);
  let loading = $state(true);
  let refreshedAt = $state<Date | null>(null);

  function percent(used: number, total: number): number {
    if (!total || total <= 0) return 0;
    return Math.min(100, Math.max(0, used / total * 100));
  }

  function pct(value: number | null | undefined, digits = 1): string {
    return `${Number(value || 0).toFixed(digits)}%`;
  }

  function fmtBytes(bytes: number): string {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    return `${(bytes / Math.pow(1024, index)).toFixed(index >= 3 ? 1 : 0)} ${units[index]}`;
  }

  function barColor(value: number, base: string): string {
    if (value >= 90) return 'bg-rose-400';
    if (value >= 75) return 'bg-amber-400';
    return base;
  }

  function statusFor(cpu: number, memory: number, disk: number): 'healthy' | 'warning' | 'critical' {
    const highest = Math.max(cpu, memory, disk);
    return highest >= 90 ? 'critical' : highest >= 75 ? 'warning' : 'healthy';
  }

  async function loadDashboard() {
    loading = true;
    try {
      const [serverData, siteData, jobData] = await Promise.all([
        apiFetch(auth.token, 'GET', 'servers').catch((e) => { if (e.message === 'session expired') throw e; return { servers: [] }; }),
        apiFetch(auth.token, 'GET', 'sites?limit=100').catch((e) => { if (e.message === 'session expired') throw e; return { sites: [] }; }),
        apiFetch(auth.token, 'GET', 'jobs?limit=5').catch((e) => { if (e.message === 'session expired') throw e; return { jobs: [] }; }),
      ]);

      const nextServers = serverData?.servers || [];
      await Promise.all(nextServers.map(async (server: any) => {
        if (!server.id) return;
        try {
          const metrics = await apiFetch(auth.token, 'GET', `servers/${server.id}/metrics/latest`);
          server.memory_used = metrics?.mem_used || 0;
          server.memory_total = metrics?.mem_total || 0;
          server.disk_used = metrics?.disk_used || 0;
          server.disk_total = metrics?.disk_total || 0;
          server.swap_used = metrics?.swap_used || 0;
          server.swap_total = metrics?.swap_total || 0;
          server.cpu_percent = metrics?.cpu_percent || 0;
          server.metrics_loaded = true;
        } catch {}
      }));

      servers = [...nextServers];
      sites = siteData?.sites || [];
      jobs = jobData?.jobs || [];

      if (nextServers.length > 0 && nextServers[0]?.id) {
        try {
          const sec = await apiFetch(auth.token, 'GET', `servers/${nextServers[0].id}/security/overview`);
          securityStatus = sec?.data || sec || null;
        } catch {}
      }

      refreshedAt = new Date();
    } catch (e: any) {
      if (e.message === 'session expired') return;
    } finally {
      loading = false;
    }
  }

  onMount(loadDashboard);

  const onlineServers = $derived(servers.filter((s: any) => s.status === 'active' || s.status === 'online').length);
  const activeSites = $derived(sites.filter((s: any) => s.status === 'active').length);
  const runningJobs = $derived(jobs.filter((j: any) => j.status === 'running' || j.status === 'queued').length);
  const fleetHealth = $derived(servers.length ? percent(onlineServers, servers.length) : 0);
</script>

<DashboardLayout active="Overview">
  {#if loading}
    <div class="flex min-h-[60vh] items-center justify-center">
      <div class="flex flex-col items-center gap-4">
        <div class="relative">
          <div class="absolute inset-0 rounded-2xl bg-sky-400/20 blur-xl animate-pulse"></div>
          <div class="relative flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 shadow-lg shadow-sky-500/25">
            <Activity size={20} class="animate-spin text-white" />
          </div>
        </div>
        <span class="text-sm text-slate-500 animate-pulse">Loading dashboard...</span>
      </div>
    </div>
  {:else}
    <div class="dashboard-page">
    <div class="pointer-events-none fixed inset-0 overflow-hidden">
      <div class="absolute left-[12%] top-1/4 h-[360px] w-[360px] rounded-full bg-sky-500/[0.025] blur-3xl"></div>
      <div class="absolute bottom-0 right-[8%] h-[300px] w-[300px] rounded-full bg-indigo-500/[0.025] blur-3xl"></div>
      <div class="absolute inset-0 opacity-[0.018]" style="background-image: linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px); background-size: 64px 64px;"></div>
    </div>

    <header class="dashboard-hero relative mb-8 overflow-hidden rounded-2xl border border-white/[0.06] p-6 sm:p-8 lg:p-10">
      <div class="hero-orb hero-orb-sky absolute right-0 top-0 h-96 w-96 -translate-y-1/2 translate-x-1/2 rounded-full"></div>
      <div class="hero-orb hero-orb-indigo absolute bottom-0 left-0 h-64 w-64 -translate-x-1/2 translate-y-1/2 rounded-full"></div>
      <div class="hero-dots absolute inset-0"></div>
      <div class="relative flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div class="mb-3 flex items-center gap-2 text-[0.65rem] font-bold uppercase tracking-[0.25em] text-sky-300/60">
            <span class="h-1.5 w-1.5 rounded-full bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.8)]"></span>
            CONTROL PLANE
          </div>
          <h1 class="text-3xl font-extrabold tracking-tight text-white sm:text-4xl lg:text-5xl">SigardaPanel</h1>
          <p class="mt-2 text-sm capitalize text-slate-400">{auth.user?.role} &bull; {auth.user?.username}</p>
          <p class="mt-4 max-w-2xl text-sm leading-relaxed dashboard-hero-description text-slate-300/75">
            Kelola server, site, SSL, backup, dan keamanan dari satu control plane.
          </p>
          <div class="mt-6 flex flex-wrap gap-3">
            <a href="/sites" class="dashboard-primary-cta group inline-flex min-h-11 items-center justify-center gap-2 rounded-xl px-5 py-3 text-sm font-semibold transition hover:-translate-y-0.5">
              <Plus size={16} /> Buat Site <ArrowRight size={14} class="transition group-hover:translate-x-0.5" />
            </a>
            <a href="/servers" class="dashboard-secondary-cta inline-flex min-h-11 items-center justify-center gap-2 rounded-xl px-5 py-3 text-sm font-medium transition hover:-translate-y-0.5">
              <Server size={15} /> Tambah Server
            </a>
          </div>
        </div>
        <div class="dashboard-fleet-card w-full max-w-xs rounded-2xl border border-white/[0.1] bg-[#061328]/70 p-4 shadow-[0_16px_40px_rgba(0,0,0,0.18)] backdrop-blur-md lg:min-w-[240px]">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-xs font-medium uppercase tracking-wider text-slate-500">Fleet health</span>
            {#if fleetHealth >= 90}<CheckCircle2 size={16} class="text-emerald-400" />{:else}<AlertTriangle size={16} class="text-amber-400" />{/if}
          </div>
          <div class="flex items-end justify-between gap-3">
            <span class="text-3xl font-bold tabular-nums text-white">{pct(fleetHealth, 0)}</span>
            <span class="pb-1 text-xs text-slate-500">{onlineServers} / {servers.length || 0} online</span>
          </div>
          <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white/[0.08]">
            <div class="h-full rounded-full bg-gradient-to-r from-emerald-400 to-sky-400 transition-all duration-700" style="width: {fleetHealth}%"></div>
          </div>
          {#if refreshedAt}<div class="mt-2 text-[0.65rem] text-slate-600">Updated {refreshedAt.toLocaleTimeString()}</div>{/if}
        </div>
      </div>
    </header>

    <section class="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-4 sm:gap-4">
      {#each [
        { label: 'Servers', value: servers.length, helper: `${onlineServers} online`, ratio: servers.length ? percent(onlineServers, servers.length) : 0, color: 'sky', icon: Server },
        { label: 'Sites', value: sites.length, helper: `${activeSites} active`, ratio: sites.length ? percent(activeSites, sites.length) : 0, color: 'emerald', icon: Globe },
        { label: 'Jobs', value: jobs.length, helper: `${runningJobs} running`, ratio: jobs.length ? percent(runningJobs, jobs.length) : 0, color: 'violet', icon: Zap },
        { label: 'Alerts', value: '—', helper: 'Feed unavailable', ratio: null, color: 'amber', icon: Activity },
      ] as stat, i}
        <div class="dashboard-page-surface group relative overflow-hidden rounded-2xl border border-white/[0.06] bg-white/[0.025] p-4 transition duration-300 hover:-translate-y-1 hover:border-white/[0.12] hover:bg-white/[0.045] sm:p-5" style="animation: fadeInUp 0.45s ease-out {i * 0.08}s both">
          <div class="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-{stat.color}-400/50 to-transparent opacity-60"></div>
          <div class="mb-4 flex items-center justify-between gap-2">
            <span class="text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-slate-500">{stat.label}</span>
            <div class="flex h-8 w-8 items-center justify-center rounded-xl bg-{stat.color}-500/10 transition group-hover:scale-110"><stat.icon size={15} class="text-{stat.color}-400" /></div>
          </div>
          <div class="text-2xl font-bold tabular-nums text-white sm:text-3xl">{stat.value}</div>
          <div class="mt-1 flex items-center justify-between gap-2 text-[0.65rem]">
            <span class="text-slate-500">{stat.helper}</span>
            <span class="font-medium text-{stat.color}-300">{stat.ratio === null ? 'N/A' : pct(stat.ratio, 0)}</span>
          </div>
          <div class="mt-3 h-1 overflow-hidden rounded-full bg-white/[0.06]">
            <div class="h-full rounded-full bg-{stat.color}-400 transition-all duration-700" style="width: {stat.ratio ?? 0}%"></div>
          </div>
        </div>
      {/each}
    </section>

    <section class="mb-8">
      <div class="mb-4 flex items-center justify-between">
        <div><h2 class="dashboard-section-title text-sm font-semibold uppercase tracking-[0.14em] text-slate-300">Quick actions</h2><p class="dashboard-section-subtitle mt-1 text-xs text-slate-600">Common operations, one click away</p></div>
        <a href="/servers" class="dashboard-section-link text-xs text-sky-400 transition hover:text-sky-300">View servers <ArrowRight size={13} class="inline" /></a>
      </div>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {#each [
          { label: 'Docker', href: '/docker', icon: Box, color: 'sky' },
          { label: 'Firewall', href: '/firewall', icon: Shield, color: 'red' },
          { label: 'Redis', href: '/redis', icon: Zap, color: 'amber' },
          { label: 'Database', href: '/databases', icon: Database, color: 'emerald' },
          { label: 'Git Deploy', href: '/git', icon: GitBranch, color: 'violet' },
          { label: 'Terminal', href: '/terminal', icon: Terminal, color: 'slate' },
        ] as action}
          <a href={action.href} class="dashboard-page-surface group flex min-h-[108px] flex-col items-center justify-center gap-2 rounded-xl border border-white/[0.06] bg-white/[0.02] p-3 transition duration-300 hover:-translate-y-1 hover:border-white/[0.12] hover:bg-white/[0.05]">
            <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-{action.color}-500/10 transition group-hover:scale-110 group-hover:bg-{action.color}-500/20"><action.icon size={18} class="text-{action.color}-400" /></div>
            <span class="text-xs font-medium text-slate-400 transition group-hover:text-white">{action.label}</span>
          </a>
        {/each}
      </div>
    </section>

    {#if securityStatus}
      <!-- Security & Threat Shield Monitor -->
      <section class="mb-8">
        <div class="mb-4 flex items-end justify-between">
          <div>
            <h2 class="dashboard-section-title text-sm font-semibold uppercase tracking-[0.14em] text-slate-300 flex items-center gap-2">
              <Shield size={16} class="text-sky-400" /> Security & Threat Shield
            </h2>
            <p class="dashboard-section-subtitle mt-1 text-xs text-slate-500">Live protection against DDoS, intrusions, malware & unauthorized access</p>
          </div>
          <a href="/fail2ban" class="dashboard-section-link text-xs text-sky-400 transition hover:text-sky-300">View firewall & rules <ArrowRight size={13} class="inline" /></a>
          <a href="/threats" class="dashboard-section-link text-xs text-amber-400 transition hover:text-amber-300 ml-3">Threat monitor <ArrowRight size={13} class="inline" /></a>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
          <!-- Firewall UFW Status -->
          <div class="p-4 rounded-xl bg-white/[0.025] border border-white/[0.06] flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl {securityStatus.firewall_active ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'} flex items-center justify-center shrink-0">
              <Shield size={18} />
            </div>
            <div class="min-w-0">
              <div class="text-xs font-semibold text-white">Firewall (UFW)</div>
              <div class="text-[0.7rem] font-medium {securityStatus.firewall_active ? 'text-emerald-400' : 'text-rose-400'}">
                {securityStatus.firewall_active ? 'Active & Enforcing' : 'Disabled / Off'}
              </div>
            </div>
          </div>

          <!-- Anti-DDoS / SYN Defense -->
          <div class="p-4 rounded-xl bg-white/[0.025] border border-white/[0.06] flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl {securityStatus.ddos_syn_defense ? 'bg-sky-500/10 text-sky-400 border border-sky-500/20' : 'bg-amber-500/10 text-amber-400 border border-amber-500/20'} flex items-center justify-center shrink-0">
              <Zap size={18} />
            </div>
            <div class="min-w-0">
              <div class="text-xs font-semibold text-white">Anti-DDoS Shield</div>
              <div class="text-[0.7rem] font-medium {securityStatus.ddos_syn_defense ? 'text-sky-400' : 'text-amber-400'}">
                {securityStatus.ddos_syn_defense ? 'SYN-Cookies & Rate Limit Active' : 'Standard Protection'}
              </div>
            </div>
          </div>

          <!-- Intrusion & Bruteforce (Fail2ban) -->
          <div class="p-4 rounded-xl bg-white/[0.025] border border-white/[0.06] flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-amber-500/10 text-amber-400 border border-amber-500/20 flex items-center justify-center shrink-0">
              <AlertTriangle size={18} />
            </div>
            <div class="min-w-0">
              <div class="text-xs font-semibold text-white">Blocked Attackers</div>
              <div class="text-[0.7rem] font-medium text-slate-300">
                <span class="text-amber-400 font-bold tabular-nums">{securityStatus.banned_ips_count}</span> IP(s) banned by Fail2ban
              </div>
            </div>
          </div>

          <!-- Database & Web Root Integrity -->
          <div class="p-4 rounded-xl bg-white/[0.025] border border-white/[0.06] flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl {securityStatus.threats_detected === 0 && !securityStatus.db_exposed_public ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'} flex items-center justify-center shrink-0">
              <CheckCircle2 size={18} />
            </div>
            <div class="min-w-0">
              <div class="text-xs font-semibold text-white">DB & File Integrity</div>
              <div class="text-[0.7rem] font-medium {securityStatus.threats_detected === 0 && !securityStatus.db_exposed_public ? 'text-emerald-400' : 'text-rose-400'}">
                {#if securityStatus.db_exposed_public}
                  Database exposed!
                {:else if securityStatus.threats_detected > 0}
                  {securityStatus.threats_detected} suspicious file(s)
                {:else}
                  DB Localhost · Web Clean
                {/if}
              </div>
            </div>
          </div>
        </div>
      </section>
    {/if}

    {#if servers.length > 0}
      <section class="mb-8">
        <div class="mb-4 flex items-end justify-between"><div><h2 class="dashboard-section-title text-sm font-semibold uppercase tracking-[0.14em] text-slate-300">Server health</h2><p class="dashboard-section-subtitle mt-1 text-xs text-slate-600">Live resource utilization per server</p></div><a href="/monitoring" class="dashboard-section-link text-xs text-sky-400 transition hover:text-sky-300">Open monitoring <ArrowRight size={13} class="inline" /></a></div>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {#each servers as srv, i}
            {@const memPct = percent(srv.memory_used, srv.memory_total)}
            {@const diskPct = percent(srv.disk_used, srv.disk_total)}
            {@const swapPct = percent(srv.swap_used, srv.swap_total)}
            {@const cpuPct = Math.min(100, Math.max(0, srv.cpu_percent || 0))}
            {@const healthStatus = statusFor(cpuPct, memPct, diskPct)}
            <article class="dashboard-page-surface group rounded-2xl border border-white/[0.06] bg-white/[0.025] p-5 transition duration-300 hover:-translate-y-1 hover:border-white/[0.12] hover:bg-white/[0.04]" style="animation: fadeInUp 0.45s ease-out {i * 0.08}s both">
              <div class="mb-5 flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20"><Server size={18} class="text-sky-400" /></div>
                <div class="min-w-0 flex-1"><h3 class="truncate text-sm font-semibold text-slate-200">{srv.name}</h3><p class="truncate text-[0.65rem] text-slate-500">{srv.hostname || 'Hostname unavailable'}</p></div>
                <div class="flex items-center gap-2"><span class="h-2 w-2 rounded-full {healthStatus === 'healthy' ? 'bg-emerald-400' : healthStatus === 'warning' ? 'bg-amber-400' : 'bg-rose-400'} shadow-[0_0_8px_currentColor]"></span><span class="dashboard-status-pill rounded-full bg-white/[0.05] px-2 py-1 text-[0.6rem] font-semibold text-slate-400">{srv.status || 'unknown'}</span></div>
              </div>
              {#if srv.metrics_loaded}
                <div class="space-y-3">
                  {#each [
                    { label: 'CPU', icon: Cpu, value: cpuPct, detail: 'utilization', color: 'bg-sky-400' },
                    { label: 'Memory', icon: MemoryStick, value: memPct, detail: `${fmtBytes(srv.memory_used)} / ${fmtBytes(srv.memory_total)}`, color: 'bg-emerald-400' },
                    { label: 'Disk', icon: HardDrive, value: diskPct, detail: `${fmtBytes(srv.disk_used)} / ${fmtBytes(srv.disk_total)}`, color: 'bg-violet-400' },
                    { label: 'Swap', icon: Activity, value: swapPct, detail: `${fmtBytes(srv.swap_used)} / ${fmtBytes(srv.swap_total)}`, color: 'bg-amber-400' },
                  ] as metric}
                    <div>
                      <div class="mb-1.5 flex items-center justify-between gap-3"><div class="flex items-center gap-1.5"><metric.icon size={11} class="text-slate-500" /><span class="text-[0.68rem] font-medium text-slate-400">{metric.label}</span></div><div class="text-right"><span class="text-[0.68rem] font-semibold tabular-nums text-slate-200">{pct(metric.value)}</span><span class="ml-2 text-[0.6rem] text-slate-600">{metric.detail}</span></div></div>
                      <div class="dashboard-meter-track h-1.5 overflow-hidden rounded-full bg-white/[0.06]"><div class="h-full rounded-full transition-all duration-700 {barColor(metric.value, metric.color)}" style="width: {metric.value}%"></div></div>
                    </div>
                  {/each}
                </div>
              {:else}
                <div class="rounded-xl border border-dashed border-white/[0.08] px-3 py-5 text-center text-xs text-slate-600">Metrics unavailable</div>
              {/if}
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if jobs.length > 0}
      <section class="mb-8">
        <div class="mb-4 flex items-end justify-between"><div><h2 class="dashboard-section-title text-sm font-semibold uppercase tracking-[0.14em] text-slate-300">Recent activity</h2><p class="dashboard-section-subtitle mt-1 text-xs text-slate-600">Latest jobs across control plane</p></div><a href="/jobs" class="dashboard-section-link text-xs text-sky-400 transition hover:text-sky-300">View jobs <ArrowRight size={13} class="inline" /></a></div>
        <div class="dashboard-page-surface overflow-hidden rounded-2xl border border-white/[0.06] bg-white/[0.025]">
          {#each jobs as job, i}
            <div class="flex items-center gap-4 border-b border-white/[0.04] px-5 py-3.5 last:border-0 hover:bg-white/[0.025]" style="animation: fadeIn 0.3s ease-out {i * 0.05}s both">
              <div class="flex h-8 w-8 items-center justify-center rounded-lg {job.status === 'running' ? 'bg-sky-500/10' : job.status === 'success' ? 'bg-emerald-500/10' : job.status === 'failed' ? 'bg-rose-500/10' : 'bg-slate-500/10'}"><Zap size={14} class="{job.status === 'running' ? 'animate-pulse text-sky-400' : job.status === 'success' ? 'text-emerald-400' : job.status === 'failed' ? 'text-rose-400' : 'text-slate-400'}" /></div>
              <div class="min-w-0 flex-1"><p class="truncate text-sm text-slate-300">{job.type || 'unknown'}</p><p class="text-[0.65rem] text-slate-500">{fmtRelative(job.created_at)}</p></div>
              <StatusPill status={job.status} />
            </div>
          {/each}
        </div>
      </section>
    {/if}
    </div>
  {/if}
</DashboardLayout>

<style>
  @keyframes fadeInUp { from { opacity: 0; transform: translateY(16px); } to { opacity: 1; transform: translateY(0); } }
  @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
</style>
