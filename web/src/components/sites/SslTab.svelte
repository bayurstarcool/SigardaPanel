<script lang="ts">
    import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
    import StatusPill from '$components/StatusPill.svelte';
  import {
    ShieldCheck, Lock, RefreshCw, Loader2, ArrowLeft, Globe,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();
  let site = $state<any>(null);
  let loading = $state(true);

  // SSL state
  let sslBusy = $state(false);
  let sslStatus = $state<any>(null);

  async function loadSite() {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      site = await apiFetch(auth.token, 'GET', `sites/${siteId}`);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally { loading = false; }
  }

  async function loadSSL() {
    try { sslStatus = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssl`); } catch {}
  }

  async function pollJob(id: number): Promise<string> {
    for (let i = 0; i < 20; i++) {
      await new Promise(r => setTimeout(r, 2000));
      try {
        const d = await apiFetch(auth.token, 'GET', `jobs/${id}`);
        const st = d?.status || '';
        if (st === 'succeeded') return 'success';
        if (st === 'failed') return `error: ${d?.error || 'Job failed'}`;
      } catch {}
    }
    return 'timeout: Job still processing, check later';
  }

  async function issueSSL() {
    sslBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/ssl/issue`);
      const jobId = d?.job_id;
      const usedEmail = d?.email || '';
      if (!jobId) { toasts.success('SSL job queued. Check later.'); return; }
      toasts.info(`SSL job #${jobId} queued${usedEmail ? ` (email: ${usedEmail})` : ''}. Waiting...`);
      await loadSSL();
      const result = await pollJob(jobId);
      if (result.startsWith('success')) {
        await loadSSL();
        toasts.success('SSL certificate issued successfully!');
      } else if (result.startsWith('error')) {
        toasts.error(result.replace(/^error: /, ''));
      } else {
        toasts.info(result.replace(/^(timeout|info): /, ''));
      }
    }
    catch (e: any) { toasts.error(e.message || 'Failed to issue SSL'); }
    finally { sslBusy = false; }
  }

  async function renewSSL() {
    sslBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/ssl/renew`);
      const jobId = d?.job_id;
      if (!jobId) { toasts.success('SSL renew job queued (no job_id). Check later.'); return; }
      toasts.info(`SSL renew job #${jobId} queued. Waiting...`);
      await loadSSL();
      const result = await pollJob(jobId);
      if (result.startsWith('success')) {
        await loadSSL();
        toasts.success('SSL certificate renewed successfully!');
      } else if (result.startsWith('error')) {
        toasts.error(result.replace(/^error: /, ''));
      } else {
        toasts.info(result.replace(/^(timeout|info): /, ''));
      }
    }
    catch (e: any) { toasts.error(e.message || 'Failed to renew SSL'); }
    finally { sslBusy = false; }
  }

  onMount(async () => {
    await loadSite();
    await loadSSL();
  });
</script>


  {#if loading}
    <div class="space-y-4">
      <div class="h-8 w-32 bg-white/[0.04] rounded-lg animate-pulse"></div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-6 space-y-3">
        {#each Array(4) as _}
          <div class="flex justify-between"><div class="h-3 w-24 bg-white/[0.04] rounded animate-pulse"></div><div class="h-3 w-32 bg-white/[0.03] rounded animate-pulse"></div></div>
        {/each}
      </div>
    </div>
  {:else if !site}
    <div class="flex flex-col items-center justify-center py-32">
      <div class="w-20 h-20 rounded-3xl bg-white/[0.03] border border-white/[0.07] flex items-center justify-center mb-6"><Globe size={36} class="text-white/[0.08]" /></div>
      <h2 class="text-lg font-semibold text-slate-300 mb-1">Site not found</h2>
    </div>
  {:else}
    <!-- Back link -->
    <!-- Header -->
    <div class="flex items-center gap-3 mb-6">
      <div class="w-10 h-10 rounded-xl bg-emerald-400/10 flex items-center justify-center"><ShieldCheck size={18} class="text-emerald-400" /></div>
      <div>
        <h1 class="text-lg font-bold text-slate-200">SSL Certificate</h1>
        <p class="text-xs text-slate-500">{site.domain}</p>
      </div>
    </div>

    <!-- SSL Status -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300 mb-4">
      <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
        <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg {sslStatus?.ssl_status && sslStatus.ssl_status !== 'none' ? 'bg-emerald-400/10 text-emerald-400' : 'bg-slate-400/10 text-slate-500'} flex items-center justify-center">
          <ShieldCheck size={13} />
        </div>
        <span class="text-sm font-semibold text-slate-200">SSL Certificate</span>
      </div>
      <div class="p-3.5 sm:p-5">
        {#if sslStatus && sslStatus.ssl_status && sslStatus.ssl_status !== 'none'}
          <div class="space-y-0 mb-6">
            <div class="flex justify-between py-2.5 border-b border-white/[0.04]"><span class="text-sm text-slate-500">Status</span><StatusPill status={sslStatus.ssl_status || 'none'} /></div>
            <div class="flex justify-between py-2.5 border-b border-white/[0.04]"><span class="text-sm text-slate-500">Issuer</span><span class="text-sm text-slate-200">{sslStatus.ssl_issuer || '—'}</span></div>
            <div class="flex justify-between py-2.5"><span class="text-sm text-slate-500">Expires</span><span class="text-sm text-slate-200">{sslStatus.ssl_expires_at ? fmtDate(sslStatus.ssl_expires_at) : '—'}</span></div>
          </div>
        {:else}
          <div class="flex flex-col items-center justify-center py-8">
            <div class="w-14 h-14 rounded-2xl bg-white/[0.03] border border-white/[0.06] flex items-center justify-center mb-3">
              <ShieldCheck size={22} class="text-white/[0.08]" />
            </div>
            <p class="text-sm font-medium text-slate-400 mb-0.5">No SSL status available</p>
            <p class="text-xs text-slate-600">Issue a certificate below</p>
          </div>
        {/if}
        <div class="border-t border-white/[0.04] pt-5">
          <h4 class="text-xs font-semibold uppercase tracking-wider text-slate-500 mb-2">Actions</h4>
          <p class="text-xs text-slate-500 mb-3 max-w-lg">Issue a Let's Encrypt SSL certificate or renew existing one. The certificate uses the site owner's email automatically.</p>
          <div class="flex gap-3 flex-wrap">
            <button onclick={issueSSL} disabled={sslBusy} class="inline-flex items-center gap-2 px-5 py-2.5 bg-gradient-to-r from-emerald-400 to-teal-400 text-[#0a1628] rounded-xl text-sm font-semibold hover:-translate-y-0.5 transition-all shadow-[0_4px_15px_rgba(52,211,153,0.15)] cursor-pointer disabled:opacity-40 border-0">
              {#if sslBusy}<Loader2 size={14} class="animate-spin" />{:else}<Lock size={14} />{/if} Issue SSL
            </button>
            <button onclick={renewSSL} disabled={sslBusy} class="inline-flex items-center gap-2 px-5 py-2.5 text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] rounded-xl text-sm font-semibold transition cursor-pointer disabled:opacity-40">
              <RefreshCw size={14} /> Renew
            </button>
          </div>
        </div>
      </div>
    </div>
  {/if}

