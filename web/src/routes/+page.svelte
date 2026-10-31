<script lang="ts">
  import { theme } from '$lib/theme.svelte';
  import Logo from '$components/Logo.svelte';
  import {
    Server, Globe, Database, HardDrive, Shield, Activity, Box, ShieldCheck, Zap, GitBranch, Search, Bell,
    ArrowRight, Sun, Moon, Check, Monitor, Cpu, LifeBuoy,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  // Real build version from the API (public endpoint); the badge just omits it
  // if the request fails.
  let version = $state('');
  onMount(() => {
    fetch('/api/v1/version')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        const v = d?.data?.version;
        if (v && v !== 'dev') version = v;
      })
      .catch(() => {});
  });

  const features = [
    { icon: Server, title: 'Multi-Server', desc: 'Manage unlimited VPS from one dashboard.' },
    { icon: Globe, title: 'Site Deploy', desc: 'PHP, Node.js, Python, Go, Docker — deploy any stack.' },
    { icon: Database, title: 'Databases', desc: 'MySQL & PostgreSQL with user management.' },
    { icon: HardDrive, title: 'Backups', desc: 'Scheduled backups to S3, R2, B2, Wasabi.' },
    { icon: ShieldCheck, title: 'SSL Auto', desc: "Auto-renewing Let's Encrypt certificates." },
    { icon: Activity, title: 'Monitoring', desc: 'CPU, memory, disk, network — real-time.' },
    { icon: Box, title: 'Docker', desc: 'Containers, images, volumes, networks.' },
    { icon: Shield, title: 'Firewall', desc: 'UFW rules, presets, IP blocking.' },
    { icon: Zap, title: 'Redis', desc: 'Stats, flush, info — full management.' },
    { icon: GitBranch, title: 'Git Deploy', desc: 'Branches, rollback, auto-deploy.' },
    { icon: Search, title: 'Global Search', desc: 'Cmd+K search across all resources.' },
    { icon: Bell, title: 'Alerts', desc: 'Telegram, Discord, Email notifications.' },
  ];

  const stats = [
    { value: '55+', label: 'CLI Commands' },
    { value: '150+', label: 'API Endpoints' },
    { value: '99.9%', label: 'Uptime' },
  ];

  const tech = ['Go', 'SvelteKit', 'SQLite', 'UFW', 'Docker', "Let's Encrypt"];

  const arch = [
    { icon: Monitor, title: 'Panel', sub: 'Web Dashboard' },
    { icon: Cpu, title: 'Agent', sub: 'Per-VPS Daemon' },
    { icon: Server, title: 'VPS', sub: 'Your Server' },
  ];
  const archLinks = ['HTTPS / Token', 'Local Execute'];

  const plans = [
    {
      name: 'Community', price: 'Free', period: '', blurb: 'Self-hosted, open source',
      cta: 'Get Started', href: '/login', featured: false,
      items: ['Single server management', 'Unlimited sites', 'Docker, Firewall, Redis', 'SSL auto-renewal', 'Git deploy & rollback', 'Database management', 'Automated backups', 'CLI + Web dashboard', 'Community support'],
    },
    {
      name: 'Premium', price: '$29', period: '/month', blurb: 'Managed panel with priority support',
      cta: 'Contact Sales', href: 'mailto:wahyu@sigarda.com?subject=Premium%20Plan', featured: true,
      items: ['Everything in Community', 'Multi-server management', 'Managed updates & patches', 'Priority support (< 24h)', 'Custom branding (white-label)', 'Advanced monitoring', 'API rate limit increase', 'Email & chat support', 'Monthly health reports', 'Staging environment'],
    },
    {
      name: 'Enterprise', price: '$99', period: '/month', blurb: 'For business and critical production',
      cta: 'Contact Sales', href: 'mailto:wahyu@sigarda.com?subject=Enterprise%20Plan', featured: false,
      items: ['Everything in Premium', 'Multi-server management', 'SLA uptime guarantee (99.99%)', 'Priority support (< 4h)', 'Dedicated account manager', 'Custom integrations', 'On-premise deployment', 'Security audit & compliance', 'Multi-team workspaces', 'Phone & video support'],
    },
  ];

  const GITHUB = 'https://github.com/bayurstarcool/SigardaPanel';
</script>

<svelte:head>
  <title>SigardaPanel — Modern VPS Control Panel</title>
  <meta name="description" content="Manage servers, sites, Docker, firewall, databases, backups — all from one dashboard." />
</svelte:head>

{#snippet githubIcon(cls: string)}
  <svg class={cls} fill="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" /></svg>
{/snippet}

<div class="min-h-dvh bg-bg text-fg">
  <!-- Nav -->
  <nav class="sticky top-0 z-40 border-b border-line bg-bg">
    <div class="mx-auto flex h-14 max-w-6xl items-center justify-between gap-2 px-4 sm:px-6">
      <a href="/" aria-label="SigardaPanel home"><Logo size={26} withText /></a>
      <div class="flex items-center gap-0.5">
        <a href="#features" class="btn btn-ghost btn-sm max-sm:hidden">Features</a>
        <a href="#architecture" class="btn btn-ghost btn-sm max-md:hidden">Architecture</a>
        <a href="#pricing" class="btn btn-ghost btn-sm">Pricing</a>
        <a href="/docs" class="btn btn-ghost btn-sm max-sm:hidden">Docs</a>
        <a href={GITHUB} target="_blank" rel="noopener" class="btn btn-ghost btn-sm btn-icon max-md:hidden" aria-label="GitHub">
          {@render githubIcon('size-4')}
        </a>
        <button
          onclick={() => theme.toggle()}
          class="btn btn-ghost btn-sm btn-icon"
          aria-label={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
          title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {#if theme.isDark}<Sun size={16} />{:else}<Moon size={16} />{/if}
        </button>
        <a href="/login" class="btn btn-primary btn-sm ml-1.5">Sign In</a>
      </div>
    </div>
  </nav>

  <!-- Hero -->
  <section class="mx-auto max-w-3xl px-4 pt-20 pb-20 text-center sm:px-6 sm:pt-28 sm:pb-24">
    <span class="badge badge-accent mb-6"><span class="dot"></span>Open Source · Self-hosted{version ? ` · v${version}` : ''}</span>
    <h1 class="text-4xl font-semibold tracking-tight text-fg sm:text-6xl">SigardaPanel</h1>
    <p class="mx-auto mt-5 max-w-xl text-base leading-relaxed text-muted sm:text-lg">
      Modern VPS Control Panel. Manage servers, Docker, firewall, databases, backups — all from one dashboard.
    </p>
    <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
      <a href="/login" class="btn btn-primary h-10 px-5">Get Started <ArrowRight size={16} /></a>
      <a href="/docs" class="btn btn-secondary h-10 px-5">View Documentation</a>
    </div>
    <div class="mt-10 flex flex-wrap items-center justify-center gap-2">
      {#each tech as t (t)}
        <span class="badge">{t}</span>
      {/each}
    </div>
  </section>

  <!-- Features -->
  <section id="features" class="scroll-mt-16 border-t border-line">
    <div class="mx-auto max-w-6xl px-4 py-20 sm:px-6">
      <div class="mb-10 max-w-xl">
        <h2 class="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">Everything you need</h2>
        <p class="mt-2 text-muted">A complete toolkit for managing your infrastructure.</p>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {#each features as f (f.title)}
          <div class="card p-5 transition-colors duration-150 hover:border-line-strong">
            <div class="mb-3 flex size-9 items-center justify-center rounded-lg bg-accent/10 text-accent">
              <f.icon size={16} />
            </div>
            <h3 class="text-sm font-semibold text-fg">{f.title}</h3>
            <p class="mt-1 text-[13px] leading-relaxed text-muted">{f.desc}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- Architecture -->
  <section id="architecture" class="scroll-mt-16 border-t border-line bg-surface">
    <div class="mx-auto max-w-6xl px-4 py-20 sm:px-6">
      <div class="mb-10 max-w-xl">
        <h2 class="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">How it works</h2>
        <p class="mt-2 text-muted">Simple, secure architecture for multi-server management.</p>
      </div>
      <div class="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
        {#each arch as a, i (a.title)}
          <div class="flex flex-1 items-center gap-3 rounded-xl border border-line bg-bg p-4">
            <div class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-fg">
              <a.icon size={18} />
            </div>
            <div>
              <div class="text-sm font-semibold text-fg">{a.title}</div>
              <div class="text-xs text-subtle">{a.sub}</div>
            </div>
          </div>
          {#if i < archLinks.length}
            <div class="flex shrink-0 items-center justify-center gap-2 text-subtle sm:flex-col sm:gap-1">
              <span class="font-mono text-[11px]">{archLinks[i]}</span>
              <ArrowRight size={14} class="rotate-90 sm:rotate-0" />
            </div>
          {/if}
        {/each}
      </div>

      <div class="mt-12 grid grid-cols-1 gap-3 sm:grid-cols-3">
        {#each stats as s (s.label)}
          <div class="rounded-xl border border-line bg-bg p-5">
            <div class="stat-label">{s.label}</div>
            <div class="stat-value text-3xl">{s.value}</div>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- Pricing -->
  <section id="pricing" class="scroll-mt-16 border-t border-line">
    <div class="mx-auto max-w-6xl px-4 py-20 sm:px-6">
      <div class="mb-10 max-w-xl">
        <h2 class="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">Simple pricing</h2>
        <p class="mt-2 text-muted">Start free, upgrade when you need enterprise features.</p>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
        {#each plans as p (p.name)}
          <div class="card flex flex-col p-6 {p.featured ? 'border-accent ring-3 ring-accent/20' : ''}">
            <div class="flex items-center justify-between">
              <div class="text-sm font-semibold {p.featured ? 'text-accent' : 'text-fg'}">{p.name}</div>
              {#if p.featured}<span class="badge badge-accent">Popular</span>{/if}
            </div>
            <div class="mt-4 flex items-baseline gap-1">
              <span class="text-4xl font-semibold tracking-tight text-fg tabular-nums">{p.price}</span>
              {#if p.period}<span class="text-sm text-subtle">{p.period}</span>{/if}
            </div>
            <p class="mt-1 text-[13px] text-muted">{p.blurb}</p>
            <a href={p.href} class="btn mt-6 w-full {p.featured ? 'btn-primary' : 'btn-secondary'}">{p.cta}</a>
            <ul class="mt-6 space-y-2.5 border-t border-line pt-6">
              {#each p.items as it (it)}
                <li class="flex items-start gap-2 text-[13px] text-muted">
                  <Check size={14} class="mt-0.5 shrink-0 text-accent" />{it}
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- CTA -->
  <section class="border-t border-line">
    <div class="mx-auto max-w-3xl px-4 py-20 text-center sm:px-6">
      <h2 class="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">Ready to get started?</h2>
      <p class="mx-auto mt-3 max-w-md text-muted">Deploy your first server in minutes. Free, open source, and self-hosted.</p>
      <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
        <a href="/login" class="btn btn-primary h-10 px-5">Start Now <ArrowRight size={16} /></a>
        <a href={GITHUB} target="_blank" rel="noopener" class="btn btn-secondary h-10 px-5">
          {@render githubIcon('size-4')} View on GitHub
        </a>
      </div>
    </div>
  </section>

  <!-- Footer -->
  <footer class="border-t border-line">
    <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-4 py-8 sm:flex-row sm:px-6">
      <p class="flex flex-wrap items-center gap-x-2 text-xs text-subtle">
        <span>&copy; {new Date().getFullYear()} <a href="https://sigarda.com" class="hover:text-fg">sigarda.com</a></span>
        <span aria-hidden="true">·</span>
        <a href="{GITHUB}/issues" target="_blank" rel="noopener" class="inline-flex items-center gap-1 hover:text-fg"><LifeBuoy size={12} />Support</a>
      </p>
      <div class="flex items-center gap-4 text-xs text-subtle">
        <a href="/docs" class="hover:text-fg">Docs</a>
        <a href={GITHUB} target="_blank" rel="noopener" class="hover:text-fg">GitHub</a>
        <a href="/login" class="hover:text-fg">Login</a>
      </div>
    </div>
  </footer>
</div>
