<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import CodeEditor from '$components/CodeEditor.svelte';
  import {
    Code, Loader2, Save, RefreshCw, Check, X, FileText, Wrench,
    ScrollText, Upload, ChevronDown, Copy, AlertTriangle,
  } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId, site, onTriggerDeploy }: { siteId: number; site: any; onTriggerDeploy?: () => void } = $props();

  let vhostConfig = $state('');
  let vhostSource = $state('');
  let vhostPath = $state('');
  let vhostLoading = $state(false);
  let vhostSaving = $state(false);
  let vhostValidating = $state(false);
  let vhostValidation = $state<any>(null);
  let vhostLoaded = $state(false);
  let vhostShowRef = $state(false);
  let vhostDirty = $state(false);
  let vhostOriginal = $state('');
  let copiedField = $state('');

  function copyText(text: string, field: string) { navigator.clipboard.writeText(text); copiedField = field; setTimeout(() => copiedField = '', 1500); }

  const vhostMeta = $derived.by(() => {
    if (!vhostConfig) return null;
    const lines = vhostConfig.split('\n');
    const listenPorts: string[] = []; const serverNames: string[] = [];
    const locations: { path: string }[] = [];
    let hasSSL = false; let hasBasicAuth = false; let hasPhp = false;
    for (const l of lines) {
      const t = l.trim();
      if (t.startsWith('listen ')) { const port = t.replace('listen ', '').replace(/;.*/, '').trim(); if (!listenPorts.includes(port)) listenPorts.push(port); }
      if (t.startsWith('server_name ')) serverNames.push(...t.replace('server_name ', '').replace(/;.*/, '').trim().split(/\s+/));
      if (t.startsWith('location ')) locations.push({ path: t.replace('location ', '').replace(/\s*\{.*/, '').trim() });
      if (t.includes('ssl_certificate')) hasSSL = true;
      if (t.includes('auth_basic')) hasBasicAuth = true;
      if (t.includes('fastcgi_pass') || t.includes('.php$')) hasPhp = true;
    }
    const proxyMatches = vhostConfig.match(/proxy_pass\s+([^;]+)/g) || [];
    const rootMatch = vhostConfig.match(/root\s+([^;]+)/);
    return { lines: lines.length, chars: vhostConfig.length, listenPorts: listenPorts.join(', ') || '80', serverNames: serverNames.join(', ') || '—', locationCount: locations.length, hasSSL, hasBasicAuth, hasPhp, proxyPass: proxyMatches[0]?.replace(/proxy_pass\s+/, '').replace(/;.*/, '').trim() || null, rootPath: rootMatch?.[1]?.trim() || null };
  });

  $effect(() => { if (site && vhostConfig === '' && !vhostLoading && !vhostLoaded) { vhostLoaded = true; loadVhost(); } });
  $effect(() => { if (vhostOriginal && vhostConfig !== vhostOriginal) vhostDirty = true; else if (vhostOriginal) vhostDirty = false; });

  async function loadVhost() {
    vhostLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/vhost`); vhostConfig = d?.vhost_config || ''; vhostOriginal = vhostConfig; vhostDirty = false; vhostSource = d?.source || ''; vhostPath = d?.config_path || ''; }
    catch (e: any) { toasts.error(e.message); } finally { vhostLoading = false; }
  }
  async function saveVhost() {
    vhostSaving = true;
    try { const d = await apiFetch(auth.token, 'PUT', `sites/${siteId}/vhost`, { vhost_config: vhostConfig }); if (d?.nginx_error) toasts.warning('⚠️ Syntax error:\n' + d.nginx_error); else { vhostOriginal = vhostConfig; vhostDirty = false; toasts.success(d?.message || 'Saved'); } }
    catch (e: any) { toasts.error(e.message); } finally { vhostSaving = false; }
  }
  async function validateVhost() {
    vhostValidating = true; vhostValidation = null;
    try { vhostValidation = await apiFetch(auth.token, 'POST', `sites/${siteId}/vhost/validate`, { vhost_config: vhostConfig }); }
    catch (e: any) { toasts.error(e.message); } finally { vhostValidating = false; }
  }
</script>

<div class="space-y-4">
  {#if vhostMeta}
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl px-4 py-3 hover:border-sky-400/20 transition-all duration-200"><div class="text-[0.65rem] font-medium text-slate-500 uppercase tracking-wider mb-1">Server Name</div><div class="text-sm text-slate-200 font-mono truncate">{vhostMeta.serverNames}</div></div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl px-4 py-3 hover:border-sky-400/20 transition-all duration-200"><div class="text-[0.65rem] font-medium text-slate-500 uppercase tracking-wider mb-1">Listen</div><div class="text-sm text-slate-200 font-mono">{vhostMeta.listenPorts}</div></div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl px-4 py-3 hover:border-sky-400/20 transition-all duration-200">
        <div class="text-[0.65rem] font-medium text-slate-500 uppercase tracking-wider mb-1">Features</div>
        <div class="flex flex-wrap gap-1 mt-0.5">
          {#if vhostMeta.hasSSL}<span class="text-[0.6rem] px-1.5 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400 font-medium">SSL</span>{/if}
          {#if vhostMeta.hasBasicAuth}<span class="text-[0.6rem] px-1.5 py-0.5 rounded-full bg-amber-500/15 text-amber-400 font-medium">Auth</span>{/if}
          {#if vhostMeta.hasPhp}<span class="text-[0.6rem] px-1.5 py-0.5 rounded-full bg-indigo-500/15 text-indigo-400 font-medium">PHP</span>{/if}
          {#if vhostMeta.proxyPass}<span class="text-[0.6rem] px-1.5 py-0.5 rounded-full bg-cyan-500/15 text-cyan-400 font-medium">Proxy</span>{/if}
          {#if !vhostMeta.hasSSL && !vhostMeta.hasBasicAuth && !vhostMeta.hasPhp && !vhostMeta.proxyPass}<span class="text-[0.6rem] px-1.5 py-0.5 rounded-full bg-white/[0.06] text-slate-500 font-medium">Static</span>{/if}
        </div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.06] rounded-xl px-4 py-3 hover:border-sky-400/20 transition-all duration-200"><div class="text-[0.65rem] font-medium text-slate-500 uppercase tracking-wider mb-1">Locations</div><div class="text-sm text-slate-200 font-mono">{vhostMeta.locationCount} block{vhostMeta.locationCount !== 1 ? 's' : ''}</div>{#if vhostMeta.proxyPass}<div class="text-[0.65rem] text-slate-500 font-mono truncate mt-0.5" title={vhostMeta.proxyPass}>→ {vhostMeta.proxyPass}</div>{/if}{#if vhostMeta.rootPath}<div class="text-[0.65rem] text-slate-500 font-mono truncate mt-0.5" title={vhostMeta.rootPath}>📁 {vhostMeta.rootPath}</div>{/if}</div>
    </div>
  {/if}

  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
    <div class="flex items-center justify-between px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 lg:py-3.5 border-b border-white/[0.05]">
      <div class="flex items-center gap-2 min-w-0">
        <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center shrink-0"><Code size={13} class="text-sky-400" /></div>
        <span class="text-xs sm:text-sm font-semibold text-slate-200 truncate">Nginx Configuration</span>
        {#if vhostSource}<span class="hidden sm:inline text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06] shrink-0">source: {vhostSource}</span>{/if}
        {#if vhostDirty}<span class="text-[0.6rem] sm:text-[0.65rem] text-amber-400 bg-amber-400/10 px-1.5 sm:px-2 py-0.5 rounded-full border border-amber-400/20 flex items-center gap-1 shrink-0"><span class="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse"></span><span class="hidden sm:inline">Unsaved</span></span>{/if}
      </div>
      <div class="flex items-center gap-1 sm:gap-2 shrink-0">
        {#if hasRole(auth.user?.role, 'admin') && onTriggerDeploy}
          <button onclick={onTriggerDeploy} class="inline-flex items-center gap-1 sm:gap-1.5 px-2 sm:px-3.5 py-1 sm:py-1.5 rounded-xl text-[0.65rem] sm:text-xs font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40"><Upload size={11} /><span class="hidden sm:inline">Git Pull</span></button>
        {/if}
        <button onclick={loadVhost} class="inline-flex items-center gap-1 px-2 sm:px-3 py-1 sm:py-1.5 rounded-xl text-[0.65rem] sm:text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition"><RefreshCw size={11} /></button>
      </div>
    </div>
    {#if vhostPath}<div class="px-3 sm:px-4 lg:px-5 py-1.5 sm:py-2 border-b border-white/[0.03] text-[0.65rem] sm:text-xs text-slate-500 font-mono flex items-center gap-1.5 sm:gap-2"><FileText size={11} class="shrink-0" /> <span class="truncate">{vhostPath}</span><button onclick={() => copyText(vhostPath, 'vhostpath')} class="text-slate-500 hover:text-slate-300 transition shrink-0">{#if copiedField === 'vhostpath'}<Check size={10} class="text-emerald-400" />{:else}<Copy size={10} />{/if}</button></div>{/if}
    {#if vhostValidation}
      <div class="px-3 sm:px-4 lg:px-5 py-2 sm:py-3 text-xs sm:text-sm flex items-center gap-2 sm:gap-2.5 {vhostValidation.valid ? 'text-emerald-400 bg-emerald-500/10 border-b border-emerald-500/10' : 'text-red-400 bg-red-500/10 border-b border-red-500/10'}">
        <div class="w-5 h-5 sm:w-6 sm:h-6 rounded-full flex items-center justify-center shrink-0 {vhostValidation.valid ? 'bg-emerald-500/20' : 'bg-red-500/20'}">{#if vhostValidation.valid}<Check size={12} />{:else}<X size={12} />{/if}</div>
        {#if vhostValidation.valid}<div><span class="font-medium">Configuration is valid</span></div>{:else}<div class="min-w-0"><span class="font-medium">Invalid</span> <span class="text-xs text-red-400/80 font-mono break-all">{vhostValidation.error || 'syntax error'}</span></div>{/if}
      </div>
    {/if}
    {#if vhostLoading}
      <div class="p-6 space-y-3">{#each Array(10) as _, i}<div class="h-3 bg-gradient-to-r from-white/[0.03] via-white/[0.06] to-white/[0.03] rounded animate-pulse" style="width: {55 + i * 3}%"></div>{/each}</div>
    {:else}
      <div class="p-1"><CodeEditor content={vhostConfig} onChange={(v: string) => vhostConfig = v} language="text" height="500px" /></div>
      <div class="px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 lg:py-3 border-t border-white/[0.05] flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 sm:gap-3 text-[0.65rem] sm:text-xs text-slate-500 min-w-0">
          <span class="flex items-center gap-1 shrink-0"><FileText size={10} /> {vhostConfig.split('\n').length} lines</span>
          {#if vhostDirty}<span class="text-amber-400/70 flex items-center gap-1 shrink-0"><span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>modified</span>{/if}
        </div>
        <div class="flex items-center gap-1 sm:gap-2 shrink-0">
          <button onclick={() => vhostShowRef = !vhostShowRef} class="inline-flex items-center gap-1 px-2 sm:px-3 py-1 sm:py-1.5 rounded-xl text-[0.65rem] sm:text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition"><ScrollText size={11} /></button>
          <button onclick={validateVhost} disabled={vhostValidating} class="inline-flex items-center gap-1 px-2 sm:px-3.5 py-1 sm:py-1.5 rounded-xl text-[0.65rem] sm:text-xs font-medium text-amber-400 hover:bg-amber-400/10 transition cursor-pointer disabled:opacity-40">{#if vhostValidating}<Loader2 size={11} class="animate-spin" />{:else}<Wrench size={11} />{/if}</button>
          <button onclick={saveVhost} disabled={vhostSaving || !vhostDirty} class="inline-flex items-center gap-1 sm:gap-1.5 px-2.5 sm:px-5 py-1.5 sm:py-2 rounded-xl text-[0.7rem] sm:text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0">{#if vhostSaving}<Loader2 size={12} class="animate-spin" />{:else}<Save size={12} />{/if} Save</button>
        </div>
      </div>
    {/if}
  </div>
</div>
