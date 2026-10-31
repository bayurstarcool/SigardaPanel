<script lang="ts">
  import { page } from '$app/stores';
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { ArrowLeft, Loader2, Check, Shield, Key, User, Globe, Trash2, Eye, RefreshCw, LogIn, AlertTriangle, Settings, Gauge } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
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

  // Confirm before removing a site assignment
  let confirmRemove = $state<any>(null);
  let confirmRemoveOpen = $state(false);
  function askRemove(site: any) { confirmRemove = site; confirmRemoveOpen = true; }

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
      goto('/sites');
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
      case 'super_admin': return 'badge-warning';
      case 'admin': return 'badge-accent';
      default: return '';
    }
  }

  function permissionBadge(perm: string) {
    switch (perm) {
      case 'owner': return 'badge-success';
      case 'admin': return 'badge-warning';
      case 'write': return 'badge-info';
      default: return '';
    }
  }

  function pct(used: number, limit: number) {
    return Math.min(100, ((used || 0) / limit) * 100);
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
  {#if impersonating}
    <div class="mb-6 flex flex-col items-start gap-3 rounded-xl bg-warning/10 p-4 ring-1 ring-warning/20 ring-inset sm:flex-row sm:items-center">
      <div class="flex items-center gap-2 text-warning">
        <AlertTriangle size={16} />
        <span class="text-sm font-medium">You are impersonating a user</span>
      </div>
      <button onclick={stopImpersonation} class="btn btn-secondary btn-sm sm:ml-auto">
        <Eye size={14} /> Stop impersonation
      </button>
    </div>
  {/if}

  <PageHeader title={user ? `Edit user — ${form.username}` : 'Edit user'} back="/users" backLabel="Users">
    {#snippet meta()}
      {#if user}
        <span class="badge {roleBadge(user.role)}">
          {#if user.role === 'super_admin'}<Shield size={11} />{/if}
          {user.role === 'super_admin' ? 'Super Admin' : user.role === 'admin' ? 'Admin' : 'User'}
        </span>
      {/if}
    {/snippet}
    {#snippet actions()}
      {#if user && hasRole(auth.user?.role, 'admin') && auth.user?.id !== userId}
        <button onclick={() => (showImpersonateDialog = true)} class="btn btn-secondary btn-sm">
          <LogIn size={14} /> Impersonate
        </button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="max-w-3xl space-y-6">
      <div class="card card-body space-y-5">
        {#each Array(4) as _, i (i)}
          <div>
            <div class="skeleton mb-2 h-3 w-20"></div>
            <div class="skeleton h-9 w-full"></div>
          </div>
        {/each}
      </div>
      <div class="card card-body space-y-3">
        <div class="skeleton h-4 w-32"></div>
        {#each Array(2) as _, i (i)}<div class="skeleton h-12 w-full"></div>{/each}
      </div>
    </div>
  {:else if user}
    <div class="max-w-3xl space-y-6">
      <!-- Edit profile -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Settings size={16} class="text-subtle" /> Profile</h2>
        </div>
        <div class="card-body">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label for="ude-name" class="label">Username</label>
              <input id="ude-name" bind:value={form.username} class="input" autocomplete="off" />
            </div>
            <div>
              <label for="ude-email" class="label">Email</label>
              <input id="ude-email" type="email" bind:value={form.email} class="input" autocomplete="off" />
            </div>
            <div>
              <label for="ude-role" class="label">Role</label>
              <select id="ude-role" bind:value={form.role} class="input">
                <option value="user">User</option>
                <option value="admin">Admin</option>
                {#if hasRole(auth.user?.role, 'super_admin')}
                  <option value="super_admin">Super Admin</option>
                {/if}
              </select>
            </div>
            <div>
              <label for="ude-pass" class="label flex items-center gap-1"><Key size={12} /> Reset password</label>
              <input id="ude-pass" type="password" bind:value={form.password} class="input" placeholder="••••••••" autocomplete="new-password" />
              <p class="hint">Leave blank to keep the current password.</p>
            </div>
          </div>
        </div>
        <div class="flex justify-end gap-2 border-t border-line px-5 py-3.5">
          <a href="/users" class="btn btn-ghost btn-sm">Cancel</a>
          <button onclick={save} disabled={busy} class="btn btn-primary btn-sm">
            {#if busy}<Loader2 size={14} class="animate-spin" /> Saving...{:else}<Check size={14} /> Save changes{/if}
          </button>
        </div>
      </section>

      <!-- Assigned sites -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2">
            <Globe size={16} class="text-subtle" /> Assigned sites
            {#if assignedSites.length > 0}<span class="badge tabular-nums">{assignedSites.length}</span>{/if}
          </h2>
          <button onclick={fetchAssignedSites} disabled={sitesLoading} class="btn btn-ghost btn-xs btn-icon" aria-label="Refresh assigned sites" title="Refresh">
            <RefreshCw size={14} class={sitesLoading ? 'animate-spin' : ''} />
          </button>
        </div>

        {#if sitesLoading}
          <div class="space-y-2 p-4">
            {#each Array(2) as _, i (i)}<div class="skeleton h-10"></div>{/each}
          </div>
        {:else if assignedSites.length === 0}
          <EmptyState compact icon={Globe as any} title="No sites assigned" description="Assign a site below to give this user access." />
        {:else}
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>Site</th>
                  <th class="max-sm:hidden">Access</th>
                  <th>Permission</th>
                  <th class="text-right"><span class="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {#each assignedSites as site (site.site_id)}
                  <tr>
                    <td>
                      <a href="/sites/{site.site_id}" class="font-medium text-fg hover:text-accent">{site.site_domain || site.site_name}</a>
                    </td>
                    <td class="text-xs text-subtle max-sm:hidden">
                      {site.is_owner ? 'Created by this user' : `Assigned ${formatDate(site.assigned_at)}`}
                    </td>
                    <td>
                      {#if site.is_owner}
                        <span class="badge {permissionBadge(site.permission)}">{site.permission}</span>
                      {:else}
                        <select
                          value={site.permission}
                          onchange={(e) => updatePermission(site.site_id, (e.target as HTMLSelectElement).value)}
                          disabled={updatingPermSiteId === site.site_id}
                          aria-label="Permission for {site.site_domain || site.site_name}"
                          class="input h-7 w-24 text-xs"
                        >
                          <option value="read">read</option>
                          <option value="write">write</option>
                          <option value="admin">admin</option>
                        </select>
                      {/if}
                    </td>
                    <td class="text-right">
                      {#if site.permission !== 'owner'}
                        <button
                          onclick={() => askRemove(site)}
                          disabled={removingSiteId === site.site_id}
                          class="btn btn-danger-ghost btn-xs btn-icon"
                          aria-label="Remove {site.site_domain || site.site_name}"
                          title="Remove site"
                        >
                          {#if removingSiteId === site.site_id}<Loader2 size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                        </button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        {#if availableSites.length > 0}
          <div class="border-t border-line p-4">
            <label for="ude-assign-site" class="label">Assign new site</label>
            <div class="flex flex-col gap-2 sm:flex-row">
              <select id="ude-assign-site" bind:value={assignSiteId} class="input flex-1">
                <option value={null} disabled>Select a site...</option>
                {#each availableSites as site (site.id)}
                  <option value={site.id}>{site.domain || site.name}</option>
                {/each}
              </select>
              <select bind:value={assignPermission} aria-label="Permission" class="input sm:w-32">
                <option value="read">Read</option>
                <option value="write">Write</option>
                <option value="admin">Admin</option>
              </select>
              <button onclick={assignSite} disabled={!assignSiteId || assigningSite} class="btn btn-primary">
                {#if assigningSite}<Loader2 size={14} class="animate-spin" />{:else}<Check size={14} /> Assign{/if}
              </button>
            </div>
          </div>
        {/if}
      </section>

      <!-- Usage & quota -->
      <section class="card">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Gauge size={16} class="text-subtle" /> Usage &amp; quota</h2>
          <button onclick={fetchUsage} disabled={usageLoading} class="btn btn-ghost btn-xs btn-icon" aria-label="Refresh usage" title="Refresh">
            <RefreshCw size={14} class={usageLoading ? 'animate-spin' : ''} />
          </button>
        </div>
        <div class="card-body">
          {#if usageLoading}
            <div class="grid grid-cols-2 gap-3">
              {#each Array(4) as _, i (i)}<div class="skeleton h-20"></div>{/each}
            </div>
          {:else if usage}
            <div class="grid grid-cols-2 gap-3">
              <div class="rounded-lg border border-line p-4">
                <div class="stat-label">Disk usage</div>
                <div class="mt-1 text-lg font-semibold text-fg tabular-nums">{formatBytes(usage.disk_usage || 0)}</div>
                {#if usage.disk_limit}
                  <div class="text-xs text-subtle">of {formatBytes(usage.disk_limit)} limit</div>
                  <div class="progress mt-2"><div class="progress-bar" style="width: {pct(usage.disk_usage, usage.disk_limit)}%"></div></div>
                {/if}
              </div>

              <div class="rounded-lg border border-line p-4">
                <div class="stat-label">Sites</div>
                <div class="mt-1 text-lg font-semibold text-fg tabular-nums">{assignedSites.length}</div>
                {#if usage.max_sites}
                  <div class="text-xs text-subtle">of {usage.max_sites} max</div>
                  <div class="progress mt-2"><div class="progress-bar" style="width: {pct(assignedSites.length, usage.max_sites)}%"></div></div>
                {/if}
              </div>

              {#if usage.bandwidth_usage !== undefined}
                <div class="rounded-lg border border-line p-4">
                  <div class="stat-label">Bandwidth</div>
                  <div class="mt-1 text-lg font-semibold text-fg tabular-nums">{formatBytes(usage.bandwidth_usage || 0)}</div>
                  {#if usage.bandwidth_limit}
                    <div class="text-xs text-subtle">of {formatBytes(usage.bandwidth_limit)} limit</div>
                    <div class="progress mt-2"><div class="progress-bar" style="width: {pct(usage.bandwidth_usage, usage.bandwidth_limit)}%"></div></div>
                  {/if}
                </div>
              {/if}

              <div class="rounded-lg border border-line p-4">
                <div class="stat-label">Created</div>
                <div class="mt-1 text-sm font-medium text-fg">{formatDate(user.created_at || usage.created_at)}</div>
              </div>
            </div>
          {:else}
            <p class="py-6 text-center text-[13px] text-muted">No usage data available</p>
          {/if}
        </div>
      </section>
    </div>
  {:else}
    <div class="card">
      <EmptyState icon={User as any} title="User not found" description="This user may have been deleted or you don't have permission to view it.">
        <a href="/users" class="btn btn-secondary btn-sm"><ArrowLeft size={14} /> Back to users</a>
      </EmptyState>
    </div>
  {/if}

  <ConfirmDialog
    bind:open={showImpersonateDialog}
    title="Impersonate user"
    message={`You will log in as ${user?.username ?? 'this user'}. Your current session will be preserved and you can stop impersonation at any time.`}
    confirmLabel="Confirm impersonation"
    onconfirm={doImpersonate}
  />

  <ConfirmDialog
    bind:open={confirmRemoveOpen}
    title="Remove site access"
    message={confirmRemove ? `Remove ${confirmRemove.site_domain || confirmRemove.site_name} from ${user?.username ?? 'this user'}? They will lose access to it.` : ''}
    confirmLabel="Remove"
    destructive
    onconfirm={() => confirmRemove && removeSite(confirmRemove.site_id)}
  />
</DashboardLayout>
