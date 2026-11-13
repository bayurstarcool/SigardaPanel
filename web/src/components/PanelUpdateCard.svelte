<script lang="ts">
  // Panel self-update (super_admin). The API finds the latest signed release;
  // the panel host's agent installs it with the embedded installer (signature
  // check, backup, automatic rollback). API, web and agent restart during the
  // update, so the card keeps polling through failed requests until the
  // agent reports a result.
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtDateTime, fmtRelative, parseDate, fmtBytes } from '$lib/format';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import {
    ArrowUpCircle, RefreshCw, LoaderCircle, CheckCircle2, AlertTriangle, ExternalLink, ChevronDown, XCircle, Undo2,
  } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';

  type Backup = {
    name: string;
    created_at: string;
    version: string;
    target?: string;
    kind: string;
    has_database: boolean;
    size_bytes: number;
  };
  type Run = {
    action?: 'update' | 'rollback';
    backup?: string;
    tag: string;
    from_version?: string;
    state: 'succeeded' | 'failed' | 'running' | 'idle';
    started_at: string;
    finished_at?: string;
    message?: string;
  };
  type UpdateInfo = {
    current_version: string;
    is_release_build: boolean;
    latest?: { tag: string; name: string; notes: string; url: string; published_at: string } | null;
    update_available: boolean;
    auto_update: boolean;
    auto_update_window?: { start: number; end: number } | null;
    server_time?: string;
    checked_at?: string;
    check_error?: string;
    agent_error?: string;
    repository: string;
    status?: {
      state: 'idle' | 'running' | 'succeeded' | 'failed';
      action?: 'update' | 'rollback';
      backup?: string;
      hold_tag?: string;
      tag?: string;
      from_version?: string;
      started_at?: string;
      finished_at?: string;
      message?: string;
      log?: string;
      supported: boolean;
      unsupported_reason?: string;
      preflight?: { name: string; ok: boolean; detail: string }[];
      history?: Run[];
    } | null;
  };

  // How long the page follows an update before giving up (the agent caps a
  // run at 45 minutes).
  const FOLLOW_LIMIT_MS = 50 * 60 * 1000;
  const BADGE_KEY = 'sigarda_panel_update';
  const hours = Array.from({ length: 24 }, (_, h) => h);

  let info = $state<UpdateInfo | null>(null);
  let loading = $state(true);
  let loadError = $state('');
  let checking = $state(false);
  let starting = $state(false);
  let savingAuto = $state(false);
  let confirmOpen = $state(false);
  let showNotes = $state(false);
  let showLog = $state(false);
  let showHistory = $state(false);
  // Set while this page follows an update it started or found running.
  let following = $state(false);
  let followSince = 0;
  let unreachable = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let destroyed = false;
  let logEl = $state<HTMLPreElement | null>(null);
  // The version the followed update or rollback should end on.
  let followTag = $state('');
  let showBackups = $state(false);
  let backups = $state<Backup[] | null>(null);
  let backupsError = $state('');
  let rollbackTarget = $state<Backup | null>(null);
  let rollbackOpen = $state(false);
  let rollingBack = $state(false);
  let winStart = $state('any');
  let winEnd = $state('6');

  const updState = $derived(info?.status?.state ?? 'idle');
  const isRollback = $derived(info?.status?.action === 'rollback');
  const running = $derived(following || updState === 'running');
  const supported = $derived(info?.status?.supported ?? false);
  const failedChecks = $derived((info?.status?.preflight ?? []).filter((c) => !c.ok));
  const history = $derived(info?.status?.history ?? []);
  const blockedReason = $derived(
    !info
      ? ''
      : !info.is_release_build
        ? `This panel is a development build (${info.current_version}); only release installs can update themselves.`
        : info.agent_error
          ? info.agent_error
          : info.status && !info.status.supported
            ? info.status.unsupported_reason || 'Self-update is not available on this host.'
            : failedChecks.length
              ? `Preflight check failed: ${failedChecks.map((c) => `${c.name} (${c.detail})`).join('; ')}.`
              : '',
  );
  const canUpdate = $derived(!!info?.update_available && supported && !running && !blockedReason);

  function clearBadgeCache() {
    try { sessionStorage.removeItem(BADGE_KEY); } catch { /* storage unavailable */ }
  }

  function syncWindow(d: UpdateInfo) {
    if (d.auto_update_window) {
      winStart = String(d.auto_update_window.start);
      winEnd = String(d.auto_update_window.end);
    } else {
      winStart = 'any';
    }
  }

  async function load(refresh = false) {
    try {
      const d: UpdateInfo = await apiFetch(auth.token, 'GET', `system/panel-update${refresh ? '?refresh=1' : ''}`);
      info = d;
      if (!savingAuto) syncWindow(d);
      if (!d.update_available) clearBadgeCache();
      loadError = '';
      unreachable = false;
      return d;
    } catch (e: any) {
      if (e?.message === 'session expired') throw e;
      if (following) {
        unreachable = true; // the API restarts during the update
        // After a rollback to a release without this endpoint, only the
        // version tells that the rollback finished.
        if (followTag) {
          const v = await apiFetch(auth.token, 'GET', 'version').catch(() => null);
          if (v?.version && sameVersion(v.version, followTag)) finishFollow(followTag);
        }
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
    clearBadgeCache();
    if (d.check_error) toasts.error(`Could not check for updates: ${d.check_error}`);
    else if (d.update_available) toasts.success(`${d.latest?.tag} is available`);
    else toasts.success('SigardaPanel is up to date');
  }

  function finishFollow(tag: string) {
    if (!following) return;
    following = false;
    clearBadgeCache();
    toasts.success(`SigardaPanel is now ${tag}. Reloading…`);
    setTimeout(() => window.location.reload(), 2500);
  }

  function startFollowing(tag = '') {
    followTag = tag;
    following = true;
    followSince = Date.now();
    showLog = true;
    schedulePoll();
  }

  function schedulePoll() {
    if (destroyed) return;
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = setTimeout(poll, 3000);
  }

  const sameVersion = (version?: string, tag?: string) =>
    !!version && !!tag && `v${version.replace(/^v/, '')}` === tag;

  async function poll() {
    if (destroyed) return;
    if (Date.now() - followSince > FOLLOW_LIMIT_MS) {
      following = false;
      toasts.error('Stopped following the update: no result after 50 minutes. Check the log or the server.');
      load();
      return;
    }
    if (document.hidden) { schedulePoll(); return; }
    const d = await load().catch(() => null);
    const st = d?.status?.state;
    if (d && st === 'succeeded' && sameVersion(d.current_version, d.status?.tag)) {
      finishFollow(d.status?.tag ?? '');
      return;
    }
    if (d && st === 'failed') {
      following = false;
      showLog = true;
      toasts.error(d.status?.message || (isRollback ? 'The rollback failed' : 'The update failed'));
      return;
    }
    // running, or succeeded while the API still reports the old version
    // (services still restarting), or unreachable: keep waiting.
    schedulePoll();
  }

  async function startUpdate() {
    starting = true;
    try {
      await apiFetch(auth.token, 'POST', 'system/panel-update', {});
      toasts.success('Update started. The panel restarts for a moment during the update.');
      startFollowing(info?.latest?.tag ?? '');
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Could not start the update');
      load();
    } finally {
      starting = false;
    }
  }

  async function loadBackups() {
    backupsError = '';
    try {
      const d = await apiFetch(auth.token, 'GET', 'system/panel-update/backups');
      backups = d?.backups ?? [];
    } catch (e: any) {
      backups = null;
      if (e?.message !== 'session expired') backupsError = e?.message || 'Could not load the backups';
    }
  }

  function toggleBackups() {
    showBackups = !showBackups;
    if (showBackups && backups === null) loadBackups();
  }

  function askRollback(b: Backup) {
    rollbackTarget = b;
    rollbackOpen = true;
  }

  async function doRollback() {
    if (!rollbackTarget) return;
    rollingBack = true;
    try {
      const d = await apiFetch(auth.token, 'POST', 'system/panel-update/rollback', { backup: rollbackTarget.name });
      toasts.success(d?.message || 'Rollback started.');
      startFollowing(d?.tag ?? '');
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Could not start the rollback');
      load();
    } finally {
      rollingBack = false;
    }
  }

  const rollbackMessage = $derived(
    rollbackTarget
      ? `The panel binary and web app go back to ${rollbackTarget.version}` +
          (rollbackTarget.has_database
            ? `, and the database is restored to its state of ${fmtDateTime(rollbackTarget.created_at)}: everything changed in the panel since then (sites, users, settings, backups records) is lost.`
            : '. This backup has no database copy, so the current database is kept.') +
          ' The current state is backed up first and restored automatically if the rolled-back panel is not healthy. Automatic updates will skip the version you roll back from.'
      : '',
  );

  async function saveSettings(body: Record<string, unknown>, okMsg: string) {
    savingAuto = true;
    try {
      const d = await apiFetch(auth.token, 'PUT', 'system/panel-update/settings', body);
      if (info) {
        info.auto_update = !!d?.auto_update;
        info.auto_update_window = d?.auto_update_window ?? null;
        syncWindow(info);
      }
      toasts.success(okMsg);
      return true;
    } catch (err: any) {
      if (err?.message !== 'session expired') toasts.error(err?.message || 'Could not save the setting');
      if (info) syncWindow(info);
      return false;
    } finally {
      savingAuto = false;
    }
  }

  async function toggleAuto(e: Event) {
    // Keep the element: currentTarget is gone once the handler awaits.
    const box = e.currentTarget as HTMLInputElement;
    const enabled = box.checked;
    const ok = await saveSettings({ auto_update: enabled }, enabled ? 'Automatic updates enabled' : 'Automatic updates disabled');
    if (!ok) box.checked = !enabled;
  }

  function saveWindow() {
    if (winStart === 'any') {
      saveSettings({ auto_update_window: null }, 'Automatic updates may run at any time');
      return;
    }
    const start = Number(winStart);
    let end = Number(winEnd);
    if (end === start) end = (start + 1) % 24;
    saveSettings(
      { auto_update_window: { start, end } },
      `Automatic updates run between ${pad(start)}:00 and ${pad(end)}:00 server time`,
    );
  }

  const pad = (n: number) => String(n).padStart(2, '0');

  function duration(r: { started_at?: string; finished_at?: string }) {
    const a = parseDate(r.started_at), b = parseDate(r.finished_at);
    if (!a || !b) return '—';
    const s = Math.max(0, Math.round((b.getTime() - a.getTime()) / 1000));
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
  }

  $effect(() => {
    // Keep the log scrolled to the newest line while it grows.
    void info?.status?.log;
    if (logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  onMount(async () => {
    const d = await load().catch(() => null);
    if (d?.status?.state === 'running') startFollowing(d.status.tag ?? '');
    if (window.location.hash === '#panel-update') {
      document.getElementById('panel-update')?.scrollIntoView({ block: 'start' });
    }
  });

  onDestroy(() => {
    destroyed = true;
    if (pollTimer) clearTimeout(pollTimer);
  });
</script>

<section id="panel-update" class="card scroll-mt-20 md:col-span-2">
  <div class="card-header">
    <h2 class="card-title flex items-center gap-2">
      <ArrowUpCircle size={16} class="text-subtle" /> Panel update
      {#if info && !loading}
        {#if running}
          <span class="badge badge-info">{isRollback ? 'Rolling back' : 'Updating'}</span>
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
          {#if running || starting}<LoaderCircle size={14} class="animate-spin" /> {isRollback ? 'Rolling back…' : 'Updating…'}{:else}<ArrowUpCircle size={14} /> Update now{/if}
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
        <div class="rounded-lg border border-line bg-surface-2 p-3 text-[13px] text-muted" aria-live="polite">
          <p class="flex items-center gap-2 font-medium text-fg">
            <LoaderCircle size={14} class="animate-spin text-accent" />
            {isRollback ? 'Rolling back' : 'Updating'}{info.status?.from_version ? ` from ${info.status.from_version}` : ''} to {info.status?.tag || followTag || info.latest?.tag}…
          </p>
          <p class="mt-1">
            {#if unreachable}
              The panel is restarting. This page reconnects by itself — do not close it.
            {:else if isRollback}
              Backing up the current state, then restoring the binary, web app and database from backup {info.status?.backup}. If the rolled-back panel is not healthy, the current state is restored automatically.
            {:else}
              Downloading and verifying the signed release, backing up, then restarting the services. If the new version is not healthy, the previous one is restored automatically.
            {/if}
          </p>
        </div>
      {:else if updState === 'succeeded' && info.status?.tag}
        <p class="flex items-center gap-1.5 text-[13px] text-success"><CheckCircle2 size={14} /> {isRollback ? 'Last rollback' : 'Last update'}: {info.status.message}{info.status.finished_at ? ` · ${fmtDateTime(info.status.finished_at)}` : ''}</p>
      {:else if updState === 'failed'}
        <p class="flex items-start gap-1.5 text-[13px] text-danger"><XCircle size={14} class="mt-0.5 shrink-0" /> {info.status?.message}</p>
      {/if}

      {#if blockedReason && info.update_available && !running}
        <p class="flex items-start gap-1.5 text-[13px] text-muted">
          <AlertTriangle size={14} class="mt-0.5 shrink-0 text-warning" />
          <span>{blockedReason} You can also update from a shell: <code class="code">sudo bash install.sh update</code></span>
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
            <ChevronDown size={14} class={showLog ? 'rotate-180' : ''} /> {isRollback ? 'Rollback log' : 'Update log'}{info.status.tag ? ` (${info.status.tag})` : ''}
          </button>
          {#if showLog}
            <pre bind:this={logEl} class="mt-2 max-h-72 overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{info.status.log}</pre>
          {/if}
        </div>
      {/if}

      {#if history.length}
        <div>
          <button class="btn btn-ghost btn-xs" onclick={() => (showHistory = !showHistory)} aria-expanded={showHistory}>
            <ChevronDown size={14} class={showHistory ? 'rotate-180' : ''} /> Earlier updates ({history.length})
          </button>
          {#if showHistory}
            <div class="table-wrap mt-2">
              <table class="table">
                <thead><tr><th>Started</th><th>Version</th><th>Result</th><th class="max-sm:hidden">Duration</th></tr></thead>
                <tbody>
                  {#each history as r (r.started_at + r.tag)}
                    <tr>
                      <td class="text-[13px] tabular-nums">{fmtDateTime(r.started_at)}</td>
                      <td class="font-mono text-[13px]">{r.action === 'rollback' ? '↩ ' : ''}{r.from_version ? `${r.from_version} → ` : ''}{r.tag}</td>
                      <td><span title={r.message}><StatusPill status={r.state === 'succeeded' ? 'success' : 'failed'} label={r.state} /></span></td>
                      <td class="text-[13px] text-muted tabular-nums max-sm:hidden">{duration(r)}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}

      {#if info.status?.hold_tag}
        <p class="flex items-start gap-1.5 text-[13px] text-warning">
          <Undo2 size={14} class="mt-0.5 shrink-0" />
          The panel was rolled back from {info.status.hold_tag}: automatic updates skip it until a newer release appears. Update now installs it anyway.
        </p>
      {/if}

      <div>
        <button class="btn btn-ghost btn-xs" onclick={toggleBackups} aria-expanded={showBackups}>
          <ChevronDown size={14} class={showBackups ? 'rotate-180' : ''} /> Backups &amp; rollback
        </button>
        {#if showBackups}
          <div class="mt-2 space-y-2">
            <p class="text-xs text-subtle">A backup of the binary, web app and database is made before every update (and before every rollback). Rolling back restores all three.</p>
            {#if backupsError}
              <p class="text-[13px] text-danger">{backupsError}</p>
            {:else if backups === null}
              <div class="skeleton h-16"></div>
            {:else if backups.length === 0}
              <p class="text-[13px] text-muted">No backups yet.</p>
            {:else}
              <div class="table-wrap">
                <table class="table">
                  <thead><tr><th>Made</th><th>Version</th><th class="max-sm:hidden">Database</th><th class="max-sm:hidden">Size</th><th></th></tr></thead>
                  <tbody>
                    {#each backups as b (b.name)}
                      <tr>
                        <td class="text-[13px] tabular-nums">
                          {fmtDateTime(b.created_at)}
                          <span class="block text-xs text-subtle">{b.kind === 'pre-rollback' ? 'before a rollback' : b.target ? `before the update to ${b.target}` : 'before an update'}</span>
                        </td>
                        <td class="font-mono text-[13px]">{b.version}</td>
                        <td class="text-[13px] max-sm:hidden">{b.has_database ? 'yes' : 'no'}</td>
                        <td class="text-[13px] text-muted tabular-nums max-sm:hidden">{fmtBytes(b.size_bytes)}</td>
                        <td class="text-right">
                          <button class="btn btn-danger-ghost btn-xs" onclick={() => askRollback(b)} disabled={running || rollingBack || !supported || b.version === info.current_version}
                            title={b.version === info.current_version ? 'This is the installed version' : `Roll back to ${b.version}`}>
                            <Undo2 size={12} /> Roll back
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <div class="space-y-3 border-t border-line pt-4">
        <div class="flex items-start gap-3">
          <input id="auto-update" type="checkbox" class="checkbox mt-0.5" checked={info.auto_update} onchange={toggleAuto} disabled={savingAuto} />
          <label for="auto-update" class="min-w-0">
            <span class="block text-sm text-fg">Install new releases automatically</span>
            <span class="hint block">The panel checks GitHub every hour and installs each new signed release once (with backup and automatic rollback). Super admins are notified of new releases and of every update result.</span>
          </label>
        </div>
        {#if info.auto_update}
          <div class="flex flex-wrap items-center gap-2 pl-7 text-[13px] text-muted">
            <label for="win-start">Only between</label>
            <select id="win-start" class="input h-8 w-auto text-[13px]" bind:value={winStart} onchange={saveWindow} disabled={savingAuto}>
              <option value="any">any time</option>
              {#each hours as h (h)}<option value={String(h)}>{pad(h)}:00</option>{/each}
            </select>
            {#if winStart !== 'any'}
              <label for="win-end">and</label>
              <select id="win-end" class="input h-8 w-auto text-[13px]" bind:value={winEnd} onchange={saveWindow} disabled={savingAuto}>
                {#each hours as h (h)}<option value={String(h)} disabled={String(h) === winStart}>{pad(h)}:00</option>{/each}
              </select>
            {/if}
            <span class="text-xs text-subtle">server time{info.server_time ? ` (now ${info.server_time.slice(11, 16)})` : ''}</span>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</section>

<ConfirmDialog
  bind:open={rollbackOpen}
  title={`Roll back to ${rollbackTarget?.version ?? ''}?`}
  message={rollbackMessage}
  confirmLabel="Roll back"
  destructive
  onconfirm={doRollback} />

<ConfirmDialog
  bind:open={confirmOpen}
  title={`Update to ${info?.latest?.tag ?? 'the latest release'}?`}
  message="The release is downloaded and its signature verified, the panel binary, web app and database are backed up, then the services restart. The panel is unavailable for about a minute; sites keep running. If the new version is not healthy, the previous one is restored automatically."
  confirmLabel="Update now"
  onconfirm={startUpdate} />
