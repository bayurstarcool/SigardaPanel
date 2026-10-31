<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import { Lock, Ban, Plus, Trash2, Save, LoaderCircle } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();

  let secEnabled = $state(false);
  let secUsername = $state('');
  let secPassword = $state('');
  let secBlockedIPs = $state<string[]>([]);
  let secLoading = $state(false);
  let secSaving = $state(false);
  let newBlockedIP = $state('');
  let secLoaded = $state(false);

  $effect(() => { if (!secLoaded && !secLoading) { secLoaded = true; loadSecurity(); } });

  async function loadSecurity() {
    secLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/security`); secEnabled = d?.basic_auth_enabled || false; secUsername = d?.basic_auth_username || ''; secBlockedIPs = d?.blocked_ips || []; }
    catch {} finally { secLoading = false; secLoaded = true; }
  }
  async function saveSecurity() {
    secSaving = true;
    try { await apiFetch(auth.token, 'PUT', `sites/${siteId}/security`, { basic_auth_enabled: secEnabled, basic_auth_username: secUsername, basic_auth_password: secPassword, blocked_ips: secBlockedIPs }); toasts.success('Security updated'); secPassword = ''; }
    catch (e: any) { toasts.error(e.message); } finally { secSaving = false; }
  }
  function addBlockedIP() { if (!newBlockedIP) return; if (!secBlockedIPs.includes(newBlockedIP)) secBlockedIPs = [...secBlockedIPs, newBlockedIP]; newBlockedIP = ''; }
  function removeBlockedIP(ip: string) { secBlockedIPs = secBlockedIPs.filter(i => i !== ip); }
</script>

<div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
  <div class="card">
    <div class="card-header">
      <h3 class="card-title">Basic authentication</h3>
      <Lock size={16} class="text-subtle" />
    </div>
    <div class="card-body">
      <p class="mb-4 text-[13px] text-muted">Protect this site with HTTP basic auth.</p>
      {#if secLoading}
        <div class="space-y-3"><div class="skeleton h-5 w-28"></div><div class="skeleton h-9"></div></div>
      {:else}
        <div class="space-y-3">
          <label class="flex cursor-pointer items-center gap-2.5">
            <input type="checkbox" bind:checked={secEnabled} class="checkbox" />
            <span class="text-sm text-fg">{secEnabled ? 'Enabled' : 'Disabled'}</span>
          </label>
          {#if secEnabled}
            <div>
              <label for="sec-user" class="label">Username</label>
              <input id="sec-user" bind:value={secUsername} class="input font-mono text-[13px]" placeholder="admin" autocomplete="off" />
            </div>
            <div>
              <label for="sec-pass" class="label">Password</label>
              <input id="sec-pass" bind:value={secPassword} type="password" class="input font-mono text-[13px]" placeholder="Leave empty to keep current" autocomplete="new-password" />
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
  <div class="card">
    <div class="card-header">
      <h3 class="card-title">IP blocking</h3>
      <Ban size={16} class="text-subtle" />
    </div>
    <div class="card-body">
      <p class="mb-4 text-[13px] text-muted">Block specific IP addresses from accessing this site.</p>
      <div class="mb-3 flex gap-2">
        <label for="sec-ip" class="sr-only">IP address to block</label>
        <input id="sec-ip" bind:value={newBlockedIP} onkeydown={(e) => e.key === 'Enter' && addBlockedIP()} class="input font-mono text-[13px]" placeholder="192.168.1.1" />
        <button onclick={addBlockedIP} class="btn btn-secondary btn-icon" aria-label="Add blocked IP"><Plus size={16} /></button>
      </div>
      {#if secBlockedIPs.length === 0}
        <EmptyState compact icon={Ban} title="No blocked IPs" description="Add an IP above to block it." />
      {:else}
        <ul class="divide-y divide-line rounded-lg border border-line">
          {#each secBlockedIPs as ip (ip)}
            <li class="flex items-center justify-between px-3 py-1.5">
              <span class="font-mono text-[13px] text-fg">{ip}</span>
              <button onclick={() => removeBlockedIP(ip)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Unblock {ip}"><Trash2 size={13} /></button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
</div>
<button onclick={saveSecurity} disabled={secSaving} class="btn btn-primary">
  {#if secSaving}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save security
</button>
