<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { ClipboardCheck, RefreshCw, ChevronLeft, ChevronRight, ChevronDown, ShieldOff } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  // GET /api/v1/audit-logs (admin only). Query: limit (1-1000), offset, user_id, resource, action.
  // Response: { entries: [{id,user_id,action,resource,resource_id,details,created_at}], limit, offset, count }
  // There is no total count, so we fetch one extra row to know whether a next page exists.
  const PAGE_SIZE = 50;

  type Entry = {
    id: number;
    user_id: number | null;
    action: string;
    resource: string;
    resource_id: number | null;
    details: string | null;
    created_at: string;
  };

  // Action names recorded by the backend (internal/audit + RecordAction call sites).
  const ACTIONS = [
    'login', 'logout', 'impersonate', 'stop_impersonation',
    'user_create', 'user_update', 'user_delete',
    'server_create', 'server_update', 'server_delete', 'server_register', 'server_command_exec', 'terminal_open',
    'site_create', 'site_update', 'site_delete', 'site_deploy', 'site_reload_nginx',
    'ssl_issue', 'ssl_renew',
    'backup_create', 'backup_restore', 'backup_delete',
    'backup_config_create', 'backup_config_update', 'backup_config_delete',
    'backup_config_run_full', 'backup_config_run_files', 'backup_config_run_db',
    'backup_provider_create', 'backup_provider_update', 'backup_provider_delete',
    'backup_schedule_create', 'backup_schedule_update', 'backup_schedule_delete',
    'database_create', 'database_update', 'database_delete', 'database_import',
    'database_backup_create', 'db_backup_create',
    'dbuser_create', 'dbuser_update', 'dbuser_delete', 'dbuser_grant', 'dbuser_grant_update', 'dbuser_password_rotate',
    'phpmyadmin_reset_passwords',
    'cron_create', 'cron_delete', 'ssh_user_create', 'ssh_user_delete',
    'cloudflare.config.update', 'cloudflare.config.delete', 'cloudflare.dns.create', 'cloudflare.dns.update',
    'cloudflare.dns.delete', 'cloudflare.dns.bulk_delete', 'cloudflare.dns.import', 'cloudflare.cache.purge',
    'alert_channel_create', 'alert_channel_delete',
    'registration_token_create', 'security_baseline_save', 'job_cancel', 'transfer', 'update',
  ];
  const RESOURCES = ['user', 'server', 'site', 'ssl', 'backup', 'session', 'job'];

  let entries = $state<Entry[]>([]);
  let users = $state<any[]>([]);
  let loading = $state(true);
  let page = $state(1);
  let hasNext = $state(false);
  let actionFilter = $state('');
  let resourceFilter = $state('');
  let userFilter = $state('');
  let expanded = $state<number | null>(null);

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));
  const userNames = $derived(new Map<number, string>(users.map((u) => [u.id, u.username || u.email])));

  onMount(() => {
    if (!auth.user || !isAdmin) { loading = false; return; }
    loadUsers();
    loadLogs();
  });

  async function loadUsers() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'users');
      users = d?.users || [];
    } catch {}
  }

  async function loadLogs() {
    loading = true;
    expanded = null;
    const q = new URLSearchParams({ limit: String(PAGE_SIZE + 1), offset: String((page - 1) * PAGE_SIZE) });
    if (actionFilter) q.set('action', actionFilter);
    if (resourceFilter) q.set('resource', resourceFilter);
    if (userFilter) q.set('user_id', userFilter);
    try {
      const d = await apiFetch(auth.token, 'GET', `audit-logs?${q}`);
      const list: Entry[] = d?.entries || [];
      hasNext = list.length > PAGE_SIZE;
      entries = list.slice(0, PAGE_SIZE);
    } catch (e: any) {
      if (e.message !== 'session expired') toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
    page = 1;
    loadLogs();
  }

  function resetFilters() {
    actionFilter = '';
    resourceFilter = '';
    userFilter = '';
    applyFilters();
  }

  function goPage(p: number) {
    page = p;
    loadLogs();
  }

  function parseDetails(details: any): Record<string, any> | null {
    if (!details) return null;
    if (typeof details === 'object') return details;
    try {
      const v = JSON.parse(details);
      return v && typeof v === 'object' ? v : null;
    } catch {
      return null;
    }
  }

  function summary(e: Entry): string {
    const d = parseDetails(e.details);
    if (!d) return e.details || '';
    const action = d.action || e.action;
    const itemType = d.item_type || (d.is_dir === true ? 'folder' : d.is_dir === false ? 'file' : 'item');
    if (String(action).startsWith('file_')) {
      if (action === 'file_rename') return `${itemType}: ${d.old_path || ''} → ${d.new_name || ''}`;
      return `${itemType}: ${d.path || d.old_path || d.new_name || ''}`;
    }
    return Object.entries(d)
      .filter(([k]) => k !== 'ip')
      .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`)
      .join(' · ');
  }

  function ipOf(e: Entry): string {
    const d = parseDetails(e.details);
    return d?.ip || d?.ip_address || d?.remote_ip || '';
  }

  function actor(e: Entry): string {
    if (e.user_id == null) return 'System';
    return userNames.get(e.user_id) ?? `User #${e.user_id}`;
  }

  function actionTone(a: string): string {
    if (/delete|bulk_delete|remove/.test(a)) return 'badge-danger';
    if (/create|issue|register/.test(a)) return 'badge-success';
    if (/login|logout|impersonat/.test(a)) return 'badge-info';
    if (/exec|terminal|restore/.test(a)) return 'badge-warning';
    return '';
  }

  const filtersActive = $derived(!!(actionFilter || resourceFilter || userFilter));
</script>

<DashboardLayout active="Audit">
  <PageHeader title="Audit log" description="Who did what, and when. Newest first.">
    {#snippet actions()}
      {#if isAdmin}
        <button onclick={loadLogs} class="btn btn-secondary btn-sm" disabled={loading}>
          <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
        </button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if !isAdmin}
    <div class="card">
      <EmptyState icon={ShieldOff as any} title="Admins only" description="The audit log is only available to administrators." />
    </div>
  {:else}
    <div class="card">
      <div class="flex flex-wrap items-end gap-3 border-b border-line px-4 py-3">
        <div class="w-full sm:w-56">
          <label class="sr-only" for="audit-action">Action</label>
          <select id="audit-action" bind:value={actionFilter} onchange={applyFilters} class="input h-8 text-[13px]">
            <option value="">All actions</option>
            {#each ACTIONS as a (a)}<option value={a}>{a}</option>{/each}
          </select>
        </div>
        <div class="w-[calc(50%-0.375rem)] sm:w-40">
          <label class="sr-only" for="audit-resource">Resource</label>
          <select id="audit-resource" bind:value={resourceFilter} onchange={applyFilters} class="input h-8 text-[13px]">
            <option value="">All resources</option>
            {#each RESOURCES as r (r)}<option value={r}>{r}</option>{/each}
          </select>
        </div>
        <div class="w-[calc(50%-0.375rem)] sm:w-48">
          <label class="sr-only" for="audit-user">User</label>
          <select id="audit-user" bind:value={userFilter} onchange={applyFilters} class="input h-8 text-[13px]">
            <option value="">All users</option>
            {#each users as u (u.id)}<option value={String(u.id)}>{u.username || u.email}</option>{/each}
          </select>
        </div>
        {#if filtersActive}
          <button onclick={resetFilters} class="btn btn-ghost btn-sm">Clear filters</button>
        {/if}
      </div>

      {#if loading && entries.length === 0}
        <div class="space-y-3 p-4">
          {#each Array(8) as _, i (i)}<div class="skeleton h-9"></div>{/each}
        </div>
      {:else if entries.length === 0}
        <EmptyState
          compact
          icon={ClipboardCheck as any}
          title={filtersActive ? 'No entries match' : 'No audit entries'}
          description={filtersActive ? 'Try a different filter.' : 'Audit log entries will appear here.'} />
      {:else}
        <div class="table-wrap {loading ? 'opacity-60' : ''} transition-opacity duration-150">
          <table class="table">
            <thead>
              <tr>
                <th>Time</th>
                <th class="max-sm:hidden">Actor</th>
                <th>Action</th>
                <th>Target</th>
                <th class="max-sm:hidden">IP</th>
                <th class="w-8"><span class="sr-only">Details</span></th>
              </tr>
            </thead>
            <tbody>
              {#each entries as e (e.id)}
                {@const sum = summary(e)}
                {@const ip = ipOf(e)}
                <tr>
                  <td class="whitespace-nowrap text-muted tabular-nums" title={new Date(e.created_at).toLocaleString('id-ID')}>
                    {fmtRelative(e.created_at)}
                  </td>
                  <td class="whitespace-nowrap max-sm:hidden">
                    <span class={e.user_id == null ? 'text-subtle' : 'text-fg'}>{actor(e)}</span>
                  </td>
                  <td>
                    <span class="badge font-mono {actionTone(e.action)}">{e.action}</span>
                    <div class="mt-1 text-xs text-subtle sm:hidden">{actor(e)}</div>
                  </td>
                  <td class="max-w-0 min-w-40 sm:w-[40%]">
                    <div class="text-[13px] text-fg">
                      {e.resource || '—'}{#if e.resource_id != null}<span class="text-subtle tabular-nums"> #{e.resource_id}</span>{/if}
                    </div>
                    {#if sum}<div class="truncate text-xs text-subtle" title={sum}>{sum}</div>{/if}
                  </td>
                  <td class="font-mono text-[13px] whitespace-nowrap text-muted max-sm:hidden">{ip || '—'}</td>
                  <td class="text-right">
                    {#if e.details}
                      <button
                        class="btn btn-ghost btn-icon btn-xs"
                        aria-label={expanded === e.id ? 'Hide details' : 'Show details'}
                        aria-expanded={expanded === e.id}
                        onclick={() => (expanded = expanded === e.id ? null : e.id)}>
                        <ChevronDown size={14} class="transition-transform duration-150 {expanded === e.id ? 'rotate-180' : ''}" />
                      </button>
                    {/if}
                  </td>
                </tr>
                {#if expanded === e.id}
                  {@const d = parseDetails(e.details)}
                  <tr class="hover:bg-transparent">
                    <td colspan="6" class="bg-surface-2/60">
                      <pre class="overflow-x-auto font-mono text-xs leading-relaxed whitespace-pre-wrap break-all text-muted">{d ? JSON.stringify(d, null, 2) : e.details}</pre>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    {#if page > 1 || hasNext}
      <div class="flex items-center justify-between gap-3 pt-4 text-[13px]">
        <span class="text-subtle tabular-nums">
          {(page - 1) * PAGE_SIZE + 1}–{(page - 1) * PAGE_SIZE + entries.length}
        </span>
        <div class="flex items-center gap-1">
          <button class="btn btn-ghost btn-xs" disabled={page === 1 || loading} onclick={() => goPage(page - 1)}>
            <ChevronLeft size={14} /> Newer
          </button>
          <span class="px-2 text-subtle tabular-nums">Page {page}</span>
          <button class="btn btn-ghost btn-xs" disabled={!hasNext || loading} onclick={() => goPage(page + 1)}>
            Older <ChevronRight size={14} />
          </button>
        </div>
      </div>
    {/if}
  {/if}
</DashboardLayout>
