<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate } from '$lib/format';
  import StatusPill from '$components/StatusPill.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { ShieldCheck, Lock, RefreshCw, LoaderCircle, Globe } from 'lucide-svelte';
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

  const active = $derived(!!sslStatus?.ssl_status && sslStatus.ssl_status !== 'none');
</script>

{#if loading}
  <div class="card space-y-3 p-5">
    <div class="skeleton h-5 w-40"></div>
    {#each Array(4) as _, i (i)}<div class="skeleton h-8"></div>{/each}
  </div>
{:else if !site}
  <div class="card"><EmptyState icon={Globe} title="Site not found" /></div>
{:else}
  <div class="card">
    <div class="card-header">
      <div class="min-w-0">
        <h3 class="card-title">SSL certificate</h3>
        <p class="truncate text-xs text-subtle">{site.domain}</p>
      </div>
      <ShieldCheck size={16} class={active ? 'text-success' : 'text-subtle'} />
    </div>
    {#if active}
      <dl class="divide-y divide-line px-5">
        <div class="flex items-center justify-between gap-4 py-2.5"><dt class="text-[13px] text-muted">Status</dt><dd><StatusPill status={sslStatus.ssl_status || 'none'} /></dd></div>
        <div class="flex items-center justify-between gap-4 py-2.5"><dt class="text-[13px] text-muted">Issuer</dt><dd class="truncate text-sm text-fg">{sslStatus.ssl_issuer || '—'}</dd></div>
        <div class="flex items-center justify-between gap-4 py-2.5"><dt class="text-[13px] text-muted">Expires</dt><dd class="text-sm text-fg tabular-nums">{sslStatus.ssl_expires_at ? fmtDate(sslStatus.ssl_expires_at) : '—'}</dd></div>
      </dl>
    {:else}
      <EmptyState compact icon={ShieldCheck} title="No SSL status available" description="Issue a certificate below." />
    {/if}
    <div class="border-t border-line px-5 py-4">
      <p class="mb-3 max-w-lg text-[13px] text-muted">Issue a Let's Encrypt SSL certificate or renew the existing one. The certificate uses the site owner's email automatically.</p>
      <div class="flex flex-wrap gap-2">
        <button onclick={issueSSL} disabled={sslBusy} class="btn btn-primary btn-sm">
          {#if sslBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Lock size={14} />{/if} Issue SSL
        </button>
        <button onclick={renewSSL} disabled={sslBusy} class="btn btn-secondary btn-sm">
          <RefreshCw size={14} class={sslBusy ? 'animate-spin' : ''} /> Renew
        </button>
      </div>
    </div>
  </div>
{/if}
