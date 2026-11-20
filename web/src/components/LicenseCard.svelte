<script lang="ts">
  // The panel's license (super_admin). Keys are issued and signed by
  // SigardaPanel; the panel verifies them offline (signature and expiry).
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtDateTime } from '$lib/format';
  import { licenseStore } from '$lib/license';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { BadgeCheck, LoaderCircle, KeyRound, AlertTriangle, ArrowUpRight, RefreshCw, ChevronDown } from 'lucide-svelte';
  import { fmtRelative } from '$lib/format';
  import { onMount } from 'svelte';

  let lic = $state<any>(null);
  let loading = $state(true);
  let key = $state('');
  let busy = $state(false);
  let confirmRemove = $state(false);
  let checking = $state(false);
  let showData = $state(false);

  async function checkinNow() {
    checking = true;
    try {
      const d = await apiFetch(auth.token, 'POST', 'license/checkin');
      toasts.success(`Checked in: ${d?.tier ?? 'free'} license${d?.message ? ' · ' + d.message : ''}`);
      await load();
      licenseStore.fetch();
    } catch (e: any) { toasts.error(e.message); await load(); }
    finally { checking = false; }
  }

  async function load() {
    try { lic = await apiFetch(auth.token, 'GET', 'license'); }
    catch (e: any) { toasts.error(e.message); }
    finally { loading = false; }
  }
  onMount(load);

  async function activate(e: Event) {
    e.preventDefault();
    if (!key.trim()) return;
    busy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', 'license/activate', { key: key.trim() });
      toasts.success(`License activated: ${d?.tier ?? ''}${d?.licensee ? ' · ' + d.licensee : ''}`);
      key = '';
      await load();
      licenseStore.fetch();
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = false; }
  }

  async function remove() {
    confirmRemove = false;
    try {
      await apiFetch(auth.token, 'DELETE', 'license');
      toasts.success('License removed: the panel runs on the free tier');
      await load();
      licenseStore.fetch();
    } catch (e: any) { toasts.error(e.message); }
  }

  const statusBadge: Record<string, string> = { active: 'badge-success', expired: 'badge-warning', invalid: 'badge-danger', none: '' };
  const statusLabel: Record<string, string> = { active: 'Active', expired: 'Expired', invalid: 'Not valid', none: 'No license' };
</script>

<section class="card">
  <div class="card-header">
    <h2 class="card-title flex items-center gap-2"><BadgeCheck size={16} class="text-subtle" /> License</h2>
    {#if lic}<span class="badge {statusBadge[lic.status] ?? ''}">{statusLabel[lic.status] ?? lic.status}</span>{/if}
  </div>
  <div class="card-body space-y-4">
    {#if loading}
      <div class="skeleton h-16"></div>
    {:else if lic}
      <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-[13px]">
        <div><dt class="text-subtle">Tier</dt><dd class="font-medium text-fg">{lic.tier_name || lic.tier}</dd></div>
        <div><dt class="text-subtle">Servers</dt><dd class="font-medium text-fg tabular-nums">{lic.limits?.servers?.current ?? 0} / {(lic.limits?.servers?.max ?? 0) < 0 ? 'Unlimited' : (lic.limits?.servers?.max ?? '—')}</dd></div>
        {#if lic.licensee}<div><dt class="text-subtle">Licensed to</dt><dd class="truncate font-medium text-fg">{lic.licensee}</dd></div>{/if}
        {#if lic.license_id}<div><dt class="text-subtle">License ID</dt><dd class="truncate font-mono text-xs text-fg">{lic.license_id}</dd></div>{/if}
        {#if lic.status !== 'none'}<div><dt class="text-subtle">Expires</dt><dd class="text-fg">{lic.expires_at ? fmtDateTime(lic.expires_at) : 'Never'}</dd></div>{/if}
        {#if lic.key}<div><dt class="text-subtle">Key</dt><dd class="truncate font-mono text-xs text-muted">{lic.key}</dd></div>{/if}
      </dl>
      <p class="text-[12px] text-subtle">
        Every feature is included on every plan. {lic.tier === 'free' ? 'Free covers one server (this one); upgrade to manage more servers from this panel.' : 'Your plan sets how many servers this panel manages.'}
      </p>
      <div class="space-y-2 rounded-lg border border-line px-3 py-2.5 text-[13px]">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="min-w-0">
            <div class="text-subtle">Installation ID</div>
            <div class="truncate font-mono text-xs text-fg">{lic.install_id}</div>
          </div>
          <div class="flex gap-2">
            {#if lic.checkin?.enabled}
              <button type="button" onclick={checkinNow} disabled={checking} class="btn btn-secondary btn-sm">
                {#if checking}<LoaderCircle size={14} class="animate-spin" />{:else}<RefreshCw size={14} />{/if} Check now
              </button>
            {/if}
            {#if lic.upgrade_url}
              <a href={lic.upgrade_url} target="_blank" rel="noopener" class="btn btn-primary btn-sm"><ArrowUpRight size={14} /> {lic.tier === 'free' ? 'Upgrade plan' : 'Manage plan'}</a>
            {/if}
          </div>
        </div>
        {#if !lic.checkin?.enabled}
          <p class="text-xs text-subtle">Check-in is off (SIGARDAPANEL_CHECKIN=off): licenses are not delivered automatically; activate keys by hand.</p>
        {:else if lic.checkin?.last_error}
          <p class="text-xs text-warning">Last check-in {lic.checkin.last_at ? fmtRelative(lic.checkin.last_at) : ''} failed: {lic.checkin.last_error}. The panel keeps working and retries.</p>
        {:else if lic.checkin?.last_at}
          <p class="text-xs text-subtle">Last check-in with {lic.checkin.master_url} {fmtRelative(lic.checkin.last_at)}.{#if lic.checkin.latest_version} Latest version: {lic.checkin.latest_version}.{/if}</p>
        {:else}
          <p class="text-xs text-subtle">The panel checks in with {lic.checkin?.master_url} once a day; the first check-in brings its free license.</p>
        {/if}
        {#if lic.checkin?.data}
          <button type="button" onclick={() => (showData = !showData)} class="flex cursor-pointer items-center gap-1 text-xs text-subtle hover:text-fg">
            <ChevronDown size={12} class={showData ? 'rotate-180' : ''} /> What is sent
          </button>
          {#if showData}
            <pre class="overflow-x-auto rounded bg-surface-2 p-2 font-mono text-[11px] text-muted">{JSON.stringify(lic.checkin.data, null, 2)}</pre>
            <p class="text-xs text-subtle">No names, domains, emails or IP addresses of your sites. Turn off with SIGARDAPANEL_CHECKIN=off.</p>
          {/if}
        {/if}
      </div>
      {#if lic.message}
        <p class="flex items-start gap-2 rounded-lg bg-warning/10 px-3 py-2 text-[13px] text-warning"><AlertTriangle size={14} class="mt-0.5 shrink-0" />{lic.message}</p>
      {/if}
      <form onsubmit={activate} class="space-y-2">
        <label for="license-key" class="label">{lic.status === 'active' ? 'Replace with another key' : 'License key'}</label>
        <textarea id="license-key" bind:value={key} rows="3" class="input font-mono text-xs" placeholder="SPL1.…" spellcheck="false" autocomplete="off"></textarea>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-xs text-subtle">Keys are signed by SigardaPanel and checked on this server, offline.</span>
          <div class="flex gap-2">
            {#if lic.status !== 'none'}<button type="button" onclick={() => (confirmRemove = true)} class="btn btn-danger-ghost btn-sm">Remove</button>{/if}
            <button type="submit" disabled={busy || !key.trim()} class="btn btn-primary btn-sm">
              {#if busy}<LoaderCircle size={14} class="animate-spin" /> Checking…{:else}<KeyRound size={14} /> Activate{/if}
            </button>
          </div>
        </div>
      </form>
    {/if}
  </div>
</section>

<ConfirmDialog open={confirmRemove} title="Remove the license?" message="The panel goes back to the free tier. You can activate the key again later." confirmLabel="Remove" destructive onconfirm={remove} oncancel={() => (confirmRemove = false)} />
