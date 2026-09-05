<script lang="ts">
  import { onMount } from 'svelte';
  import { theme } from '$lib/theme.svelte';
  import { Server, Globe, Database, HardDrive, Shield, Activity, Box, ShieldCheck, Zap, GitBranch, Search, Bell, Mail, ExternalLink, ArrowRight, Sun, Moon } from 'lucide-svelte';

  let visible = $state(false);
  let countersVisible = $state(false);

  const features = [
    { icon: Server, title: 'Multi-Server', desc: 'Manage unlimited VPS from one dashboard.', color: 'sky' },
    { icon: Globe, title: 'Site Deploy', desc: 'PHP, Node.js, Python, Go, Docker — deploy any stack.', color: 'emerald' },
    { icon: Database, title: 'Databases', desc: 'MySQL & PostgreSQL with user management.', color: 'violet' },
    { icon: HardDrive, title: 'Backups', desc: 'Scheduled backups to S3, R2, B2, Wasabi.', color: 'amber' },
    { icon: ShieldCheck, title: 'SSL Auto', desc: 'Auto-renewing Let\'s Encrypt certificates.', color: 'green' },
    { icon: Activity, title: 'Monitoring', desc: 'CPU, memory, disk, network — real-time.', color: 'rose' },
    { icon: Box, title: 'Docker', desc: 'Containers, images, volumes, networks.', color: 'sky' },
    { icon: Shield, title: 'Firewall', desc: 'UFW rules, presets, IP blocking.', color: 'red' },
    { icon: Zap, title: 'Redis', desc: 'Stats, flush, info — full management.', color: 'amber' },
    { icon: GitBranch, title: 'Git Deploy', desc: 'Branches, rollback, auto-deploy.', color: 'indigo' },
    { icon: Search, title: 'Global Search', desc: 'Cmd+K search across all resources.', color: 'cyan' },
    { icon: Bell, title: 'Alerts', desc: 'Telegram, Discord, Email notifications.', color: 'pink' },
  ];

  const stats = [
    { value: 55, suffix: '+', label: 'CLI Commands' },
    { value: 150, suffix: '+', label: 'API Endpoints' },
    { value: 99.9, suffix: '%', label: 'Uptime' },
  ];

  let counterValues = $state(stats.map(() => 0));

  onMount(() => {
    const t1 = setTimeout(() => { visible = true; }, 100);



    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          countersVisible = true;
          animateCounters();
          observer.disconnect();
        }
      });
    }, { threshold: 0.3 });

    const statsEl = document.getElementById('stats-section');
    if (statsEl) observer.observe(statsEl);

    return () => {
      clearTimeout(t1);
      observer.disconnect();
    };
  });

  function animateCounters() {
    const duration = 2000;
    const start = performance.now();
    const targets = stats.map((s) => s.value);

    function tick(now: number) {
      const elapsed = now - start;
      const progress = Math.min(elapsed / duration, 1);
      const ease = 1 - Math.pow(1 - progress, 3);

      counterValues = targets.map((t) => {
        if (Number.isInteger(t)) return Math.round(t * ease);
        return parseFloat((t * ease).toFixed(1));
      });

      if (progress < 1) requestAnimationFrame(tick);
    }
    requestAnimationFrame(tick);
  }
</script>

<svelte:head>
  <title>SigardaPanel — Modern VPS Control Panel</title>
  <meta name="description" content="Manage servers, sites, Docker, firewall, databases, backups — all from one dashboard." />
</svelte:head>

<div class="landing-page min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)] overflow-hidden">
  <!-- Animated Background -->
  <div class="fixed inset-0 z-0 pointer-events-none">
    <!-- Ambient orb -->
    <div class="mesh-orb mesh-orb-1 absolute top-1/4 left-1/4 w-[500px] h-[500px] rounded-full opacity-[0.07]"></div>
    <!-- Particles -->
    {#each Array(6) as _, i}
      <div class="landing-particle absolute w-1 h-1 rounded-full bg-white/20"
        style="left: {5 + (i * 5)}%; animation-duration: {20 + (i * 2)}s; animation-delay: {i * 0.5}s;"></div>
    {/each}
    <!-- Grid -->
    <div class="absolute inset-0 opacity-[0.03]" style="background-image: radial-gradient(rgba(255,255,255,0.03) 1px, transparent 1px); background-size: 32px 32px;"></div>
  </div>

  <!-- Fixed Navbar -->
  <nav class="fixed top-0 left-0 right-0 z-50 bg-[var(--bg-primary)]/95 border-b border-[var(--border-primary)]">
    <div class="flex items-center justify-between px-6 lg:px-12 py-4 max-w-7xl mx-auto">
      <div class="flex items-center gap-3">
        <a href="/" class="flex items-center gap-3">
          <div class="w-9 h-9 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_4px_16px_rgba(56,189,248,0.3)]">
            <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          <span class="text-lg font-bold tracking-tight hidden sm:block">SigardaPanel</span>
        </a>
      </div>
      <div class="flex items-center gap-1">
        <a href="#features" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all">Features</a>
        <a href="#architecture" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden sm:block">Architecture</a>
        <a href="#pricing" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all">Pricing</a>
        <a href="/docs" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden sm:block">Docs</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden md:block">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
        </a>
        <button onclick={() => theme.toggle()}
          class="px-2.5 py-2 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all"
          title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}>
          {#if theme.isDark}
            <Sun size={16} />
          {:else}
            <Moon size={16} />
          {/if}
        </button>
        <div class="w-px h-5 bg-white/[0.08] mx-1 hidden sm:block"></div>
        <a href="/login" class="px-4 py-2 rounded-xl text-sm font-semibold bg-gradient-to-r from-emerald-400 to-sky-400 text-[#040a14] hover:shadow-[0_0_24px_rgba(56,189,248,0.4)] transition-all duration-300 hover:scale-105">
          Sign In
        </a>
      </div>
    </div>
  </nav>

  <!-- Spacer for fixed nav -->
  <div class="h-16"></div>

  <!-- Hero -->
  <section class="relative z-10 max-w-5xl mx-auto px-6 lg:px-12 pt-20 pb-32 text-center {visible ? 'fade-in-up' : 'opacity-0'}">
    <div class="inline-flex items-center gap-2 px-4 py-1.5 rounded-full border border-sky-400/20 bg-sky-400/5 text-sky-300 text-xs font-medium mb-8">
      <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
      Open Source &bull; Self-hosted &bull; v0.5.3
    </div>
    <h1 class="text-5xl sm:text-6xl lg:text-8xl font-extrabold tracking-tight leading-[1.05] mb-6">
      <span class="bg-gradient-to-r from-white via-sky-200 to-indigo-300 bg-clip-text text-transparent hero-gradient">SigardaPanel</span>
    </h1>
    <p class="text-lg sm:text-xl text-slate-400 max-w-2xl mx-auto leading-relaxed mb-10">
      Modern VPS Control Panel. Manage servers, Docker, firewall, databases, backups — all from one dashboard.
    </p>
    <div class="flex flex-wrap items-center justify-center gap-4">
      <a href="/login" class="group relative inline-flex items-center justify-center px-8 py-3.5 rounded-xl text-sm font-bold text-[#040a14] transition-all duration-300 hover:scale-105">
        <div class="absolute inset-0 bg-gradient-to-r from-sky-400 to-indigo-400 group-hover:from-sky-300 group-hover:to-indigo-300 rounded-xl transition-all duration-300"></div>
        <div class="absolute inset-0 rounded-xl shadow-[0_4px_24px_rgba(56,189,248,0.3)] group-hover:shadow-[0_8px_40px_rgba(56,189,248,0.5)] transition-shadow duration-300"></div>
        <span class="relative z-10 flex items-center gap-2">
          Get Started
          <svg class="w-4 h-4 group-hover:translate-x-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
        </span>
      </a>
      <a href="/docs" class="inline-flex items-center gap-2 px-8 py-3.5 rounded-xl text-sm font-medium text-slate-300 border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] hover:border-white/20 hover:text-white transition-all duration-300 hover:scale-105">
        View Documentation
      </a>
    </div>
    <!-- Tech badges -->
    <div class="flex flex-wrap items-center justify-center gap-3 mt-12">
      {#each ['Go', 'SvelteKit', 'SQLite', 'UFW', 'Docker', 'Let\'s Encrypt'] as tech}
        <span class="px-3 py-1 rounded-full text-[0.65rem] font-medium text-slate-500 border border-white/[0.06] bg-white/[0.02]">{tech}</span>
      {/each}
    </div>
  </section>

  <!-- Features Grid -->
  <section id="features" class="relative z-10 max-w-6xl mx-auto px-6 lg:px-12 pb-24">
    <div class="text-center mb-16">
      <h2 class="text-2xl sm:text-3xl md:text-4xl font-extrabold tracking-tight mb-4">
        <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">Everything you need</span>
      </h2>
      <p class="text-slate-500 max-w-lg mx-auto">A complete toolkit for managing your infrastructure.</p>
    </div>
    <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3 sm:gap-4">
      {#each features as feat, i}
        <div class="feature-card group relative p-5 rounded-2xl border border-white/[0.06] bg-white/[0.04] hover:border-white/[0.12] hover:bg-white/[0.04] transition-all duration-300 hover:-translate-y-1" style="animation: fadeInUp 0.5s ease-out {i * 0.05}s both">
          <div class="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-sky-400/20 to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>
          <div class="w-10 h-10 rounded-xl bg-{feat.color}-500/10 flex items-center justify-center mb-3 group-hover:scale-110 transition-transform duration-300"><feat.icon size={20} class="text-{feat.color}-400" /></div>
          <h3 class="text-sm font-bold text-white mb-1">{feat.title}</h3>
          <p class="text-xs text-slate-500 leading-relaxed">{feat.desc}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- Architecture -->
  <section id="architecture" class="relative z-10 max-w-4xl mx-auto px-4 sm:px-6 lg:px-12 pb-16 sm:pb-24">
    <div class="text-center mb-16">
      <h2 class="text-2xl sm:text-3xl md:text-4xl font-extrabold tracking-tight mb-4">
        <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">How it works</span>
      </h2>
      <p class="text-slate-500 max-w-lg mx-auto">Simple, secure architecture for multi-server management.</p>
    </div>
    <div class="flex flex-col sm:flex-row items-center justify-center gap-4 sm:gap-6 md:gap-8">
      <div class="arch-box flex flex-col items-center p-6 rounded-2xl border border-sky-400/20 bg-sky-400/[0.04] w-44 text-center hover:border-sky-400/40 transition-all duration-300">
        <div class="w-12 h-12 rounded-2xl bg-gradient-to-br from-sky-400/20 to-sky-500/10 flex items-center justify-center mb-3 shadow-[0_0_20px_rgba(56,189,248,0.15)]">
          <svg class="w-6 h-6 text-sky-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M9 17.25v1.007a3 3 0 01-.879 2.122L7.5 21h9l-.621-.621A3 3 0 0115 18.257V17.25m6-12V15a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 15V5.25A2.25 2.25 0 015.25 3h13.5A2.25 2.25 0 0121 5.25z" /></svg>
        </div>
        <div class="text-sm font-bold text-white">Panel</div>
        <div class="text-[0.65rem] text-slate-500">Web Dashboard</div>
      </div>
      <div class="text-center text-[0.6rem] font-mono text-sky-400/60 px-3 py-1.5 rounded-lg border border-sky-400/10 bg-sky-400/[0.03]">HTTPS / Token</div>
      <div class="arch-box flex flex-col items-center p-6 rounded-2xl border border-indigo-400/20 bg-indigo-400/[0.04] w-44 text-center hover:border-indigo-400/40 transition-all duration-300">
        <div class="w-12 h-12 rounded-2xl bg-gradient-to-br from-indigo-400/20 to-indigo-500/10 flex items-center justify-center mb-3 shadow-[0_0_20px_rgba(99,102,241,0.15)]">
          <svg class="w-6 h-6 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" /></svg>
        </div>
        <div class="text-sm font-bold text-white">Agent</div>
        <div class="text-[0.65rem] text-slate-500">Per-VPS Daemon</div>
      </div>
      <div class="text-center text-[0.6rem] font-mono text-indigo-400/60 px-3 py-1.5 rounded-lg border border-indigo-400/10 bg-indigo-400/[0.03]">Local Execute</div>
      <div class="arch-box flex flex-col items-center p-6 rounded-2xl border border-violet-400/20 bg-violet-400/[0.04] w-44 text-center hover:border-violet-400/40 transition-all duration-300">
        <div class="w-12 h-12 rounded-2xl bg-gradient-to-br from-violet-400/20 to-violet-500/10 flex items-center justify-center mb-3 shadow-[0_0_20px_rgba(139,92,246,0.15)]">
          <svg class="w-6 h-6 text-violet-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M21.75 17.25v-.228a4.5 4.5 0 00-.12-1.03l-2.268-9.64a3.375 3.375 0 00-3.285-2.602H7.923a3.375 3.375 0 00-3.285 2.602l-2.268 9.64a4.5 4.5 0 00-.12 1.03v.228m19.5 0a3 3 0 01-3 3H5.25a3 3 0 01-3-3m19.5 0a3 3 0 00-3-3H5.25a3 3 0 00-3 3m16.5 0h.008v.008h-.008v-.008zm-3 0h.008v.008h-.008v-.008z" /></svg>
        </div>
        <div class="text-sm font-bold text-white">VPS</div>
        <div class="text-[0.65rem] text-slate-500">Your Server</div>
      </div>
    </div>
  </section>

  <!-- Stats -->
  <section id="stats-section" class="relative z-10 max-w-4xl mx-auto px-4 sm:px-6 lg:px-12 pb-16 sm:pb-24">
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 sm:gap-6">
      {#each stats as stat, i}
        <div class="text-center p-4 sm:p-6 rounded-2xl border border-white/[0.06] bg-white/[0.02]">
          <div class="text-3xl sm:text-4xl md:text-5xl font-extrabold bg-gradient-to-r from-sky-400 to-indigo-400 bg-clip-text text-transparent mb-2">
            {counterValues[i]}{stat.suffix}
          </div>
          <div class="text-sm text-slate-500">{stat.label}</div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Pricing -->
  <section id="features" class="relative z-10 max-w-6xl mx-auto px-6 lg:px-12 pb-24">
    <div class="text-center mb-16">
      <h2 class="text-2xl sm:text-3xl md:text-4xl font-extrabold tracking-tight mb-4">
        <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">Simple Pricing</span>
      </h2>
      <p class="text-slate-500 max-w-lg mx-auto">Start free, upgrade when you need enterprise features.</p>
    </div>
    <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-6">
      <!-- Community -->
      <div class="pricing-card group relative p-8 rounded-2xl border border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12] transition-all duration-300">
        <div class="text-xs font-bold uppercase tracking-wider text-slate-500 mb-2">Community</div>
        <div class="flex items-baseline gap-1 mb-1">
          <span class="text-4xl font-extrabold text-white">Free</span>
        </div>
        <p class="text-sm text-slate-500 mb-6">Self-hosted, open source</p>
        <a href="/login" class="block w-full text-center py-3 rounded-xl text-sm font-semibold border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] hover:border-white/20 text-white transition-all duration-300 mb-8">
          Get Started
        </a>
        <div class="space-y-3">
          {#each ['Single server management', 'Unlimited sites', 'Docker, Firewall, Redis', 'SSL auto-renewal', 'Git deploy & rollback', 'Database management', 'Automated backups', 'CLI + Web dashboard', 'Community support'] as feat}
            <div class="flex items-center gap-2 text-sm text-slate-400">
              <svg class="w-4 h-4 text-emerald-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
              {feat}
            </div>
          {/each}
        </div>
      </div>

      <!-- Premium -->
      <div class="pricing-card group relative p-8 rounded-2xl border border-sky-400/30 bg-sky-400/[0.04] hover:border-sky-400/50 transition-all duration-300">
        <div class="absolute top-0 left-1/2 -translate-x-1/2 -translate-y-1/2 px-3 py-1 rounded-full bg-sky-400 text-[0.65rem] font-bold text-[#040a14] uppercase tracking-wider">Popular</div>
        <div class="text-xs font-bold uppercase tracking-wider text-sky-400 mb-2">Premium</div>
        <div class="flex items-baseline gap-1 mb-1">
          <span class="text-4xl font-extrabold text-white">$29</span>
          <span class="text-sm text-slate-500">/month</span>
        </div>
        <p class="text-sm text-slate-500 mb-6">Managed panel with priority support</p>
        <a href="mailto:wahyu@sigarda.com?subject=Premium%20Plan" class="block w-full text-center py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all duration-300 mb-8">
          Contact Sales
        </a>
        <div class="space-y-3">
          {#each ['Everything in Community', 'Multi-server management', 'Managed updates & patches', 'Priority support (< 24h)', 'Custom branding (white-label)', 'Advanced monitoring', 'API rate limit increase', 'Email & chat support', 'Monthly health reports', 'Staging environment'] as feat}
            <div class="flex items-center gap-2 text-sm text-slate-400">
              <svg class="w-4 h-4 text-sky-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
              {feat}
            </div>
          {/each}
        </div>
      </div>

      <!-- Enterprise -->
      <div class="pricing-card group relative p-8 rounded-2xl border border-violet-400/30 bg-violet-400/[0.04] hover:border-violet-400/50 transition-all duration-300">
        <div class="text-xs font-bold uppercase tracking-wider text-violet-400 mb-2">Enterprise</div>
        <div class="flex items-baseline gap-1 mb-1">
          <span class="text-4xl font-extrabold text-white">$99</span>
          <span class="text-sm text-slate-500">/month</span>
        </div>
        <p class="text-sm text-slate-500 mb-6">For business and critical production</p>
        <a href="mailto:wahyu@sigarda.com?subject=Enterprise%20Plan" class="block w-full text-center py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-violet-400 to-purple-400 text-white hover:shadow-[0_4px_24px_rgba(139,92,246,0.3)] transition-all duration-300 mb-8">
          Contact Sales
        </a>
        <div class="space-y-3">
          {#each ['Everything in Premium', 'Multi-server management', 'SLA uptime guarantee (99.99%)', 'Priority support (< 4h)', 'Dedicated account manager', 'Custom integrations', 'On-premise deployment', 'Security audit & compliance', 'Multi-team workspaces', 'Phone & video support'] as feat}
            <div class="flex items-center gap-2 text-sm text-slate-400">
              <svg class="w-4 h-4 text-violet-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
              {feat}
            </div>
          {/each}
        </div>
      </div>
    </div>
  </section>

  <!-- CTA -->
  <section class="relative z-10 max-w-3xl mx-auto px-6 lg:px-12 pb-24 text-center">
    <div class="p-10 rounded-3xl border border-white/[0.06] bg-white/[0.04]">
      <h2 class="text-2xl sm:text-3xl font-extrabold tracking-tight mb-4">
        <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">Ready to get started?</span>
      </h2>
      <p class="text-slate-500 mb-8 max-w-md mx-auto">Deploy your first server in minutes. Free, open source, and self-hosted.</p>
      <div class="flex flex-wrap items-center justify-center gap-4">
        <a href="/login" class="group relative inline-flex items-center justify-center px-8 py-3.5 rounded-xl text-sm font-bold text-[#040a14] transition-all duration-300 hover:scale-105">
          <div class="absolute inset-0 bg-gradient-to-r from-sky-400 to-indigo-400 group-hover:from-sky-300 group-hover:to-indigo-300 rounded-xl transition-all duration-300"></div>
          <div class="absolute inset-0 rounded-xl shadow-[0_4px_24px_rgba(56,189,248,0.3)] group-hover:shadow-[0_8px_40px_rgba(56,189,248,0.5)] transition-shadow duration-300"></div>
          <span class="relative z-10 flex items-center gap-2">
            Start Now
            <svg class="w-4 h-4 group-hover:translate-x-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
          </span>
        </a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="inline-flex items-center gap-2 px-8 py-3.5 rounded-xl text-sm font-medium text-slate-300 border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] hover:border-white/20 hover:text-white transition-all duration-300 hover:scale-105">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
          View on GitHub
        </a>
      </div>
    </div>
  </section>

  <!-- Footer -->
  <footer class="relative z-10 border-t border-white/[0.06] py-8">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-12 flex flex-col sm:flex-row items-center justify-between gap-4">
      <p class="text-xs text-slate-500">&copy; {new Date().getFullYear()} <a href="https://sigarda.com" class="text-sky-400/60 hover:text-sky-400 transition-colors">sigarda.com</a> &middot; <a href="https://github.com/bayurstarcool/SigardaPanel/issues" target="_blank" class="text-slate-500 hover:text-sky-400 transition-colors flex items-center gap-1 inline-flex"><svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 5.636l-3.536 3.536m0 5.656l3.536 3.536M9.172 9.172L5.636 5.636m3.536 9.192l-3.536 3.536M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-5 0a4 4 0 11-8 0 4 4 0 018 0z"/></svg>Support</a></p>
      <div class="flex items-center gap-4">
        <a href="/docs" class="text-xs text-slate-500 hover:text-white transition-colors">Docs</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="text-xs text-slate-500 hover:text-white transition-colors">GitHub</a>
        <a href="/login" class="text-xs text-slate-500 hover:text-white transition-colors">Login</a>
      </div>
    </div>
  </footer>
</div>

<style>
  @keyframes fadeInUp { from { opacity: 0; transform: translateY(20px); } to { opacity: 1; transform: translateY(0); } }
  @keyframes mesh-float-1 { 0%, 100% { transform: translate(0, 0); } 25% { transform: translate(50px, -30px); } 50% { transform: translate(-20px, 50px); } 75% { transform: translate(30px, 20px); } }
  @keyframes landing-particle-float { 0% { transform: translateY(100vh); opacity: 0; } 10% { opacity: 1; } 90% { opacity: 1; } 100% { transform: translateY(-20vh); opacity: 0; } }
  @keyframes hero-gradient { 0%, 100% { background-position: 0% 50%; } 50% { background-position: 100% 50%; } }
  @keyframes glow-pulse-sm { 0%, 100% { box-shadow: 0 4px 16px rgba(56,189,248,0.3); } 50% { box-shadow: 0 4px 24px rgba(56,189,248,0.5); } }

  .mesh-orb-1 { animation: mesh-float-1 30s ease-in-out infinite; will-change: transform; background: radial-gradient(circle, rgba(56,189,248,0.15), rgba(99,102,241,0.08), transparent 70%); }
  .landing-particle { animation: landing-particle-float linear infinite; will-change: transform, opacity; }
  .hero-gradient { background-size: 200% 200%; animation: hero-gradient 6s ease infinite; }
  .logo-glow-sm { animation: glow-pulse-sm 3s ease-in-out infinite; }
</style>
