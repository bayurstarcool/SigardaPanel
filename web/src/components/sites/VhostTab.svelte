<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { Code, Save, RefreshCw, Check, X, FileText, Wrench, Upload, Copy, LoaderCircle } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId, site, onTriggerDeploy }: { siteId: number; site: any; onTriggerDeploy?: () => void } = $props();

  // CodeMirror is heavy; fetch it only when this tab renders.
  const editorModule = import('$components/CodeEditor.svelte');

  let vhostConfig = $state('');
  let vhostSource = $state('');
  let vhostPath = $state('');
  let vhostLoading = $state(false);
  let vhostSaving = $state(false);
  let vhostValidating = $state(false);
  let vhostValidation = $state<any>(null);
  let vhostLoaded = $state(false);
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
    try { const d = await apiFetch(auth.token, 'PUT', `sites/${siteId}/vhost`, { vhost_config: vhostConfig }); if (d?.nginx_error) toasts.warning('Syntax error:\n' + d.nginx_error); else { vhostOriginal = vhostConfig; vhostDirty = false; toasts.success(d?.message || 'Saved'); } }
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
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <div class="card min-w-0 p-4">
        <div class="stat-label">Server name</div>
        <div class="mt-1 truncate font-mono text-[13px] text-fg" title={vhostMeta.serverNames}>{vhostMeta.serverNames}</div>
      </div>
      <div class="card min-w-0 p-4">
        <div class="stat-label">Listen</div>
        <div class="mt-1 truncate font-mono text-[13px] text-fg">{vhostMeta.listenPorts}</div>
      </div>
      <div class="card min-w-0 p-4">
        <div class="stat-label">Features</div>
        <div class="mt-1.5 flex flex-wrap gap-1">
          {#if vhostMeta.hasSSL}<span class="badge badge-success">SSL</span>{/if}
          {#if vhostMeta.hasBasicAuth}<span class="badge badge-warning">Auth</span>{/if}
          {#if vhostMeta.hasPhp}<span class="badge badge-accent">PHP</span>{/if}
          {#if vhostMeta.proxyPass}<span class="badge badge-info">Proxy</span>{/if}
          {#if !vhostMeta.hasSSL && !vhostMeta.hasBasicAuth && !vhostMeta.hasPhp && !vhostMeta.proxyPass}<span class="badge">Static</span>{/if}
        </div>
      </div>
      <div class="card min-w-0 p-4">
        <div class="stat-label">Locations</div>
        <div class="mt-1 font-mono text-[13px] text-fg tabular-nums">{vhostMeta.locationCount} block{vhostMeta.locationCount !== 1 ? 's' : ''}</div>
        {#if vhostMeta.proxyPass}<div class="mt-0.5 truncate font-mono text-xs text-subtle" title={vhostMeta.proxyPass}>→ {vhostMeta.proxyPass}</div>{/if}
        {#if vhostMeta.rootPath}<div class="mt-0.5 truncate font-mono text-xs text-subtle" title={vhostMeta.rootPath}>root {vhostMeta.rootPath}</div>{/if}
      </div>
    </div>
  {/if}

  <div class="card overflow-hidden">
    <div class="card-header">
      <div class="flex min-w-0 items-center gap-2">
        <Code size={16} class="shrink-0 text-subtle" />
        <h3 class="card-title truncate">Nginx configuration</h3>
        {#if vhostSource}<span class="badge max-sm:hidden">source: {vhostSource}</span>{/if}
        {#if vhostDirty}<span class="badge badge-warning"><span class="dot"></span>Unsaved</span>{/if}
      </div>
      <div class="flex shrink-0 items-center gap-1">
        {#if hasRole(auth.user?.role, 'admin') && onTriggerDeploy}
          <button onclick={onTriggerDeploy} class="btn btn-secondary btn-sm"><Upload size={14} /><span class="max-sm:hidden">Git pull</span></button>
        {/if}
        <button onclick={loadVhost} class="btn btn-ghost btn-icon btn-sm" aria-label="Reload configuration" title="Reload">
          <RefreshCw size={14} class={vhostLoading ? 'animate-spin' : ''} />
        </button>
      </div>
    </div>
    {#if vhostPath}
      <div class="flex items-center gap-2 border-b border-line px-5 py-2 font-mono text-xs text-subtle">
        <FileText size={12} class="shrink-0" /> <span class="truncate">{vhostPath}</span>
        <button onclick={() => copyText(vhostPath, 'vhostpath')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy config path">
          {#if copiedField === 'vhostpath'}<Check size={12} class="text-success" />{:else}<Copy size={12} />{/if}
        </button>
      </div>
    {/if}
    {#if vhostValidation}
      <div class="flex items-start gap-2 border-b border-line px-5 py-2.5 text-[13px] {vhostValidation.valid ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}">
        {#if vhostValidation.valid}
          <Check size={14} class="mt-0.5 shrink-0" /><span class="font-medium">Configuration is valid</span>
        {:else}
          <X size={14} class="mt-0.5 shrink-0" />
          <div class="min-w-0"><span class="font-medium">Invalid</span> <span class="font-mono text-xs break-all">{vhostValidation.error || 'syntax error'}</span></div>
        {/if}
      </div>
    {/if}
    {#if vhostLoading}
      <div class="space-y-2.5 p-5">{#each Array(10) as _, i (i)}<div class="skeleton h-3" style="width: {55 + i * 3}%"></div>{/each}</div>
    {:else}
      {#await editorModule}
        <div class="skeleton m-1 h-[500px]"></div>
      {:then m}
        <div class="p-1"><m.default content={vhostConfig} onChange={(v: string) => vhostConfig = v} language="text" height="500px" /></div>
      {:catch}
        <p class="p-5 text-[13px] text-danger">Failed to load the editor. Reload the page and try again.</p>
      {/await}
      <div class="flex items-center justify-between gap-2 border-t border-line px-5 py-2.5">
        <div class="flex min-w-0 items-center gap-3 text-xs text-subtle">
          <span class="flex items-center gap-1 tabular-nums"><FileText size={12} /> {vhostConfig.split('\n').length} lines</span>
          {#if vhostDirty}<span class="flex items-center gap-1 text-warning"><span class="dot"></span>modified</span>{/if}
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button onclick={validateVhost} disabled={vhostValidating} class="btn btn-secondary btn-sm">
            {#if vhostValidating}<LoaderCircle size={14} class="animate-spin" />{:else}<Wrench size={14} />{/if}
            <span class="max-sm:hidden">Validate</span>
          </button>
          <button onclick={saveVhost} disabled={vhostSaving || !vhostDirty} class="btn btn-primary btn-sm">
            {#if vhostSaving}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save
          </button>
        </div>
      </div>
    {/if}
  </div>
</div>
