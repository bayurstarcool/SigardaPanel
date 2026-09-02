<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Cloud, Settings, Check, Trash2, ExternalLink, Shield, AlertTriangle, Eye, EyeOff } from 'lucide-svelte';
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

  // Form inputs
  let formToken = $state('');
  let formEmail = $state('');

  // Verification
  let verified = $state(false);
  let verifiedUserId = $state<number | null>(null);

  // Zones
  let zones = $state<any[]>([]);
  let zonesLoading = $state(false);

  // Password visibility
  let showPassword = $state(false);

  onMount(async () => {
    if (!auth.token) { loading = false; return; }
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
    if (!confirm('Remove Cloudflare configuration? This will disconnect your Cloudflare account.')) return;
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
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent flex items-center gap-2.5">
        <Cloud size={28} class="text-sky-400" />
        Cloudflare
      </h1>
      <p class="text-sm text-slate-500 mt-1">
        {#if loading}
          Loading...
        {:else if configured}
          {zones.length} zone{zones.length !== 1 ? 's' : ''} connected
        {:else}
          Configure your API token to manage DNS zones
        {/if}
      </p>
    </div>
    {#if configured && !showConfigForm}
      <div class="flex items-center gap-2">
        <button
          onclick={startChangeToken}
          class="inline-flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-xl text-sm font-medium bg-white/[0.04] border border-white/[0.12] text-slate-300 hover:bg-white/[0.08] hover:border-white/20 transition-all duration-150 cursor-pointer whitespace-nowrap"
        >
          <Settings size={14} />
          Change Token
        </button>
        <button
          onclick={removeConfig}
          disabled={deleting}
          class="inline-flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-xl text-sm font-medium bg-red-500/10 border border-red-500/20 text-red-400 hover:bg-red-500/20 transition-all duration-150 cursor-pointer whitespace-nowrap disabled:opacity-40"
        >
          <Trash2 size={14} />
          {deleting ? 'Removing...' : 'Remove'}
        </button>
      </div>
    {:else if !configured && !showConfigForm}
      <button
        onclick={startNewConfig}
        class="inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-medium bg-sky-500/10 border border-sky-500/20 text-sky-400 hover:bg-sky-500/20 hover:border-sky-400/30 transition-all duration-150 cursor-pointer whitespace-nowrap"
      >
        <Shield size={14} />
        Configure API
      </button>
    {/if}
  </div>

  <!-- Alerts -->


  <!-- Config Card: when configured and NOT editing -->
  {#if configured && !showConfigForm}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-10 h-10 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
          <Check size={20} class="text-emerald-400" />
        </div>
        <div>
          <h3 class="text-sm font-semibold text-slate-200">Configuration Active</h3>
          <p class="text-xs text-slate-500">Your Cloudflare API token is configured and verified.</p>
        </div>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <!-- Masked Token -->
        <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
          <label class="block text-[0.7rem] font-medium uppercase tracking-wider text-slate-500 mb-2">API Token</label>
          <div class="flex items-center gap-2">
            <code class="text-sm text-slate-300 font-mono break-all">{maskedToken}</code>
          </div>
        </div>

        <!-- Email -->
        {#if configEmail}
          <div class="bg-white/[0.02] border border-white/[0.05] rounded-xl p-4">
            <label class="block text-[0.7rem] font-medium uppercase tracking-wider text-slate-500 mb-2">Email</label>
            <span class="text-sm text-slate-300">{configEmail}</span>
          </div>
        {/if}
      </div>

      <!-- Verification badge -->
      {#if verified && verifiedUserId !== null}
        <div class="mt-4 flex items-center gap-2 text-xs text-emerald-400/80">
          <Check size={12} />
          Verified — user ID: {verifiedUserId}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Config Form -->
  {#if showConfigForm}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6">
      <div class="flex items-center gap-3 mb-5">
        <div class="w-10 h-10 rounded-xl bg-sky-500/10 border border-sky-500/20 flex items-center justify-center">
          <Shield size={20} class="text-sky-400" />
        </div>
        <div>
          <h3 class="text-sm font-semibold text-slate-200">Cloudflare API Configuration</h3>
          <p class="text-xs text-slate-500">Enter your Cloudflare API token to connect your account.</p>
        </div>
      </div>

      <div class="space-y-4">
        <!-- API Token -->
        <div>
          <label for="cf-api-token" class="block text-xs font-medium text-slate-400 mb-1.5">API Token</label>
          <div class="relative">
            <input
              id="cf-api-token"
              type={showPassword ? 'text' : 'password'}
              bind:value={formToken}
              placeholder={configured ? '••••••••••••••••••••••••••••••••' : 'Enter your Cloudflare API token'}
              class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 pr-11 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-sky-400/40 transition-colors"
            />
            <button
              type="button"
              onclick={() => showPassword = !showPassword}
              class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300 transition cursor-pointer"
            >
              {#if showPassword}
                <EyeOff size={16} />
              {:else}
                <Eye size={16} />
              {/if}
            </button>
          </div>
          {#if configured && !formToken}
            <p class="mt-1.5 text-xs text-slate-500 flex items-center gap-1.5">
              <AlertTriangle size={11} />
              Leave empty to keep current token
            </p>
          {/if}
        </div>

        <!-- Email -->
        <div>
          <label for="cf-email" class="block text-xs font-medium text-slate-400 mb-1.5">
            Email
            <span class="text-slate-600">(optional)</span>
          </label>
          <input
            id="cf-email"
            type="email"
            bind:value={formEmail}
            placeholder="Cloudflare account email"
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-sky-400/40 transition-colors"
          />
        </div>
      </div>

      <div class="flex gap-3 mt-5">
        <button
          onclick={saveConfig}
          disabled={saving || (!formToken && !configured)}
          class="px-5 py-2.5 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition-all duration-150 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer border-0"
        >
          {#if saving}
            <span class="flex items-center gap-2">
              <svg class="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" class="opacity-25" /><path d="M4 12a8 8 0 018-8" stroke="currentColor" stroke-width="4" stroke-linecap="round" class="opacity-75" /></svg>
              Saving...
            </span>
          {:else}
            <span class="flex items-center gap-1.5">
              <Check size={14} />
              Save Configuration
            </span>
          {/if}
        </button>
        <button
          onclick={cancelConfig}
          class="px-5 py-2.5 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition-colors cursor-pointer border-0"
        >
          Cancel
        </button>
      </div>
    </div>
  {/if}

  <!-- Zones Section -->
  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6 text-center py-20">
      <div class="animate-pulse text-slate-500">Loading...</div>
    </div>
  {:else if !configured && !showConfigForm}
    <!-- Empty state: not configured -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-12 text-center">
      <div class="w-16 h-16 rounded-2xl bg-sky-500/10 border border-sky-500/20 flex items-center justify-center mx-auto mb-5">
        <Cloud size={32} class="text-sky-400 opacity-60" />
      </div>
      <h2 class="text-lg font-semibold text-slate-200 mb-2">Connect Cloudflare</h2>
      <p class="text-sm text-slate-500 max-w-md mx-auto mb-6">
        Configure your Cloudflare API token to manage DNS zones and records.
      </p>
      <button
        onclick={startNewConfig}
        class="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl text-sm font-medium bg-sky-500/10 border border-sky-500/20 text-sky-400 hover:bg-sky-500/20 transition-all duration-150 cursor-pointer"
      >
        <Shield size={14} />
        Configure API Token
      </button>
    </div>
  {:else if zones.length === 0 && !zonesLoading}
    <!-- Empty state: configured but no zones -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-12 text-center">
      <div class="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center mx-auto mb-5">
        <Cloud size={32} class="text-indigo-400 opacity-60" />
      </div>
      <h2 class="text-lg font-semibold text-slate-200 mb-2">No Zones Found</h2>
      <p class="text-sm text-slate-500 max-w-md mx-auto">
        No Cloudflare zones were found for your account. Check that your API token has the correct permissions.
      </p>
    </div>
  {:else}
    <!-- Zone table -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl overflow-hidden">
      <div class="px-6 py-4 border-b border-white/[0.05]">
        <h3 class="text-sm font-semibold text-slate-200 flex items-center gap-2">
          <Cloud size={16} class="text-sky-400" />
          DNS Zones
          <span class="text-xs text-slate-500 font-normal">({zones.length})</span>
        </h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead>
            <tr class="text-left">
              <th class="px-6 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
              <th class="px-6 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Status</th>
              <th class="px-6 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] hidden sm:table-cell">Plan</th>
              <th class="px-6 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each zones as z (z.id)}
              <tr class="hover:bg-white/[0.02] transition-colors">
                <td class="px-6 py-3.5 border-b border-white/[0.03]">
                  <span class="font-medium text-slate-200 text-sm">{z.name}</span>
                  {#if z.id}
                    <span class="block text-xs text-slate-600 font-mono mt-0.5">{z.id}</span>
                  {/if}
                </td>
                <td class="px-6 py-3.5 border-b border-white/[0.03]">
                  <StatusPill status={z.status || 'active'} />
                </td>
                <td class="px-6 py-3.5 border-b border-white/[0.03] hidden sm:table-cell">
                  <span class="text-sm text-slate-400">{z.plan?.name || z.plan?.title || '—'}</span>
                </td>
                <td class="px-6 py-3.5 border-b border-white/[0.03]">
                  <a
                    href="/cloudflare/{z.id}"
                    class="inline-flex items-center gap-1.5 text-xs font-medium text-sky-400 hover:text-sky-300 transition-colors"
                  >
                    <ExternalLink size={12} />
                    Manage DNS
                  </a>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</DashboardLayout>
