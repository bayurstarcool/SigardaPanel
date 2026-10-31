<script lang="ts">
  import { auth, bearerHeaders } from '$lib/auth.svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { ShieldCheck, ShieldOff, AlertCircle, CheckCircle2, KeyRound } from 'lucide-svelte';

  let loading = $state(true);
  let enabled = $state(false);
  let setupPhase = $state(false);
  let verifyPhase = $state(false);
  let qrUrl = $state('');
  let qrDataUrl = $state('');
  let secret = $state('');
  let code = $state('');
  let error = $state('');
  let success = $state('');
  let recoveryCodes = $state<string[]>([]);

  onMount(async () => {
    if (!auth.user) { goto('/login'); return; }
    await checkStatus();
  });

  async function checkStatus() {
    loading = true;
    try {
      const res = await fetch('/api/v1/auth/2fa/status', {
        headers: { ...bearerHeaders() }
      });
      const data = await res.json();
      enabled = data.enabled;
      setupPhase = data.setup && !data.enabled;
    } catch (e) {
      error = 'Failed to check 2FA status';
    } finally {
      loading = false;
    }
  }

  async function startSetup() {
    error = '';
    try {
      const res = await fetch('/api/v1/auth/2fa/setup', {
        method: 'POST',
        headers: { ...bearerHeaders() }
      });
      const data = await res.json();
      if (data.error) { error = data.error; return; }
      qrUrl = data.qr_url;
      secret = data.secret;
      setupPhase = true;
      // Render the otpauth:// URI locally — it contains the TOTP secret and
      // must never be sent to a third-party QR service.
      const QR = await import('qrcode');
      qrDataUrl = await QR.toDataURL(qrUrl, { width: 360, margin: 1 });
    } catch (e) {
      error = 'Failed to start 2FA setup';
    }
  }

  async function enable2FA() {
    error = '';
    if (code.length !== 6) {
      error = 'Please enter a 6-digit code';
      return;
    }
    try {
      const res = await fetch('/api/v1/auth/2fa/enable', {
        method: 'POST',
        headers: {
          ...bearerHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ code })
      });
      const data = await res.json();
      if (data.error) { error = data.error; return; }
      enabled = true;
      setupPhase = false;
      success = '2FA enabled successfully!';
      code = '';
      await getRecoveryCodes();
    } catch (e) {
      error = 'Failed to enable 2FA';
    }
  }

  async function disable2FA() {
    error = '';
    if (code.length !== 6) {
      error = 'Please enter a 6-digit code';
      return;
    }
    try {
      const res = await fetch('/api/v1/auth/2fa/disable', {
        method: 'POST',
        headers: {
          ...bearerHeaders(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ code })
      });
      const data = await res.json();
      if (data.error) { error = data.error; return; }
      enabled = false;
      setupPhase = false;
      success = '2FA disabled successfully';
      code = '';
      recoveryCodes = [];
    } catch (e) {
      error = 'Failed to disable 2FA';
    }
  }

  async function getRecoveryCodes() {
    try {
      const res = await fetch('/api/v1/auth/2fa/recovery-codes', {
        method: 'POST',
        headers: { ...bearerHeaders() }
      });
      const data = await res.json();
      if (data.recovery_codes) {
        recoveryCodes = data.recovery_codes;
      }
    } catch (e) {
      console.error('Failed to get recovery codes');
    }
  }
</script>

<DashboardLayout active="Settings">
  <PageHeader title="Two-factor authentication" description="Add an extra layer of security to your account" back="/settings" backLabel="Settings" />

  <div class="max-w-2xl space-y-4">
    {#if loading}
      <div class="card card-body space-y-3">
        <div class="skeleton h-5 w-40"></div>
        <div class="skeleton h-9 w-32"></div>
      </div>
    {:else}
      {#if error}
        <div class="flex items-start gap-2 rounded-lg bg-danger/10 px-3 py-2.5 text-[13px] text-danger ring-1 ring-danger/20 ring-inset" role="alert">
          <AlertCircle size={16} class="mt-px shrink-0" /><span>{error}</span>
        </div>
      {/if}
      {#if success}
        <div class="flex items-start gap-2 rounded-lg bg-success/10 px-3 py-2.5 text-[13px] text-success ring-1 ring-success/20 ring-inset" role="status">
          <CheckCircle2 size={16} class="mt-px shrink-0" /><span>{success}</span>
        </div>
      {/if}

      <section class="card">
        <div class="card-header">
          <div class="flex items-center gap-2.5">
            <span class="flex size-8 items-center justify-center rounded-lg {enabled ? 'bg-success/10 text-success' : 'bg-surface-2 text-subtle'}">
              {#if enabled}<ShieldCheck size={16} />{:else}<ShieldOff size={16} />{/if}
            </span>
            <h2 class="card-title">{enabled ? '2FA is enabled' : '2FA is disabled'}</h2>
          </div>
          <span class="badge {enabled ? 'badge-success' : ''}"><span class="dot"></span>{enabled ? 'On' : 'Off'}</span>
        </div>

        <div class="card-body">
          {#if !enabled && !setupPhase}
            <p class="mb-4 text-[13px] text-muted">Use an authenticator app (Google Authenticator, Authy, 1Password…) to generate a one-time code at sign-in.</p>
            <button class="btn btn-primary btn-sm" onclick={startSetup}><KeyRound size={14} /> Enable 2FA</button>
          {/if}

          {#if setupPhase && !enabled}
            <div class="space-y-5">
              <div>
                <h3 class="text-sm font-semibold text-fg">1. Scan QR code</h3>
                <p class="mt-1 text-[13px] text-muted">Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)</p>
                {#if qrDataUrl}
                  <div class="mt-3 inline-block rounded-lg border border-line bg-white p-3">
                    <img src={qrDataUrl} alt="QR code for your authenticator app" width="180" height="180" />
                  </div>
                {:else if qrUrl}
                  <div class="skeleton mt-3 size-[206px]"></div>
                {/if}
              </div>

              <div>
                <p class="text-[13px] text-muted">Or enter this secret manually:</p>
                <code class="mt-1.5 block rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-sm tracking-widest break-all text-fg select-all">{secret}</code>
              </div>

              <div>
                <h3 class="text-sm font-semibold text-fg">2. Verify</h3>
                <label for="totp-code" class="label mt-2">Enter verification code</label>
                <div class="flex gap-2">
                  <input
                    id="totp-code"
                    type="text"
                    bind:value={code}
                    placeholder="000000"
                    maxlength="6"
                    pattern="[0-9]*"
                    inputmode="numeric"
                    autocomplete="one-time-code"
                    class="input max-w-40 text-center font-mono tracking-[0.3em]"
                  />
                  <button class="btn btn-primary" onclick={enable2FA}>Verify &amp; enable</button>
                </div>
              </div>
            </div>
          {/if}

          {#if enabled}
            <p class="text-[13px] text-muted">Two-factor authentication is currently <strong class="font-semibold text-fg">enabled</strong>.</p>

            {#if recoveryCodes.length > 0}
              <div class="mt-5 rounded-lg bg-warning/10 p-4 ring-1 ring-warning/20 ring-inset">
                <h3 class="text-sm font-semibold text-fg">Recovery codes</h3>
                <p class="mt-1 text-[13px] text-muted">Save these codes securely. Each code can only be used once.</p>
                <div class="mt-3 grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                  {#each recoveryCodes as rc, i (i)}
                    <code class="rounded-md border border-line bg-surface px-2.5 py-1.5 font-mono text-[13px] text-fg select-all">{rc}</code>
                  {/each}
                </div>
              </div>
            {/if}

            <div class="mt-6 border-t border-line pt-5">
              <h3 class="text-sm font-semibold text-fg">Disable 2FA</h3>
              <label for="totp-disable" class="label mt-2">Enter a verification code to disable 2FA</label>
              <div class="flex gap-2">
                <input
                  id="totp-disable"
                  type="text"
                  bind:value={code}
                  placeholder="000000"
                  maxlength="6"
                  pattern="[0-9]*"
                  inputmode="numeric"
                  autocomplete="one-time-code"
                  class="input max-w-40 text-center font-mono tracking-[0.3em]"
                />
                <button class="btn btn-danger" onclick={disable2FA}>Disable 2FA</button>
              </div>
            </div>
          {/if}
        </div>
      </section>
    {/if}
  </div>
</DashboardLayout>
