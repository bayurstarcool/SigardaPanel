<script lang="ts">
  import { Shield, Server, Globe, Database, HardDrive, ShieldCheck, Activity, Box, Zap, GitBranch, Search, Bell, Mail, Check, ArrowRight, Headphones, Clock, Users, Lock, Cloud, Code } from 'lucide-svelte';

  const plans = [
    {
      name: 'Community',
      price: 'Free',
      period: '',
      desc: 'Self-hosted, open source',
      color: 'emerald',
      cta: 'Get Started',
      ctaHref: '/login',
      popular: false,
      features: [
        { text: 'Single server management', included: true },
        { text: 'Unlimited sites', included: true },
        { text: 'Docker, Firewall, Redis', included: true },
        { text: 'SSL auto-renewal', included: true },
        { text: 'Git deploy & rollback', included: true },
        { text: 'Database management', included: true },
        { text: 'Automated backups', included: true },
        { text: 'CLI + Web dashboard', included: true },
        { text: 'Community support', included: true },
        { text: 'Multi-server management', included: false },
        { text: 'Priority support', included: false },
        { text: 'Custom branding', included: false },
      ]
    },
    {
      name: 'Premium',
      price: '$29',
      period: '/month',
      desc: 'Managed panel with priority support',
      color: 'sky',
      cta: 'Contact Sales',
      ctaHref: 'mailto:wahyu@sigarda.com?subject=Premium%20Plan',
      popular: true,
      features: [
        { text: 'Everything in Community', included: true },
        { text: 'Multi-server management', included: true },
        { text: 'Managed updates & patches', included: true },
        { text: 'Priority support (< 24h)', included: true },
        { text: 'Custom branding (white-label)', included: true },
        { text: 'Advanced monitoring', included: true },
        { text: 'API rate limit increase', included: true },
        { text: 'Email & chat support', included: true },
        { text: 'Monthly health reports', included: true },
        { text: 'Staging environment', included: true },
        { text: 'SLA uptime guarantee', included: false },
        { text: 'Dedicated account manager', included: false },
      ]
    },
    {
      name: 'Enterprise',
      price: '$99',
      period: '/month',
      desc: 'For business and critical production',
      color: 'violet',
      cta: 'Contact Sales',
      ctaHref: 'mailto:wahyu@sigarda.com?subject=Enterprise%20Plan',
      popular: false,
      features: [
        { text: 'Everything in Premium', included: true },
        { text: 'SLA uptime guarantee (99.99%)', included: true },
        { text: 'Priority support (< 4h)', included: true },
        { text: 'Dedicated account manager', included: true },
        { text: 'Custom integrations', included: true },
        { text: 'On-premise deployment', included: true },
        { text: 'Security audit & compliance', included: true },
        { text: 'Multi-team workspaces', included: true },
        { text: 'Phone & video support', included: true },
        { text: 'Custom SLA contracts', included: true },
        { text: '24/7 emergency support', included: true },
        { text: 'Source code access', included: true },
      ]
    }
  ];
</script>

<svelte:head>
  <title>Pricing — SigardaPanel</title>
  <meta name="description" content="Simple pricing for SigardaPanel. Start free, upgrade when you need enterprise features." />
</svelte:head>

<div class="min-h-screen bg-[#060d1f] text-white">
  <!-- Fixed Navbar -->
  <nav class="fixed top-0 left-0 right-0 z-50 bg-[#060d1f]/80 backdrop-blur-xl border-b border-white/[0.06]">
    <div class="flex items-center justify-between px-6 lg:px-12 py-4 max-w-7xl mx-auto">
      <a href="/" class="flex items-center gap-3">
        <div class="w-9 h-9 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_4px_16px_rgba(56,189,248,0.3)]">
          <Shield size={18} class="text-white" />
        </div>
        <span class="text-lg font-bold tracking-tight hidden sm:block">SigardaPanel</span>
      </a>
      <div class="flex items-center gap-1">
        <a href="/#features" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all">Features</a>
        <a href="/pricing" class="px-3 py-2 rounded-lg text-sm text-white bg-white/[0.08] transition-all">Pricing</a>
        <a href="/docs" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden sm:block">Docs</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="px-3 py-2 rounded-lg text-sm text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden md:block">
          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
        </a>
        <div class="w-px h-5 bg-white/[0.08] mx-1 hidden sm:block"></div>
        <a href="/login" class="px-4 py-2 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-500 text-white hover:shadow-[0_0_24px_rgba(56,189,248,0.4)] transition-all duration-300 hover:scale-105">
          Sign In
        </a>
      </div>
    </div>
  </nav>

  <div class="h-16"></div>

  <!-- Hero -->
  <section class="relative max-w-5xl mx-auto px-6 lg:px-12 pt-16 pb-12 text-center">
    <div class="inline-flex items-center gap-2 px-4 py-1.5 rounded-full border border-sky-400/20 bg-sky-400/5 text-sky-300 text-xs font-medium mb-6">
      <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-pulse"></span>
      Simple & Transparent
    </div>
    <h1 class="text-4xl sm:text-5xl lg:text-6xl font-extrabold tracking-tight mb-4">
      <span class="bg-gradient-to-r from-white via-sky-200 to-indigo-300 bg-clip-text text-transparent">Simple Pricing</span>
    </h1>
    <p class="text-lg text-slate-400 max-w-2xl mx-auto leading-relaxed">
      Start free, upgrade when you need enterprise features. No hidden fees.
    </p>
  </section>

  <!-- Pricing Cards -->
  <section class="max-w-6xl mx-auto px-6 lg:px-12 pb-20">
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      {#each plans as plan}
        <div class="relative p-8 rounded-2xl border {plan.popular ? 'border-sky-400/30 bg-sky-400/[0.04]' : 'border-white/[0.06] bg-white/[0.02]'} hover:border-white/[0.12] transition-all duration-300">
          {#if plan.popular}
            <div class="absolute top-0 left-1/2 -translate-x-1/2 -translate-y-1/2 px-3 py-1 rounded-full bg-sky-400 text-[0.65rem] font-bold text-[#040a14] uppercase tracking-wider">Popular</div>
          {/if}
          <div class="text-xs font-bold uppercase tracking-wider text-{plan.color}-400 mb-2">{plan.name}</div>
          <div class="flex items-baseline gap-1 mb-1">
            <span class="text-4xl font-extrabold text-white">{plan.price}</span>
            {#if plan.period}
              <span class="text-sm text-slate-500">{plan.period}</span>
            {/if}
          </div>
          <p class="text-sm text-slate-500 mb-6">{plan.desc}</p>
          <a href={plan.ctaHref} class="block w-full text-center py-3 rounded-xl text-sm font-semibold {plan.popular ? 'bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)]' : 'border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-white'} transition-all duration-300 mb-8">
            {plan.cta}
          </a>
          <div class="space-y-3">
            {#each plan.features as feat}
              <div class="flex items-center gap-2 text-sm {feat.included ? 'text-slate-300' : 'text-slate-600'}">
                {#if feat.included}
                  <Check size={16} class="text-{plan.color}-400 shrink-0" />
                {:else}
                  <div class="w-4 h-4 rounded-full border border-slate-700 shrink-0"></div>
                {/if}
                {feat.text}
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- FAQ -->
  <section class="max-w-3xl mx-auto px-6 lg:px-12 pb-20">
    <h2 class="text-2xl font-extrabold text-center mb-10">
      <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">Frequently Asked Questions</span>
    </h2>
    <div class="space-y-4">
      {#each [
        { q: 'Can I try Premium before buying?', a: 'Yes! Contact us for a 14-day free trial of the Premium plan.' },
        { q: 'What payment methods do you accept?', a: 'We accept bank transfer, PayPal, and credit cards for annual plans.' },
        { q: 'Can I upgrade or downgrade anytime?', a: 'Yes, you can change your plan at any time. Changes take effect immediately with prorated billing.' },
        { q: 'Is there a discount for annual payment?', a: 'Yes! Annual plans get 2 months free (save ~17%).' },
        { q: 'What does "managed" mean in Premium?', a: 'We handle updates, patches, monitoring, and provide priority support. You focus on your business.' },
      ] as faq}
        <div class="p-5 rounded-xl border border-white/[0.06] bg-white/[0.02]">
          <h3 class="text-sm font-semibold text-white mb-2">{faq.q}</h3>
          <p class="text-sm text-slate-400">{faq.a}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- CTA -->
  <section class="max-w-3xl mx-auto px-6 lg:px-12 pb-20 text-center">
    <div class="p-10 rounded-3xl border border-white/[0.06] bg-white/[0.02]">
      <h2 class="text-2xl font-extrabold mb-4">
        <span class="bg-gradient-to-r from-white to-slate-300 bg-clip-text text-transparent">Questions?</span>
      </h2>
      <p class="text-slate-500 mb-8 max-w-md mx-auto">Contact us and we'll help you choose the right plan.</p>
      <a href="mailto:wahyu@sigarda.com?subject=SigardaPanel%20Pricing" class="inline-flex items-center gap-2 px-8 py-3.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all duration-300 hover:scale-105">
        <Mail size={16} />
        Contact Sales
      </a>
    </div>
  </section>

  <!-- Footer -->
  <footer class="border-t border-white/[0.06] py-8">
    <div class="max-w-7xl mx-auto px-6 lg:px-12 flex flex-col sm:flex-row items-center justify-between gap-4">
      <p class="text-xs text-slate-500">&copy; {new Date().getFullYear()} <a href="https://sigarda.com" class="text-sky-400/60 hover:text-sky-400 transition-colors">sigarda.com</a> &middot; <a href="https://github.com/bayurstarcool/SigardaPanel/issues" target="_blank" class="text-slate-500 hover:text-sky-400 transition-colors">Support</a></p>
      <div class="flex items-center gap-4">
        <a href="/" class="text-xs text-slate-500 hover:text-white transition-colors">Home</a>
        <a href="/docs" class="text-xs text-slate-500 hover:text-white transition-colors">Docs</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="text-xs text-slate-500 hover:text-white transition-colors">GitHub</a>
        <a href="/login" class="text-xs text-slate-500 hover:text-white transition-colors">Login</a>
      </div>
    </div>
  </footer>
</div>
