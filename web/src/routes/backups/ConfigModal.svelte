<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import Modal from '$components/Modal.svelte';
  import { LoaderCircle } from 'lucide-svelte';

  let {
    open = $bindable(false),
    config = $bindable(),
    providers,
    allSites,
    onsaved,
  }: {
    open: boolean;
    config: any;
    providers: any[];
    allSites: any[];
    onsaved: () => Promise<void> | void;
  } = $props();

  let saving = $state(false);

  const presets = [
    { label: 'Every hour', cron: '0 * * * *' },
    { label: 'Every 6 hours', cron: '0 */6 * * *' },
    { label: 'Daily at…', cron: 'daily' },
    { label: 'Weekly (Mon)', cron: '0 3 * * 1' },
    { label: 'Weekly (Sun)', cron: '0 3 * * 0' },
    { label: 'Monthly (1st)', cron: '0 3 1 * *' },
    { label: 'Custom', cron: 'custom' },
  ];

  function close() {
    open = false;
    config = null;
  }

  function pickPreset(cron: string) {
    config._preset = cron;
    if (cron === 'daily') config.cron_expr = `0 ${config._hour || 2} * * *`;
    else if (cron !== 'custom') config.cron_expr = cron;
  }

  function toggleSite(id: number) {
    if (config.site_ids?.includes(id)) config.site_ids = config.site_ids.filter((x: number) => x !== id);
    else config.site_ids = [...(config.site_ids || []), id];
    config._allSites = config.site_ids?.length === allSites.length;
  }

  async function saveConfig() {
    saving = true;
    try {
      const body = { ...config };
      // Convert exclude_patterns_text → array
      if (body.exclude_patterns_text !== undefined) {
        body.exclude_patterns = body.exclude_patterns_text
          ? body.exclude_patterns_text.split(',').map((s: string) => s.trim()).filter((s: string) => s)
          : [];
        delete body.exclude_patterns_text;
      }
      // Handle exclude_patterns from API response (array)
      if (Array.isArray(body.exclude_patterns)) {
        body.exclude_patterns_text = body.exclude_patterns.join(', ');
      }
      // Empty site_ids enables auto-discovery for database/full configs.
      if (body._autoDiscover && (body.scope_type === 'database' || body.scope_type === 'both')) {
        body.site_ids = [];
      }
      delete body._autoDiscover;
      delete body._allSites;
      delete body._preset;
      delete body._hour;
      // Clean up undefined values
      Object.keys(body).forEach((k) => { if (body[k] === undefined) delete body[k]; });
      if (config.id) {
        await apiFetch(auth.token, 'PATCH', `backup-configs/${config.id}`, body);
        toasts.success('Config updated');
      } else {
        await apiFetch(auth.token, 'POST', 'backup-configs', body);
        toasts.success('Config created');
      }
      close();
      await onsaved();
    } catch (e: any) { toasts.error(e.message); }
    finally { saving = false; }
  }
</script>

<Modal bind:open title={config?.id ? 'Edit backup config' : 'Add backup config'} maxWidth="lg" onclose={close}>
  {#if config}
    <form id="config-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); saveConfig(); }}>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="cfg-name" class="label">Config name</label>
          <input id="cfg-name" type="text" bind:value={config.name} placeholder="Daily files" class="input" />
        </div>
        <div>
          <label for="cfg-scope" class="label">Scope</label>
          <select id="cfg-scope" bind:value={config.scope_type} class="input">
            <option value="files">Files</option>
            <option value="database">Databases</option>
            <option value="both">Full (files + DB)</option>
          </select>
        </div>
      </div>

      <fieldset>
        <legend class="label">Schedule</legend>
        <div class="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
          {#each presets as p (p.cron)}
            <button
              type="button"
              onclick={() => pickPreset(p.cron)}
              aria-pressed={config._preset === p.cron}
              class="btn btn-xs {config._preset === p.cron ? 'bg-accent/10 text-accent ring-1 ring-accent/30 ring-inset' : 'btn-secondary'}">
              {p.label}
            </button>
          {/each}
        </div>
        {#if config._preset === 'daily'}
          <div class="mt-2 flex items-center gap-2">
            <label for="cfg-hour" class="text-[13px] text-muted">Every day at</label>
            <select id="cfg-hour" bind:value={config._hour} onchange={() => { config.cron_expr = `0 ${config._hour} * * *`; }} class="input h-8 w-28 text-[13px] tabular-nums">
              {#each Array.from({ length: 24 }, (_, i) => i) as h (h)}
                <option value={h}>{String(h).padStart(2, '0')}:00</option>
              {/each}
            </select>
          </div>
        {/if}
        {#if config._preset === 'custom'}
          <div class="mt-2">
            <label for="cfg-cron" class="sr-only">Cron expression</label>
            <input id="cfg-cron" type="text" bind:value={config.cron_expr} placeholder="0 2 * * *" class="input font-mono" />
          </div>
        {/if}
        {#if config.cron_expr}
          <p class="hint">Cron: <code class="code">{config.cron_expr}</code></p>
        {/if}
      </fieldset>

      <fieldset>
        <legend class="label">Sites to back up</legend>
        {#if (config.scope_type === 'database' || config.scope_type === 'both') && config._autoDiscover}
          <div class="rounded-lg bg-success/10 px-3 py-2 text-[13px] text-success">
            Auto-discovery active. All active sites and newly created sites are included.
          </div>
        {:else}
          <div class="max-h-40 space-y-0.5 overflow-y-auto rounded-lg border border-line p-1.5">
            <label class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium text-fg hover:bg-surface-2">
              <input
                type="checkbox"
                class="checkbox"
                bind:checked={config._allSites}
                onchange={() => { config.site_ids = config._allSites ? allSites.map((s: any) => s.id) : []; }} />
              All sites
            </label>
            {#each allSites as s (s.id)}
              <label class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[13px] text-fg hover:bg-surface-2">
                <input type="checkbox" class="checkbox" checked={config.site_ids?.includes(s.id)} onchange={() => toggleSite(s.id)} />
                {s.domain || s.name || `Site #${s.id}`}
              </label>
            {/each}
          </div>
        {/if}
      </fieldset>

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="cfg-provider" class="label">Provider</label>
          <select id="cfg-provider" bind:value={config.provider_id} class="input">
            <option value={undefined}>Local only</option>
            {#each providers as p (p.id)}
              <option value={p.id}>{p.name}</option>
            {/each}
          </select>
        </div>
        <div>
          <label for="cfg-retention" class="label">Retention (days)</label>
          <input id="cfg-retention" type="number" bind:value={config.retention_days} placeholder="7" min="0" class="input tabular-nums" />
        </div>
      </div>
      <div>
        <label for="cfg-exclude" class="label">Exclude patterns</label>
        <input id="cfg-exclude" type="text" bind:value={config.exclude_patterns_text} placeholder="node_modules, vendor, .git" class="input font-mono" />
        <p class="hint">Comma-separated.</p>
      </div>
      <label class="flex cursor-pointer items-center gap-2 text-sm text-fg">
        <input type="checkbox" bind:checked={config.enabled} class="checkbox" />
        Schedule enabled
      </label>
    </form>
  {/if}
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={close}>Cancel</button>
    <button type="submit" form="config-form" class="btn btn-primary btn-sm" disabled={saving}>
      {#if saving}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}{config?.id ? 'Update' : 'Create'}{/if}
    </button>
  {/snippet}
</Modal>
