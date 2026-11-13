<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import { Lock, Ban, Plus, Trash2, Save, LoaderCircle, ShieldCheck } from 'lucide-svelte';
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
    catch (e: any) { if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load'); } finally { secLoading = false; secLoaded = true; }
  }
  async function saveSecurity() {
    secSaving = true;
    try { await apiFetch(auth.token, 'PUT', `sites/${siteId}/security`, { basic_auth_enabled: secEnabled, basic_auth_username: secUsername, basic_auth_password: secPassword, blocked_ips: secBlockedIPs }); toasts.success('Security updated'); secPassword = ''; }
    catch (e: any) { toasts.error(e.message); } finally { secSaving = false; }
  }
  function addBlockedIP() { if (!newBlockedIP) return; if (!secBlockedIPs.includes(newBlockedIP)) secBlockedIPs = [...secBlockedIPs, newBlockedIP]; newBlockedIP = ''; }
  function removeBlockedIP(ip: string) { secBlockedIPs = secBlockedIPs.filter(i => i !== ip); }

  // Access rules: "allow" entries turn the site into an allowlist (everyone
  // else gets 403); "deny" entries block like the list above.
  let rules = $state<any[]>([]);
  let rulesLoaded = $state(false);
  let newRule = $state({ ip: '', action: 'allow', reason: '' });
  let ruleBusy = $state(false);
  const hasAllow = $derived(rules.some((r) => r.action === 'allow'));

  $effect(() => { if (!rulesLoaded) { rulesLoaded = true; loadRules(); } });

  async function loadRules() {
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/ip-restrictions`); rules = d?.restrictions || []; }
    catch (e: any) { if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load access rules'); }
  }
  async function addRule() {
    if (!newRule.ip.trim()) return;
    if (newRule.action === 'allow' && !hasAllow && !confirm('After this rule, only allowed addresses can open the site; everyone else gets 403. Continue?')) return;
    ruleBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/ip-restrictions`, newRule); newRule = { ip: '', action: newRule.action, reason: '' }; toasts.success('Rule added; the vhost is being updated'); await loadRules(); }
    catch (e: any) { toasts.error(e.message); } finally { ruleBusy = false; }
  }
  async function deleteRule(r: any) {
    if (!confirm(`Remove the ${r.action} rule for ${r.ip}?`)) return;
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/ip-restrictions/${r.id}`); rules = rules.filter((x) => x.id !== r.id); toasts.success('Rule removed; the vhost is being updated'); }
    catch (e: any) { toasts.error(e.message); }
  }
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
<div class="card mb-4">
  <div class="card-header">
    <h3 class="card-title">Access rules</h3>
    <ShieldCheck size={16} class="text-subtle" />
  </div>
  <div class="card-body">
    <p class="mb-4 text-[13px] text-muted">
      {#if hasAllow}Only the allowed addresses below can open this site; everyone else gets 403. Deny rules still win.{:else}Add an <strong>allow</strong> rule to restrict the site to specific IPs or ranges (e.g. your office). Rules apply right away, without the Save button.{/if}
    </p>
    <div class="mb-3 flex flex-col gap-2 sm:flex-row">
      <label for="rule-ip" class="sr-only">IP address or CIDR</label>
      <input id="rule-ip" bind:value={newRule.ip} class="input font-mono text-[13px] sm:w-56" placeholder="203.0.113.0/24" />
      <label for="rule-action" class="sr-only">Action</label>
      <select id="rule-action" bind:value={newRule.action} class="input sm:w-28"><option value="allow">Allow</option><option value="deny">Deny</option></select>
      <label for="rule-reason" class="sr-only">Note</label>
      <input id="rule-reason" bind:value={newRule.reason} class="input flex-1 text-[13px]" placeholder="Note (optional)" />
      <button onclick={addRule} disabled={ruleBusy || !newRule.ip.trim()} class="btn btn-secondary"><Plus size={14} /> Add</button>
    </div>
    {#if rules.length === 0}
      <EmptyState compact icon={ShieldCheck} title="No access rules" description="Everyone not blocked above can open the site." />
    {:else}
      <ul class="divide-y divide-line rounded-lg border border-line">
        {#each rules as r (r.id)}
          <li class="flex items-center justify-between gap-3 px-3 py-2">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <span class="badge {r.action === 'allow' ? 'badge-success' : 'badge-danger'}">{r.action}</span>
              <span class="font-mono text-[13px] text-fg">{r.ip}</span>
              {#if r.reason}<span class="truncate text-[12px] text-muted">{r.reason}</span>{/if}
            </div>
            <button onclick={() => deleteRule(r)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Remove rule for {r.ip}"><Trash2 size={14} /></button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
<button onclick={saveSecurity} disabled={secSaving} class="btn btn-primary">
  {#if secSaving}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save security
</button>
