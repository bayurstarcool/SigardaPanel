<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Users, Search, Trash2, RefreshCw, X, Plus, Loader2, Check, Shield, Key, LogIn, Settings } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { toasts } from '$lib/toast.svelte';

  let users = $state<any[]>([]);
  let loading = $state(true);
  let showCreate = $state(false);
  let form = $state({ username: '', email: '', password: '', role: 'user' });
  let busy = $state(false);

  // Search
  let search = $state('');

  // Reset password state
  let resetTarget = $state<string | null>(null);
  let resetPassword = $state('');
  let resetBusy = $state(false);
  let resetResult = $state('');

  // Impersonate state
  let impersonateTarget = $state<string | null>(null);
  let impersonateBusy = $state(false);

  const filtered = $derived.by(() => {
    const q = search.toLowerCase().trim();
    if (!q) return users;
    return users.filter(u =>
      (u.username || '').toLowerCase().includes(q) || (u.email || '').toLowerCase().includes(q)
    );
  });


  onMount(async () => {
    if (!auth.user) { loading = false; return; }
    try {
      const d = await apiFetch(auth.token, 'GET', 'users');
      users = d?.users || [];
    } catch (e: any) { toasts.error(e.message); }
    finally { loading = false; }
  });

  async function createUser() {
    if (!form.username || !form.email || !form.password) return;
    busy = true;
    try {
      await apiFetch(auth.token, 'POST', 'users', form);
      showCreate = false;
      form = { username: '', email: '', password: '', role: 'user' };
      const d = await apiFetch(auth.token, 'GET', 'users');
      users = d?.users || [];
      toasts.success('User created successfully');
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = false; }
  }

  // Confirm dialogs (replace window.confirm)
  let confirmDelete = $state<any>(null);
  let confirmDeleteOpen = $state(false);
  let confirmImp = $state<any>(null);
  let confirmImpOpen = $state(false);

  function askDelete(u: any) { confirmDelete = u; confirmDeleteOpen = true; }
  function askImpersonate(u: any) { confirmImp = u; confirmImpOpen = true; }

  // Reset-password modal
  let resetOpen = $state(false);
  const resetUser = $derived(users.find((x) => x.id === resetTarget) ?? null);
  function openReset(u: any) { resetTarget = u.id; resetPassword = ''; resetResult = ''; resetOpen = true; }
  function closeReset() { resetTarget = null; resetPassword = ''; resetResult = ''; }

  async function deleteUser(u: any) {
    try {
      await apiFetch(auth.token, 'DELETE', `users/${u.id}`);
      users = users.filter(x => x.id !== u.id);
      toasts.success(`User "${u.username}" deleted`);
    } catch (e: any) { toasts.error(e.message); }
  }

  async function resetUserPassword(u: any) {
    if (!resetPassword) return;
    resetBusy = true;
    resetResult = '';
    try {
      await apiFetch(auth.token, 'PATCH', `users/${u.id}`, { password: resetPassword });
      resetResult = `Password reset for "${u.username}"`;
      resetPassword = '';
      toasts.success(`Password reset for "${u.username}"`);
    } catch (e: any) { toasts.error(e.message); }
    finally { resetBusy = false; }
  }

  async function impersonateUser(u: any) {
    impersonateTarget = u.id;
    impersonateBusy = true;
    try {
      await auth.impersonateUser(u.id);
      goto('/sites');
    } catch (e: any) {
      toasts.error(e.message || 'Impersonation failed');
      impersonateBusy = false;
      impersonateTarget = null;
    }
  }

  function roleBadge(role: string) {
    switch (role) {
      case 'super_admin': return 'badge-warning';
      case 'admin': return 'badge-accent';
      default: return '';
    }
  }

  function roleLabel(role: string) {
    return role === 'super_admin' ? 'Super Admin' : role === 'admin' ? 'Admin' : 'User';
  }

  function fmtDate(d: string | null | undefined) {
    return d ? new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—';
  }

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));
</script>

<DashboardLayout active="Users">
  <PageHeader title="Users">
    {#snippet meta()}
      {#if !loading}<span class="badge tabular-nums">{users.length}</span>{/if}
    {/snippet}
    {#snippet actions()}
      {#if isAdmin}
        <button onclick={() => (showCreate = true)} class="btn btn-primary btn-sm"><Plus size={14} /> Create user</button>
      {/if}
    {/snippet}
  </PageHeader>

  <div class="card">
    {#if !loading && users.length > 0}
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <p class="text-[13px] text-muted">
          {users.length} user{users.length !== 1 ? 's' : ''} total
          {#if search && filtered.length !== users.length}
            <span class="text-subtle">· {filtered.length} match{filtered.length !== 1 ? 'es' : ''}</span>
          {/if}
        </p>
        <div class="relative w-full sm:w-72">
          <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
          <input bind:value={search} type="text" aria-label="Search users" placeholder="Search by username or email…" class="input h-8 pr-8 pl-8" />
          {#if search}
            <button onclick={() => (search = '')} class="btn btn-ghost btn-xs btn-icon absolute top-1/2 right-0.5 -translate-y-1/2" aria-label="Clear search"><X size={12} /></button>
          {/if}
        </div>
      </div>
    {/if}

    {#if loading}
      <div class="divide-y divide-line">
        {#each Array(5) as _, i (i)}
          <div class="flex items-center gap-3 px-4 py-3">
            <div class="skeleton size-8 rounded-full"></div>
            <div class="flex-1 space-y-1.5">
              <div class="skeleton h-3 w-32"></div>
              <div class="skeleton h-2.5 w-48"></div>
            </div>
            <div class="skeleton h-5 w-16"></div>
          </div>
        {/each}
      </div>
    {:else if users.length === 0}
      <EmptyState
        icon={Users as any}
        title="No users yet"
        description={isAdmin ? 'Create your first user to get started.' : 'No users have been created yet. Ask an administrator to create one.'}
        actionLabel={isAdmin ? 'Create user' : ''}
        onaction={() => (showCreate = true)}
      />
    {:else if filtered.length === 0 && search}
      <EmptyState compact icon={Search as any} title="No results" description={`No users match "${search}"`} actionLabel="Clear search" onaction={() => (search = '')} />
    {:else}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>User</th>
              <th class="max-md:hidden">Email</th>
              <th>Role</th>
              <th class="max-sm:hidden">Created</th>
              <th class="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each filtered as u (u.id)}
              {@const self = u.id === auth.user?.id}
              <tr>
                <td>
                  <a href="/users/{u.id}" class="flex items-center gap-2.5">
                    <span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-surface-2 text-xs font-semibold text-muted">
                      {(u.username || '?')[0].toUpperCase()}
                    </span>
                    <span class="min-w-0">
                      <span class="flex items-center gap-1.5 font-medium text-fg hover:text-accent">
                        {u.username}
                        {#if self}<span class="badge badge-accent px-1.5 py-0 text-[11px]">you</span>{/if}
                      </span>
                      <span class="block truncate text-xs text-subtle md:hidden">{u.email || 'No email'}</span>
                    </span>
                  </a>
                </td>
                <td class="text-muted max-md:hidden">{u.email || '—'}</td>
                <td>
                  <span class="badge {roleBadge(u.role)}">
                    {#if u.role === 'super_admin'}<Shield size={11} />{/if}
                    {roleLabel(u.role)}
                  </span>
                </td>
                <td class="text-xs whitespace-nowrap text-subtle tabular-nums max-sm:hidden">{fmtDate(u.created_at)}</td>
                <td>
                  <div class="flex items-center justify-end gap-0.5">
                    <a href="/users/{u.id}" class="btn btn-ghost btn-xs" title="Manage user">
                      <Settings size={12} /><span class="max-lg:hidden">Manage</span>
                    </a>
                    {#if isAdmin && !self}
                      <button onclick={() => askImpersonate(u)} disabled={impersonateBusy} class="btn btn-ghost btn-xs" title="Impersonate user" aria-label="Impersonate {u.username}">
                        {#if impersonateTarget === u.id && impersonateBusy}
                          <Loader2 size={12} class="animate-spin" />
                        {:else}
                          <LogIn size={12} />
                        {/if}
                        <span class="max-lg:hidden">Impersonate</span>
                      </button>
                      <button onclick={() => openReset(u)} class="btn btn-ghost btn-xs" title="Reset password" aria-label="Reset password for {u.username}">
                        <Key size={12} /><span class="max-lg:hidden">Reset</span>
                      </button>
                      <button onclick={() => askDelete(u)} class="btn btn-danger-ghost btn-xs" title="Delete user" aria-label="Delete {u.username}">
                        <Trash2 size={12} /><span class="max-lg:hidden">Delete</span>
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <!-- Create user -->
  <Modal bind:open={showCreate} title="Create user" maxWidth="lg">
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <div>
        <label for="usr-name" class="label">Username <span class="text-danger">*</span></label>
        <input id="usr-name" bind:value={form.username} class="input" autocomplete="off" />
      </div>
      <div>
        <label for="usr-email" class="label">Email <span class="text-danger">*</span></label>
        <input id="usr-email" type="email" required bind:value={form.email} placeholder="user@example.com" class="input" />
      </div>
      <div>
        <label for="usr-pass" class="label">Password <span class="text-danger">*</span></label>
        <input id="usr-pass" type="password" bind:value={form.password} class="input" autocomplete="new-password" />
      </div>
      <div>
        <label for="usr-role" class="label">Role</label>
        <select id="usr-role" bind:value={form.role} class="input">
          <option value="user">User</option>
          <option value="admin">Admin</option>
          {#if hasRole(auth.user?.role, 'super_admin')}
            <option value="super_admin">Super Admin</option>
          {/if}
        </select>
      </div>
    </div>
    {#snippet footer()}
      <button onclick={() => (showCreate = false)} class="btn btn-secondary btn-sm">Cancel</button>
      <button onclick={createUser} disabled={busy || !form.username || !form.email || !form.password} class="btn btn-primary btn-sm">
        {#if busy}<Loader2 size={14} class="animate-spin" /> Creating...{:else}<Check size={14} /> Create{/if}
      </button>
    {/snippet}
  </Modal>

  <!-- Reset password -->
  <Modal bind:open={resetOpen} title="Reset password" maxWidth="sm" onclose={closeReset}>
    {#if resetResult}
      <div class="flex items-center gap-2 rounded-lg bg-success/10 px-3 py-2.5 text-[13px] text-success ring-1 ring-success/20 ring-inset">
        <Check size={14} /> {resetResult}
      </div>
    {:else if resetUser}
      <label for="usr-reset" class="label">New password for <span class="font-semibold text-fg">{resetUser.username}</span></label>
      <input
        id="usr-reset"
        bind:value={resetPassword}
        type="password"
        placeholder="New password"
        autocomplete="new-password"
        class="input"
        onkeydown={(e) => { if (e.key === 'Enter') resetUserPassword(resetUser); }}
      />
    {/if}
    {#snippet footer()}
      {#if resetResult}
        <button onclick={() => { resetOpen = false; closeReset(); }} class="btn btn-secondary btn-sm">Done</button>
      {:else}
        <button onclick={() => { resetOpen = false; closeReset(); }} class="btn btn-secondary btn-sm">Cancel</button>
        <button onclick={() => resetUser && resetUserPassword(resetUser)} disabled={resetBusy || !resetPassword} class="btn btn-primary btn-sm">
          {#if resetBusy}<Loader2 size={14} class="animate-spin" />{:else}<RefreshCw size={14} />{/if} Reset
        </button>
      {/if}
    {/snippet}
  </Modal>

  <ConfirmDialog
    bind:open={confirmDeleteOpen}
    title="Delete user"
    message={confirmDelete ? `Delete user "${confirmDelete.username}"? This cannot be undone.` : ''}
    confirmLabel="Delete"
    destructive
    onconfirm={() => confirmDelete && deleteUser(confirmDelete)}
  />

  <ConfirmDialog
    bind:open={confirmImpOpen}
    title="Impersonate user"
    message={confirmImp ? `Login as ${confirmImp.username}?` : ''}
    confirmLabel="Login as user"
    onconfirm={() => confirmImp && impersonateUser(confirmImp)}
  />
</DashboardLayout>
