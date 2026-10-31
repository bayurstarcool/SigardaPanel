<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { Shield, Copy, Check, ArrowRight, Server, Globe, Activity, Key, CreditCard, LayoutDashboard } from 'lucide-svelte';
  import StatCard from '$components/StatCard.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { onMount } from 'svelte';

  let token = $state('');
  let copied = $state(false);
  let servers = $state(0);
  let sites = $state(0);

  onMount(() => {
    token = auth.token || '';
    // Fetch real stats — the HttpOnly session cookie authenticates via the
    // proxy; an explicit Bearer header is only needed for impersonation.
    if (auth.user) {
      fetch('/api/proxy/servers')
        .then(r => r.json())
        .then(d => { if (d.data) servers = d.data.count || (d.data.servers || []).length || 0; })
        .catch(() => {});
      fetch('/api/proxy/sites?limit=100')
        .then(r => r.json())
        .then(d => { if (d.data) sites = d.data.count || d.data.total_count || (d.data.sites || []).length || 0; })
        .catch(() => {});
    }
  });

  function copyToken() {
    navigator.clipboard.writeText(token);
    copied = true;
    setTimeout(() => copied = false, 2000);
  }

  const links = [
    { href: '/member/api-keys', title: 'API Keys', desc: 'Manage your API tokens and access.', icon: Key },
    { href: '/member/billing', title: 'Billing', desc: 'View plans and manage subscription.', icon: CreditCard },
    { href: '/dashboard', title: 'Panel Dashboard', desc: 'Go to VPS management panel.', icon: LayoutDashboard },
  ];
</script>

<svelte:head>
  <title>Member Dashboard — SigardaPanel</title>
</svelte:head>

<PageHeader title="Welcome back, {auth.user?.username ?? ''}" description="Manage your account and API access." />

<div class="space-y-6">
  <!-- Plan -->
  <section class="card card-body">
    <div class="flex flex-wrap items-center gap-2">
      <Shield size={16} class="text-accent" />
      <span class="text-sm font-semibold text-fg">Community Plan</span>
      <span class="badge badge-success"><span class="dot"></span>Active</span>
    </div>
    <p class="mt-2 text-[13px] text-muted">You're using the free Community plan. Upgrade for multi-server management and priority support.</p>
    <a href="/pricing" class="btn btn-secondary btn-sm mt-4">View plans <ArrowRight size={14} /></a>
  </section>

  <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <StatCard label="Servers" value={servers} icon={Server as any} />
    <StatCard label="Sites" value={sites} icon={Globe as any} />
    <StatCard label="Plan" value="Free" icon={Shield as any} tone="accent" />
    <StatCard label="Status" value="Active" icon={Activity as any} tone="success" />
  </div>

  <!-- API token -->
  <section class="card">
    <div class="card-header">
      <h2 class="card-title">API token</h2>
    </div>
    <div class="card-body">
      <p class="mb-3 text-[13px] text-muted">Use this token to authenticate API requests and CLI commands.</p>
      <div class="flex items-center gap-2">
        <div class="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] text-muted">
          {token || 'No token available'}
        </div>
        <button onclick={copyToken} class="btn btn-secondary btn-icon" aria-label="Copy token" title="Copy token" disabled={!token}>
          {#if copied}<Check size={14} class="text-success" />{:else}<Copy size={14} />{/if}
        </button>
      </div>
      <div class="mt-4 rounded-lg border border-line bg-surface-2 p-3">
        <p class="mb-1.5 text-xs text-subtle">Quick start with CLI:</p>
        <code class="font-mono text-[13px] break-all text-fg">sigardapanel login --api-url https://panel.sigarda.dev</code>
      </div>
    </div>
  </section>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
    {#each links as l (l.href)}
      <a href={l.href} class="card group p-5 transition-colors duration-150 hover:border-line-strong hover:bg-surface-2">
        <div class="mb-3 flex size-9 items-center justify-center rounded-lg bg-accent/10 text-accent">
          <l.icon size={16} />
        </div>
        <h3 class="text-sm font-semibold text-fg">{l.title}</h3>
        <p class="mt-1 text-[13px] text-muted">{l.desc}</p>
      </a>
    {/each}
  </div>
</div>
