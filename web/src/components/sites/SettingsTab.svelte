<script lang="ts">
    import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
    import {
    Settings, Globe, ArrowLeft, Save, Loader2, Code, Zap,
    Play, Ban, RefreshCw, Pencil, AlertTriangle, Box,
  } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();
  let site = $state<any>(null);
  let loading = $state(true);

  // Config state
  let configForm = $state({ root_path: '', proxy_pass_url: '' });
  let configBusy = $state(false);

  // App process state
  let appStatus = $state('');
  let appPort = $state(0);
  let appCommand = $state('');
  let appLoaded = $state(false);
  let appBusy = $state(false);
  let appEditCmd = $state('');
  let appEditPort = $state(0);
  let appEditMem = $state(512);
  let appEditing = $state(false);

  // PHP state
  let phpVersion = $state('');
  let phpIni = $state('');
  let phpVersions = $state<string[]>([]);
  let phpLoaded = $state(false);
  let phpBusy = $state(false);

  function appStatusClass(status: string) {
    return { running: 'bg-green-500/20 text-green-400', errored: 'bg-red-500/20 text-red-400', stopped: 'bg-slate-500/20 text-slate-400', starting: 'bg-amber-500/20 text-amber-400', stopping: 'bg-amber-500/20 text-amber-400', restarting: 'bg-amber-500/20 text-amber-400' }[status] || 'bg-slate-500/20 text-slate-400';
  }

  async function pollJob(id: number): Promise<string> {
    for (let i = 0; i < 20; i++) {
      await new Promise(r => setTimeout(r, 2000));
      try { const d = await apiFetch(auth.token, 'GET', `jobs/${id}`); const st = d?.status || ''; if (st === 'succeeded') return 'success'; if (st === 'failed') return `error: ${d?.error || 'Job failed'}`; } catch {}
    }
    return 'timeout';
  }

  async function saveConfig() {
    configBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/config`, { proxy_pass_url: configForm.proxy_pass_url }); toasts.success('Config saved'); }
    catch (e: any) { toasts.error(e.message); } finally { configBusy = false; }
  }

  async function loadApp() {
    try {
      const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/app/status`);
      if (d) { appStatus = d.app_status || 'stopped'; appPort = d.app_port || 0; appCommand = d.app_command || ''; appEditCmd = appCommand; appEditPort = appPort; appEditMem = d.app_memory_max || 512; }
    } catch { appStatus = 'stopped'; }
    appLoaded = true;
  }
  async function startApp() {
    appBusy = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/app/start`, { command: appEditCmd || undefined }); const jobId = d?.job_id; if (jobId) { toasts.info('App start queued. Waiting...'); await pollJob(jobId); await loadApp(); toasts.success('App started'); } else { await loadApp(); toasts.success('App started'); } }
    catch (e: any) { toasts.error(e.message || 'Failed'); } finally { appBusy = false; }
  }
  async function stopApp() {
    appBusy = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/app/stop`); const jobId = d?.job_id; if (jobId) { toasts.info('App stop queued...'); await pollJob(jobId); await loadApp(); toasts.success('App stopped'); } else { await loadApp(); toasts.success('App stopped'); } }
    catch (e: any) { toasts.error(e.message || 'Failed'); } finally { appBusy = false; }
  }
  async function restartApp() {
    appBusy = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/app/restart`); const jobId = d?.job_id; if (jobId) { toasts.info('App restart queued...'); await pollJob(jobId); await loadApp(); toasts.success('App restarted'); } else { await loadApp(); toasts.success('App restarted'); } }
    catch (e: any) { toasts.error(e.message || 'Failed'); } finally { appBusy = false; }
  }
  async function saveAppConfig() {
    appBusy = true;
    try { await apiFetch(auth.token, 'PUT', `sites/${siteId}/app/config`, { command: appEditCmd, port: appEditPort, memory_max_mb: appEditMem }); await loadApp(); appEditing = false; toasts.success('App config saved. Click Start to apply.'); }
    catch (e: any) { toasts.error(e.message || 'Failed'); } finally { appBusy = false; }
  }

  async function loadPHP() {
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/php/config`); if (d) { phpVersion = d.php_version || ''; phpIni = d.php_ini || ''; phpVersions = d.available_versions || []; } } catch {}
    phpLoaded = true;
  }
  async function savePHP() {
    phpBusy = true;
    try { await apiFetch(auth.token, 'PUT', `sites/${siteId}/php/config`, { php_version: phpVersion, php_ini: phpIni }); toasts.success('PHP config saved. Vhost regenerating...'); await loadPHP(); }
    catch (e: any) { toasts.error(e.message); } finally { phpBusy = false; }
  }

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try { site = await apiFetch(auth.token, 'GET', `sites/${siteId}`); if (site) configForm = { root_path: site.root_path || '', proxy_pass_url: site.proxy_pass_url || '' }; }
    catch (e: any) { if (e.message === 'session expired') return; toasts.error(e.message); }
    finally { loading = false; }
    await Promise.all([loadApp(), loadPHP()]);
  });
</script>


  {#if loading}
    <div class="space-y-4"><div class="h-8 w-32 bg-white/[0.04] rounded-lg animate-pulse"></div></div>
  {:else if !site}
    <div class="flex flex-col items-center justify-center py-32"><Globe size={36} class="text-white/[0.08]" /><h2 class="text-lg font-semibold text-slate-300 mt-4">Site not found</h2></div>
  {:else}
    <div class="flex items-center gap-3 mb-6">
      <div class="w-10 h-10 rounded-xl bg-sky-400/10 flex items-center justify-center"><Settings size={18} class="text-sky-400" /></div>
      <div><h1 class="text-lg font-bold text-slate-200">Site Settings</h1><p class="text-xs text-slate-500">{site.domain}</p></div>
    </div>

    <!-- ═══ Site Configuration ═══ -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden max-w-2xl hover:border-white/[0.12] transition-all duration-300 mb-4">
      <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
        <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Settings size={13} class="text-sky-400" /></div>
        <span class="text-sm font-semibold text-slate-200">Site Configuration</span>
      </div>
      <div class="p-5 space-y-4">
        <div><label for="sd-root" class="block text-xs text-slate-500 mb-1.5">Root Path <span class="text-slate-600">(read-only)</span></label><input id="sd-root" value={configForm.root_path} readonly class="w-full bg-white/[0.02] border border-white/[0.05] rounded-xl px-4 py-2.5 text-sm text-slate-400 font-mono cursor-not-allowed" /></div>
        <div><label for="sd-proxy" class="block text-xs text-slate-500 mb-1.5">Proxy Pass URL</label><input id="sd-proxy" bind:value={configForm.proxy_pass_url} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 font-mono placeholder:text-slate-600" placeholder="http://127.0.0.1:3000" /></div>
        <div class="flex gap-3 pt-2">
          <button onclick={saveConfig} disabled={configBusy} class="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{#if configBusy}<Loader2 size={14} class="animate-spin" />{:else}<Save size={14} />{/if} {configBusy ? 'Saving...' : 'Save Config'}</button>
        </div>
      </div>
    </div>

    <!-- ═══ App Process ═══ -->
    {#if site.runtime !== 'php'}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300 mb-4">
        <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
          <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Zap size={13} class="text-sky-400" /></div>
          <span class="text-sm font-semibold text-slate-200">App Status</span>
          {#if appStatus}<span class="ml-auto text-xs px-2 py-0.5 rounded-full font-medium {appStatusClass(appStatus)}">{appStatus || 'unknown'}</span>{/if}
        </div>
        <div class="p-5 space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div><label class="block text-xs text-slate-500 mb-1">Port</label><span class="text-sm text-slate-200 font-mono">{appPort || '—'}</span></div>
            <div><label class="block text-xs text-slate-500 mb-1">Runtime</label><span class="text-sm text-slate-200 font-mono">{site?.runtime || '—'}</span></div>
            <div><label class="block text-xs text-slate-500 mb-1">Max Memory</label><span class="text-sm text-slate-200 font-mono">{appEditMem || 512} MB</span></div>
          </div>
          <div><label class="block text-xs text-slate-500 mb-1">Start Command</label><code class="text-sm text-slate-200 font-mono bg-white/[0.04] px-3 py-1.5 rounded-lg block">{appCommand || '—'}</code></div>
          <div class="flex flex-wrap gap-2 pt-2">
            <button onclick={startApp} disabled={appBusy || appStatus === 'running'} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-green-500 to-emerald-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0">{#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}<Play size={12} /> Start</button>
            <button onclick={stopApp} disabled={appBusy || (appStatus !== 'running' && appStatus !== 'errored')} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-red-500 to-rose-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0">{#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}<Ban size={12} /> Stop</button>
            <button onclick={restartApp} disabled={appBusy || (appStatus !== 'running' && appStatus !== 'errored')} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-amber-500 to-orange-500 text-white hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0">{#if appBusy}<Loader2 size={12} class="animate-spin" />{/if}<RefreshCw size={12} /> Restart</button>
            <button onclick={() => { appEditing = !appEditing; if (appEditing) { appEditCmd = appCommand; appEditPort = appPort; appEditMem = site?.app_memory_max || 512; }}} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium text-slate-300 hover:text-slate-100 bg-white/[0.04] hover:bg-white/[0.08] transition border border-white/[0.06] cursor-pointer"><Pencil size={12} /> {appEditing ? 'Cancel' : 'Edit Config'}</button>
          </div>
          {#if appEditing}
            <div class="mt-4 space-y-3 p-4 bg-white/[0.02] border border-white/[0.06] rounded-xl">
              <div><label class="block text-xs text-slate-500 mb-1.5">Start Command</label><input bind:value={appEditCmd} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 placeholder:text-slate-600" placeholder="e.g. node server.js" /></div>
              <div><label class="block text-xs text-slate-500 mb-1.5">Port</label><input bind:value={appEditPort} type="number" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200" /></div>
              <div><label class="block text-xs text-slate-500 mb-1.5">Max Memory (MB)</label><input bind:value={appEditMem} type="number" min="64" max="4096" step="64" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200" /><p class="text-xs text-slate-500 mt-1">Min 64 MB. Default 512 MB. Restart required to apply.</p></div>
              <button onclick={saveAppConfig} disabled={appBusy} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition-all cursor-pointer disabled:opacity-40 border-0"><Save size={12} /> Save Config</button>
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <!-- ═══ Docker Config ═══ -->
    {#if site.runtime === 'docker'}
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300 mb-4">
        <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
          <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Box size={13} class="text-sky-400" /></div>
          <span class="text-sm font-semibold text-slate-200">Docker Configuration</span>
        </div>
        <div class="p-5">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
            <div><label class="block text-slate-500 mb-1">Container Name</label><span class="text-slate-200 font-mono">site-{site.name}</span></div>
            <div><label class="block text-slate-500 mb-1">Proxy Pass</label><span class="text-slate-200 font-mono">{site.proxy_pass_url || '—'}</span></div>
            <div><label class="block text-slate-500 mb-1">Nginx Config</label><span class="text-slate-200 font-mono truncate block">{site.conf_file || '/etc/nginx/sites-available/' + site.domain + '.conf'}</span></div>
            <div><label class="block text-slate-500 mb-1">Restart Policy</label><span class="text-slate-200 font-mono">unless-stopped</span></div>
          </div>
          <p class="text-[0.65rem] text-slate-600 mt-3">Container management available in the <strong>App</strong> tab.</p>
        </div>
      </div>
    {/if}

    <!-- ═══ PHP Config ═══ -->
    {#if site.runtime === 'php'}
      <div class="space-y-4 mb-4">
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden">
          <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
            <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-indigo-400/10 flex items-center justify-center"><Code size={13} class="text-indigo-400" /></div>
            <span class="text-sm font-semibold text-slate-200">PHP Version</span>
          </div>
          <div class="p-5 space-y-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">PHP-FPM Version</label>
              <p class="text-[11px] text-slate-600 mb-2">Select which PHP version this site uses.</p>
              <div class="grid grid-cols-5 sm:grid-cols-10 gap-1.5">
                {#each phpVersions as v}
                  <button class="px-2 py-1.5 rounded-lg border text-xs font-mono transition-all {phpVersion === v ? 'bg-indigo-500/20 border-indigo-500/40 text-indigo-300' : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:border-white/[0.15] hover:text-slate-300'}" onclick={() => phpVersion = phpVersion === v ? '' : v}>{v}</button>
                {/each}
              </div>
              {#if phpVersion}<p class="mt-2 text-[11px] text-indigo-400/70">Selected: PHP {phpVersion} → fastcgi_pass 127.0.0.1:{phpVersion.replace('.', '') + '000'}</p>
              {:else}<p class="mt-2 text-[11px] text-slate-600">No version selected → uses CloudPanel default</p>{/if}
            </div>
          </div>
        </div>
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden">
          <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
            <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-amber-400/10 flex items-center justify-center"><Settings size={13} class="text-amber-400" /></div>
            <span class="text-sm font-semibold text-slate-200">Custom php.ini</span>
          </div>
          <div class="p-5 space-y-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">PHP directives</label>
              <p class="text-[11px] text-slate-600 mb-2">One directive per line. Per-site only.</p>
              <textarea bind:value={phpIni} rows={6} placeholder={"upload_max_filesize = 64M\npost_max_size = 64M\nmax_execution_time = 300\nmemory_limit = 256M"} class="w-full bg-black/20 border border-white/[0.07] rounded-xl px-3 py-2.5 text-sm font-mono text-slate-300 placeholder:text-slate-700 focus:outline-none focus:border-indigo-500/40 transition"></textarea>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <button onclick={savePHP} disabled={phpBusy} class="px-5 py-2 rounded-xl bg-indigo-500/20 border border-indigo-500/30 text-sm text-indigo-300 hover:bg-indigo-500/30 transition disabled:opacity-50">{#if phpBusy}<Loader2 size={14} class="animate-spin inline mr-1.5" /> Saving...{:else}Save PHP Config{/if}</button>
          <span class="text-[11px] text-slate-600">Vhost will be regenerated automatically</span>
        </div>
      </div>
    {/if}
  {/if}

