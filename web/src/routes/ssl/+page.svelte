<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { ShieldCheck, Plus, RefreshCw, AlertTriangle, ShieldAlert, ShieldOff, Calendar } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let allSites = $state<any[]>([]);
  let loading = $state(true);
  let showIssue = $state(false);
  let issueDomain = $state('');
  let issuing = $state(false);
  let renewing = $state<string | null>(null);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await loadData();
  });

  async function loadData() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'sites?limit=100');
      allSites = d?.sites || [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally { loading = false; }
  }

  async function issueCert() {
    if (!issueDomain) return;
    issuing = true;
    try {
      await apiFetch(auth.token, 'POST', 'ssl/issue', { domain: issueDomain });
      toasts.success(`SSL certificate issued for ${issueDomain}`);
      showIssue = false; issueDomain = '';
      await loadData();
    } catch (e: any) { toasts.error(e.message); }
    finally { issuing = false; }
  }

  async function renewCert(domain: string) {
    renewing = domain;
    try {
      await apiFetch(auth.token, 'POST', 'ssl/renew', { domain });
      toasts.success(`Certificate renewal started for ${domain}`);
      await loadData();
    } catch (e: any) { toasts.error(e.message); }
    finally { renewing = null; }
  }

  function sslStatus(expiresAt: string | null, status: string): string {
    if (!status || status === 'none') return 'none';
    if (!expiresAt) return 'valid';
    const ms = Date.now() - new Date(expiresAt).getTime();
    if (ms > 0) return 'expired';
    if (ms > -14 * 24 * 3600 * 1000) return 'expiring_soon';
    return 'valid';
  }

  function daysLeftText(expiresAt: string | null): string {
    if (!expiresAt) return '—';
    const days = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24));
    if (days < 0) return `${Math.abs(days)}d overdue`;
    if (days === 0) return 'Expires today';
    if (days === 1) return 'Expires tomorrow';
    return `${days}d left`;
  }

  function healthIcon(st: string) {
    if (st === 'expired') return ShieldOff;
    if (st === 'expiring_soon') return ShieldAlert;
    if (st === 'valid') return ShieldCheck;
    return ShieldCheck;
  }
  function healthColor(st: string) {
    if (st === 'expired') return 'text-red-400';
    if (st === 'expiring_soon') return 'text-amber-400';
    if (st === 'valid') return 'text-emerald-400';
    return 'text-slate-500';
  }
  function healthBg(st: string) {
    if (st === 'expired') return 'bg-red-500/10';
    if (st === 'expiring_soon') return 'bg-amber-500/10';
    if (st === 'valid') return 'bg-emerald-500/10';
    return 'bg-white/[0.03]';
  }

  let certSites = $derived(allSites.filter((s: any) => s.ssl_status && s.ssl_status !== 'none'));
  let validCount = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'valid').length);
  let expiringCount = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'expiring_soon').length);
  let expiredCount = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'expired').length);
  let noSSLCount = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'none').length);
  let expiringList = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'expiring_soon'));
  let expiredList = $derived(allSites.filter((s: any) => sslStatus(s.ssl_expires_at, s.ssl_status) === 'expired'));
</script>

<DashboardLayout active="SSL">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">SSL Certificates</h1>
      <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${allSites.length} site(s) tracked`}</p>
    </div>
    <div class="flex items-center gap-3">
      <button onclick={loadData}
        class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition cursor-pointer border border-white/[0.07]">
        <RefreshCw size={14} /> Refresh
      </button>
      <button onclick={() => showIssue = !showIssue}
        class="inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer border-0 whitespace-nowrap">
        <Plus size={14} /> Issue Certificate
      </button>
    </div>
  </div>

  {#if showIssue}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
      <h3 class="text-sm font-semibold text-slate-200 mb-4">Issue New SSL Certificate</h3>
      <div class="flex flex-col sm:flex-row gap-4">
        <div class="flex-1">
          <label class="block text-xs text-slate-500 mb-1.5">Domain</label>
          <select bind:value={issueDomain}
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40">
            <option value="">Select domain...</option>
            {#each allSites as s}<option value={s.domain}>{s.domain}</option>{/each}
          </select>
        </div>
        <div class="flex items-end gap-3">
          <button onclick={issueCert} disabled={issuing || !issueDomain}
            class="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
            {issuing ? 'Issuing...' : 'Issue'}
          </button>
          <button onclick={() => { showIssue = false; issueDomain = ''; }}
            class="px-4 py-2.5 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
        </div>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <div class="animate-pulse text-slate-500">Loading certificates...</div>
    </div>
  {:else}
    <!-- ═══ Summary cards ═══ -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4"><div class="flex items-center gap-2 mb-2"><ShieldCheck size={15} class="text-emerald-400" /><span class="text-xs text-slate-500">Valid</span></div><div class="text-2xl font-bold text-emerald-400">{validCount}</div></div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4"><div class="flex items-center gap-2 mb-2"><AlertTriangle size={15} class="text-amber-400" /><span class="text-xs text-slate-500">Expiring Soon</span></div><div class="text-2xl font-bold text-amber-400">{expiringCount}</div></div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4"><div class="flex items-center gap-2 mb-2"><AlertTriangle size={15} class="text-red-400" /><span class="text-xs text-slate-500">Expired</span></div><div class="text-2xl font-bold text-red-400">{expiredCount}</div></div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4"><div class="flex items-center gap-2 mb-2"><ShieldCheck size={15} class="text-slate-500" /><span class="text-xs text-slate-500">No SSL</span></div><div class="text-2xl font-bold text-slate-400">{noSSLCount}</div></div>
    </div>

    <!-- ═══ Expiring/Expired alerts ═══ -->
    {#if expiringList.length > 0 || expiredList.length > 0}
      <div class="bg-amber-500/5 border border-amber-500/20 rounded-xl p-5 mb-6">
        <div class="flex items-center gap-2 mb-3"><AlertTriangle size={18} class="text-amber-400" /><h3 class="text-sm font-semibold text-amber-300">Attention Required</h3></div>
        <div class="space-y-2">
          {#each expiredList as cert}<div class="flex items-center justify-between py-2 border-b border-amber-500/10 last:border-0"><div class="flex items-center gap-2"><ShieldOff size={14} class="text-red-400" /><span class="text-sm text-white font-medium">{cert.domain}</span></div><span class="text-xs text-red-400 font-medium">{daysLeftText(cert.ssl_expires_at)}</span></div>{/each}
          {#each expiringList as cert}<div class="flex items-center justify-between py-2 border-b border-amber-500/10 last:border-0"><div class="flex items-center gap-2"><ShieldAlert size={14} class="text-amber-400" /><span class="text-sm text-white font-medium">{cert.domain}</span></div><span class="text-xs text-amber-400 font-medium">{daysLeftText(cert.ssl_expires_at)}</span></div>{/each}
        </div>
      </div>
    {/if}

    <!-- ═══ All certificates table ═══ -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead><tr class="text-left">
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Domain</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] hidden sm:table-cell">Issuer</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Status</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Expires</th>
            <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] text-right">Actions</th>
          </tr></thead>
          <tbody>
            {#each allSites as s}
              {@const st = sslStatus(s.ssl_expires_at, s.ssl_status)}
              {@const Icon = healthIcon(st)}
              <tr class="hover:bg-white/[0.02] transition">
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <a href="/sites/{s.id}" class="text-sm font-medium text-white hover:text-sky-400 transition">{s.domain}</a>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-xs text-slate-400 align-middle hidden sm:table-cell">{s.ssl_issuer || '—'}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium {healthBg(st)} {healthColor(st)}"><Icon size={12} />{st === 'valid' ? 'Active' : st === 'expiring_soon' ? 'Expiring' : st === 'expired' ? 'Expired' : 'None'}</span>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  {#if s.ssl_expires_at}
                    <div class="flex items-center gap-1.5"><Calendar size={11} class="text-slate-500" /><span class="text-xs text-slate-500">{new Date(s.ssl_expires_at).toLocaleDateString('id-ID')}</span><span class="text-[0.6rem] font-medium {healthColor(st)}">({daysLeftText(s.ssl_expires_at)})</span></div>
                  {:else}
                    <span class="text-xs text-slate-600">—</span>
                  {/if}
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-right align-middle">
                  {#if st !== 'none'}
                    <button onclick={() => renewCert(s.domain)} disabled={renewing === s.domain}
                      class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition-all duration-150 cursor-pointer border-0 whitespace-nowrap disabled:opacity-40">
                      <RefreshCw size={12} class={renewing === s.domain ? 'animate-spin' : ''} />
                      {renewing === s.domain ? 'Renewing...' : 'Renew'}
                    </button>
                  {:else}
                    <button onclick={() => { issueDomain = s.domain; showIssue = true; }}
                      class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-medium text-sky-400 hover:bg-sky-400/10 transition-all cursor-pointer border-0 whitespace-nowrap">Issue</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</DashboardLayout>
