<script lang="ts">
  import { page } from '$app/stores';
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { ArrowLeft, X, Loader2, Check, Shield, Key, User, Globe, Trash2, Eye, RefreshCw, LogIn, AlertTriangle, ExternalLink, Settings } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  const userId = $derived(Number($page.params.id));

  // User data
  let user = $state<any>(null);
  let loading = $state(true);

  // Edit form
  let form = $state({ username: '', email: '', role: 'user', password: '' });
  let busy = $state(false);

  // Assigned sites
  let assignedSites = $state<any[]>([]);
  let sitesLoading = $state(false);
  let allSites = $state<any[]>([]);
  let assignSiteId = $state<number | null>(null);
  let assignPermission = $state('read');
  let assigningSite = $state(false);
  let removingSiteId = $state<number | null>(null);
  let updatingPermSiteId = $state<number | null>(null);

  // Impersonation
  let showImpersonateDialog = $state(false);
  let impersonating = $state(false);

  // Usage
  let usage = $state<any>(null);
  let usageLoading = $state(false);

  onMount(async () => {
    if (!auth.user) { loading = false; return; }
    try {
      const d = await apiFetch(auth.token, 'GET', 'users');
      user = (d?.users || []).find((u: any) => u.id === userId);
      if (user) {
        form = { username: user.username, email: user.email || '', role: user.role, password: '' };
        // Fetch additional data in parallel
        fetchAssignedSites();
        fetchAllSites();
        fetchUsage();
        checkImpersonation();
      }
    } catch (e: any) { toasts.error(e.message); }
    finally { loading = false; }
  });

  // ─── Edit user ───
  async function save() {
    busy = true;
    try {
      const body: any = {};
      if (form.username !== user.username) body.username = form.username;
      if (form.email !== (user.email || '')) body.email = form.email;
      if (form.role !== user.role) body.role = form.role;
      if (form.password) body.password = form.password;
      await apiFetch(auth.token, 'PATCH', `users/${userId}`, body);
      if (form.password) {
        user = { ...user, password_set: true };
        form = { ...form, password: '' };
      }
      toasts.success('User updated successfully');
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = false; }
  }

  // ─── Assigned sites ───
  async function fetchAssignedSites() {
    if (!auth.user) return;
    sitesLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `users/${userId}/sites`);
      assignedSites = d?.sites || [];
    } catch (e: any) {
      console.error('Failed to fetch assigned sites:', e);
    } finally {
      sitesLoading = false;
    }
  }

  async function fetchAllSites() {
    if (!auth.user) return;
    try {
      const d = await apiFetch(auth.token, 'GET', 'sites');
      allSites = d?.sites || [];
    } catch (e: any) {
      console.error('Failed to fetch sites:', e);
    }
  }

  async function assignSite() {
    if (!auth.user || !assignSiteId) return;
    assigningSite = true;
    try {
      await apiFetch(auth.token, 'POST', `users/${userId}/sites`, {
        site_id: assignSiteId,
        permission: assignPermission
      });
      assignSiteId = null;
      assignPermission = 'read';
      toasts.success('Site assigned successfully');
      await fetchAssignedSites();
    } catch (e: any) { toasts.error(e.message); }
    finally { assigningSite = false; }
  }

  async function removeSite(siteId: number) {
    if (!auth.user) return;
    removingSiteId = siteId;
    try {
      await apiFetch(auth.token, 'DELETE', `users/${userId}/sites/${siteId}`);
      assignedSites = assignedSites.filter(s => s.site_id !== siteId);
      toasts.success('Site removed');
    } catch (e: any) { toasts.error(e.message); }
    finally { removingSiteId = null; }
  }

  async function updatePermission(siteId: number, newPerm: string) {
    if (!auth.user) return;
    updatingPermSiteId = siteId;
    try {
      await apiFetch(auth.token, 'PATCH', `users/${userId}/sites/${siteId}`, { permission: newPerm });
      assignedSites = assignedSites.map(s => s.site_id === siteId ? { ...s, permission: newPerm } : s);
      toasts.success('Permission updated');
    } catch (e: any) { toasts.error(e.message); }
    finally { updatingPermSiteId = null; }
  }

  // Available sites (not yet assigned)
  let availableSites = $derived(
    allSites.filter(s => !assignedSites.some(a => a.site_id === s.id))
  );

  // ─── Impersonation ───
  function checkImpersonation() {
    impersonating = auth.isImpersonating;
  }

  async function doImpersonate() {
    if (!auth.user) return;
    try {
      await auth.impersonateUser(userId);
      showImpersonateDialog = false;
      window.location.href = '/sites';
    } catch (e: any) {
      toasts.error(e.message || 'Impersonation failed');
      showImpersonateDialog = false;
    }
  }

  async function stopImpersonation() {
    try {
      await auth.stopImpersonation();
      impersonating = false;
    } catch (e: any) {
      toasts.error(e.message || 'Failed to stop impersonation');
    }
  }

  // ─── Usage ───
  async function fetchUsage() {
    if (!auth.user) return;
    usageLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `users/${userId}/usage`);
      usage = d;
    } catch (e: any) {
      console.error('Failed to fetch usage:', e);
    } finally {
      usageLoading = false;
    }
  }

  // ─── Helpers ───
  function roleBadge(role: string) {
    switch (role) {
      case 'super_admin': return 'bg-amber-500/15 text-amber-400 ring-1 ring-amber-400/20';
      case 'admin': return 'bg-sky-500/15 text-sky-400 ring-1 ring-sky-400/20';
      default: return 'bg-white/5 text-slate-400 ring-1 ring-white/10';
    }
  }

  function permissionBadge(perm: string) {
    switch (perm) {
      case 'owner': return 'bg-emerald-500/15 text-emerald-400 ring-1 ring-emerald-400/20';
      case 'admin': return 'bg-amber-500/15 text-amber-400 ring-1 ring-amber-400/20';
      case 'write': return 'bg-sky-500/15 text-sky-400 ring-1 ring-sky-400/20';
      default: return 'bg-white/5 text-slate-400 ring-1 ring-white/10';
    }
  }

  function formatBytes(bytes: number) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  function formatDate(dateStr: string) {
    if (!dateStr) return '—';
    return new Date(dateStr).toLocaleDateString('en-US', {
      year: 'numeric', month: 'short', day: 'numeric'
    });
  }
</script>

<DashboardLayout active="Users">
  <!-- Impersonation banner -->
  {#if impersonating}
    <div class="mb-6 bg-amber-500/10 border border-amber-500/20 rounded-xl p-4 flex flex-col sm:flex-row items-start sm:items-center gap-3">
      <div class="flex items-center gap-2 text-amber-400">
        <AlertTriangle size={18} />
        <span class="text-sm font-medium">You are impersonating a user</span>
      </div>
      <button
        onclick={stopImpersonation}
        class="ml-0 sm:ml-auto inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium bg-amber-500/15 text-amber-400 hover:bg-amber-500/25 transition cursor-pointer border-0"
      >
        <Eye size={12} /> Stop Impersonation
      </button>
    </div>
  {/if}

  <a href="/users" class="inline-flex items-center gap-1.5 text-sm text-slate-400 hover:text-slate-200 transition mb-6">
    <ArrowLeft size={14} /> Back to Users
  </a>

  <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent mb-6">
    Edit User {#if user}— {form.username}{/if}
  </h1>

  <!-- Loading skeleton -->
  {#if loading}
    <div class="space-y-6 max-w-3xl">
      <!-- Edit form skeleton -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
        <div class="space-y-5">
          {#each Array(4) as _}
            <div>
              <div class="h-2.5 w-20 bg-white/[0.06] rounded-lg animate-pulse mb-2"></div>
              <div class="h-10 w-full bg-white/[0.04] rounded-xl animate-pulse"></div>
            </div>
          {/each}
          <div class="flex gap-3 pt-2">
            <div class="h-9 w-32 bg-white/[0.06] rounded-xl animate-pulse"></div>
            <div class="h-9 w-20 bg-white/[0.04] rounded-xl animate-pulse"></div>
          </div>
        </div>
      </div>
      <!-- Sites skeleton -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
        <div class="h-5 w-32 bg-white/[0.06] rounded-lg animate-pulse mb-5"></div>
        {#each Array(2) as _}
          <div class="h-14 w-full bg-white/[0.04] rounded-xl animate-pulse mb-3"></div>
        {/each}
      </div>
      <!-- Usage skeleton -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
        <div class="h-5 w-28 bg-white/[0.06] rounded-lg animate-pulse mb-5"></div>
        <div class="grid grid-cols-2 gap-4">
          {#each Array(4) as _}
            <div class="h-16 bg-white/[0.04] rounded-xl animate-pulse"></div>
          {/each}
        </div>
      </div>
    </div>

  {:else if user}
    <div class="space-y-6 max-w-3xl">

      <!-- ═══════════ SECTION: Edit User ═══════════ -->
      <div class="bg-[#040a14] border border-white/[0.07] backdrop-blur-sm rounded-xl p-6">
        <div class="flex items-center gap-2 mb-5 pb-4 border-b border-white/[0.05]">
          <Settings size={16} class="text-sky-400" />
          <h2 class="text-sm font-semibold text-slate-200">Edit Profile</h2>
        </div>

        <!-- User header -->
        <div class="flex items-center gap-3 mb-6">
          <div class="w-12 h-12 rounded-xl bg-white/[0.05] flex items-center justify-center">
            <User size={20} class="text-slate-400" />
          </div>
          <div>
            <div class="text-base font-medium text-slate-200">{user.username}</div>
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-lg text-[0.65rem] font-medium {roleBadge(user.role)} mt-1">
              {#if user.role === 'super_admin'}<Shield size={9} />{/if}
              {user.role === 'super_admin' ? 'Super Admin' : user.role === 'admin' ? 'Admin' : 'User'}
            </span>
          </div>
        </div>

        <div class="space-y-4">
          <div>
            <label for="ude-name" class="block text-xs text-slate-500 mb-1.5">Username</label>
            <input id="ude-name" bind:value={form.username} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
          </div>
          <div>
            <label for="ude-email" class="block text-xs text-slate-500 mb-1.5">Email</label>
            <input id="ude-email" bind:value={form.email} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
          </div>
          <div>
            <label for="ude-role" class="block text-xs text-slate-500 mb-1.5">Role</label>
            <select id="ude-role" bind:value={form.role} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition">
              <option value="user">User</option>
              <option value="admin">Admin</option>
              {#if hasRole(auth.user?.role, 'super_admin')}
                <option value="super_admin">Super Admin</option>
              {/if}
            </select>
          </div>
          <div>
            <label for="ude-pass" class="block text-xs text-slate-500 mb-1.5">
              <span class="flex items-center gap-1"><Key size={10} /> New Password (leave blank to keep current)</span>
            </label>
            <input id="ude-pass" type="password" bind:value={form.password} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" placeholder="••••••••" />
          </div>
        </div>

        <div class="flex gap-3 mt-6 pt-4 border-t border-white/[0.05]">
          <button onclick={save} disabled={busy}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
            {#if busy}
              <Loader2 size={14} class="animate-spin" /> Saving...
            {:else}
              <Check size={14} /> Save Changes
            {/if}
          </button>
          <a href="/users" class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition inline-flex items-center">Cancel</a>
        </div>
      </div>

      <!-- ═══════════ SECTION: Impersonate ═══════════ -->
      {#if hasRole(auth.user?.role, 'admin') && auth.user?.id !== userId}
        <div class="bg-[#040a14] border border-white/[0.07] backdrop-blur-sm rounded-xl p-6">
          <div class="flex items-center gap-2 mb-5 pb-4 border-b border-white/[0.05]">
            <Eye size={16} class="text-indigo-400" />
            <h2 class="text-sm font-semibold text-slate-200">Impersonate</h2>
          </div>
          <p class="text-sm text-slate-400 mb-4">
            Log in as this user to view the panel from their perspective. Your original session is preserved.
          </p>
          <button
            onclick={() => showImpersonateDialog = true}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-indigo-500/15 text-indigo-400 hover:bg-indigo-500/25 ring-1 ring-indigo-400/20 transition cursor-pointer border-0"
          >
            <LogIn size={14} /> Impersonate User
          </button>
        </div>
      {/if}

      <!-- ═══════════ SECTION: Assigned Sites ═══════════ -->
      <div class="bg-[#040a14] border border-white/[0.07] backdrop-blur-sm rounded-xl p-6">
        <div class="flex items-center justify-between mb-5 pb-4 border-b border-white/[0.05]">
          <div class="flex items-center gap-2">
            <Globe size={16} class="text-sky-400" />
            <h2 class="text-sm font-semibold text-slate-200">Assigned Sites</h2>
            {#if assignedSites.length > 0}
              <span class="text-[0.65rem] px-1.5 py-0.5 rounded-lg bg-white/[0.06] text-slate-500 font-medium">{assignedSites.length}</span>
            {/if}
          </div>
          <button
            onclick={fetchAssignedSites}
            disabled={sitesLoading}
            class="text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1"
            title="Refresh"
          >
            <RefreshCw size={14} class={sitesLoading ? 'animate-spin' : ''} />
          </button>
        </div>

        {#if sitesLoading}
          <div class="space-y-3">
            {#each Array(2) as _}
              <div class="h-14 bg-white/[0.03] rounded-xl animate-pulse"></div>
            {/each}
          </div>
        {:else if assignedSites.length === 0}
          <div class="text-center py-10">
            <div class="w-12 h-12 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-3">
              <Globe size={20} class="text-slate-600" />
            </div>
            <p class="text-sm text-slate-500">No sites assigned to this user</p>
          </div>
        {:else}
          <div class="space-y-2 mb-5">
            {#each assignedSites as site}
              <div class="flex items-center gap-3 bg-white/[0.03] border border-white/[0.05] rounded-xl px-4 py-3 group">
                <Globe size={16} class="text-sky-400 shrink-0" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm text-slate-200 font-medium truncate">{site.site_domain || site.site_name}</div>
                  <div class="text-[0.65rem] text-slate-500 mt-0.5">
                    {#if site.is_owner}
                      Created by this user
                    {:else}
                      Assigned {formatDate(site.assigned_at)}
                    {/if}
                  </div>
                </div>
                {#if site.is_owner}
                  <span class="inline-flex items-center px-2 py-0.5 rounded-lg text-[0.65rem] font-medium {permissionBadge(site.permission)} shrink-0">
                    {site.permission}
                  </span>
                {:else}
                  <select
                    value={site.permission}
                    onchange={(e) => updatePermission(site.site_id, (e.target as HTMLSelectElement).value)}
                    disabled={updatingPermSiteId === site.site_id}
                    class="text-[0.65rem] font-medium px-1.5 py-0.5 rounded-lg border border-white/[0.1] bg-white/[0.04] text-slate-300 outline-none cursor-pointer hover:border-sky-400/30 transition shrink-0"
                  >
                    <option value="read">read</option>
                    <option value="write">write</option>
                    <option value="admin">admin</option>
                  </select>
                {/if}
                {#if site.permission !== 'owner'}
                  <button
                    onclick={() => removeSite(site.site_id)}
                    disabled={removingSiteId === site.site_id}
                    class="text-slate-600 hover:text-red-400 transition cursor-pointer border-0 bg-transparent p-1 opacity-0 group-hover:opacity-100 shrink-0"
                    title="Remove site"
                  >
                    {#if removingSiteId === site.site_id}
                      <Loader2 size={14} class="animate-spin text-red-400" />
                    {:else}
                      <Trash2 size={14} />
                    {/if}
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <!-- Assign new site -->
        {#if availableSites.length > 0}
          <div class="pt-4 border-t border-white/[0.05]">
            <label class="block text-xs text-slate-500 mb-1.5">Assign New Site</label>
            <div class="flex flex-col sm:flex-row gap-2">
              <select
                bind:value={assignSiteId}
                class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-xl px-3 py-2 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition"
              >
                <option value={null} disabled>Select a site...</option>
                {#each availableSites as site}
                  <option value={site.id}>{site.domain || site.name}</option>
                {/each}
              </select>
              <select
                bind:value={assignPermission}
                class="w-full sm:w-32 bg-white/[0.04] border border-white/[0.07] rounded-xl px-3 py-2 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition"
              >
                <option value="read">Read</option>
                <option value="write">Write</option>
                <option value="admin">Admin</option>
              </select>
              <button
                onclick={assignSite}
                disabled={!assignSiteId || assigningSite}
                class="inline-flex items-center justify-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0 shrink-0"
              >
                {#if assigningSite}
                  <Loader2 size={14} class="animate-spin" />
                {:else}
                  <Check size={14} /> Assign
                {/if}
              </button>
            </div>
          </div>
        {/if}
      </div>

      <!-- ═══════════ SECTION: Usage / Quota ═══════════ -->
      <div class="bg-[#040a14] border border-white/[0.07] backdrop-blur-sm rounded-xl p-6">
        <div class="flex items-center justify-between mb-5 pb-4 border-b border-white/[0.05]">
          <div class="flex items-center gap-2">
            <Shield size={16} class="text-sky-400" />
            <h2 class="text-sm font-semibold text-slate-200">Usage &amp; Quota</h2>
          </div>
          <button
            onclick={fetchUsage}
            disabled={usageLoading}
            class="text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1"
            title="Refresh"
          >
            <RefreshCw size={14} class={usageLoading ? 'animate-spin' : ''} />
          </button>
        </div>

        {#if usageLoading}
          <div class="grid grid-cols-2 gap-3">
            {#each Array(4) as _}
              <div class="h-16 bg-white/[0.03] rounded-xl animate-pulse"></div>
            {/each}
          </div>
        {:else if usage}
          <div class="grid grid-cols-2 gap-3">
            <!-- Disk usage -->
            <div class="bg-white/[0.03] border border-white/[0.05] rounded-xl p-4">
              <div class="text-[0.65rem] text-slate-500 mb-1">Disk Usage</div>
              <div class="text-lg font-semibold text-slate-200">{formatBytes(usage.disk_usage || 0)}</div>
              {#if usage.disk_limit}
                <div class="text-[0.65rem] text-slate-500 mt-0.5">
                  of {formatBytes(usage.disk_limit)} limit
                </div>
                <div class="mt-2 h-1.5 bg-white/[0.06] rounded-full overflow-hidden">
                  <div
                    class="h-full bg-sky-400 rounded-full transition-all"
                    style="width: {Math.min(100, ((usage.disk_usage || 0) / usage.disk_limit) * 100)}%"
                  ></div>
                </div>
              {/if}
            </div>

            <!-- Site count -->
            <div class="bg-white/[0.03] border border-white/[0.05] rounded-xl p-4">
              <div class="text-[0.65rem] text-slate-500 mb-1">Sites</div>
              <div class="text-lg font-semibold text-slate-200">{assignedSites.length}</div>
              {#if usage.max_sites}
                <div class="text-[0.65rem] text-slate-500 mt-0.5">
                  of {usage.max_sites} max
                </div>
                <div class="mt-2 h-1.5 bg-white/[0.06] rounded-full overflow-hidden">
                  <div
                    class="h-full bg-indigo-400 rounded-full transition-all"
                    style="width: {Math.min(100, (assignedSites.length / usage.max_sites) * 100)}%"
                  ></div>
                </div>
              {/if}
            </div>

            <!-- Bandwidth -->
            {#if usage.bandwidth_usage !== undefined}
              <div class="bg-white/[0.03] border border-white/[0.05] rounded-xl p-4">
                <div class="text-[0.65rem] text-slate-500 mb-1">Bandwidth</div>
                <div class="text-lg font-semibold text-slate-200">{formatBytes(usage.bandwidth_usage || 0)}</div>
                {#if usage.bandwidth_limit}
                  <div class="text-[0.65rem] text-slate-500 mt-0.5">
                    of {formatBytes(usage.bandwidth_limit)} limit
                  </div>
                  <div class="mt-2 h-1.5 bg-white/[0.06] rounded-full overflow-hidden">
                    <div
                      class="h-full bg-sky-400 rounded-full transition-all"
                      style="width: {Math.min(100, ((usage.bandwidth_usage || 0) / usage.bandwidth_limit) * 100)}%"
                    ></div>
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Created at -->
            <div class="bg-white/[0.03] border border-white/[0.05] rounded-xl p-4">
              <div class="text-[0.65rem] text-slate-500 mb-1">Created</div>
              <div class="text-sm font-medium text-slate-200">{formatDate(user.created_at || usage.created_at)}</div>
            </div>
          </div>
        {:else}
          <div class="text-center py-8">
            <p class="text-sm text-slate-500">No usage data available</p>
          </div>
        {/if}
      </div>
    </div>

  {:else}
    <div class="bg-[#040a14] border border-white/[0.07] rounded-xl py-16 px-6 text-center">
      <div class="w-16 h-16 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-5">
        <User size={28} class="text-slate-600" />
      </div>
      <h2 class="text-lg font-semibold text-slate-300 mb-2">User not found</h2>
      <p class="text-sm text-slate-500 mb-5">This user may have been deleted or you don't have permission to view it.</p>
      <a href="/users" class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-white/[0.05] text-slate-300 hover:bg-white/[0.08] transition">
        <ArrowLeft size={14} /> Back to Users
      </a>
    </div>
  {/if}

  <!-- ═══════════ Impersonate Confirmation Dialog ═══════════ -->
  {#if showImpersonateDialog}
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
      <!-- Backdrop -->
      <div
        class="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onclick={() => showImpersonateDialog = false}
        role="button"
        tabindex="-1"
        onkeydown={(e) => e.key === 'Escape' && (showImpersonateDialog = false)}
      ></div>

      <!-- Dialog -->
      <div class="relative bg-[#0a1120] border border-white/[0.1] rounded-2xl p-6 max-w-sm w-full shadow-2xl">
        <button
          onclick={() => showImpersonateDialog = false}
          class="absolute top-4 right-4 text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent"
        >
          <X size={16} />
        </button>

        <div class="w-12 h-12 rounded-xl bg-indigo-500/15 flex items-center justify-center mb-4">
          <AlertTriangle size={22} class="text-indigo-400" />
        </div>

        <h3 class="text-base font-semibold text-slate-200 mb-2">Impersonate User</h3>
        <p class="text-sm text-slate-400 mb-6">
          You will log in as <span class="text-slate-200 font-medium">{user?.username}</span>. Your current session will be preserved and you can stop impersonation at any time.
        </p>

        <div class="flex gap-3 justify-end">
          <button
            onclick={() => showImpersonateDialog = false}
            class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0 bg-transparent"
          >
            Cancel
          </button>
          <button
            onclick={doImpersonate}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-indigo-500 text-white hover:bg-indigo-400 transition cursor-pointer border-0"
          >
            <LogIn size={14} /> Confirm Impersonation
          </button>
        </div>
      </div>
    </div>
  {/if}
</DashboardLayout>
