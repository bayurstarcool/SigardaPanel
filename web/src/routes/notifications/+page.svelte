<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Bell, AlertTriangle, Info, CheckCircle2, XCircle, Filter, Trash2, Eye, EyeOff, RefreshCw, Shield, Server, Database, Globe, RotateCcw } from 'lucide-svelte';
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

  let notifications = $state<Notification[]>([]);
  let total = $state(0);
  let unread = $state(0);
  let loading = $state(true);
  let filter = $state<'all' | 'unread' | 'info' | 'warning' | 'error'>('all');
  let error = $state('');
  let acting = $state<number | null>(null);

  function severityIcon(sev: string) {
    switch (sev) {
      case 'error': return XCircle;
      case 'warning': return AlertTriangle;
      case 'success': return CheckCircle2;
      default: return Info;
    }
  }

  function severityColor(sev: string) {
    switch (sev) {
      case 'error': return 'text-red-400 bg-red-500/10 border-red-500/20';
      case 'warning': return 'text-amber-400 bg-amber-500/10 border-amber-500/20';
      case 'success': return 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20';
      default: return 'text-sky-400 bg-sky-500/10 border-sky-400/20';
    }
  }

  function typeIcon(type: string) {
    switch (type) {
      case 'security_changed': return Shield;
      case 'ssl_issued': return Shield;
      case 'site_created': case 'site_deleted': case 'site_assigned': case 'site_removed': return Globe;
      case 'db_created': return Database;
      case 'config_updated': return Server;
      default: return Bell;
    }
  }

  function typeLabel(type: string) {
    switch (type) {
      case 'site_created': return 'Site';
      case 'site_deleted': return 'Site';
      case 'site_assigned': return 'Site';
      case 'site_removed': return 'Site';
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
        notifications = notifications.map(x => x.id === n.id ? { ...x, read: false } : x);
        unread = notifications.filter(x => !x.read).length;
      } else {
        await apiFetch(auth.token, 'POST', `notifications/${n.id}/read`);
        notifications = notifications.map(x => x.id === n.id ? { ...x, read: true } : x);
        unread = notifications.filter(x => !x.read).length;
      }
    } catch {
      // silent
    } finally {
      acting = null;
    }
  }

  async function markAllRead() {
    try {
      await apiFetch(auth.token, 'POST', 'notifications/read-all');
      notifications = notifications.map(n => ({ ...n, read: true }));
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
      notifications = notifications.filter(n => n.id !== id);
      total = notifications.length;
      unread = notifications.filter(n => !n.read).length;
    } catch {
      // silent
    } finally {
      acting = null;
    }
  }

  async function clearAllRead() {
    try {
      await apiFetch(auth.token, 'DELETE', 'notifications/read');
      notifications = notifications.filter(n => !n.read);
      total = notifications.length;
      unread = notifications.filter(n => !n.read).length;
    } catch {
      // silent
    }
  }

  const filtered = $derived(() => {
    switch (filter) {
      case 'unread': return notifications.filter(n => !n.read);
      case 'info': return notifications.filter(n => n.severity === 'info');
      case 'warning': return notifications.filter(n => n.severity === 'warning');
      case 'error': return notifications.filter(n => n.severity === 'error');
      default: return notifications;
    }
  });

  const readCount = $derived(notifications.filter(n => n.read).length);

  const grouped = $derived(() => {
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today.getTime() - 86400000);
    const groups: { label: string; items: Notification[] }[] = [];
    const todayItems: Notification[] = [];
    const yesterdayItems: Notification[] = [];
    const olderItems: Notification[] = [];

    for (const n of filtered()) {
      const d = new Date(n.created_at);
      if (d >= today) todayItems.push(n);
      else if (d >= yesterday) yesterdayItems.push(n);
      else olderItems.push(n);
    }

    if (todayItems.length) groups.push({ label: 'Today', items: todayItems });
    if (yesterdayItems.length) groups.push({ label: 'Yesterday', items: yesterdayItems });
    if (olderItems.length) groups.push({ label: 'Older', items: olderItems });
    return groups;
  });

  onMount(loadNotifications);
</script>

<DashboardLayout active="Notifications">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-5 sm:mb-6">
    <div class="flex items-center gap-3">
      <div class="w-10 h-10 sm:w-11 sm:h-11 rounded-xl bg-gradient-to-br from-sky-400/15 to-indigo-400/10 flex items-center justify-center border border-sky-400/10">
        <Bell size={20} class="text-sky-400" />
      </div>
      <div>
        <h1 class="text-lg sm:text-xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Notifications</h1>
        <p class="text-[0.7rem] sm:text-xs text-slate-500 mt-0.5">
          {#if unread > 0}
            <span class="text-sky-400 font-medium">{unread} unread</span> · {total} total
          {:else}
            {total} notifications
          {/if}
        </p>
      </div>
    </div>
    <div class="flex items-center gap-2">
      {#if unread > 0}
        <button onclick={markAllRead}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-[0.7rem] sm:text-xs font-medium bg-white/[0.04] text-slate-400 border border-white/[0.07] hover:bg-white/[0.06] hover:text-slate-300 transition-all cursor-pointer">
          <Eye size={13} /> Mark all read
        </button>
      {/if}
      {#if readCount > 0}
        <button onclick={clearAllRead}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-[0.7rem] sm:text-xs font-medium bg-white/[0.04] text-slate-400 border border-white/[0.07] hover:bg-red-400/10 hover:text-red-400 transition-all cursor-pointer">
          <RotateCcw size={13} /> Clear all read
        </button>
      {/if}
      <button onclick={loadNotifications}
        class="p-2 rounded-xl bg-white/[0.04] text-slate-400 border border-white/[0.07] hover:bg-white/[0.06] hover:text-slate-300 transition-all cursor-pointer"
        title="Refresh">
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
      </button>
    </div>
  </div>

  <!-- Filter Tabs -->
  <div class="flex items-center gap-1.5 mb-4 sm:mb-5 overflow-x-auto pb-1">
    {#each [
      { key: 'all', label: 'All', count: notifications.length },
      { key: 'unread', label: 'Unread', count: unread },
      { key: 'error', label: 'Errors', count: notifications.filter(n => n.severity === 'error').length },
      { key: 'warning', label: 'Warnings', count: notifications.filter(n => n.severity === 'warning').length },
      { key: 'info', label: 'Info', count: notifications.filter(n => n.severity === 'info').length },
    ] as tab}
      <button onclick={() => filter = tab.key as typeof filter}
        class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[0.7rem] sm:text-xs font-medium whitespace-nowrap transition-all cursor-pointer border
          {filter === tab.key
            ? 'bg-sky-400/10 text-sky-400 border-sky-400/20'
            : 'bg-white/[0.02] text-slate-500 border-white/[0.05] hover:bg-white/[0.04] hover:text-slate-400'}">
        {tab.label}
        {#if tab.count > 0}
          <span class="text-[0.6rem] px-1 py-0 rounded-full {filter === tab.key ? 'bg-sky-400/20' : 'bg-white/[0.06]'}">{tab.count}</span>
        {/if}
      </button>
    {/each}
  </div>

  <!-- Content -->
  {#if loading}
    <div class="space-y-3">
      {#each Array(4) as _}
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-4 animate-pulse">
          <div class="flex items-start gap-3">
            <div class="w-8 h-8 rounded-lg bg-white/[0.04] shrink-0"></div>
            <div class="flex-1 space-y-2">
              <div class="h-3.5 w-48 bg-white/[0.04] rounded-lg"></div>
              <div class="h-3 w-72 bg-white/[0.03] rounded-lg"></div>
              <div class="h-2.5 w-20 bg-white/[0.03] rounded-lg"></div>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {:else if error}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-8 text-center">
      <div class="w-12 h-12 rounded-2xl bg-red-500/10 border border-red-500/20 flex items-center justify-center mx-auto mb-3">
        <XCircle size={22} class="text-red-400" />
      </div>
      <p class="text-sm font-medium text-slate-400 mb-1">Failed to load</p>
      <p class="text-xs text-slate-600 mb-3">{error}</p>
      <button onclick={loadNotifications}
        class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold bg-sky-400/10 text-sky-400 border border-sky-400/20 hover:bg-sky-400/20 transition-all cursor-pointer">
        <RefreshCw size={12} /> Retry
      </button>
    </div>
  {:else if notifications.length === 0}
    <!-- Empty State -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-10 sm:p-14 text-center">
      <div class="w-16 h-16 rounded-2xl bg-white/[0.03] border border-white/[0.06] flex items-center justify-center mx-auto mb-4">
        <Bell size={28} class="text-white/[0.08]" />
      </div>
      <p class="text-sm font-medium text-slate-400 mb-1">No notifications</p>
      <p class="text-xs text-slate-600 max-w-xs mx-auto">
        System alerts and events will appear here.
      </p>
    </div>
  {:else if grouped().length === 0}
    <!-- Empty after filter -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl p-10 text-center">
      <div class="w-12 h-12 rounded-2xl bg-white/[0.03] border border-white/[0.06] flex items-center justify-center mx-auto mb-3">
        <Filter size={22} class="text-white/[0.08]" />
      </div>
      <p class="text-sm font-medium text-slate-400 mb-0.5">No matching notifications</p>
      <p class="text-xs text-slate-600">Try a different filter</p>
    </div>
  {:else}
    <div class="space-y-5">
      {#each grouped() as group}
        <!-- Group Header -->
        <div class="flex items-center gap-2.5 mb-2">
          <span class="text-[0.65rem] sm:text-xs font-semibold text-slate-500 uppercase tracking-wider">{group.label}</span>
          <div class="flex-1 h-px bg-white/[0.04]"></div>
          <span class="text-[0.6rem] text-slate-600">{group.items.length}</span>
        </div>

        <div class="space-y-1.5">
          {#each group.items as n (n.id)}
            {@const SevIcon = severityIcon(n.severity)}
            {@const TypeIcon = typeIcon(n.type)}
            <div class="group flex items-start gap-2.5 sm:gap-3 p-3 sm:p-3.5 rounded-xl transition-all duration-200
              {n.read
                ? 'bg-white/[0.015] hover:bg-white/[0.03] border border-transparent hover:border-white/[0.05]'
                : 'bg-white/[0.035] hover:bg-white/[0.05] border border-white/[0.06] hover:border-white/[0.1]'}">

              <!-- Severity Icon -->
              <div class="shrink-0 w-7 h-7 sm:w-8 sm:h-8 rounded-lg flex items-center justify-center border {severityColor(n.severity)}">
                <SevIcon size={14} />
              </div>

              <!-- Content -->
              <div class="flex-1 min-w-0">
                <div class="flex items-start justify-between gap-2 mb-0.5">
                  <p class="text-[0.8rem] sm:text-sm font-medium {n.read ? 'text-slate-400' : 'text-slate-200'} truncate">
                    {n.title}
                  </p>
                  <div class="flex items-center gap-1 shrink-0">
                    <!-- Type Badge -->
                    <span class="hidden sm:inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[0.55rem] font-medium bg-white/[0.04] text-slate-500 border border-white/[0.05]">
                      <TypeIcon size={9} />
                      {typeLabel(n.type)}
                    </span>
                    {#if !n.read}
                      <span class="w-1.5 h-1.5 rounded-full bg-sky-400 shadow-[0_0_6px_rgba(56,189,248,0.6)]"></span>
                    {/if}
                  </div>
                </div>
                {#if n.message}
                  <p class="text-[0.7rem] sm:text-xs {n.read ? 'text-slate-600' : 'text-slate-500'} line-clamp-2 mb-1">{n.message}</p>
                {/if}
                <div class="flex items-center gap-2">
                  <span class="text-[0.6rem] sm:text-[0.65rem] text-slate-600">{fmtRelative(n.created_at)}</span>
                </div>
              </div>

              <!-- Actions -->
              <div class="shrink-0 flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                <button onclick={() => toggleRead(n)}
                  class="p-1.5 rounded-lg text-slate-500 hover:text-sky-400 hover:bg-sky-400/10 transition border-0 cursor-pointer disabled:opacity-50"
                  disabled={acting === n.id}
                  title={n.read ? 'Mark unread' : 'Mark read'}>
                  {#if n.read}<EyeOff size={12} />{:else}<Eye size={12} />{/if}
                </button>
                <button onclick={() => deleteNotification(n.id)}
                  class="p-1.5 rounded-lg text-slate-500 hover:text-red-400 hover:bg-red-400/10 transition border-0 cursor-pointer disabled:opacity-50"
                  disabled={acting === n.id}
                  title="Delete">
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/each}
    </div>
  {/if}
</DashboardLayout>
