<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { Key, Copy, Check } from 'lucide-svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { onMount } from 'svelte';

  let token = $state('');
  let copied = $state(false);
  let keys = $state<any[]>([]);

  onMount(() => {
    token = auth.token || '';
    keys = [
      { id: 1, name: 'Default', token: token, created: '2026-07-12', lastUsed: 'Just now', active: true },
    ];
  });

  function copyToken(t: string) {
    navigator.clipboard.writeText(t);
    copied = true;
    setTimeout(() => copied = false, 2000);
  }

  function maskToken(t: string) {
    if (!t) return '••••••••••••••••';
    return t.slice(0, 8) + '...' + t.slice(-4);
  }

  const usage = [
    { label: 'CLI Login', cmd: 'sigardapanel login --api-url https://panel.sigarda.dev' },
    { label: 'API Request', cmd: 'curl -H "Authorization: Bearer YOUR_TOKEN" https://panel.sigarda.dev/api/v1/servers' },
    { label: 'Environment Variable', cmd: 'export SIGARDAPANEL_TOKEN=YOUR_TOKEN' },
  ];
</script>

<svelte:head>
  <title>API Keys — SigardaPanel</title>
</svelte:head>

<PageHeader title="API keys" description="Manage your API tokens for CLI and integrations." />

<div class="space-y-6">
  <section class="card card-body">
    <div class="flex items-center gap-2">
      <Key size={16} class="text-accent" />
      <h2 class="text-sm font-semibold text-fg">Primary token</h2>
    </div>
    <p class="mt-1 mb-3 text-[13px] text-muted">This is your main API token for CLI login and API access.</p>
    <div class="flex items-center gap-2">
      <div class="min-w-0 flex-1 truncate rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] text-muted">
        {maskToken(token)}
      </div>
      <button onclick={() => copyToken(token)} class="btn btn-secondary btn-icon" aria-label="Copy token" title="Copy token" disabled={!token}>
        {#if copied}<Check size={14} class="text-success" />{:else}<Copy size={14} />{/if}
      </button>
    </div>
  </section>

  <section class="card">
    <div class="card-header">
      <h2 class="card-title">All keys</h2>
    </div>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th class="max-sm:hidden">Created</th>
            <th class="max-sm:hidden">Last used</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {#each keys as key (key.id)}
            <tr>
              <td class="font-medium">
                <span class="flex items-center gap-2"><Key size={14} class="text-subtle" />{key.name}</span>
              </td>
              <td class="text-muted tabular-nums max-sm:hidden">{key.created}</td>
              <td class="text-muted max-sm:hidden">{key.lastUsed}</td>
              <td>
                <span class="badge {key.active ? 'badge-success' : ''}"><span class="dot"></span>{key.active ? 'Active' : 'Revoked'}</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <section class="card">
    <div class="card-header">
      <h2 class="card-title">How to use</h2>
    </div>
    <div class="card-body space-y-3">
      {#each usage as u (u.label)}
        <div>
          <p class="mb-1 text-xs text-subtle">{u.label}</p>
          <pre class="overflow-x-auto rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] text-fg">{u.cmd}</pre>
        </div>
      {/each}
    </div>
  </section>
</div>
