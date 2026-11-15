<script lang="ts">
  // Provisioning progress of a new site: registration, server setup (Linux
  // user, folder, PHP pool, Nginx vhost), Cloudflare DNS, SSL and "live",
  // each with its current state. Data comes from the site's jobs
  // (site.create, ssl.issue), the site itself and the DNS check; it refreshes
  // itself while something is still running.
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { onMount } from 'svelte';
  import { Check, LoaderCircle, Circle, TriangleAlert, X, Minus, ExternalLink } from 'lucide-svelte';

  let { site, dns, onsite }: { site: any; dns: any; onsite?: (fresh: any) => void } = $props();

  type State = 'done' | 'active' | 'pending' | 'warn' | 'error' | 'skip';
  type Step = { key: string; title: string; state: State; detail: string; items?: { label: string; done: boolean }[] };

  let jobs = $state<any[]>([]);
  let loaded = $state(false);
  let dismissed = $state(false);
  const dismissKey = $derived(`sp-provisioning-dismissed-${site.id}`);

  async function loadJobs() {
    try {
      const d = await apiFetch(auth.token, 'GET', `jobs?site_id=${site.id}&limit=30`);
      jobs = d?.jobs || [];
    } catch {}
    loaded = true;
  }

  const setupJob = $derived(
    jobs.filter((j) => j.type === 'site.create').sort((a, b) => a.id - b.id)[0] ?? null,
  );
  const sslJob = $derived(jobs.filter((j) => j.type === 'ssl.issue').sort((a, b) => b.id - a.id)[0] ?? null);
  const setupResult = $derived.by(() => {
    try { return setupJob?.result ? JSON.parse(setupJob.result) : null; } catch { return null; }
  });

  const steps = $derived.by((): Step[] => {
    const out: Step[] = [];
    out.push({ key: 'record', title: 'Site registered', state: 'done', detail: `${site.domain} on the selected server` });

    // Server setup (the site.create job on the site's agent).
    let setup: Step = { key: 'setup', title: 'Server setup', state: 'pending', detail: 'Waiting to start' };
    const php = site.runtime === 'php';
    const items = (done: boolean) => [
      { label: 'Linux user and site folder', done },
      ...(php ? [{ label: 'PHP-FPM pool for this site', done }] : []),
      { label: 'Nginx vhost, tested and reloaded', done },
    ];
    if (!setupJob) {
      setup = site.status === 'active'
        ? { ...setup, state: 'done', detail: 'Configured', items: items(true) }
        : { ...setup, state: loaded ? 'pending' : 'active', detail: loaded ? 'Waiting for the server' : 'Checking…', items: items(false) };
    } else if (setupJob.status === 'succeeded') {
      const r = setupResult || {};
      setup = { ...setup, state: 'done', detail: r.root_path ? `Folder ${r.root_path}` : 'Configured', items: items(true) };
    } else if (setupJob.status === 'failed' || setupJob.status === 'cancelled') {
      setup = { ...setup, state: 'error', detail: setupJob.error || 'The server could not set the site up', items: items(false) };
    } else if (setupJob.status === 'running') {
      setup = { ...setup, state: 'active', detail: 'Creating the folder, PHP pool and Nginx vhost on the server…', items: items(false) };
    } else {
      setup = setupJob.attempts > 0 && setupJob.error
        ? { ...setup, state: 'active', detail: `Retrying (attempt ${setupJob.attempts + 1} of ${setupJob.max_attempts}): ${setupJob.error}`, items: items(false) }
        : { ...setup, state: 'active', detail: 'Queued, waiting for the server', items: items(false) };
    }
    out.push(setup);

    // Cloudflare DNS (done when the site was created; checked again here).
    let dnsStep: Step = { key: 'dns', title: 'DNS (Cloudflare)', state: 'skip', detail: 'No Cloudflare account connected' };
    if (dns) {
      const ok = ['managed', 'exists', 'created', 'updated', 'replaced'].includes(dns.status);
      dnsStep = {
        ...dnsStep,
        state: ok ? 'done' : dns.status === 'conflict' ? 'warn' : dns.status === 'error' ? 'error' : dns.status === 'missing' ? 'pending' : 'skip',
        detail: dns.message || '',
      };
    }
    out.push(dnsStep);

    // SSL: queued by the panel after a successful setup.
    let ssl: Step = { key: 'ssl', title: 'SSL certificate', state: 'pending', detail: 'Starts after the server setup' };
    if (site.ssl_status === 'active') ssl = { ...ssl, state: 'done', detail: "Let's Encrypt certificate installed" };
    else if (sslJob?.status === 'succeeded') ssl = { ...ssl, state: 'done', detail: "Let's Encrypt certificate installed" };
    else if (sslJob?.status === 'running') ssl = { ...ssl, state: 'active', detail: "Requesting a certificate from Let's Encrypt…" };
    else if (sslJob?.status === 'queued') ssl = { ...ssl, state: 'active', detail: sslJob.error || "Requesting a certificate from Let's Encrypt…" };
    else if (sslJob?.status === 'failed') ssl = { ...ssl, state: 'warn', detail: `Not issued: ${sslJob.error || 'unknown error'} The site works over HTTP meanwhile.` };
    else if (setup.state === 'done') ssl = { ...ssl, state: 'skip', detail: 'Not requested (issue one from the SSL tab)' };
    else if (setup.state === 'error') ssl = { ...ssl, state: 'skip', detail: 'Skipped' };
    out.push(ssl);

    const live: Step = setup.state === 'done' && site.status === 'active'
      ? { key: 'live', title: 'Live', state: 'done', detail: `Serving ${site.domain}` }
      : setup.state === 'error'
        ? { key: 'live', title: 'Live', state: 'error', detail: 'Not live: fix the setup error, then delete and recreate the site' }
        : { key: 'live', title: 'Live', state: 'pending', detail: 'After the server setup' };
    out.push(live);
    return out;
  });

  const busy = $derived(steps.some((s) => s.state === 'active' || (s.state === 'pending' && s.key !== 'dns')));
  const failed = $derived(steps.some((s) => s.state === 'error'));
  const attention = $derived(steps.some((s) => s.state === 'warn'));
  const doneCount = $derived(steps.filter((s) => s.state === 'done' || s.state === 'skip').length);
  const recent = $derived(site.created_at ? Date.now() - new Date(site.created_at).getTime() < 30 * 60 * 1000 : false);
  const visible = $derived(!dismissed && loaded && (busy || failed || attention || recent));

  onMount(() => {
    try { dismissed = localStorage.getItem(dismissKey) === '1'; } catch {}
    void loadJobs();
  });

  // Refresh while something is still running.
  $effect(() => {
    if (!busy) return;
    const timer = setInterval(async () => {
      await loadJobs();
      try { const fresh = await apiFetch(auth.token, 'GET', `sites/${site.id}`); if (fresh) onsite?.(fresh); } catch {}
    }, 2500);
    return () => clearInterval(timer);
  });

  // Ask for the certificate again (after fixing DNS or the firewall).
  let retrying = $state(false);
  async function retrySSL() {
    retrying = true;
    try {
      await apiFetch(auth.token, 'POST', `sites/${site.id}/ssl/issue`);
      await loadJobs();
    } catch {} finally { retrying = false; }
  }

  function dismiss() {
    dismissed = true;
    try { localStorage.setItem(dismissKey, '1'); } catch {}
  }

  const ring: Record<State, string> = {
    done: 'bg-success text-white ring-success/20',
    active: 'bg-accent/10 text-accent ring-accent/30',
    pending: 'bg-surface-2 text-subtle ring-line',
    warn: 'bg-warning/10 text-warning ring-warning/30',
    error: 'bg-danger/10 text-danger ring-danger/30',
    skip: 'bg-surface-2 text-subtle ring-line',
  };
</script>

{#if visible}
  <section class="card mb-6 overflow-hidden animate-fade-in" aria-live="polite">
    <div class="card-header">
      <div class="min-w-0">
        <h2 class="card-title">
          {#if busy}Setting up {site.domain}{:else if failed}Setup needs attention{:else if attention}Site is up, with warnings{:else}Site is ready{/if}
        </h2>
        <p class="mt-0.5 text-xs text-subtle tabular-nums">{doneCount} of {steps.length} steps complete</p>
      </div>
      <div class="flex items-center gap-2">
        {#if !busy && !failed && site.status === 'active'}
          <a href={`http${site.ssl_status === 'active' ? 's' : ''}://${site.domain}`} target="_blank" rel="noopener" class="btn btn-secondary btn-sm"><ExternalLink size={14} /> Visit</a>
        {/if}
        {#if !busy}
          <button onclick={dismiss} class="btn btn-ghost btn-icon btn-sm" aria-label="Hide setup progress" title="Hide"><X size={16} /></button>
        {/if}
      </div>
    </div>

    <div class="progress h-1 rounded-none"><div class="progress-bar rounded-none transition-[width] duration-500 {failed ? 'bg-danger' : ''}" style="width: {Math.round((doneCount / steps.length) * 100)}%"></div></div>

    <ol class="px-5 py-4">
      {#each steps as step, i (step.key)}
        <li class="relative flex gap-3.5 pb-5 last:pb-0">
          {#if i < steps.length - 1}
            <span class="absolute top-7 bottom-0 left-[13px] w-px {step.state === 'done' ? 'bg-success/40' : 'bg-line'}" aria-hidden="true"></span>
          {/if}
          <span class="relative z-10 flex size-7 shrink-0 items-center justify-center rounded-full ring-1 ring-inset {ring[step.state]}">
            {#if step.state === 'done'}<Check size={14} strokeWidth={2.5} />
            {:else if step.state === 'active'}<LoaderCircle size={14} class="animate-spin" />
            {:else if step.state === 'warn'}<TriangleAlert size={13} />
            {:else if step.state === 'error'}<X size={14} strokeWidth={2.5} />
            {:else if step.state === 'skip'}<Minus size={14} />
            {:else}<Circle size={8} class="fill-current" />{/if}
          </span>
          <div class="min-w-0 flex-1 pt-0.5">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-[13.5px] font-medium {step.state === 'pending' || step.state === 'skip' ? 'text-muted' : 'text-fg'}">{step.title}</span>
              {#if step.state === 'active'}<span class="badge badge-accent">In progress</span>{/if}
              {#if step.state === 'skip'}<span class="badge">Skipped</span>{/if}
            </div>
            {#if step.detail}
              <p class="mt-0.5 text-[12.5px] break-words {step.state === 'error' ? 'text-danger' : step.state === 'warn' ? 'text-warning' : 'text-subtle'}">{step.detail}</p>
            {/if}
            {#if step.key === 'ssl' && step.state === 'warn' && site.can_write}
              <button onclick={retrySSL} disabled={retrying} class="btn btn-secondary btn-xs mt-2">
                {#if retrying}<LoaderCircle size={12} class="animate-spin" />{/if} Retry SSL
              </button>
            {/if}
            {#if step.items && step.state !== 'pending'}
              <ul class="mt-2 space-y-1">
                {#each step.items as it (it.label)}
                  <li class="flex items-center gap-2 text-[12.5px] {it.done ? 'text-muted' : 'text-subtle'}">
                    {#if it.done}<Check size={12} class="text-success" />{:else if step.state === 'active'}<LoaderCircle size={12} class="animate-spin text-accent" />{:else}<Minus size={12} />{/if}
                    {it.label}
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </li>
      {/each}
    </ol>
  </section>
{/if}
