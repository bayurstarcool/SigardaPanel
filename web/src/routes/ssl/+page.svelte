<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import { ShieldCheck, Plus, RefreshCw, ShieldAlert, ShieldOff, ShieldQuestion, Search } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  type Health = 'valid' | 'expiring_soon' | 'expired' | 'none';

  let allSites = $state<any[]>([]);
  let loading = $state(true);
  let showIssue = $state(false);
  let issueSiteId = $state<number | ''>('');
  let issuing = $state(false);
  let renewing = $state<number | null>(null);
  let renewingAll = $state(false);
  let query = $state('');
  let filter = $state<'all' | Health>('all');

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'sites?limit=500');
      allSites = d?.sites || [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  async function issueCert() {
    if (!issueSiteId) return;
    const site = allSites.find((s) => s.id === issueSiteId);
    issuing = true;
    try {
      await apiFetch(auth.token, 'POST', `sites/${issueSiteId}/ssl/issue`, {});
      toasts.success(`Certificate requested for ${site?.domain ?? 'site'} — it will appear once the job finishes.`);
      showIssue = false;
      issueSiteId = '';
      await loadData();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      issuing = false;
    }
  }

  async function renewCert(site: any) {
    renewing = site.id;
    try {
      await apiFetch(auth.token, 'POST', `sites/${site.id}/ssl/renew`, {});
      toasts.success(`Renewal started for ${site.domain}`);
      await loadData();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      renewing = null;
    }
  }

  async function renewAll() {
    renewingAll = true;
    try {
      const d = await apiFetch(auth.token, 'POST', 'ssl/renew-all', {});
      toasts.success(d?.message || 'Renewal queued for expiring certificates');
      await loadData();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      renewingAll = false;
    }
  }

  function health(s: any): Health {
    if (!s.ssl_status || s.ssl_status === 'none') return 'none';
    if (!s.ssl_expires_at) return 'valid';
    const ms = new Date(s.ssl_expires_at).getTime() - Date.now();
    if (ms <= 0) return 'expired';
    if (ms < 14 * 864e5) return 'expiring_soon';
    return 'valid';
  }

  function daysLeft(expiresAt: string | null): string {
    if (!expiresAt) return '';
    const days = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 864e5);
    if (days < 0) return `${Math.abs(days)}d overdue`;
    if (days === 0) return 'today';
    return `${days}d left`;
  }

  const meta: Record<Health, { label: string; badge: string; icon: any }> = {
    valid: { label: 'Active', badge: 'badge-success', icon: ShieldCheck },
    expiring_soon: { label: 'Expiring', badge: 'badge-warning', icon: ShieldAlert },
    expired: { label: 'Expired', badge: 'badge-danger', icon: ShieldOff },
    none: { label: 'No SSL', badge: '', icon: ShieldQuestion },
  };

  const rows = $derived(allSites.map((s) => ({ ...s, health: health(s) })));
  const counts = $derived({
    valid: rows.filter((r) => r.health === 'valid').length,
    expiring_soon: rows.filter((r) => r.health === 'expiring_soon').length,
    expired: rows.filter((r) => r.health === 'expired').length,
    none: rows.filter((r) => r.health === 'none').length,
  });
  const visible = $derived(
    rows
      .filter((r) => filter === 'all' || r.health === filter)
      .filter((r) => !query || r.domain.toLowerCase().includes(query.toLowerCase()))
      // Most urgent first: expired, expiring, none, valid.
      .sort((a, b) => ['expired', 'expiring_soon', 'none', 'valid'].indexOf(a.health) - ['expired', 'expiring_soon', 'none', 'valid'].indexOf(b.health)),
  );
</script>

<DashboardLayout active="SSL">
  <PageHeader title="SSL certificates" description={loading ? 'Loading…' : `${allSites.length} sites tracked`}>
    {#snippet actions()}
      <button onclick={loadData} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if isAdmin}
        <button onclick={renewAll} class="btn btn-secondary btn-sm" disabled={renewingAll || counts.expiring_soon + counts.expired === 0}>
          <RefreshCw size={14} class={renewingAll ? 'animate-spin' : ''} /> Renew expiring
        </button>
      {/if}
      <button onclick={() => (showIssue = true)} class="btn btn-primary btn-sm">
        <Plus size={14} /> Issue certificate
      </button>
    {/snippet}
  </PageHeader>

  <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
    <StatCard label="Active" value={loading ? '–' : counts.valid} icon={ShieldCheck} tone="success" />
    <StatCard label="Expiring in 14 days" value={loading ? '–' : counts.expiring_soon} icon={ShieldAlert} tone="warning" />
    <StatCard label="Expired" value={loading ? '–' : counts.expired} icon={ShieldOff} tone="danger" />
    <StatCard label="Without SSL" value={loading ? '–' : counts.none} icon={ShieldQuestion} />
  </div>

  <div class="card">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
      <div class="segmented">
        {#each [['all', 'All'], ['expired', 'Expired'], ['expiring_soon', 'Expiring'], ['none', 'No SSL'], ['valid', 'Active']] as [key, label] (key)}
          <button class="segment {filter === key ? 'segment-active' : ''}" onclick={() => (filter = key as any)}>{label}</button>
        {/each}
      </div>
      <div class="relative w-full sm:w-64">
        <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
        <input bind:value={query} class="input h-8 pl-8" placeholder="Filter domains…" />
      </div>
    </div>

    {#if loading}
      <div class="space-y-3 p-4">
        {#each Array(5) as _, i (i)}<div class="skeleton h-9"></div>{/each}
      </div>
    {:else if visible.length === 0}
      <EmptyState compact icon={ShieldCheck} title="No certificates match" description="Try another filter, or issue a certificate for one of your sites." />
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>Domain</th>
              <th class="max-sm:hidden">Issuer</th>
              <th>Status</th>
              <th>Expires</th>
              <th class="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each visible as s (s.id)}
              {@const m = meta[s.health as Health]}
              <tr>
                <td><a href="/sites/{s.id}" class="font-medium text-fg hover:text-accent">{s.domain}</a></td>
                <td class="text-muted max-sm:hidden">{s.ssl_issuer || '—'}</td>
                <td><span class="badge {m.badge}"><m.icon size={12} />{m.label}</span></td>
                <td class="whitespace-nowrap">
                  {#if s.ssl_expires_at}
                    <span class="text-muted tabular-nums">{new Date(s.ssl_expires_at).toLocaleDateString('id-ID')}</span>
                    <span class="ml-1 text-xs {s.health === 'expired' ? 'text-danger' : s.health === 'expiring_soon' ? 'text-warning' : 'text-subtle'}">{daysLeft(s.ssl_expires_at)}</span>
                  {:else}
                    <span class="text-subtle">—</span>
                  {/if}
                </td>
                <td class="text-right">
                  {#if s.health === 'none'}
                    <button onclick={() => { issueSiteId = s.id; showIssue = true; }} class="btn btn-ghost btn-xs text-accent">Issue</button>
                  {:else}
                    <button onclick={() => renewCert(s)} disabled={renewing === s.id} class="btn btn-ghost btn-xs">
                      <RefreshCw size={12} class={renewing === s.id ? 'animate-spin' : ''} />
                      {renewing === s.id ? 'Renewing…' : 'Renew'}
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <Modal bind:open={showIssue} title="Issue SSL certificate" maxWidth="md" onclose={() => (issueSiteId = '')}>
    <label class="label" for="issue-site">Site</label>
    <select id="issue-site" bind:value={issueSiteId} class="input">
      <option value="">Select a site…</option>
      {#each rows as s (s.id)}
        <option value={s.id}>{s.domain}{s.health !== 'none' ? ` — ${meta[s.health as Health].label.toLowerCase()}` : ''}</option>
      {/each}
    </select>
    <p class="hint">A Let's Encrypt certificate is requested in the background. The domain must already point to this server.</p>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showIssue = false)}>Cancel</button>
      <button class="btn btn-primary btn-sm" onclick={issueCert} disabled={issuing || !issueSiteId}>
        {issuing ? 'Requesting…' : 'Issue certificate'}
      </button>
    {/snippet}
  </Modal>
</DashboardLayout>
