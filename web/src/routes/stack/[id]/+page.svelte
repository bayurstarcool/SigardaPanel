<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import {
    Layers, Globe, Code2, FileCode, Box, Lock, Database, Cpu, Zap,
    ArrowLeft, RefreshCw, Download, Trash2, Play, Square, RotateCw,
    CheckCircle2, XCircle, AlertCircle, Loader2, Settings, Terminal
  } from 'lucide-svelte';
  import PhpManager from './PhpManager.svelte';

  const serverId = $derived($page.params.id);

  let server = $state<any>(null);
  let packages = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let actionPending = $state('');
  let actionResult = $state<{ type: 'success' | 'error'; message: string } | null>(null);
  let osInfo = $state('');
  let showOutput = $state(false);
  let lastOutput = $state('');
  let showPhpManager = $state(false);

  const knownPackages = [
    { name: 'nginx', label: 'Nginx', icon: Globe, color: 'emerald', desc: 'Web server & reverse proxy' },
    { name: 'php', label: 'PHP', icon: FileCode, color: 'indigo', desc: 'PHP-FPM runtime + extensions' },
    { name: 'node', label: 'Node.js', icon: Code2, color: 'green', desc: 'JavaScript runtime + npm' },
    { name: 'python', label: 'Python', icon: Terminal, color: 'yellow', desc: 'Python 3 + pip + venv' },
    { name: 'go', label: 'Go', icon: Box, color: 'cyan', desc: 'Go programming language' },
    { name: 'docker', label: 'Docker', icon: Box, color: 'sky', desc: 'Container runtime' },
    { name: 'certbot', label: 'Certbot', icon: Lock, color: 'amber', desc: 'SSL/TLS certificates' },
    { name: 'mariadb', label: 'MariaDB', icon: Database, color: 'purple', desc: 'MySQL-compatible database' },
    { name: 'redis', label: 'Redis', icon: Zap, color: 'red', desc: 'In-memory data store' },
    { name: 'supervisor', label: 'Supervisor', icon: Settings, color: 'slate', desc: 'Process manager' },
  ];

  function loadServer() {
    if (!auth.token) { window.location.href = '/login'; return; }
    apiFetch(auth.token, 'GET', `servers/${serverId}`)
      .then((d: any) => server = d)
      .catch((e) => { error = e.message; });
  }

  function loadStack() {
    if (!auth.token) return;
    loading = true;
    error = '';
    apiFetch(auth.token, 'GET', `servers/${serverId}/stack`)
      .then((d: any) => {
        // API returns { data: { packages: [...] } } after apiFetch extracts json.data
        const stackData = d?.data ?? d;
        packages = stackData?.packages ?? [];
        osInfo = `${stackData?.os ?? 'unknown'} / ${stackData?.arch ?? 'unknown'}`;
      })
      .catch((e) => { error = e.message; })
      .finally(() => loading = false);
  }

  onMount(() => { loadServer(); loadStack(); });

  function getPackageInfo(name: string) {
    return packages.find(p => p.name === name);
  }

  function statusConfig(status: string) {
    switch (status) {
      case 'running': return { label: 'Running', color: 'emerald', dot: 'bg-emerald-400', text: 'text-emerald-400', bg: 'bg-emerald-400/10', border: 'border-emerald-400/20' };
      case 'installed': return { label: 'Installed', color: 'sky', dot: 'bg-sky-400', text: 'text-sky-400', bg: 'bg-sky-400/10', border: 'border-sky-400/20' };
      case 'stopped': return { label: 'Stopped', color: 'yellow', dot: 'bg-yellow-400', text: 'text-yellow-400', bg: 'bg-yellow-400/10', border: 'border-yellow-400/20' };
      case 'missing': return { label: 'Not Installed', color: 'slate', dot: 'bg-slate-500', text: 'text-slate-500', bg: 'bg-slate-500/10', border: 'border-slate-500/20' };
      default: return { label: 'Unknown', color: 'slate', dot: 'bg-slate-500', text: 'text-slate-500', bg: 'bg-slate-500/10', border: 'border-slate-500/20' };
    }
  }

  async function doAction(pkg: string, action: string, version?: string) {
    if (!auth.token) return;
    actionPending = `${pkg}:${action}`;
    actionResult = null;
    lastOutput = '';
    try {
      const body: any = { package: pkg, action };
      if (version) body.version = version;
      const result = await apiFetch(auth.token, 'POST', `servers/${serverId}/stack/action`, body);
      lastOutput = result?.output || '';
      if (result?.ok) {
        actionResult = { type: 'success', message: result?.message || `${action} ${pkg} completed` };
      } else {
        actionResult = { type: 'error', message: result?.message || `${action} ${pkg} failed` };
      }
      setTimeout(loadStack, 1500);
    } catch (e: any) {
      actionResult = { type: 'error', message: e.message };
    } finally {
      actionPending = '';
    }
  }

  function pkgColor(name: string) {
    const pkg = knownPackages.find(p => p.name === name);
    return pkg?.color || 'slate';
  }

  function pkgIcon(name: string) {
    const pkg = knownPackages.find(p => p.name === name);
    return pkg?.icon || Box;
  }
</script>

<DashboardLayout active="Stack">
  <!-- Breadcrumb -->
  <div class="flex items-center gap-2 text-sm text-slate-500 mb-6">
    <a href="/stack" class="hover:text-white transition no-underline text-slate-500 flex items-center gap-1">
      <ArrowLeft size={14} />
      Stack
    </a>
    <span class="text-slate-600">/</span>
    <span class="text-white">{server?.name ?? '...'}</span>
  </div>

  <!-- Hero Header -->
  <header class="relative mb-8 overflow-hidden rounded-2xl border border-white/[0.06] p-8 lg:p-10 bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-secondary)]/80">
    <div class="absolute top-0 right-0 w-96 h-96 rounded-full bg-sky-500/5 blur-3xl -translate-y-1/2 translate-x-1/2"></div>
    <div class="absolute inset-0" style="background-image: radial-gradient(rgba(255,255,255,0.15) 1px, transparent 1px); background-size: 24px 24px;"></div>
    <div class="relative flex items-start justify-between">
      <div>
        <p class="mb-2 text-[0.65rem] font-bold uppercase tracking-[0.25em] text-sky-300/60">SERVER STACK</p>
        <h1 class="text-2xl lg:text-3xl font-extrabold tracking-tight bg-gradient-to-r from-white via-sky-100 to-indigo-200 bg-clip-text text-transparent">
          {server?.name ?? 'Loading...'}
        </h1>
        <p class="mt-2 text-sm text-slate-400">
          {#if osInfo}
            <span class="font-mono text-xs bg-white/[0.05] px-2 py-0.5 rounded">{osInfo}</span>
          {/if}
          {server?.hostname ? `· ${server.hostname}` : ''}
        </p>
      </div>
      <button
        onclick={() => loadStack()}
        disabled={loading}
        class="flex items-center gap-2 px-4 py-2 rounded-xl bg-white/[0.05] border border-white/[0.08] text-sm text-slate-300 hover:bg-white/[0.08] hover:text-white transition-all disabled:opacity-50 cursor-pointer"
      >
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
        Refresh
      </button>
    </div>
  </header>

  <!-- Error / Result Toast -->
  {#if error}
    <div class="mb-6 p-4 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm flex items-center gap-2">
      <XCircle size={16} />
      {error}
    </div>
  {/if}

  {#if actionResult}
    <div class="mb-6 p-4 rounded-xl flex items-center gap-2 text-sm {actionResult.type === 'success' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-400' : 'bg-red-500/10 border border-red-500/20 text-red-400'}">
      {#if actionResult.type === 'success'}
        <CheckCircle2 size={16} />
      {:else}
        <XCircle size={16} />
      {/if}
      {actionResult.message}
      {#if lastOutput}
        <button onclick={() => showOutput = !showOutput} class="ml-auto text-xs underline opacity-70 hover:opacity-100 cursor-pointer bg-transparent border-0 text-inherit">
          {showOutput ? 'Hide' : 'Show'} output
        </button>
      {/if}
    </div>
    {#if showOutput && lastOutput}
      <div class="mb-6 p-4 rounded-xl bg-black/30 border border-white/[0.06] font-mono text-xs text-slate-400 max-h-48 overflow-auto whitespace-pre-wrap">
        {lastOutput}
      </div>
    {/if}
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-20">
      <div class="flex flex-col items-center gap-4">
        <Loader2 size={24} class="text-sky-400 animate-spin" />
        <span class="text-sm text-slate-500">Detecting installed software...</span>
      </div>
    </div>
  {:else}
    <!-- Quick Actions Bar -->
    <div class="mb-6 p-4 rounded-2xl bg-white/[0.02] border border-white/[0.06]">
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-xs text-slate-500 font-medium mr-2">Quick Actions:</span>
        <button
          onclick={() => doAction('nginx', 'restart')}
          disabled={actionPending !== ''}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/20 transition-all disabled:opacity-40 cursor-pointer"
        >
          {#if actionPending === 'nginx:restart'}
            <Loader2 size={12} class="animate-spin" />
          {:else}
            <RotateCw size={12} />
          {/if}
          Restart Nginx
        </button>
        <button
          onclick={() => doAction('php', 'restart')}
          disabled={actionPending !== ''}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs bg-indigo-500/10 text-indigo-400 hover:bg-indigo-500/20 border border-indigo-500/20 transition-all disabled:opacity-40 cursor-pointer"
        >
          {#if actionPending === 'php:restart'}
            <Loader2 size={12} class="animate-spin" />
          {:else}
            <RotateCw size={12} />
          {/if}
          Restart PHP
        </button>
        <button
          onclick={() => doAction('redis', 'restart')}
          disabled={actionPending !== ''}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs bg-red-500/10 text-red-400 hover:bg-red-500/20 border border-red-500/20 transition-all disabled:opacity-40 cursor-pointer"
        >
          {#if actionPending === 'redis:restart'}
            <Loader2 size={12} class="animate-spin" />
          {:else}
            <RotateCw size={12} />
          {/if}
          Restart Redis
        </button>
        <button
          onclick={() => doAction('docker', 'restart')}
          disabled={actionPending !== ''}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs bg-sky-500/10 text-sky-400 hover:bg-sky-500/20 border border-sky-500/20 transition-all disabled:opacity-40 cursor-pointer"
        >
          {#if actionPending === 'docker:restart'}
            <Loader2 size={12} class="animate-spin" />
          {:else}
            <RotateCw size={12} />
          {/if}
          Restart Docker
        </button>
      </div>
    </div>

    <!-- Package Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
      {#each knownPackages as pkg}
        {@const info = getPackageInfo(pkg.name)}
        {@const status = info?.status ?? 'missing'}
        {@const sc = statusConfig(status)}
        {@const Icon = pkg.icon}
        {@const isActive = actionPending.startsWith(pkg.name + ':')}

        <div class="group relative overflow-hidden rounded-2xl border border-white/[0.06] bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-secondary)]/80 hover:border-white/[0.12] transition-all duration-300">
          <!-- Header -->
          <div class="p-5 pb-4">
            <div class="flex items-start justify-between mb-4">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-xl bg-{pkg.color}-500/10 flex items-center justify-center border border-white/[0.06]">
                  <Icon size={18} class="text-{pkg.color}-400" />
                </div>
                <div>
                  <h3 class="font-semibold text-white">{pkg.label}</h3>
                  <p class="text-xs text-slate-500 mt-0.5">{pkg.desc}</p>
                </div>
              </div>
              <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded-lg text-[0.65rem] font-medium {sc.bg} {sc.text} border {sc.border}">
                <span class="w-1.5 h-1.5 rounded-full {sc.dot} {status === 'running' ? 'animate-pulse' : ''}"></span>
                {sc.label}
              </span>
            </div>

            {#if info?.version}
              <div class="text-xs text-slate-400 font-mono bg-white/[0.03] rounded-lg px-3 py-1.5 mb-4">
                v{info.version}
              </div>
            {/if}
          </div>

          <!-- Actions -->
          <div class="px-5 pb-5 pt-2 border-t border-white/[0.04]">
            {#if pkg.name === 'php'}
              <!-- PHP: Show Manage button for multiple versions -->
              <button
                onclick={() => showPhpManager = true}
                class="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white text-sm font-medium hover:from-indigo-400 hover:to-purple-400 transition-all shadow-lg shadow-indigo-500/20 no-underline cursor-pointer border-0">
                <Settings size={14} />
                Manage PHP Versions
              </button>
            {:else if status === 'missing'}
              <button
                onclick={() => doAction(pkg.name, 'install')}
                disabled={actionPending !== ''}
                class="w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-gradient-to-r from-sky-500 to-indigo-500 text-white text-sm font-medium hover:from-sky-400 hover:to-indigo-400 transition-all shadow-lg shadow-sky-500/20 disabled:opacity-50 cursor-pointer border-0"
              >
                {#if isActive}
                  <Loader2 size={14} class="animate-spin" />
                  Installing...
                {:else}
                  <Download size={14} />
                  Install {pkg.label}
                {/if}
              </button>
            {:else}
              <div class="flex items-center gap-1.5">
                {#if status === 'stopped'}
                  <button
                    onclick={() => doAction(pkg.name, 'start')}
                    disabled={actionPending !== ''}
                    class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/20 text-xs font-medium transition-all disabled:opacity-40 cursor-pointer"
                  >
                    {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<Play size={12} />{/if}
                    Start
                  </button>
                {/if}
                {#if status === 'running'}
                  <button
                    onclick={() => doAction(pkg.name, 'stop')}
                    disabled={actionPending !== ''}
                    class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20 border border-yellow-500/20 text-xs font-medium transition-all disabled:opacity-40 cursor-pointer"
                  >
                    {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<Square size={12} />{/if}
                    Stop
                  </button>
                {/if}
                <button
                  onclick={() => doAction(pkg.name, 'restart')}
                  disabled={actionPending !== ''}
                  class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-white/[0.03] text-slate-300 hover:bg-white/[0.06] border border-white/[0.06] text-xs font-medium transition-all disabled:opacity-40 cursor-pointer"
                >
                  {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<RotateCw size={12} />{/if}
                  Restart
                </button>
                <button
                  onclick={() => doAction(pkg.name, 'update')}
                  disabled={actionPending !== ''}
                  class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-white/[0.03] text-slate-300 hover:bg-white/[0.06] border border-white/[0.06] text-xs font-medium transition-all disabled:opacity-40 cursor-pointer"
                >
                  {#if isActive}<Loader2 size={12} class="animate-spin" />{:else}<RefreshCw size={12} />{/if}
                  Update
                </button>
                <button
                  onclick={() => { if (confirm(`Uninstall ${pkg.label}?`)) doAction(pkg.name, 'uninstall'); }}
                  disabled={actionPending !== ''}
                  class="flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-red-500/5 text-red-400/60 hover:bg-red-500/10 hover:text-red-400 border border-red-500/10 text-xs font-medium transition-all disabled:opacity-40 cursor-pointer"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</DashboardLayout>

<PhpManager open={showPhpManager} serverId={serverId} onClose={() => showPhpManager = false} onInstall={(ver) => { loadStack(); }} />
