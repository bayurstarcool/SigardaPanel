<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { onMount } from 'svelte';

  let loading = $state(true);
  let enabled = $state(false);
  let setupPhase = $state(false);
  let verifyPhase = $state(false);
  let qrUrl = $state('');
  let secret = $state('');
  let code = $state('');
  let error = $state('');
  let success = $state('');
  let recoveryCodes = $state<string[]>([]);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await checkStatus();
  });

  async function checkStatus() {
    loading = true;
    try {
      const res = await fetch('/api/v1/auth/2fa/status', {
        headers: { 'Authorization': `Bearer ${auth.token}` }
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
        headers: { 'Authorization': `Bearer ${auth.token}` }
      });
      const data = await res.json();
      if (data.error) { error = data.error; return; }
      qrUrl = data.qr_url;
      secret = data.secret;
      setupPhase = true;
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
          'Authorization': `Bearer ${auth.token}`,
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
          'Authorization': `Bearer ${auth.token}`,
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
        headers: { 'Authorization': `Bearer ${auth.token}` }
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

<div class="twofa-page">
  <div class="header">
    <h1>Two-Factor Authentication</h1>
    <p>Add an extra layer of security to your account</p>
  </div>

  {#if loading}
    <div class="loading">Loading...</div>
  {:else}
    {#if error}
      <div class="alert alert-error">{error}</div>
    {/if}
    {#if success}
      <div class="alert alert-success">{success}</div>
    {/if}

    <div class="card">
      <div class="status">
        <span class="dot" class:active={enabled}></span>
        <span>{enabled ? '2FA is enabled' : '2FA is disabled'}</span>
      </div>

      {#if !enabled && !setupPhase}
        <button class="btn btn-primary" onclick={startSetup}>
          Enable 2FA
        </button>
      {/if}

      {#if setupPhase && !enabled}
        <div class="setup-section">
          <h3>Scan QR Code</h3>
          <p>Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)</p>
          
          {#if qrUrl}
            <div class="qr-container">
              <img src="https://api.qrserver.com/v1/create-qr-code/?size=200x200&data={encodeURIComponent(qrUrl)}" alt="QR Code" />
            </div>
          {/if}

          <div class="manual-entry">
            <p>Or enter this secret manually:</p>
            <code>{secret}</code>
          </div>

          <div class="verify-section">
            <label for="totp-code">Enter verification code:</label>
            <div class="input-group">
              <input
                id="totp-code"
                type="text"
                bind:value={code}
                placeholder="000000"
                maxlength="6"
                pattern="[0-9]*"
                inputmode="numeric"
              />
              <button class="btn btn-primary" onclick={enable2FA}>
                Verify & Enable
              </button>
            </div>
          </div>
        </div>
      {/if}

      {#if enabled}
        <div class="enabled-section">
          <p>Two-factor authentication is currently <strong>enabled</strong>.</p>
          
          {#if recoveryCodes.length > 0}
            <div class="recovery-codes">
              <h3>Recovery Codes</h3>
              <p>Save these codes securely. Each code can only be used once.</p>
              <div class="codes-grid">
                {#each recoveryCodes as code}
                  <code>{code}</code>
                {/each}
              </div>
            </div>
          {/if}

          <div class="disable-section">
            <h3>Disable 2FA</h3>
            <p>Enter a verification code to disable 2FA:</p>
            <div class="input-group">
              <input
                type="text"
                bind:value={code}
                placeholder="000000"
                maxlength="6"
                pattern="[0-9]*"
                inputmode="numeric"
              />
              <button class="btn btn-danger" onclick={disable2FA}>
                Disable 2FA
              </button>
            </div>
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .twofa-page {
    max-width: 600px;
    margin: 2rem auto;
    padding: 0 1rem;
  }
  .header {
    text-align: center;
    margin-bottom: 2rem;
  }
  .header h1 {
    font-size: 1.5rem;
    margin-bottom: 0.5rem;
  }
  .header p {
    color: #666;
  }
  .card {
    background: white;
    border: 1px solid #e5e7eb;
    border-radius: 12px;
    padding: 2rem;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1.5rem;
    font-weight: 500;
  }
  .dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #ef4444;
  }
  .dot.active {
    background: #22c55e;
  }
  .btn {
    padding: 0.75rem 1.5rem;
    border: none;
    border-radius: 8px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
  }
  .btn-primary {
    background: #3b82f6;
    color: white;
  }
  .btn-primary:hover {
    background: #2563eb;
  }
  .btn-danger {
    background: #ef4444;
    color: white;
  }
  .btn-danger:hover {
    background: #dc2626;
  }
  .setup-section, .enabled-section {
    margin-top: 1.5rem;
  }
  .setup-section h3, .enabled-section h3 {
    font-size: 1.125rem;
    margin-bottom: 0.75rem;
  }
  .qr-container {
    text-align: center;
    margin: 1.5rem 0;
    padding: 1rem;
    background: #f9fafb;
    border-radius: 8px;
  }
  .manual-entry {
    margin: 1rem 0;
    padding: 1rem;
    background: #f3f4f6;
    border-radius: 8px;
  }
  .manual-entry code {
    display: block;
    margin-top: 0.5rem;
    font-family: monospace;
    font-size: 1.125rem;
    letter-spacing: 2px;
  }
  .verify-section {
    margin-top: 1.5rem;
  }
  .input-group {
    display: flex;
    gap: 0.75rem;
    margin-top: 0.5rem;
  }
  .input-group input {
    flex: 1;
    padding: 0.75rem;
    border: 1px solid #d1d5db;
    border-radius: 8px;
    font-family: monospace;
    font-size: 1.25rem;
    text-align: center;
    letter-spacing: 4px;
  }
  .recovery-codes {
    margin: 1.5rem 0;
    padding: 1rem;
    background: #fef3c7;
    border: 1px solid #fbbf24;
    border-radius: 8px;
  }
  .codes-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
    margin-top: 0.75rem;
  }
  .codes-grid code {
    padding: 0.5rem;
    background: white;
    border-radius: 4px;
    font-family: monospace;
  }
  .disable-section {
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid #e5e7eb;
  }
  .alert {
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }
  .alert-error {
    background: #fef2f2;
    color: #dc2626;
    border: 1px solid #fecaca;
  }
  .alert-success {
    background: #f0fdf4;
    color: #16a34a;
    border: 1px solid #bbf7d0;
  }
</style>
