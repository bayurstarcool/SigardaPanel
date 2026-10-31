<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Bell, AlertTriangle, Info, CheckCircle2, XCircle, Filter, Trash2, Eye, EyeOff, RefreshCw, Shield, Server, Database, Globe, RotateCcw, User } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type Notification = {
    id: number;
    user_id: number;
    type: string;
    title: string;
    message: string;
    severity: string;
    read: boolean;
    entity_type: string;
    entity_id: number;
    created_at: string;
  };

  type FilterKey = 'all' | 'unread' | 'info' | 'warning' | 'error';

  let notifications = $state<Notification[]>([]);
  let total = $state(0);
  let unread = $state(0);
  let loading = $state(true);
  let filter = $state<FilterKey>('all');
  let error = $state('');
  let acting = $state<number | null>(null);

  const severity: Record<string, { icon: any; cls: string }> = {
    error: { icon: XCircle, cls: 'bg-danger/10 text-danger' },
    warning: { icon: AlertTriangle, cls: 'bg-warning/10 text-warning' },
    success: { icon: CheckCircle2, cls: 'bg-success/10 text-success' },
    info: { icon: Info, cls: 'bg-info/10 text-info' },
  };

  function typeIcon(type: string) {
    switch (type) {
      case 'security_changed': case 'ssl_issued': return Shield;
      case 'site_created': case 'site_deleted': case 'site_assigned': case 'site_removed': return Globe;
      case 'db_created': return Database;
      case 'config_updated': return Server;
      case 'user_created': return User;
      default: return Bell;
    }
  }

  function typeLabel(type: string) {
    switch (type) {
      case 'site_created': case 'site_deleted': case 'site_assigned': case 'site_removed': return 'Site';
      case 'ssl_issued': return 'SSL';
      case 'db_created': return 'Database';
      case 'security_changed': return 'Security';
      case 'config_updated': return 'Config';
      case 'user_created': return 'User';
      default: return 'System';
    }
  }

  async function loadNotifications() {
    loading = true;
    error = '';
    try {
      const data = await apiFetch(auth.token, 'GET', 'notifications?limit=50');
      notifications = data?.notifications || [];
      total = data?.total || 0;
      unread = data?.unread || 0;
    } catch (e: any) {
      if (e?.message?.includes('forbidden') || e?.message?.includes('admin')) {
        notifications = [];
      } else {
        error = e?.message || 'Failed to load notifications';
      }
    } finally {
      loading = false;
    }
  }

  async function toggleRead(n: Notification) {
    if (acting) return;
    acting = n.id;
    try {
      if (n.read) {
        // No "mark unread" API endpoint, so just toggle locally
        notifications = notifications.map((x) => (x.id === n.id ? { ...x, read: false } : x));
      } else {
        await apiFetch(auth.token, 'POST', `notifications/${n.id}/read`);
        notifications = notifications.map((x) => (x.id === n.id ? { ...x, read: true } : x));
      }
      unread = notifications.filter((x) => !x.read).length;
    } catch {
      // silent
    } finally {
      acting = null;
    }
  }

  async function markAllRead() {
    try {
      await apiFetch(auth.token, 'POST', 'notifications/read-all');
      notifications = notifications.map((n) => ({ ...n, read: true }));
      unread = 0;
    } catch {
      // silent
    }
  }

  async function deleteNotification(id: number) {
    if (acting) return;
    acting = id;
    try {
      await apiFetch(auth.token, 'DELETE', `notifications/${id}`);
      notifications = notifications.filter((n) => n.id !== id);
      total = notifications.length;
      unread = notifications.filter((n) => !n.read).length;
    } catch {
      // silent
    } finally {
      acting = null;
    }
  }

  async function clearAllRead() {
    try {
      await apiFetch(auth.token, 'DELETE', 'notifications/read');
      notifications = notifications.filter((n) => !n.read);
      total = notifications.length;
      unread = notifications.filter((n) => !n.read).length;
    } catch {
      // silent
    }
  }

  const counts = $derived<Record<FilterKey, number>>({
    all: notifications.length,
    unread,
    error: notifications.filter((n) => n.severity === 'error').length,
    warning: notifications.filter((n) => n.severity === 'warning').length,
    info: notifications.filter((n) => n.severity === 'info').length,
  });

  const filtered = $derived.by(() => {
    switch (filter) {
      case 'unread': return notifications.filter((n) => !n.read);
      case 'info': case 'warning': case 'error': return notifications.filter((n) => n.severity === filter);
      default: return notifications;
    }
  });

  const readCount = $derived(notifications.filter((n) => n.read).length);

  const grouped = $derived.by(() => {
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today.getTime() - 86400000);
    const buckets: Record<string, Notification[]> = { Today: [], Yesterday: [], Older: [] };
    for (const n of filtered) {
      const d = new Date(n.created_at);
      if (d >= today) buckets.Today.push(n);
      else if (d >= yesterday) buckets.Yesterday.push(n);
      else buckets.Older.push(n);
    }
    return Object.entries(buckets)
      .filter(([, items]) => items.length)
      .map(([label, items]) => ({ label, items }));
  });

  const tabs: [FilterKey, string][] = [['all', 'All'], ['unread', 'Unread'], ['error', 'Errors'], ['warning', 'Warnings'], ['info', 'Info']];

  onMount(loadNotifications);
</script>

<DashboardLayout active="Notifications">
  <PageHeader title="Notifications" description={loading ? 'Loading…' : unread > 0 ? `${unread} unread · ${total} total` : `${total} notifications`}>
    {#snippet actions()}
      {#if unread > 0}
        <button onclick={markAllRead} class="btn btn-secondary btn-sm"><Eye size={14} /> Mark all read</button>
      {/if}
      {#if readCount > 0}
        <button onclick={clearAllRead} class="btn btn-secondary btn-sm"><RotateCcw size={14} /> Clear all read</button>
      {/if}
      <button onclick={loadNotifications} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
    {/snippet}
  </PageHeader>

  <div class="card overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 border-b border-line px-4 py-3">
      <div class="segmented max-w-full overflow-x-auto">
        {#each tabs as [key, label] (key)}
          <button class="segment {filter === key ? 'segment-active' : ''}" onclick={() => (filter = key)}>
            {label}
            {#if counts[key] > 0}<span class="text-subtle tabular-nums">{counts[key]}</span>{/if}
          </button>
        {/each}
      </div>
    </div>

    {#if loading}
      <div class="divide-y divide-line">
        {#each Array(4) as _, i (i)}
          <div class="flex items-start gap-3 px-4 py-3.5">
            <div class="skeleton size-8 shrink-0"></div>
            <div class="flex-1 space-y-2">
              <div class="skeleton h-3.5 w-48"></div>
              <div class="skeleton h-3 w-72 max-w-full"></div>
            </div>
          </div>
        {/each}
      </div>
    {:else if error}
      <EmptyState compact icon={XCircle as any} title="Failed to load" description={error} actionLabel="Retry" onaction={loadNotifications} />
    {:else if notifications.length === 0}
      <EmptyState compact icon={Bell as any} title="No notifications" description="System alerts and events will appear here." />
    {:else if grouped.length === 0}
      <EmptyState compact icon={Filter as any} title="No matching notifications" description="Try a different filter." />
    {:else}
      {#each grouped as group (group.label)}
        <div class="flex items-center justify-between border-b border-line bg-surface-2/60 px-4 py-1.5 text-xs font-medium text-subtle">
          <span>{group.label}</span>
          <span class="tabular-nums">{group.items.length}</span>
        </div>
        <ul class="divide-y divide-line border-b border-line last:border-b-0">
          {#each group.items as n (n.id)}
            {@const sev = severity[n.severity] ?? severity.info}
            {@const TypeIcon = typeIcon(n.type)}
            <li class="group flex items-start gap-3 px-4 py-3.5 transition-colors duration-100 hover:bg-surface-2/60">
              <div class="flex size-8 shrink-0 items-center justify-center rounded-lg {sev.cls}">
                <sev.icon size={16} />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  {#if !n.read}<span class="dot text-accent" aria-label="Unread"></span>{/if}
                  <p class="truncate text-sm font-medium {n.read ? 'text-muted' : 'text-fg'}">{n.title}</p>
                  <span class="badge ml-auto max-sm:hidden"><TypeIcon size={12} />{typeLabel(n.type)}</span>
                </div>
                {#if n.message}
                  <p class="mt-0.5 line-clamp-2 text-[13px] {n.read ? 'text-subtle' : 'text-muted'}">{n.message}</p>
                {/if}
                <p class="mt-1 text-xs text-subtle">{fmtRelative(n.created_at)}</p>
              </div>
              <div class="flex shrink-0 items-center gap-0.5 transition-opacity duration-150 sm:opacity-0 sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
                <button
                  onclick={() => toggleRead(n)}
                  class="btn btn-ghost btn-icon btn-xs"
                  disabled={acting === n.id}
                  title={n.read ? 'Mark unread' : 'Mark read'}
                  aria-label={n.read ? 'Mark unread' : 'Mark read'}>
                  {#if n.read}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                </button>
                <button
                  onclick={() => deleteNotification(n.id)}
                  class="btn btn-danger-ghost btn-icon btn-xs"
                  disabled={acting === n.id}
                  title="Delete"
                  aria-label="Delete notification">
                  <Trash2 size={14} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
      {/each}
    {/if}
  </div>
</DashboardLayout>
