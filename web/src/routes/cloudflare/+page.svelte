<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Cloud, Settings, Check, Trash2, ChevronRight, Shield, Eye, EyeOff, Search, LoaderCircle } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let loading = $state(true);

  // Config state
  let configured = $state(false);
  let maskedToken = $state('');
  let configEmail = $state('');
  let showConfigForm = $state(false);
  let saving = $state(false);
  let deleting = $state(false);
  let confirmRemove = $state(false);

  // Form inputs
  let formToken = $state('');
  let formEmail = $state('');

  // Verification
  let verified = $state(false);
  let verifiedUserId = $state<number | null>(null);

  // Zones
  let zones = $state<any[]>([]);
  let zonesLoading = $state(false);
  let query = $state('');

  // Password visibility
  let showPassword = $state(false);

  const visibleZones = $derived(
    query ? zones.filter((z) => String(z.name || '').toLowerCase().includes(query.toLowerCase())) : zones,
  );

  onMount(async () => {
    if (!auth.user) { loading = false; return; }
    try {
      await loadConfig();
      if (configured) {
        await loadZones();
      }
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load');
    } finally {
      loading = false;
    }
  });

  async function loadConfig() {
    const cfg = await apiFetch(auth.token, 'GET', 'cloudflare/config');
    if (cfg && cfg.configured) {
      configured = true;
      maskedToken = cfg.api_token || '';
      configEmail = cfg.email || '';
      formEmail = configEmail;
    } else {
      configured = false;
      maskedToken = '';
      configEmail = '';
    }
  }

  async function loadZones() {
    zonesLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'cloudflare/zones');
      zones = d?.zones || [];
    } catch (e: any) {
      // Don't overwrite main error, just silently fail for zones
      console.error('Failed to load zones:', e);
    } finally {
      zonesLoading = false;
    }
  }

  async function saveConfig() {
    saving = true;
    try {
      const body: Record<string, string> = {};
      if (formToken) body.api_token = formToken;
      if (formEmail) body.email = formEmail;

      const res = await apiFetch(auth.token, 'PUT', 'cloudflare/config', body);

      // Verification status
      if (res) {
        verified = true;
        verifiedUserId = res.user_id || res.id || null;
      }

      // Reload config to get masked token
      await loadConfig();
      showConfigForm = false;
      formToken = '';
      formEmail = configEmail;
      showPassword = false;
      toasts.success('Configuration saved successfully.');
      // Load zones after successful config
      await loadZones();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to save configuration');
    } finally {
      saving = false;
    }
  }

  async function removeConfig() {
    deleting = true;
    try {
      await apiFetch(auth.token, 'DELETE', 'cloudflare/config');
      configured = false;
      maskedToken = '';
      configEmail = '';
      zones = [];
      verified = false;
      verifiedUserId = null;
      toasts.success('Configuration removed.');
    } catch (e: any) {
      toasts.error(e.message || 'Failed to remove configuration');
    } finally {
      deleting = false;
    }
  }

  function startChangeToken() {
    showConfigForm = true;
    formToken = '';
    formEmail = configEmail;
    showPassword = false;
  }

  function startNewConfig() {
    showConfigForm = true;
    formToken = '';
    formEmail = '';
    showPassword = false;
  }

  function cancelConfig() {
    showConfigForm = false;
    formToken = '';
    formEmail = configEmail;
    showPassword = false;
  }
</script>

<svelte:head>
  <title>Cloudflare — SigardaPanel</title>
</svelte:head>

<DashboardLayout active="Cloudflare">
  <PageHeader
    title="Cloudflare"
    description={loading ? 'Loading…' : configured ? `${zones.length} zone${zones.length !== 1 ? 's' : ''} connected` : 'Connect an API token to manage DNS zones'}>
    {#snippet meta()}
      {#if !loading && configured}<span class="badge badge-success"><span class="dot"></span>Connected</span>{/if}
    {/snippet}
    {#snippet actions()}
      {#if configured && !showConfigForm}
        <button onclick={startChangeToken} class="btn btn-secondary btn-sm"><Settings size={14} /> Change token</button>
        <button onclick={() => (confirmRemove = true)} disabled={deleting} class="btn btn-danger-ghost btn-sm">
          <Trash2 size={14} /> {deleting ? 'Removing…' : 'Remove'}
        </button>
      {:else if !configured && !showConfigForm && !loading}
        <button onclick={startNewConfig} class="btn btn-primary btn-sm"><Shield size={14} /> Configure API</button>
      {/if}
    {/snippet}
  </PageHeader>

  <!-- Config summary: configured and not editing -->
  {#if configured && !showConfigForm}
    <div class="card mb-6">
      <div class="card-header">
        <div class="flex items-center gap-2">
          <Check size={16} class="text-success" />
          <h2 class="card-title">Configuration active</h2>
        </div>
        {#if verified && verifiedUserId !== null}
          <span class="text-xs text-subtle">Verified — user ID {verifiedUserId}</span>
        {/if}
      </div>
      <dl class="grid grid-cols-1 gap-4 p-5 sm:grid-cols-2">
        <div class="min-w-0">
          <dt class="stat-label">API token</dt>
          <dd class="mt-1 font-mono text-[13px] break-all text-fg">{maskedToken}</dd>
        </div>
        {#if configEmail}
          <div class="min-w-0">
            <dt class="stat-label">Email</dt>
            <dd class="mt-1 truncate text-sm text-fg">{configEmail}</dd>
          </div>
        {/if}
      </dl>
    </div>
  {/if}

  <!-- Config form -->
  {#if showConfigForm}
    <form class="card mb-6" onsubmit={(e) => { e.preventDefault(); saveConfig(); }}>
      <div class="card-header">
        <div>
          <h2 class="card-title">Cloudflare API configuration</h2>
          <p class="mt-0.5 text-xs text-subtle">Enter your Cloudflare API token to connect your account.</p>
        </div>
      </div>
      <div class="card-body space-y-4">
        <div>
          <label for="cf-api-token" class="label">API token</label>
          <div class="relative">
            <input
              id="cf-api-token"
              type={showPassword ? 'text' : 'password'}
              bind:value={formToken}
              autocomplete="off"
              placeholder={configured ? '••••••••••••••••••••••••' : 'Enter your Cloudflare API token'}
              class="input pr-10 font-mono" />
            <button
              type="button"
              onclick={() => (showPassword = !showPassword)}
              class="btn btn-ghost btn-icon btn-xs absolute top-1/2 right-1.5 -translate-y-1/2"
              aria-label={showPassword ? 'Hide token' : 'Show token'}>
              {#if showPassword}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
            </button>
          </div>
          {#if configured && !formToken}
            <p class="hint">Leave empty to keep the current token.</p>
          {/if}
        </div>
        <div>
          <label for="cf-email" class="label">Email <span class="font-normal text-subtle">(optional)</span></label>
          <input id="cf-email" type="email" bind:value={formEmail} placeholder="Cloudflare account email" class="input" />
        </div>
      </div>
      <div class="flex justify-end gap-2 border-t border-line px-5 py-3">
        <button type="button" onclick={cancelConfig} class="btn btn-secondary btn-sm">Cancel</button>
        <button type="submit" disabled={saving || (!formToken && !configured)} class="btn btn-primary btn-sm">
          {#if saving}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}<Check size={14} /> Save configuration{/if}
        </button>
      </div>
    </form>
  {/if}

  <!-- Zones -->
  {#if loading}
    <div class="card space-y-3 p-4">
      {#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}
    </div>
  {:else if !configured && !showConfigForm}
    <div class="card">
      <EmptyState
        icon={Cloud as any}
        title="Connect Cloudflare"
        description="Configure your Cloudflare API token to manage DNS zones and records."
        actionLabel="Configure API token"
        onaction={startNewConfig} />
    </div>
  {:else if configured}
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 class="card-title">DNS zones <span class="font-normal text-subtle tabular-nums">{zones.length}</span></h2>
        {#if zones.length > 0}
          <div class="relative w-full sm:w-64">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input bind:value={query} class="input h-8 pl-8" placeholder="Filter zones…" aria-label="Filter zones" />
          </div>
        {/if}
      </div>

      {#if zonesLoading && zones.length === 0}
        <div class="space-y-3 p-4">
          {#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}
        </div>
      {:else if zones.length === 0}
        <EmptyState
          compact
          icon={Cloud as any}
          title="No zones found"
          description="No Cloudflare zones were found for your account. Check that your API token has the correct permissions." />
      {:else if visibleZones.length === 0}
        <EmptyState compact icon={Search as any} title="No zones match" description="Try a different filter." />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Status</th>
                <th class="max-sm:hidden">Plan</th>
                <th class="text-right"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {#each visibleZones as z (z.id)}
                <tr>
                  <td>
                    <a href="/cloudflare/{z.id}" class="font-medium text-fg hover:text-accent">{z.name}</a>
                    {#if z.id}<div class="font-mono text-xs text-subtle max-sm:hidden">{z.id}</div>{/if}
                  </td>
                  <td><StatusPill status={z.status || 'active'} /></td>
                  <td class="text-muted max-sm:hidden">{z.plan?.name || z.plan?.title || '—'}</td>
                  <td class="text-right">
                    <a href="/cloudflare/{z.id}" class="btn btn-ghost btn-xs">Manage DNS <ChevronRight size={14} /></a>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}

  <ConfirmDialog
    bind:open={confirmRemove}
    title="Remove Cloudflare configuration?"
    message="This will disconnect your Cloudflare account. DNS records at Cloudflare are not changed."
    confirmLabel="Remove"
    destructive
    onconfirm={removeConfig} />
</DashboardLayout>
