<script lang="ts">
  import { CreditCard, Check, ExternalLink, ArrowRight } from 'lucide-svelte';

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

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold text-white mb-1">Billing</h1>
    <p class="text-sm text-slate-500">Manage your subscription and plan.</p>
  </div>

  <!-- Current Plan -->
  <div class="p-6 rounded-2xl border border-emerald-400/20 bg-emerald-400/[0.04]">
    <div class="flex items-center gap-2 mb-2">
      <CreditCard size={16} class="text-emerald-400" />
      <span class="text-sm font-semibold text-emerald-400">Current Plan: Community</span>
    </div>
    <p class="text-xs text-slate-400">You're on the free plan. No billing required.</p>
  </div>

  <!-- Plans -->
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
    {#each plans as plan}
      <div class="relative p-6 rounded-2xl border {plan.current ? 'border-emerald-400/30 bg-emerald-400/[0.04]' : 'border-white/[0.06] bg-white/[0.02]'} transition-all">
        {#if plan.current}
          <div class="absolute top-3 right-3 px-2 py-0.5 rounded-full bg-emerald-500/10 text-[0.6rem] font-bold text-emerald-400">Current</div>
        {/if}
        <div class="text-xs font-bold uppercase tracking-wider text-slate-500 mb-2">{plan.name}</div>
        <div class="text-2xl font-extrabold text-white mb-1">{plan.price}</div>
        <p class="text-xs text-slate-500 mb-4">{plan.desc}</p>
        {#if !plan.current}
          <a href="mailto:wahyu@sigarda.com?subject={plan.name}%20Plan" class="block w-full text-center py-2.5 rounded-xl text-xs font-semibold border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-white transition-all mb-4">
            Upgrade
          </a>
        {/if}
        <div class="space-y-2">
          {#each plan.features as feat}
            <div class="flex items-center gap-2 text-xs text-slate-400">
              <Check size={12} class="{plan.current ? 'text-emerald-400' : 'text-slate-600'} shrink-0" />
              {feat}
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>

  <!-- Contact -->
  <div class="p-6 rounded-2xl border border-white/[0.06] bg-white/[0.02] text-center">
    <p class="text-sm text-slate-400 mb-4">Need a custom plan or have questions?</p>
    <a href="mailto:wahyu@sigarda.com?subject=SigardaPanel%20Billing" class="inline-flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all">
      Contact Sales <ArrowRight size={14} />
    </a>
  </div>
</div>
