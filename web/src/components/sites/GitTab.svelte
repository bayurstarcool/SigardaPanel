<script lang="ts">
  // A site's Git tab: connect it to a GitHub / GitLab / Gitea repository (or
  // any repository URL), deploy settings (auto deploy on push, build, deploy
  // script) and the deploy history with logs, redeploy and rollback.
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtRelative, fmtDateTime } from '$lib/format';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import {
    GitBranch, GitCommitHorizontal, LoaderCircle, Upload, RotateCcw, RefreshCw, ExternalLink, Check, X, Clock,
    Search, Link2, Unplug, Webhook, KeyRound, ScrollText, Save, Lock, Globe,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let { siteId, site, onsite }: { siteId: string | number; site: any; onsite?: () => void } = $props();

  let info = $state<any>(null);
  let loading = $state(true);

  async function load() {
    try {
      info = await apiFetch(auth.token, 'GET', `sites/${siteId}/git`);
      if (info?.git) {
        autoDeploy = info.git.auto_deploy;
        build = info.git.build_enabled;
        if (!settingsDirty) script = info.git.deploy_script || '';
      }
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  const providerName: Record<string, string> = { github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' };
  const connected = $derived(!!info?.git?.connection_id);
  const configured = $derived(!!info?.repo);
  const active = $derived((info?.active_deploys || 0) > 0);

  // Poll while a deploy is queued or running.
  $effect(() => {
    if (!active) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  });

  // ── connect wizard
  let mode = $state<'provider' | 'url'>('provider');
  let conns = $state<any[]>([]);
  let connID = $state<number>(0);
  let repoQuery = $state('');
  let repos = $state<any[]>([]);
  let reposLoading = $state(false);
  let repo = $state<any>(null);
  let branches = $state<any[]>([]);
  let branch = $state('');
  let connectBusy = $state(false);
  let manualURL = $state('');
  let manualBranch = $state('main');

  async function loadConns() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'git/connections');
      conns = d?.connections || [];
      if (conns.length && !connID) connID = conns[0].id;
      if (!conns.length) mode = 'url';
    } catch {}
  }
  let searchTimer: ReturnType<typeof setTimeout>;
  async function searchRepos() {
    if (!connID) return;
    reposLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `git/connections/${connID}/repos?q=${encodeURIComponent(repoQuery.trim())}`);
      repos = d?.repos || [];
    } catch (e: any) {
      toasts.error(e.message);
      repos = [];
    } finally {
      reposLoading = false;
    }
  }
  $effect(() => {
    // Re-search when the connection or the query changes (debounced).
    void connID; void repoQuery;
    if (!connID || configured) return;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(searchRepos, 300);
    return () => clearTimeout(searchTimer);
  });
  async function pickRepo(r: any) {
    repo = r;
    branch = r.default_branch || 'main';
    branches = [];
    try {
      const d = await apiFetch(auth.token, 'GET', `git/connections/${connID}/branches?repo=${encodeURIComponent(r.full_name)}`);
      branches = d?.branches || [];
    } catch {}
  }
  async function connect() {
    if (!repo) return;
    connectBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/git/connect`, {
        connection_id: connID, repo: repo.full_name, branch, auto_deploy: autoDeploy, build_enabled: build, deploy_script: script, deploy_now: true,
      });
      toasts.success(`Connected to ${d.repo}; the first deploy is running`);
      for (const w of d.warnings || []) toasts.warning(w, 8000);
      repo = null;
      settingsDirty = false;
      await load();
      onsite?.();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      connectBusy = false;
    }
  }
  async function saveManual() {
    connectBusy = true;
    try {
      await apiFetch(auth.token, 'PATCH', `sites/${siteId}/git`, { repo_url: manualURL.trim(), branch: manualBranch.trim() || 'main', auto_deploy: autoDeploy, build_enabled: build, deploy_script: script });
      toasts.success('Repository saved');
      settingsDirty = false;
      await load();
      onsite?.();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      connectBusy = false;
    }
  }

  // ── settings
  let autoDeploy = $state(true);
  let build = $state(true);
  let script = $state('');
  let settingsDirty = $state(false);
  let settingsBusy = $state(false);
  let branchEdit = $state('');
  async function saveSettings() {
    settingsBusy = true;
    try {
      const body: any = { auto_deploy: autoDeploy, build_enabled: build, deploy_script: script };
      if (branchEdit && branchEdit !== info.branch) body.branch = branchEdit;
      await apiFetch(auth.token, 'PATCH', `sites/${siteId}/git`, body);
      toasts.success('Deploy settings saved');
      settingsDirty = false;
      await load();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      settingsBusy = false;
    }
  }
  async function loadBranchesForSite() {
    if (!connected || !info?.connection_owned) return;
    try {
      const d = await apiFetch(auth.token, 'GET', `git/connections/${info.git.connection_id}/branches?repo=${encodeURIComponent(info.git.repo_full_name)}`);
      branches = d?.branches || [];
    } catch {}
  }

  // ── actions
  let deployBusy = $state(false);
  async function deployNow() {
    deployBusy = true;
    try {
      await apiFetch(auth.token, 'POST', `sites/${siteId}/deploy`);
      toasts.success('Deploy queued');
      await load();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      deployBusy = false;
    }
  }
  let disconnectOpen = $state(false);
  async function disconnect() {
    try {
      const d = await apiFetch(auth.token, 'DELETE', `sites/${siteId}/git`);
      toasts.success('Disconnected; the site files stay as they are');
      for (const n of d.notes || []) toasts.warning(n, 8000);
      await load();
      onsite?.();
      void loadConns();
    } catch (e: any) {
      toasts.error(e.message);
    }
  }
  let confirmDep = $state<{ dep: any; kind: 'rollback' | 'redeploy' } | null>(null);
  let confirmOpen = $state(false);
  async function runAgain() {
    if (!confirmDep) return;
    try {
      await apiFetch(auth.token, 'POST', `sites/${siteId}/deployments/${confirmDep.dep.id}/${confirmDep.kind}`);
      toasts.success(confirmDep.kind === 'rollback' ? `Rolling back to ${confirmDep.dep.commit_sha.slice(0, 7)}` : 'Redeploy queued');
      await load();
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  // ── log viewer
  let logOpen = $state(false);
  let logDep = $state<any>(null);
  let logTimer: ReturnType<typeof setInterval> | undefined;
  async function openLog(d: any) {
    logDep = { ...d, log: '' };
    logOpen = true;
    await refreshLog();
    clearInterval(logTimer);
    logTimer = setInterval(() => {
      if (!logOpen || !['queued', 'running'].includes(logDep?.status)) { clearInterval(logTimer); return; }
      void refreshLog();
    }, 2500);
  }
  async function refreshLog() {
    try { logDep = await apiFetch(auth.token, 'GET', `sites/${siteId}/deployments/${logDep.id}`); } catch {}
  }

  function commitURL(sha: string): string {
    const base = info?.git?.repo_html_url;
    if (!base || !sha) return '';
    return info.git.provider === 'gitlab' ? `${base}/-/commit/${sha}` : `${base}/commit/${sha}`;
  }
  function duration(d: any): string {
    if (!d.started_at || !d.finished_at) return '';
    const s = Math.max(0, Math.round((new Date(d.finished_at).getTime() - new Date(d.started_at).getTime()) / 1000));
    return s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s`;
  }
  const triggerLabel: Record<string, string> = { manual: 'Manual', push: 'Push', rollback: 'Rollback', redeploy: 'Redeploy', connect: 'First deploy' };
  const statusTone: Record<string, string> = { succeeded: 'badge-success', failed: 'badge-danger', running: 'badge-accent', queued: '' };
  const lastGood = $derived(info?.deployments?.find((d: any) => d.status === 'succeeded'));

  onMount(() => {
    void load();
    void loadConns();
    return () => clearInterval(logTimer);
  });
  $effect(() => { if (info) branchEdit = info.branch; });
  $effect(() => { if (connected && info?.connection_owned && branches.length === 0) void loadBranchesForSite(); });
</script>

{#if loading}
  <div class="space-y-4">{#each Array(2) as _, i (i)}<div class="skeleton h-40 rounded-xl"></div>{/each}</div>
{:else if info}
  <div class="space-y-4">
    {#if !configured}
      <!-- Connect -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><GitBranch size={16} class="text-subtle" /> Deploy from git</h2>
        </div>
        <div class="card-body space-y-4">
          {#if !info.can_manage && !info.can_configure}
            <p class="text-[13px] text-muted">No repository is configured. The site owner can connect one.</p>
          {:else}
            <div class="segmented" role="group" aria-label="Source">
              <button onclick={() => (mode = 'provider')} class="segment {mode === 'provider' ? 'segment-active' : ''}">GitHub · GitLab · Gitea</button>
              <button onclick={() => (mode = 'url')} class="segment {mode === 'url' ? 'segment-active' : ''}">Repository URL</button>
            </div>

            {#if mode === 'provider'}
              {#if !info.can_manage}
                <p class="text-[13px] text-muted">Only the site owner can connect it through their git account.</p>
              {:else if conns.length === 0}
                <div class="rounded-lg border border-dashed border-line p-5 text-center">
                  <p class="text-sm font-medium text-fg">Connect your git account first</p>
                  <p class="mt-1 text-[13px] text-muted">Private repositories, automatic deploys on push and deploy status on your commits.</p>
                  <a href="/settings/integrations" class="btn btn-primary btn-sm mt-3"><Link2 size={14} /> Connect GitHub, GitLab or Gitea</a>
                </div>
              {:else}
                <div class="grid gap-4 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
                  <div>
                    <label for="git-conn" class="label">Account</label>
                    <select id="git-conn" bind:value={connID} onchange={() => { repo = null; }} class="input">
                      {#each conns as c (c.id)}<option value={c.id}>{providerName[c.provider] || c.provider} · @{c.username}</option>{/each}
                    </select>
                    <a href="/settings/integrations" class="hint inline-block hover:text-fg">Manage accounts</a>
                  </div>
                  <div class="min-w-0">
                    <label for="git-search" class="label">Repository</label>
                    {#if repo}
                      <div class="flex items-center justify-between gap-2 rounded-lg border border-accent/40 bg-accent/5 px-3 py-2">
                        <span class="flex min-w-0 items-center gap-2 text-[13.5px] font-medium text-fg">
                          {#if repo.private}<Lock size={13} class="shrink-0 text-subtle" />{:else}<Globe size={13} class="shrink-0 text-subtle" />{/if}
                          <span class="truncate">{repo.full_name}</span>
                        </span>
                        <button onclick={() => (repo = null)} class="btn btn-ghost btn-xs">Change</button>
                      </div>
                    {:else}
                      <div class="relative">
                        <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
                        <input id="git-search" bind:value={repoQuery} class="input pl-8" placeholder="Search your repositories" autocomplete="off" />
                      </div>
                      <div class="mt-2 max-h-64 overflow-y-auto rounded-lg border border-line">
                        {#if reposLoading && repos.length === 0}
                          <div class="flex items-center gap-2 px-3 py-3 text-[13px] text-subtle"><LoaderCircle size={14} class="animate-spin" /> Loading repositories…</div>
                        {:else if repos.length === 0}
                          <div class="px-3 py-3 text-[13px] text-subtle">No repositories found{repoQuery ? ` for “${repoQuery}”` : ''}.</div>
                        {:else}
                          {#each repos as r (r.full_name)}
                            <button onclick={() => pickRepo(r)} class="flex w-full cursor-pointer items-center gap-2.5 border-b border-line px-3 py-2 text-left last:border-b-0 hover:bg-surface-2">
                              {#if r.private}<Lock size={13} class="shrink-0 text-subtle" />{:else}<Globe size={13} class="shrink-0 text-subtle" />{/if}
                              <span class="min-w-0 flex-1">
                                <span class="block truncate text-[13px] font-medium text-fg">{r.full_name}</span>
                                {#if r.description}<span class="block truncate text-[11.5px] text-subtle">{r.description}</span>{/if}
                              </span>
                              {#if !r.can_admin}<span class="badge badge-warning" title="Admin/maintainer access is needed for the deploy key and webhook">no admin</span>{/if}
                            </button>
                          {/each}
                        {/if}
                      </div>
                    {/if}
                  </div>
                </div>
                {#if repo}
                  <div>
                    <label for="git-branch-pick" class="label">Branch</label>
                    {#if branches.length}
                      <select id="git-branch-pick" bind:value={branch} class="input font-mono text-[13px] md:w-72">
                        {#each branches as b (b.name)}<option value={b.name}>{b.name}</option>{/each}
                      </select>
                    {:else}
                      <input id="git-branch-pick" bind:value={branch} class="input font-mono text-[13px] md:w-72" />
                    {/if}
                  </div>
                {/if}
              {/if}
            {:else}
              <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_14rem]">
                <div>
                  <label for="git-url" class="label">Repository URL</label>
                  <input id="git-url" bind:value={manualURL} class="input font-mono text-[13px]" placeholder="https://github.com/user/repo.git" />
                  <p class="hint">Public repositories over https. For private ones, connect your git account instead (it sets up a deploy key).</p>
                </div>
                <div>
                  <label for="git-url-branch" class="label">Branch</label>
                  <input id="git-url-branch" bind:value={manualBranch} class="input font-mono text-[13px]" />
                </div>
              </div>
            {/if}

            {#if (mode === 'provider' && repo) || mode === 'url'}
              {@render settingsFields()}
              <div class="flex items-center gap-2">
                {#if mode === 'provider'}
                  <button onclick={connect} disabled={connectBusy || !repo || !repo.can_admin} class="btn btn-primary btn-sm">
                    {#if connectBusy}<LoaderCircle size={14} class="animate-spin" /> Connecting…{:else}<Link2 size={14} /> Connect and deploy{/if}
                  </button>
                  <p class="text-[12px] text-subtle">Adds a read-only deploy key{info.public_url_set ? ' and a push webhook' : ''} to {repo.full_name}.</p>
                {:else}
                  <button onclick={saveManual} disabled={connectBusy || !manualURL.trim() || !info.can_configure} class="btn btn-primary btn-sm"><Save size={14} /> Save repository</button>
                {/if}
              </div>
            {/if}
          {/if}
        </div>
      </section>
    {:else}
      <!-- Repository -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><GitBranch size={16} class="text-subtle" /> Repository</h2>
          <div class="flex items-center gap-2">
            {#if info.can_deploy}
              <button onclick={deployNow} disabled={deployBusy || active} class="btn btn-primary btn-sm">
                {#if deployBusy || active}<LoaderCircle size={14} class="animate-spin" /> {active ? 'Deploying…' : 'Queuing…'}{:else}<Upload size={14} /> Deploy now{/if}
              </button>
            {/if}
            {#if info.can_manage}
              <button onclick={() => (disconnectOpen = true)} class="btn btn-ghost btn-sm" title="Stop deploying from git"><Unplug size={14} /> Disconnect</button>
            {/if}
          </div>
        </div>
        <div class="card-body grid gap-x-6 gap-y-4 text-[13px] sm:grid-cols-2 lg:grid-cols-4">
          <div class="min-w-0">
            <div class="stat-label">Repository</div>
            {#if connected}
              <a href={info.git.repo_html_url} target="_blank" rel="noopener" class="mt-1 flex min-w-0 items-center gap-1 font-medium text-fg hover:text-accent">
                <span class="truncate">{info.git.repo_full_name}</span><ExternalLink size={12} class="shrink-0 text-subtle" />
              </a>
              <div class="mt-0.5 text-[12px] text-subtle">{providerName[info.git.provider] || info.git.provider}{#if info.connection} · @{info.connection.username}{/if}</div>
            {:else}
              <div class="mt-1 truncate font-mono text-[12.5px] text-fg" title={info.repo}>{info.repo}</div>
              <div class="mt-0.5 text-[12px] text-subtle">Repository URL</div>
            {/if}
          </div>
          <div>
            <div class="stat-label">Branch</div>
            <div class="mt-1 flex items-center gap-1.5 font-mono text-[12.5px] text-fg"><GitBranch size={13} class="text-subtle" /> {info.branch}</div>
          </div>
          <div>
            <div class="stat-label">Deployed commit</div>
            {#if info.commit}
              {@const url = commitURL(info.commit)}
              <div class="mt-1 flex items-center gap-1.5 font-mono text-[12.5px] text-fg">
                <GitCommitHorizontal size={13} class="text-subtle" />
                {#if url}<a href={url} target="_blank" rel="noopener" class="hover:text-accent">{info.commit.slice(0, 7)}</a>{:else}{info.commit.slice(0, 7)}{/if}
              </div>
              {#if lastGood}<div class="mt-0.5 truncate text-[12px] text-subtle" title={lastGood.commit_message}>{lastGood.commit_message}</div>{/if}
            {:else}
              <div class="mt-1 text-subtle">Not deployed yet</div>
            {/if}
          </div>
          <div>
            <div class="stat-label">Automation</div>
            {#if connected}
              <div class="mt-1 space-y-1 text-[12.5px]">
                <div class="flex items-center gap-1.5 {info.git.webhook_id ? 'text-success' : 'text-subtle'}"><Webhook size={13} /> {info.git.webhook_id ? (info.git.auto_deploy ? 'Deploys on push' : 'Webhook on, auto deploy off') : 'No webhook'}</div>
                <div class="flex items-center gap-1.5 text-success"><KeyRound size={13} /> Read-only deploy key</div>
              </div>
            {:else}
              <div class="mt-1 text-[12.5px] text-subtle">{info.git?.auto_deploy === false ? 'Auto deploy off' : 'Push webhook (manual setup)'}</div>
            {/if}
          </div>
        </div>
        {#if !connected && info.can_configure}
          <div class="border-t border-line px-5 py-3 text-[12.5px] text-muted">
            Deploy on push: add <span class="font-mono text-fg">{info.webhook_url.startsWith('/') ? window.location.origin + info.webhook_url : info.webhook_url}</span> as a push webhook (JSON) with the secret from the Info tab.
          </div>
        {:else if connected && !info.git.webhook_id && info.can_configure}
          <div class="border-t border-line px-5 py-3 text-[12.5px] text-muted">
            {info.public_url_set ? 'The webhook could not be created; deploy from here, or reconnect.' : 'Pushes cannot reach this panel (no public https URL set as SIGARDAPANEL_PUBLIC_URL), so deploy from here.'}
          </div>
        {/if}
      </section>

      <!-- Settings -->
      {#if info.can_configure}
        <section class="card">
          <div class="card-header">
            <h2 class="card-title">Deploy settings</h2>
            <button onclick={saveSettings} disabled={settingsBusy || (!settingsDirty && branchEdit === info.branch)} class="btn btn-secondary btn-sm">
              {#if settingsBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save
            </button>
          </div>
          <div class="card-body space-y-4">
            <div class="md:w-72">
              <label for="git-branch-edit" class="label">Branch</label>
              {#if branches.length}
                <select id="git-branch-edit" bind:value={branchEdit} class="input font-mono text-[13px]">
                  {#each branches as b (b.name)}<option value={b.name}>{b.name}</option>{/each}
                </select>
              {:else}
                <input id="git-branch-edit" bind:value={branchEdit} class="input font-mono text-[13px]" />
              {/if}
            </div>
            {@render settingsFields()}
          </div>
        </section>
      {/if}

      <!-- History -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Clock size={16} class="text-subtle" /> Deployments</h2>
          <button onclick={load} class="btn btn-ghost btn-icon btn-sm" aria-label="Refresh" title="Refresh"><RefreshCw size={14} class={active ? 'animate-spin' : ''} /></button>
        </div>
        {#if info.deployments.length === 0}
          <p class="card-body text-[13px] text-subtle">No deployments yet.</p>
        {:else}
          <ul class="divide-y divide-line">
            {#each info.deployments as d (d.id)}
              {@const url = commitURL(d.commit_sha)}
              <li class="flex flex-wrap items-center gap-x-3 gap-y-2 px-5 py-3">
                <span class="flex size-7 shrink-0 items-center justify-center rounded-full ring-1 ring-inset
                  {d.status === 'succeeded' ? 'bg-success/10 text-success ring-success/30' : d.status === 'failed' ? 'bg-danger/10 text-danger ring-danger/30' : 'bg-accent/10 text-accent ring-accent/30'}">
                  {#if d.status === 'succeeded'}<Check size={14} strokeWidth={2.5} />{:else if d.status === 'failed'}<X size={14} strokeWidth={2.5} />{:else}<LoaderCircle size={14} class="animate-spin" />{/if}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                    <span class="truncate text-[13.5px] font-medium text-fg" title={d.commit_message}>{d.commit_message || (d.status === 'queued' ? 'Waiting to start' : d.status === 'running' ? 'Deploying…' : 'Deploy')}</span>
                    <span class="badge {statusTone[d.status] || ''}">{d.status}</span>
                  </div>
                  <div class="mt-0.5 flex flex-wrap items-center gap-x-2 text-[12px] text-subtle">
                    {#if d.commit_sha}
                      {#if url}<a href={url} target="_blank" rel="noopener" class="font-mono hover:text-fg">{d.commit_sha.slice(0, 7)}</a>{:else}<span class="font-mono">{d.commit_sha.slice(0, 7)}</span>{/if}
                    {/if}
                    {#if d.commit_author}<span>· {d.commit_author}</span>{/if}
                    <span>· {triggerLabel[d.triggered_by] || d.triggered_by}</span>
                    <span title={fmtDateTime(d.created_at)}>· {fmtRelative(d.created_at)}</span>
                    {#if duration(d)}<span>· {duration(d)}</span>{/if}
                  </div>
                  {#if d.status === 'failed' && d.error}<div class="mt-1 truncate text-[12px] text-danger" title={d.error}>{d.error}</div>{/if}
                </div>
                <div class="flex items-center gap-1">
                  <button onclick={() => openLog(d)} class="btn btn-ghost btn-sm"><ScrollText size={14} /> Log</button>
                  {#if info.can_deploy && d.commit_sha && d.status !== 'queued' && d.status !== 'running'}
                    {#if d.commit_sha === info.commit}
                      <button onclick={() => { confirmDep = { dep: d, kind: 'redeploy' }; confirmOpen = true; }} disabled={active} class="btn btn-ghost btn-sm" title="Deploy this commit again"><RefreshCw size={14} /> Redeploy</button>
                    {:else if d.status === 'succeeded'}
                      <button onclick={() => { confirmDep = { dep: d, kind: 'rollback' }; confirmOpen = true; }} disabled={active} class="btn btn-ghost btn-sm" title="Deploy this commit"><RotateCcw size={14} /> Roll back</button>
                    {/if}
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
  </div>
{/if}

{#snippet settingsFields()}
  <div class="grid gap-3 sm:grid-cols-2">
    <label class="flex cursor-pointer items-start gap-2.5 rounded-lg border border-line p-3">
      <input type="checkbox" bind:checked={autoDeploy} onchange={() => (settingsDirty = true)} class="checkbox mt-0.5" />
      <span><span class="block text-[13px] font-medium text-fg">Deploy on push</span><span class="block text-[12px] text-subtle">Every push to the branch deploys automatically.</span></span>
    </label>
    <label class="flex cursor-pointer items-start gap-2.5 rounded-lg border border-line p-3">
      <input type="checkbox" bind:checked={build} onchange={() => (settingsDirty = true)} class="checkbox mt-0.5" />
      <span><span class="block text-[13px] font-medium text-fg">Install and build</span><span class="block text-[12px] text-subtle">composer install, npm ci + npm run build, pip / go, when the repository has them.</span></span>
    </label>
  </div>
  <div>
    <label for="git-script" class="label">Deploy script <span class="text-subtle">(optional)</span></label>
    <textarea id="git-script" bind:value={script} oninput={() => (settingsDirty = true)} rows="4" spellcheck="false" class="input min-h-24 font-mono text-[12.5px] leading-relaxed"
      placeholder={site?.runtime === 'php' ? 'php artisan migrate --force\nphp artisan config:cache' : 'npm run migrate'}></textarea>
    <p class="hint">Runs after the build in the site folder, as the site's Linux user (bash, stops at the first failing command). Tracked files changed on the server are replaced by the repository's on deploy; untracked files such as uploads and .env stay.</p>
  </div>
{/snippet}

<Modal bind:open={logOpen} title={logDep ? `Deployment #${logDep.id}` : 'Deployment'} maxWidth="4xl" onclose={() => clearInterval(logTimer)}>
  {#if logDep}
    <div class="mb-3 flex flex-wrap items-center gap-2 text-[12.5px] text-muted">
      <span class="badge {statusTone[logDep.status] || ''}">{logDep.status}</span>
      {#if logDep.commit_sha}<span class="font-mono">{logDep.commit_sha.slice(0, 7)}</span>{/if}
      {#if logDep.commit_message}<span class="truncate">{logDep.commit_message}</span>{/if}
      {#if duration(logDep)}<span>· {duration(logDep)}</span>{/if}
    </div>
    <pre class="max-h-[60vh] overflow-auto rounded-lg border border-line bg-surface-2 p-3 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-fg">{logDep.log || (['queued', 'running'].includes(logDep.status) ? 'Waiting for the server…' : logDep.error || 'No log')}</pre>
  {/if}
</Modal>

<ConfirmDialog bind:open={disconnectOpen} title="Disconnect the repository?"
  message={connected ? `The deploy key and webhook are removed from ${info?.git?.repo_full_name}, and the site stops deploying from git. Its files stay as they are.` : 'The site stops deploying from git. Its files stay as they are.'}
  confirmLabel="Disconnect" destructive onconfirm={disconnect} />

<ConfirmDialog bind:open={confirmOpen}
  title={confirmDep?.kind === 'rollback' ? `Roll back to ${confirmDep?.dep?.commit_sha?.slice(0, 7)}?` : 'Redeploy this commit?'}
  message={confirmDep ? `${confirmDep.dep.commit_message || confirmDep.dep.commit_sha} is checked out, built and deployed again${confirmDep.kind === 'rollback' ? '. The next push deploys the branch head again.' : '.'}` : ''}
  confirmLabel={confirmDep?.kind === 'rollback' ? 'Roll back' : 'Redeploy'} onconfirm={runAgain} />
