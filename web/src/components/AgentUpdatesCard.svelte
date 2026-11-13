<script lang="ts">
  // Remote agent updates (super_admin): every remote server's agent can update
  // itself to the panel's version (signed release, binary backup, automatic
  // restore). The panel host's agent is updated together with the panel.
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtDateTime } from '$lib/format';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Server, RefreshCw, LoaderCircle, ArrowUpCircle, AlertTriangle, ChevronDown } from 'lucide-svelte';
  import { onMount, onDestroy } from 'svelte';

  type Row = {
    server_id: number;
    name: string;
    hostname: string;
    status: string;
    is_local: boolean;
    agent_version: string;
    update_available: boolean;
    error?: string;
    update?: {
      state: 'idle' | 'running' | 'succeeded' | 'failed';
      tag?: string;
      message?: string;
      finished_at?: string;
      log?: string;
      supported: boolean;
      unsupported_reason?: string;
    };
  };

  let rows = $state<Row[]>([]);
  let target = $state('');
  let autoUpdate = $state(false);
  let loading = $state(true);
  let refreshing = $state(false);
  let loadError = $state('');
  let starting = $state<Record<number, boolean>>({});
  let bulkOpen = $state(false);
  let bulkStarting = $state(false);
  let savingAuto = $state(false);
  let openLog = $state<number | null>(null);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let destroyed = false;

  const remote = $derived(rows.filter((r) => !r.is_local));
  const outdated = $derived(remote.filter((r) => r.update_available && !r.error && r.update?.supported && r.update?.state !== 'running'));
  const anyRunning = $derived(remote.some((r) => r.update?.state === 'running' || starting[r.server_id]));

  async function load(manual = false) {
    if (manual) refreshing = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'system/agent-updates');
      rows = d?.servers ?? [];
      target = d?.target_version ?? '';
      autoUpdate = !!d?.auto_update;
      loadError = '';
    } catch (e: any) {
      if (e?.message !== 'session expired') loadError = e?.message || 'Could not load the servers';
    } finally {
      loading = false;
      refreshing = false;
      schedule();
    }
  }

  // Poll quickly while an update runs, slowly otherwise.
  function schedule() {
    if (destroyed) return;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      if (document.hidden) schedule();
      else load();
    }, anyRunning ? 4000 : 60000);
  }

  async function updateOne(r: Row) {
    starting[r.server_id] = true;
    try {
      await apiFetch(auth.token, 'POST', `servers/${r.server_id}/agent-update`, {});
      toasts.success(`Updating the agent on ${r.name} to ${target}`);
      await load();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(`${r.name}: ${e?.message || 'could not start the update'}`);
    } finally {
      starting[r.server_id] = false;
    }
  }

  async function updateAll() {
    bulkStarting = true;
    try {
      const d = await apiFetch(auth.token, 'POST', 'system/agent-updates', { all: true });
      const failed = (d?.results ?? []).filter((x: any) => !x.ok);
      if (d?.started) toasts.success(`Updating ${d.started} agent${d.started === 1 ? '' : 's'} to ${target}`);
      for (const f of failed) toasts.error(`${f.name}: ${f.message}`);
      if (!d?.started && !failed.length) toasts.success('All agents are up to date');
      await load();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Could not start the updates');
    } finally {
      bulkStarting = false;
    }
  }

  async function toggleAuto(e: Event) {
    const box = e.currentTarget as HTMLInputElement;
    const enabled = box.checked;
    savingAuto = true;
    try {
      await apiFetch(auth.token, 'PUT', 'system/agent-updates/settings', { auto_update: enabled });
      autoUpdate = enabled;
      toasts.success(enabled ? 'Agents are updated automatically' : 'Automatic agent updates disabled');
    } catch (err: any) {
      box.checked = !enabled;
      if (err?.message !== 'session expired') toasts.error(err?.message || 'Could not save the setting');
    } finally {
      savingAuto = false;
    }
  }

  function stateOf(r: Row): { label: string; tone: string } {
    if (r.is_local) return { label: 'With the panel', tone: 'badge' };
    if (starting[r.server_id] || r.update?.state === 'running') return { label: 'Updating', tone: 'badge badge-info' };
    if (r.error) return { label: 'Unavailable', tone: 'badge badge-warning' };
    if (r.update && !r.update.supported) return { label: 'Not supported', tone: 'badge badge-warning' };
    if (r.update_available) return { label: 'Update available', tone: 'badge badge-accent' };
    return { label: 'Up to date', tone: 'badge badge-success' };
  }

  onMount(() => load());
  onDestroy(() => {
    destroyed = true;
    if (timer) clearTimeout(timer);
  });
</script>

<section class="card md:col-span-2">
  <div class="card-header">
    <h2 class="card-title flex items-center gap-2">
      <Server size={16} class="text-subtle" /> Server agents
      {#if target}<span class="text-xs font-normal text-subtle">target {target}</span>{/if}
    </h2>
    <div class="flex items-center gap-2">
      <button class="btn btn-secondary btn-sm" onclick={() => load(true)} disabled={refreshing || loading} aria-label="Refresh agent list">
        <RefreshCw size={14} class={refreshing ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if outdated.length}
        <button class="btn btn-primary btn-sm" onclick={() => (bulkOpen = true)} disabled={bulkStarting}>
          {#if bulkStarting}<LoaderCircle size={14} class="animate-spin" />{:else}<ArrowUpCircle size={14} />{/if}
          Update {outdated.length} agent{outdated.length === 1 ? '' : 's'}
        </button>
      {/if}
    </div>
  </div>

  <div class="card-body space-y-4">
    {#if loading}
      <div class="space-y-2">{#each [1, 2] as i (i)}<div class="skeleton h-9"></div>{/each}</div>
    {:else if loadError}
      <p class="text-[13px] text-danger">{loadError}</p>
    {:else if remote.length === 0}
      <p class="text-[13px] text-muted">No remote servers. The panel host's agent is updated together with the panel.</p>
    {:else}
      {#if !target}
        <p class="flex items-start gap-1.5 text-[13px] text-warning"><AlertTriangle size={14} class="mt-0.5 shrink-0" /> The panel is a development build: agents can only be updated to a release.</p>
      {/if}
      <div class="table-wrap">
        <table class="table">
          <thead><tr><th>Server</th><th>Agent</th><th>Status</th><th></th></tr></thead>
          <tbody>
            {#each rows as r (r.server_id)}
              {@const st = stateOf(r)}
              <tr>
                <td>
                  <span class="block text-sm text-fg">{r.name}</span>
                  <span class="block text-xs text-subtle max-sm:hidden">{r.hostname}</span>
                </td>
                <td class="font-mono text-[13px]">{r.agent_version || '—'}</td>
                <td class="text-[13px]">
                  <span class={st.tone}>{st.label}</span>
                  {#if r.error}
                    <span class="mt-1 block text-xs text-muted">{r.error}</span>
                  {:else if r.update && !r.update.supported && !r.is_local}
                    <span class="mt-1 block text-xs text-muted">{r.update.unsupported_reason}</span>
                  {:else if r.update?.state === 'failed'}
                    <span class="mt-1 block text-xs text-danger">{r.update.message}</span>
                  {:else if r.update?.state === 'succeeded' && r.update.finished_at}
                    <span class="mt-1 block text-xs text-subtle">{r.update.message} · {fmtDateTime(r.update.finished_at)}</span>
                  {/if}
                  {#if r.update?.log}
                    <button class="btn btn-ghost btn-xs mt-1" onclick={() => (openLog = openLog === r.server_id ? null : r.server_id)} aria-expanded={openLog === r.server_id}>
                      <ChevronDown size={12} class={openLog === r.server_id ? 'rotate-180' : ''} /> Log
                    </button>
                  {/if}
                </td>
                <td class="text-right">
                  {#if !r.is_local && r.update_available && !r.error && r.update?.supported}
                    <button class="btn btn-secondary btn-xs" onclick={() => updateOne(r)} disabled={starting[r.server_id] || r.update?.state === 'running'}>
                      {#if starting[r.server_id] || r.update?.state === 'running'}<LoaderCircle size={12} class="animate-spin" />{:else}<ArrowUpCircle size={12} />{/if} Update
                    </button>
                  {/if}
                </td>
              </tr>
              {#if openLog === r.server_id && r.update?.log}
                <tr>
                  <td colspan="4">
                    <pre class="max-h-60 overflow-auto rounded-lg border border-line bg-bg p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted">{r.update.log}</pre>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      <p class="text-xs text-subtle">Agents installed before this version need one manual update with the agent install command (Servers → Add server); after that they update from here.</p>
    {/if}

    <div class="flex items-start gap-3 border-t border-line pt-4">
      <input id="agent-auto-update" type="checkbox" class="checkbox mt-0.5" checked={autoUpdate} onchange={toggleAuto} disabled={savingAuto || loading} />
      <label for="agent-auto-update" class="min-w-0">
        <span class="block text-sm text-fg">Update remote agents automatically</span>
        <span class="hint block">After the panel is updated, every remote agent older than the panel is updated to the same version (once per server and release). Super admins are notified of every result.</span>
      </label>
    </div>
  </div>
</section>

<ConfirmDialog
  bind:open={bulkOpen}
  title={`Update ${outdated.length} agent${outdated.length === 1 ? '' : 's'} to ${target}?`}
  message="Each agent downloads and verifies the signed release, backs up its binary and restarts (a few seconds without the agent; sites keep running). An agent that does not come back healthy restores its previous version by itself."
  confirmLabel="Update"
  onconfirm={updateAll} />
