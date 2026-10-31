<script lang="ts">
  import { Mail, Check, Minus } from 'lucide-svelte';
  import Logo from '$components/Logo.svelte';

  const plans = [
    {
      name: 'Community',
      price: 'Free',
      period: '',
      desc: 'Self-hosted, open source',
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

  const faqs = [
    { q: 'Can I try Premium before buying?', a: 'Yes! Contact us for a 14-day free trial of the Premium plan.' },
    { q: 'What payment methods do you accept?', a: 'We accept bank transfer, PayPal, and credit cards for annual plans.' },
    { q: 'Can I upgrade or downgrade anytime?', a: 'Yes, you can change your plan at any time. Changes take effect immediately with prorated billing.' },
    { q: 'Is there a discount for annual payment?', a: 'Yes! Annual plans get 2 months free (save ~17%).' },
    { q: 'What does "managed" mean in Premium?', a: 'We handle updates, patches, monitoring, and provide priority support. You focus on your business.' },
  ];

  const GITHUB = 'https://github.com/bayurstarcool/SigardaPanel';
</script>

<svelte:head>
  <title>Pricing — SigardaPanel</title>
  <meta name="description" content="Simple pricing for SigardaPanel. Start free, upgrade when you need enterprise features." />
</svelte:head>

<div class="min-h-dvh bg-bg text-fg">
  <nav class="sticky top-0 z-40 border-b border-line bg-bg">
    <div class="mx-auto flex h-14 max-w-6xl items-center justify-between gap-2 px-4 sm:px-6">
      <a href="/" aria-label="SigardaPanel home"><Logo size={26} withText /></a>
      <div class="flex items-center gap-0.5">
        <a href="/#features" class="btn btn-ghost btn-sm max-sm:hidden">Features</a>
        <a href="/pricing" class="btn btn-ghost btn-sm bg-surface-2 text-fg" aria-current="page">Pricing</a>
        <a href="/docs" class="btn btn-ghost btn-sm max-sm:hidden">Docs</a>
        <a href={GITHUB} target="_blank" rel="noopener" class="btn btn-ghost btn-sm btn-icon max-md:hidden" aria-label="GitHub">
          <svg class="size-4" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" /></svg>
        </a>
        <a href="/login" class="btn btn-primary btn-sm ml-1.5">Sign In</a>
      </div>
    </div>
  </nav>

  <section class="mx-auto max-w-3xl px-4 pt-16 pb-12 text-center sm:px-6 sm:pt-20">
    <span class="badge badge-accent mb-5"><span class="dot"></span>Simple &amp; Transparent</span>
    <h1 class="text-4xl font-semibold tracking-tight text-fg sm:text-5xl">Simple pricing</h1>
    <p class="mx-auto mt-4 max-w-xl text-base leading-relaxed text-muted sm:text-lg">
      Start free, upgrade when you need enterprise features. No hidden fees.
    </p>
  </section>

  <section class="mx-auto max-w-6xl px-4 pb-20 sm:px-6">
    <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
      {#each plans as plan (plan.name)}
        <div class="card flex flex-col p-6 {plan.popular ? 'border-accent ring-3 ring-accent/20' : ''}">
          <div class="flex items-center justify-between">
            <div class="text-sm font-semibold {plan.popular ? 'text-accent' : 'text-fg'}">{plan.name}</div>
            {#if plan.popular}<span class="badge badge-accent">Popular</span>{/if}
          </div>
          <div class="mt-4 flex items-baseline gap-1">
            <span class="text-4xl font-semibold tracking-tight text-fg tabular-nums">{plan.price}</span>
            {#if plan.period}<span class="text-sm text-subtle">{plan.period}</span>{/if}
          </div>
          <p class="mt-1 text-[13px] text-muted">{plan.desc}</p>
          <a href={plan.ctaHref} class="btn mt-6 w-full {plan.popular ? 'btn-primary' : 'btn-secondary'}">{plan.cta}</a>
          <ul class="mt-6 space-y-2.5 border-t border-line pt-6">
            {#each plan.features as feat (feat.text)}
              <li class="flex items-start gap-2 text-[13px] {feat.included ? 'text-muted' : 'text-subtle'}">
                {#if feat.included}
                  <Check size={14} class="mt-0.5 shrink-0 text-accent" />
                {:else}
                  <Minus size={14} class="mt-0.5 shrink-0 text-subtle" />
                {/if}
                <span class={feat.included ? '' : 'line-through decoration-line-strong'}>{feat.text}</span>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
  </section>

  <section class="border-t border-line">
    <div class="mx-auto max-w-3xl px-4 py-20 sm:px-6">
      <h2 class="mb-8 text-2xl font-semibold tracking-tight text-fg">Frequently asked questions</h2>
      <div class="card divide-y divide-line">
        {#each faqs as faq (faq.q)}
          <div class="p-5">
            <h3 class="text-sm font-semibold text-fg">{faq.q}</h3>
            <p class="mt-1.5 text-[13px] leading-relaxed text-muted">{faq.a}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <section class="border-t border-line">
    <div class="mx-auto max-w-3xl px-4 py-20 text-center sm:px-6">
      <h2 class="text-2xl font-semibold tracking-tight text-fg">Questions?</h2>
      <p class="mx-auto mt-3 max-w-md text-muted">Contact us and we'll help you choose the right plan.</p>
      <a href="mailto:wahyu@sigarda.com?subject=SigardaPanel%20Pricing" class="btn btn-primary mt-8 h-10 px-5">
        <Mail size={16} /> Contact Sales
      </a>
    </div>
  </section>

  <footer class="border-t border-line">
    <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-4 py-8 sm:flex-row sm:px-6">
      <p class="text-xs text-subtle">
        &copy; {new Date().getFullYear()} <a href="https://sigarda.com" class="hover:text-fg">sigarda.com</a>
        <span class="mx-1.5">·</span>
        <a href="{GITHUB}/issues" target="_blank" rel="noopener" class="hover:text-fg">Support</a>
      </p>
      <div class="flex items-center gap-4 text-xs text-subtle">
        <a href="/" class="hover:text-fg">Home</a>
        <a href="/docs" class="hover:text-fg">Docs</a>
        <a href={GITHUB} target="_blank" rel="noopener" class="hover:text-fg">GitHub</a>
        <a href="/login" class="hover:text-fg">Login</a>
      </div>
    </div>
  </footer>
</div>
