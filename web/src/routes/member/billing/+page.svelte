<script lang="ts">
  import { CreditCard, Check, ArrowRight } from 'lucide-svelte';
  import PageHeader from '$components/PageHeader.svelte';

  const plans = [
    {
      name: 'Community',
      price: 'Free',
      desc: 'Self-hosted, open source',
      current: true,
      features: ['Single server', 'Unlimited sites', 'Docker, Firewall, Redis', 'SSL auto-renewal', 'Community support'],
    },
    {
      name: 'Premium',
      price: '$29/mo',
      desc: 'Managed with priority support',
      current: false,
      features: ['Multi-server', 'Managed updates', 'Priority support (< 24h)', 'White-label', 'Email & chat support'],
    },
    {
      name: 'Enterprise',
      price: '$99/mo',
      desc: 'For business & critical production',
      current: false,
      features: ['Multi-server', 'SLA 99.99%', 'Priority support (< 4h)', 'Dedicated manager', 'Phone & video support'],
    },
  ];
</script>

<svelte:head>
  <title>Billing — SigardaPanel</title>
</svelte:head>

<PageHeader title="Billing" description="Manage your subscription and plan." />

<div class="space-y-6">
  <section class="card card-body flex items-start gap-3">
    <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-success/10 text-success"><CreditCard size={16} /></span>
    <div>
      <h2 class="text-sm font-semibold text-fg">Current plan: Community</h2>
      <p class="mt-0.5 text-[13px] text-muted">You're on the free plan. No billing required.</p>
    </div>
  </section>

  <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
    {#each plans as plan (plan.name)}
      <div class="card flex flex-col p-5 {plan.current ? 'border-accent ring-3 ring-accent/20' : ''}">
        <div class="flex items-center justify-between">
          <div class="text-sm font-semibold text-fg">{plan.name}</div>
          {#if plan.current}<span class="badge badge-accent">Current</span>{/if}
        </div>
        <div class="mt-3 text-2xl font-semibold tracking-tight text-fg tabular-nums">{plan.price}</div>
        <p class="mt-0.5 text-[13px] text-muted">{plan.desc}</p>
        {#if !plan.current}
          <a href="mailto:wahyu@sigarda.com?subject={plan.name}%20Plan" class="btn btn-secondary btn-sm mt-4 w-full">Upgrade</a>
        {/if}
        <ul class="mt-4 space-y-2 border-t border-line pt-4">
          {#each plan.features as feat (feat)}
            <li class="flex items-center gap-2 text-[13px] text-muted">
              <Check size={14} class="shrink-0 {plan.current ? 'text-accent' : 'text-subtle'}" />
              {feat}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>

  <section class="card card-body flex flex-col items-center gap-3 text-center sm:flex-row sm:justify-between sm:text-left">
    <p class="text-sm text-muted">Need a custom plan or have questions?</p>
    <a href="mailto:wahyu@sigarda.com?subject=SigardaPanel%20Billing" class="btn btn-primary btn-sm">
      Contact Sales <ArrowRight size={14} />
    </a>
  </section>
</div>
