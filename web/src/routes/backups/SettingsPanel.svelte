<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import ProviderModal from './ProviderModal.svelte';
  import ConfigModal from './ConfigModal.svelte';
  import { Cloud, Plus, Pencil, Trash2, Play, Clock, CalendarClock } from 'lucide-svelte';

  let {
    providers,
    providerUsage,
    configs,
    allSites,
    loadProviders,
    loadConfigs,
  }: {
    providers: any[];
    providerUsage: Record<number, any>;
    configs: any[];
    allSites: any[];
    loadProviders: () => Promise<void>;
    loadConfigs: () => Promise<void>;
  } = $props();

  let showProviderForm = $state(false);
  let editingProvider = $state<any>(null);
  let showConfigForm = $state(false);
  let editingConfig = $state<any>(null);

  let confirm = $state({ open: false, title: '', message: '', run: () => {} });
  function ask(title: string, message: string, run: () => void) {
    confirm = { open: true, title, message, run };
  }

  function newProvider() {
    editingProvider = {};
    showProviderForm = true;
  }

  function editProvider(p: any) {
    editingProvider = { ...p };
    showProviderForm = true;
  }

  function deleteProvider(p: any) {
    ask(`Delete provider "${p.name}"?`, 'Backups already stored in the bucket are not deleted.', async () => {
      try {
        await apiFetch(auth.token, 'DELETE', `backup-providers/${p.id}`);
        toasts.success('Provider deleted');
        await loadProviders();
      } catch (e: any) { toasts.error(e.message); }
    });
  }

  function newConfig() {
    editingConfig = {
      name: 'Daily Databases', scope_type: 'database', cron_expr: '0 0 * * *', enabled: true,
      site_ids: [], database_ids: [], retention_days: 7, _autoDiscover: true, _allSites: false,
    };
    showConfigForm = true;
  }

  function editConfig(c: any) {
    const cfg = { ...c };
    if (Array.isArray(cfg.exclude_patterns)) cfg.exclude_patterns_text = cfg.exclude_patterns.join(', ');
    cfg.site_ids = cfg.site_ids || [];
    cfg._autoDiscover = cfg.auto_discover_sites === true;
    cfg._allSites = !cfg._autoDiscover && cfg.site_ids.length === allSites.length;
    editingConfig = cfg;
    showConfigForm = true;
  }

  function deleteConfig(c: any) {
    ask(`Delete config "${c.name || 'Unnamed config'}"?`, 'The schedule stops. Existing backups are kept.', async () => {
      try {
        await apiFetch(auth.token, 'DELETE', `backup-configs/${c.id}`);
        toasts.success('Config deleted');
        await loadConfigs();
      } catch (e: any) { toasts.error(e.message); }
    });
  }

  async function runConfig(id: number) {
    try {
      await apiFetch(auth.token, 'POST', `backup-configs/${id}/run`);
      toasts.success('Backup run triggered');
    } catch (e: any) { toasts.error(e.message); }
  }

  const providerName = $derived(new Map<number, string>(providers.map((p) => [p.id, p.name])));
</script>

<div class="space-y-6">
  <section class="card">
    <div class="card-header">
      <h2 class="card-title">Storage providers</h2>
      <button onclick={newProvider} class="btn btn-secondary btn-sm"><Plus size={14} /> Add provider</button>
    </div>
    {#if providers.length === 0}
      <EmptyState compact icon={Cloud as any} title="No providers configured" description="Backups are stored locally until you add an S3-compatible provider." />
    {:else}
      <ul class="divide-y divide-line">
        {#each providers as p (p.id)}
          {@const u = providerUsage[p.id]}
          <li class="flex items-center gap-3 px-5 py-3.5">
            <div class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-surface-2 text-muted"><Cloud size={16} /></div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-medium text-fg">{p.name}</span>
                {#if p.is_default}<span class="badge badge-accent">Default</span>{/if}
              </div>
              <p class="truncate text-xs text-subtle">{p.provider_type} · <span class="font-mono">{p.bucket}</span></p>
              <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                {#if u}
                  <span class={u.connected ? 'text-success' : 'text-danger'}><span class="dot mr-1"></span>{u.connected ? 'Connected' : 'Disconnected'}</span>
                  <span class="text-subtle tabular-nums">{u.object_count || 0} objects{u.total_mb ? ` · ${u.total_mb.toFixed(0)} MB` : ''}</span>
                {:else}
                  <span class="skeleton h-3 w-32"></span>
                {/if}
              </div>
            </div>
            <div class="flex shrink-0 gap-0.5">
              <button onclick={() => editProvider(p)} class="btn btn-ghost btn-icon btn-xs" title="Edit" aria-label="Edit provider {p.name}"><Pencil size={14} /></button>
              <button onclick={() => deleteProvider(p)} class="btn btn-danger-ghost btn-icon btn-xs" title="Delete" aria-label="Delete provider {p.name}"><Trash2 size={14} /></button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <div class="card-header">
      <h2 class="card-title">Backup configs</h2>
      <button onclick={newConfig} class="btn btn-secondary btn-sm"><Plus size={14} /> Add config</button>
    </div>
    {#if configs.length === 0}
      <EmptyState compact icon={CalendarClock as any} title="No backup configs" description="Create a schedule to back up sites and databases automatically." />
    {:else}
      <ul class="divide-y divide-line">
        {#each configs as c (c.id)}
          <li class="flex items-start gap-3 px-5 py-3.5">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-medium text-fg">{c.name || 'Unnamed config'}</span>
                <span class="badge">{c.scope_type}</span>
                {#if c.enabled}
                  <span class="badge badge-success"><span class="dot"></span>Active</span>
                {:else}
                  <span class="badge">Inactive</span>
                {/if}
              </div>
              <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-subtle">
                <span class="inline-flex items-center gap-1"><Clock size={12} /><span class="font-mono">{c.cron_expr || 'Not scheduled'}</span></span>
                {#if c.retention_days}<span>· {c.retention_days}d retention</span>{/if}
                {#if c.provider_id}<span>· {providerName.get(c.provider_id) ?? `Provider #${c.provider_id}`}</span>{/if}
                {#if c.last_run_at}<span>· Last run {fmtRelative(c.last_run_at)}</span>{/if}
              </p>
            </div>
            <div class="flex shrink-0 gap-0.5">
              <button onclick={() => runConfig(c.id)} class="btn btn-ghost btn-icon btn-xs" title="Run now" aria-label="Run {c.name || 'config'} now"><Play size={14} /></button>
              <button onclick={() => editConfig(c)} class="btn btn-ghost btn-icon btn-xs" title="Edit" aria-label="Edit {c.name || 'config'}"><Pencil size={14} /></button>
              <button onclick={() => deleteConfig(c)} class="btn btn-danger-ghost btn-icon btn-xs" title="Delete" aria-label="Delete {c.name || 'config'}"><Trash2 size={14} /></button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<ProviderModal bind:open={showProviderForm} bind:provider={editingProvider} onsaved={loadProviders} />
<ConfigModal bind:open={showConfigForm} bind:config={editingConfig} {providers} {allSites} onsaved={loadConfigs} />

<ConfirmDialog
  bind:open={confirm.open}
  title={confirm.title}
  message={confirm.message}
  confirmLabel="Delete"
  destructive
  onconfirm={() => confirm.run()} />
