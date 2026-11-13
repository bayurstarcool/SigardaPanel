<script lang="ts">
  // Panel self-update (super_admin). The API finds the latest signed release;
  // the panel host's agent installs it with the embedded installer (signature
  // check, backup, automatic rollback). API, web and agent restart during the
  // update, so the card keeps polling through failed requests until the
  // agent reports a result.
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtDateTime, fmtRelative } from '$lib/format';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { ArrowUpCircle, RefreshCw, LoaderCircle, CheckCircle2, AlertTriangle, ExternalLink, ChevronDown } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';

  type UpdateInfo = {
    current_version: string;
    is_release_build: boolean;
    latest?: { tag: string; name: string; notes: string; url: string; published_at: string } | null;
    update_available: boolean;
    auto_update: boolean;
    checked_at?: string;
    check_error?: string;
    agent_error?: string;
    repository: string;
    status?: {
      state: 'idle' | 'running' | 'succeeded' | 'failed';
      tag?: string;
      from_version?: string;
      started_at?: string;
      finished_at?: string;
      message?: string;
      log?: string;
      supported: boolean;
      unsupported_reason?: string;
    } | null;
  };

  let info = $state<UpdateInfo | null>(null);
  let loading = $state(true);
  let loadError = $state('');
  let checking = $state(false);
  let starting = $state(false);
  let savingAuto = $state(false);
  let confirmOpen = $state(false);
  let showNotes = $state(false);
  let showLog = $state(false);
  // Set while this page follows an update it started or found running.
  let following = $state(false);
  let unreachable = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let destroyed = false;
  let logEl = $state<HTMLPreElement | null>(null);

  const updState = $derived(info?.status?.state ?? 'idle');
  const running = $derived(following || updState === 'running');
  const supported = $derived(info?.status?.supported ?? false);
  const blockedReason = $derived(
    !info
      ? ''
      : !info.is_release_build
        ? `This panel is a development build (${info.current_version}); only release installs can update themselves.`
        : info.agent_error
          ? info.agent_error
          : info.status && !info.status.supported
            ? info.status.unsupported_reason || 'Self-update is not available on this host.'
            : '',
  );
  const canUpdate = $derived(!!info?.update_available && supported && !running && !blockedReason);

  async function load(refresh = false) {
    try {
      const d: UpdateInfo = await apiFetch(auth.token, 'GET', `system/panel-update${refresh ? '?refresh=1' : ''}`);
      info = d;
      loadError = '';
      unreachable = false;
      return d;
    } catch (e: any) {
      if (e?.message === 'session expired') throw e;
      if (following) {
        unreachable = true; // the API restarts during the update
        return null;
      }
      loadError = e?.message || 'Failed to load update status';
      return null;
    } finally {
      loading = false;
    }
  }

  async function checkNow() {
    checking = true;
    const d = await load(true).catch(() => null);
    checking = false;
    if (!d) return;
    if (d.check_error) toasts.error(`Could not check for updates: ${d.check_error}`);
    else if (d.update_available) toasts.success(`${d.latest?.tag} is available`);
    else toasts.success('SigardaPanel is up to date');
  }

  function schedulePoll() {
    if (destroyed) return;
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = setTimeout(poll, 3000);
  }

  async function poll() {
    if (destroyed) return;
    if (document.hidden) { schedulePoll(); return; }
    const before = info?.status?.tag;
    const d = await load().catch(() => null);
    const st = d?.status?.state;
    if (d && st !== 'running') {
      if (st === 'succeeded' && d.current_version && `v${d.current_version.replace(/^v/, '')}` === (d.status?.tag ?? before)) {
        following = false;
        toasts.success(`SigardaPanel updated to ${d.status?.tag}. Reloading…`);
        setTimeout(() => window.location.reload(), 2500);
        return;
      }
      if (st === 'failed') {
        following = false;
        showLog = true;
        toasts.error(d.status?.message || 'The update failed');
        return;
      }
      // succeeded but the API still reports the old version: services are
      // still restarting. Anything else: keep waiting a little.
    }
    schedulePoll();
  }

  async function startUpdate() {
    starting = true;
    try {
      await apiFetch(auth.token, 'POST', 'system/panel-update', {});
      following = true;
      showLog = true;
      toasts.success('Update started. The panel restarts for a moment during the update.');
      schedulePoll();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Could not start the update');
      load();
    } finally {
      starting = false;
    }
  }

  async function toggleAuto(e: Event) {
    const enabled = (e.currentTarget as HTMLInputElement).checked;
    savingAuto = true;
    try {
      await apiFetch(auth.token, 'PUT', 'system/panel-update/settings', { auto_update: enabled });
      if (info) info.auto_update = enabled;
      toasts.success(enabled ? 'Automatic updates enabled' : 'Automatic updates disabled');
    } catch (err: any) {
      (e.currentTarget as HTMLInputElement).checked = !enabled;
      if (err?.message !== 'session expired') toasts.error(err?.message || 'Could not save the setting');
    } finally {
      savingAuto = false;
    }
  }

  $effect(() => {
    // Keep the log scrolled to the newest line while it grows.
    void info?.status?.log;
    if (logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  onMount(async () => {
    const d = await load().catch(() => null);
    if (d?.status?.state === 'running') {
      following = true;
      showLog = true;
      schedulePoll();
    }
  });

  onDestroy(() => {
    destroyed = true;
    if (pollTimer) clearTimeout(pollTimer);
  });
</script>

<section class="card md:col-span-2">
  <div class="card-header">
    <h2 class="card-title flex items-center gap-2">
      <ArrowUpCircle size={16} class="text-subtle" /> Panel update
      {#if info && !loading}
        {#if running}
          <span class="badge badge-info">Updating</span>
        {:else if info.update_available}
          <span class="badge badge-accent">Update available</span>
        {:else if info.latest}
          <span class="badge badge-success">Up to date</span>
        {/if}
      {/if}
    </h2>
    <div class="flex items-center gap-2">
      <button class="btn btn-secondary btn-sm" onclick={checkNow} disabled={checking || running || loading}>
        <RefreshCw size={14} class={checking ? 'animate-spin' : ''} /> Check now
      </button>
      {#if info?.update_available || running}
        <button class="btn btn-primary btn-sm" onclick={() => (confirmOpen = true)} disabled={!canUpdate || starting}>
          {#if running || starting}<LoaderCircle size={14} class="animate-spin" /> Updating…{:else}<ArrowUpCircle size={14} /> Update now{/if}
        </button>
      {/if}
    </div>
  </div>

  <div class="card-body space-y-4">
    {#if loading}
      <div class="space-y-2.5">{#each [1, 2] as i (i)}<div class="skeleton h-7"></div>{/each}</div>
    {:else if loadError && !info}
      <p class="text-[13px] text-danger">{loadError}</p>
    {:else if info}
      <dl class="grid gap-3 text-sm sm:grid-cols-3">
        <div>
          <dt class="text-xs text-subtle">Installed</dt>
          <dd class="mt-0.5"><code class="code">{info.current_version}</code></dd>
        </div>
        <div>
          <dt class="text-xs text-subtle">Latest release</dt>
          <dd class="mt-0.5 flex items-center gap-2">
            {#if info.latest}
              <code class="code">{info.latest.tag}</code>
              {#if info.latest.published_at}<span class="text-xs text-subtle">{fmtRelative(info.latest.published_at)}</span>{/if}
            {:else}
              <span class="text-muted">—</span>
            {/if}
          </dd>
        </div>
        <div>
          <dt class="text-xs text-subtle">Last checked</dt>
          <dd class="mt-0.5 text-muted">{info.checked_at ? fmtDateTime(info.checked_at) : 'Never'}</dd>
        </div>
      </dl>

      {#if info.check_error}
        <p class="flex items-start gap-1.5 text-[13px] text-warning"><AlertTriangle size={14} class="mt-0.5 shrink-0" /> Could not check GitHub for releases: {info.check_error}</p>
      {/if}

      {#if running}
        <div class="rounded-lg border border-line bg-surface-2 p-3 text-[13px] text-muted">
          <p class="flex items-center gap-2 font-medium text-fg">
            <LoaderCircle size={14} class="animate-spin text-accent" />
            Updating{info.status?.from_version ? ` from ${info.status.from_version}` : ''} to {info.status?.tag ?? info.latest?.tag}…
          </p>
          <p class="mt-1">
            {#if unreachable}
              The panel is restarting. This page reconnects by itself — do not close it.
            {:else}
              Downloading and verifying the signed release, backing up, then restarting the services. If the new version is not healthy, the previous one is restored automatically.
            {/if}
          </p>
        </div>
      {:else if updState === 'succeeded' && info.status?.tag}
        <p class="flex items-center gap-1.5 text-[13px] text-success"><CheckCircle2 size={14} /> Last update: {info.status.message}{info.status.finished_at ? ` · ${fmtDateTime(info.status.finished_at)}` : ''}</p>
      {:else if updState === 'failed'}
        <p class="flex items-start gap-1.5 text-[13px] text-danger"><AlertTriangle size={14} class="mt-0.5 shrink-0" /> {info.status?.message}</p>
      {/if}

      {#if blockedReason && info.update_available && !running}
        <p class="text-[13px] text-muted">
          {blockedReason} Update from a shell instead: <code class="code">sudo bash install.sh update</code>
        </p>
      {/if}

      {#if info.update_available && info.latest?.notes}
        <div>
          <button class="btn btn-ghost btn-xs" onclick={() => (showNotes = !showNotes)} aria-expanded={showNotes}>
            <ChevronDown size={14} class={showNotes ? 'rotate-180' : ''} /> Release notes for {info.latest.tag}
          </button>
          {#if showNotes}
            <pre class="mt-2 max-h-72 overflow-auto rounded-lg border border-line bg-bg p-3 text-xs leading-relaxed whitespace-pre-wrap text-muted">{info.latest.notes}</pre>
          {/if}
          {#if info.latest.url}
            <a href={info.latest.url} target="_blank" rel="noopener noreferrer" class="link mt-2 inline-flex items-center gap-1 text-xs">View on GitHub <ExternalLink size={12} /></a>
          {/if}
        </div>
      {/if}

      {#if info.status?.log}
        <div>
          <button class="btn btn-ghost btn-xs" onclick={() => (showLog = !showLog)} aria-expanded={showLog}>
            <ChevronDown size={14} class={showLog ? 'rotate-180' : ''} /> Update log
          </button>
          {#if showLog}
            <pre bind:this={logEl} class="mt-2 max-h-72 overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{info.status.log}</pre>
          {/if}
        </div>
      {/if}

      <div class="flex items-start gap-3 border-t border-line pt-4">
        <input id="auto-update" type="checkbox" class="checkbox mt-0.5" checked={info.auto_update} onchange={toggleAuto} disabled={savingAuto} />
        <label for="auto-update" class="min-w-0">
          <span class="block text-sm text-fg">Install new releases automatically</span>
          <span class="hint block">The panel checks GitHub every hour and installs each new signed release once (with backup and automatic rollback). Super admins are notified either way.</span>
        </label>
      </div>
    {/if}
  </div>
</section>

<ConfirmDialog
  bind:open={confirmOpen}
  title={`Update to ${info?.latest?.tag ?? 'the latest release'}?`}
  message="The release is downloaded and its signature verified, the panel binary, web app and database are backed up, then the services restart. The panel is unavailable for about a minute; sites keep running. If the new version is not healthy, the previous one is restored automatically."
  confirmLabel="Update now"
  onconfirm={startUpdate} />
