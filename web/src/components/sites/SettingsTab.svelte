<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import { Settings, Globe, Save, LoaderCircle, Code, Zap, Play, Ban, RefreshCw, Pencil, Box } from 'lucide-svelte';
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

  const canStop = $derived(appStatus === 'running' || appStatus === 'errored');
</script>

{#if loading}
  <div class="max-w-2xl space-y-3">
    <div class="skeleton h-40 rounded-xl"></div>
    <div class="skeleton h-56 rounded-xl"></div>
  </div>
{:else if !site}
  <div class="card"><EmptyState icon={Globe} title="Site not found" /></div>
{:else}
  <div class="space-y-4">
    <!-- Site configuration -->
    <div class="card max-w-2xl">
      <div class="card-header">
        <h3 class="card-title">Site configuration</h3>
        <Settings size={16} class="text-subtle" />
      </div>
      <div class="card-body space-y-4">
        <div>
          <label for="sd-root" class="label">Root path <span class="text-subtle">(read-only)</span></label>
          <input id="sd-root" value={configForm.root_path} readonly class="input cursor-not-allowed bg-surface-2 font-mono text-[13px] text-muted" />
        </div>
        <div>
          <label for="sd-proxy" class="label">Proxy pass URL</label>
          <input id="sd-proxy" bind:value={configForm.proxy_pass_url} class="input font-mono text-[13px]" placeholder="http://127.0.0.1:3000" />
        </div>
        <button onclick={saveConfig} disabled={configBusy} class="btn btn-primary btn-sm">
          {#if configBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if} {configBusy ? 'Saving…' : 'Save config'}
        </button>
      </div>
    </div>

    <!-- App process -->
    {#if site.runtime !== 'php'}
      <div class="card">
        <div class="card-header">
          <div class="flex items-center gap-2">
            <Zap size={16} class="text-subtle" />
            <h3 class="card-title">App status</h3>
          </div>
          {#if appStatus}<StatusPill status={appStatus} />{:else if !appLoaded}<div class="skeleton h-5 w-16"></div>{/if}
        </div>
        <div class="card-body space-y-4">
          <dl class="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div><dt class="stat-label">Port</dt><dd class="mt-1 font-mono text-[13px] text-fg">{appPort || '—'}</dd></div>
            <div><dt class="stat-label">Runtime</dt><dd class="mt-1 font-mono text-[13px] text-fg">{site?.runtime || '—'}</dd></div>
            <div><dt class="stat-label">Max memory</dt><dd class="mt-1 font-mono text-[13px] text-fg tabular-nums">{appEditMem || 512} MB</dd></div>
          </dl>
          <div>
            <div class="stat-label mb-1">Start command</div>
            <code class="block rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] break-all text-fg">{appCommand || '—'}</code>
          </div>
          <div class="flex flex-wrap gap-2">
            <button onclick={startApp} disabled={appBusy || appStatus === 'running'} class="btn btn-primary btn-sm">
              {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Play size={14} />{/if} Start
            </button>
            <button onclick={stopApp} disabled={appBusy || !canStop} class="btn btn-secondary btn-sm">
              {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<Ban size={14} />{/if} Stop
            </button>
            <button onclick={restartApp} disabled={appBusy || !canStop} class="btn btn-secondary btn-sm">
              {#if appBusy}<LoaderCircle size={14} class="animate-spin" />{:else}<RefreshCw size={14} />{/if} Restart
            </button>
            <button onclick={() => { appEditing = !appEditing; if (appEditing) { appEditCmd = appCommand; appEditPort = appPort; appEditMem = site?.app_memory_max || 512; }}} class="btn btn-ghost btn-sm">
              <Pencil size={14} /> {appEditing ? 'Cancel' : 'Edit config'}
            </button>
          </div>
          {#if appEditing}
            <div class="space-y-3 rounded-lg border border-line bg-surface-2/50 p-4">
              <div>
                <label for="app-cmd" class="label">Start command</label>
                <input id="app-cmd" bind:value={appEditCmd} class="input font-mono text-[13px]" placeholder="e.g. node server.js" />
              </div>
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label for="app-port" class="label">Port</label>
                  <input id="app-port" bind:value={appEditPort} type="number" class="input font-mono text-[13px]" />
                </div>
                <div>
                  <label for="app-mem" class="label">Max memory (MB)</label>
                  <input id="app-mem" bind:value={appEditMem} type="number" min="64" max="4096" step="64" class="input font-mono text-[13px]" />
                </div>
              </div>
              <p class="hint mt-0">Min 64 MB. Default 512 MB. Restart required to apply.</p>
              <button onclick={saveAppConfig} disabled={appBusy} class="btn btn-primary btn-sm"><Save size={14} /> Save config</button>
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <!-- Docker config -->
    {#if site.runtime === 'docker'}
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Docker configuration</h3>
          <Box size={16} class="text-subtle" />
        </div>
        <div class="card-body">
          <dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div><dt class="stat-label">Container name</dt><dd class="mt-1 font-mono text-[13px] text-fg">site-{site.name}</dd></div>
            <div><dt class="stat-label">Proxy pass</dt><dd class="mt-1 font-mono text-[13px] text-fg">{site.proxy_pass_url || '—'}</dd></div>
            <div class="min-w-0"><dt class="stat-label">Nginx config</dt><dd class="mt-1 truncate font-mono text-[13px] text-fg">{site.conf_file || '/etc/nginx/sites-available/' + site.domain + '.conf'}</dd></div>
            <div><dt class="stat-label">Restart policy</dt><dd class="mt-1 font-mono text-[13px] text-fg">unless-stopped</dd></div>
          </dl>
          <p class="hint mt-4">Container management is available in the <strong class="font-medium text-muted">App</strong> tab.</p>
        </div>
      </div>
    {/if}

    <!-- PHP config -->
    {#if site.runtime === 'php'}
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">PHP version</h3>
          <Code size={16} class="text-subtle" />
        </div>
        <div class="card-body">
          <p class="mb-3 text-[13px] text-muted">Select which PHP-FPM version this site uses.</p>
          {#if !phpLoaded}
            <div class="skeleton h-8 w-full max-w-md"></div>
          {:else}
            <div class="flex flex-wrap gap-1.5">
              {#each phpVersions as v (v)}
                <button
                  class="btn btn-sm min-w-14 font-mono {phpVersion === v ? 'border border-accent bg-accent/10 text-accent' : 'btn-secondary'}"
                  aria-pressed={phpVersion === v}
                  onclick={() => (phpVersion = phpVersion === v ? '' : v)}>{v}</button>
              {/each}
            </div>
          {/if}
          {#if phpVersion}
            <p class="hint">Selected: PHP {phpVersion} → <span class="font-mono">fastcgi_pass 127.0.0.1:{phpVersion.replace('.', '') + '000'}</span></p>
          {:else}
            <p class="hint">No version selected → uses CloudPanel default</p>
          {/if}
        </div>
      </div>
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Custom php.ini</h3>
          <Settings size={16} class="text-subtle" />
        </div>
        <div class="card-body">
          <label for="php-ini" class="label">PHP directives</label>
          <textarea id="php-ini" bind:value={phpIni} rows={6} placeholder={"upload_max_filesize = 64M\npost_max_size = 64M\nmax_execution_time = 300\nmemory_limit = 256M"} class="input font-mono text-[13px]"></textarea>
          <p class="hint">One directive per line. Per-site only.</p>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <button onclick={savePHP} disabled={phpBusy} class="btn btn-primary">
          {#if phpBusy}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}<Save size={14} /> Save PHP config{/if}
        </button>
        <span class="text-xs text-subtle">Vhost will be regenerated automatically</span>
      </div>
    {/if}
  </div>
{/if}
