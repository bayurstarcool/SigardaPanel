<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Users, Search, Trash2, RefreshCw, X, Plus, Loader2, Check, Shield, Key, LogIn, Settings, Eye } from 'lucide-svelte';
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

  async function deleteUser(u: any) {
    if (!confirm(`Delete user "${u.username}"? This cannot be undone.`)) return;
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
    if (!confirm(`Login as ${u.username}?`)) return;
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
      case 'super_admin': return 'bg-amber-500/15 text-amber-400 ring-1 ring-amber-400/20';
      case 'admin': return 'bg-sky-500/15 text-sky-400 ring-1 ring-sky-400/20';
      default: return 'bg-white/5 text-slate-400 ring-1 ring-white/10';
    }
  }

  function roleIcon(role: string) {
    if (role === 'super_admin') return Shield;
    return null;
  }
</script>

<DashboardLayout active="Users">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Users</h1>
      <p class="text-sm text-slate-500 mt-1">
        {#if loading}
          Loading users...
        {:else}
          {users.length} user{users.length !== 1 ? 's' : ''} total
          {#if search && filtered.length !== users.length}
            <span class="text-sky-400/60">· {filtered.length} match{filtered.length !== 1 ? 'es' : ''}</span>
          {/if}
        {/if}
      </p>
    </div>
    {#if hasRole(auth.user?.role, 'admin')}
      <button onclick={() => showCreate = !showCreate}
        class="inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer border-0 whitespace-nowrap">
        <Plus size={15} /> Create User
      </button>
    {/if}
  </div>

  <!-- Search bar -->
  {#if !loading && users.length > 0}
    <div class="relative mb-5">
      <Search size={16} class="absolute left-4 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
      <input
        bind:value={search}
        type="text"
        placeholder="Search by username or email..."
        class="w-full bg-white/[0.03] border border-white/[0.07] rounded-xl pl-10 pr-10 py-2.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-sky-400/30 focus:ring-1 focus:ring-sky-400/10 transition"
      />
      {#if search}
        <button onclick={() => search = ''} class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1">
          <X size={14} />
        </button>
      {/if}
    </div>
  {/if}

  <!-- Create form -->
  {#if showCreate}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] backdrop-blur-sm rounded-xl p-6">
      <div class="flex items-center gap-2 mb-4">
        <Plus size={16} class="text-sky-400" />
        <h3 class="text-sm font-semibold text-slate-200">Create User</h3>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label for="usr-name" class="block text-xs text-slate-500 mb-1.5">Username *</label>
          <input id="usr-name" bind:value={form.username} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
        </div>
        <div>
          <label for="usr-email" class="block text-xs text-slate-500 mb-1.5">Email <span class="text-red-400">*</span></label>
          <input id="usr-email" type="email" required bind:value={form.email} placeholder="user@example.com" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
        </div>
        <div>
          <label for="usr-pass" class="block text-xs text-slate-500 mb-1.5">Password *</label>
          <input id="usr-pass" type="password" bind:value={form.password} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
        </div>
        <div>
          <label for="usr-role" class="block text-xs text-slate-500 mb-1.5">Role</label>
          <select id="usr-role" bind:value={form.role} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition">
            <option value="user">User</option>
            <option value="admin">Admin</option>
            {#if hasRole(auth.user?.role, 'super_admin')}
              <option value="super_admin">Super Admin</option>
            {/if}
          </select>
        </div>
      </div>
      <div class="flex gap-3 mt-5">
        <button onclick={createUser} disabled={busy || !form.username || !form.email || !form.password}
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
          {#if busy}
            <Loader2 size={14} class="animate-spin" /> Creating...
          {:else}
            <Check size={14} /> Create
          {/if}
        </button>
        <button onclick={() => showCreate = false} class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
      </div>
    </div>
  {/if}

  <!-- Loading skeleton -->
  {#if loading}
    <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
      <div class="p-5 space-y-0">
        {#each Array(5) as _}
          <div class="flex items-center gap-4 py-3.5 border-b border-white/[0.03] last:border-0">
            <div class="w-8 h-8 rounded-lg bg-white/[0.04] animate-pulse"></div>
            <div class="flex-1 space-y-2">
              <div class="h-3 w-32 bg-white/[0.06] rounded-lg animate-pulse"></div>
              <div class="h-2.5 w-48 bg-white/[0.04] rounded-lg animate-pulse"></div>
            </div>
            <div class="h-5 w-16 bg-white/[0.05] rounded-full animate-pulse"></div>
            <div class="h-5 w-16 bg-white/[0.04] rounded-lg animate-pulse hidden sm:block"></div>
          </div>
        {/each}
      </div>
    </div>

  <!-- Empty state -->
  {:else if users.length === 0}
    <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
      <div class="w-16 h-16 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-5">
        <Users size={28} class="text-slate-600" />
      </div>
      <h2 class="text-lg font-semibold text-slate-300 mb-2">No users yet</h2>
      <p class="text-sm text-slate-500 mb-6 max-w-sm mx-auto">
        {#if hasRole(auth.user?.role, 'admin')}
          Create your first user to get started.
        {:else}
          No users have been created yet. Ask an administrator to create one.
        {/if}
      </p>
      {#if hasRole(auth.user?.role, 'admin')}
        <button onclick={() => showCreate = true}
          class="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all duration-150 cursor-pointer border-0">
          <Plus size={15} /> Create User
        </button>
      {/if}
    </div>

  <!-- No search results -->
  {:else if filtered.length === 0 && search}
    <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
      <div class="w-16 h-16 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-5">
        <Search size={28} class="text-slate-600" />
      </div>
      <h2 class="text-lg font-semibold text-slate-300 mb-2">No results</h2>
      <p class="text-sm text-slate-500">No users match "<span class="text-sky-400">{search}</span>"</p>
      <button onclick={() => search = ''} class="mt-4 inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">
        <X size={14} /> Clear search
      </button>
    </div>

  {:else}
    <!-- Desktop table (hidden on small screens) -->
    <div class="hidden md:block bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead>
            <tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Username</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Email</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Role</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Status</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each filtered as u (u.id)}
              <tr class="hover:bg-white/[0.02] transition group">
                <td class="px-5 py-3.5 border-b border-white/[0.03] font-medium align-middle">
                  <div class="flex items-center gap-2.5">
                    <div class="w-8 h-8 rounded-lg bg-white/[0.05] flex items-center justify-center text-xs font-semibold text-slate-400">
                      {(u.username || '?')[0].toUpperCase()}
                    </div>
                    <span class="text-slate-200">{u.username}</span>
                    {#if u.id === auth.user?.id}
                      <span class="text-[0.65rem] px-1.5 py-0.5 rounded-md bg-sky-500/10 text-sky-400/70 font-medium">you</span>
                    {/if}
                  </div>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-400 align-middle">{u.email || '—'}</td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg text-[0.72rem] font-medium {roleBadge(u.role)}">
                    {#if roleIcon(u.role)}
                      <Shield size={10} />
                    {/if}
                    {u.role === 'super_admin' ? 'Super Admin' : u.role === 'admin' ? 'Admin' : 'User'}
                  </span>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <div class="text-xs text-slate-500">
                    {u.created_at ? new Date(u.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'}
                  </div>
                </td>
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <div class="flex items-center gap-1 flex-wrap">
                    <!-- Manage button -->
                    <a href="/users/{u.id}"
                      class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5"
                      title="Manage user">
                      <Settings size={12} /> Manage
                    </a>
                    <!-- Edit link -->
                    <a href="/users/{u.id}"
                      class="inline-flex items-center gap-1 text-xs text-sky-400/70 hover:text-sky-400 transition px-2 py-1 rounded-lg hover:bg-sky-400/5"
                      title="Edit user">
                      <Eye size={12} /> Edit
                    </a>
                    <!-- Impersonate (admin/super_admin only, not self) -->
                    {#if hasRole(auth.user?.role, 'admin') && u.id !== auth.user?.id}
                      {#if impersonateTarget === u.id && impersonateBusy}
                        <span class="inline-flex items-center gap-1 text-xs text-purple-400/70 px-2 py-1 rounded-lg">
                          <Loader2 size={12} class="animate-spin" /> Logging in...
                        </span>
                      {:else}
                        <button onclick={() => impersonateUser(u)}
                          class="inline-flex items-center gap-1 text-xs text-purple-400/60 hover:text-purple-400 transition px-2 py-1 rounded-lg hover:bg-purple-400/5 cursor-pointer border-0 bg-transparent"
                          title="Impersonate user">
                          <LogIn size={12} /> Impersonate
                        </button>
                      {/if}
                    {/if}
                    <!-- Reset password (admin only, not self) -->
                    {#if hasRole(auth.user?.role, 'admin') && u.id !== auth.user?.id}
                      <button onclick={() => { resetTarget = resetTarget === u.id ? null : u.id; resetPassword = ''; resetResult = ''; }}
                        class="inline-flex items-center gap-1 text-xs text-amber-400/70 hover:text-amber-400 transition px-2 py-1 rounded-lg hover:bg-amber-400/5 cursor-pointer border-0 bg-transparent"
                        title="Reset password">
                        <Key size={12} /> Reset
                      </button>
                      <button onclick={() => deleteUser(u)} class="inline-flex items-center gap-1 text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent">
                        <Trash2 size={12} /> Delete
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
              {#if resetTarget === u.id}
                <tr>
                  <td colspan="5" class="px-5 py-3 border-b border-white/[0.03]">
                    <div class="bg-amber-500/5 border border-amber-500/15 rounded-xl p-4 flex flex-col sm:flex-row items-start sm:items-center gap-3">
                      <div class="flex items-center gap-2 text-amber-400 shrink-0">
                        <Key size={14} />
                        <span class="text-xs font-medium">Reset password for <strong>{u.username}</strong></span>
                      </div>
                      {#if resetResult}
                        <div class="flex items-center gap-2 text-emerald-400 text-xs">
                          <Check size={13} /> {resetResult}
                        </div>
                      {:else}
                        <input
                          bind:value={resetPassword}
                          type="password"
                          placeholder="New password"
                          class="flex-1 min-w-0 bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-sm text-slate-200 outline-none focus:border-amber-400/40 transition"
                          onkeydown={(e) => { if (e.key === 'Enter') resetUserPassword(u); }}
                        />
                        <button onclick={() => resetUserPassword(u)} disabled={resetBusy || !resetPassword}
                          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-amber-500/20 text-amber-400 hover:bg-amber-500/30 transition disabled:opacity-40 cursor-pointer border-0 whitespace-nowrap">
                          {#if resetBusy}
                            <Loader2 size={12} class="animate-spin" />
                          {:else}
                            <RefreshCw size={12} />
                          {/if}
                          Reset
                        </button>
                      {/if}
                      <button onclick={() => { resetTarget = null; resetPassword = ''; resetResult = ''; }} class="text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1"><X size={14} /></button>
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <!-- Mobile cards (visible on small screens) -->
    <div class="md:hidden space-y-3">
      {#each filtered as u (u.id)}
        <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl p-4 space-y-3">
          <div class="flex items-start justify-between">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-white/[0.05] flex items-center justify-center text-sm font-semibold text-slate-400">
                {(u.username || '?')[0].toUpperCase()}
              </div>
              <div>
                <div class="text-sm font-medium text-slate-200 flex items-center gap-2">
                  {u.username}
                  {#if u.id === auth.user?.id}
                    <span class="text-[0.6rem] px-1.5 py-0.5 rounded-md bg-sky-500/10 text-sky-400/70 font-medium">you</span>
                  {/if}
                </div>
                <div class="text-xs text-slate-500 mt-0.5">{u.email || 'No email'}</div>
              </div>
            </div>
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-lg text-[0.65rem] font-medium {roleBadge(u.role)}">
              {#if roleIcon(u.role)}<Shield size={9} />{/if}
              {u.role === 'super_admin' ? 'Super Admin' : u.role === 'admin' ? 'Admin' : 'User'}
            </span>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-[0.65rem] text-slate-600">
              {u.created_at ? 'Created ' + new Date(u.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : ''}
            </span>
            <div class="flex items-center gap-1">
              <!-- Manage -->
              <a href="/users/{u.id}"
                class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5"
                title="Manage">
                <Settings size={12} />
              </a>
              <!-- Edit -->
              <a href="/users/{u.id}"
                class="inline-flex items-center gap-1 text-xs text-sky-400/70 hover:text-sky-400 transition px-2 py-1 rounded-lg hover:bg-sky-400/5"
                title="Edit">
                <Eye size={12} />
              </a>
              <!-- Impersonate -->
              {#if hasRole(auth.user?.role, 'admin') && u.id !== auth.user?.id}
                {#if impersonateTarget === u.id && impersonateBusy}
                  <span class="inline-flex items-center text-xs text-purple-400/70 px-2 py-1 rounded-lg">
                    <Loader2 size={12} class="animate-spin" />
                  </span>
                {:else}
                  <button onclick={() => impersonateUser(u)}
                    class="inline-flex items-center text-xs text-purple-400/60 hover:text-purple-400 transition px-2 py-1 rounded-lg hover:bg-purple-400/5 cursor-pointer border-0 bg-transparent"
                    title="Impersonate">
                    <LogIn size={12} />
                  </button>
                {/if}
              {/if}
              <!-- Reset -->
              {#if hasRole(auth.user?.role, 'admin') && u.id !== auth.user?.id}
                <button onclick={() => { resetTarget = resetTarget === u.id ? null : u.id; resetPassword = ''; resetResult = ''; }}
                  class="inline-flex items-center text-xs text-amber-400/70 hover:text-amber-400 transition px-2 py-1 rounded-lg hover:bg-amber-400/5 cursor-pointer border-0 bg-transparent"
                  title="Reset password">
                  <Key size={12} />
                </button>
                <button onclick={() => deleteUser(u)} class="inline-flex items-center text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent" title="Delete">
                  <Trash2 size={12} />
                </button>
              {/if}
            </div>
          </div>

          {#if resetTarget === u.id}
            <div class="bg-amber-500/5 border border-amber-500/15 rounded-xl p-3 space-y-2">
              {#if resetResult}
                <div class="flex items-center gap-2 text-emerald-400 text-xs">
                  <Check size={13} /> {resetResult}
                </div>
              {:else}
                <label class="text-[0.65rem] text-amber-400/70 font-medium">New password for {u.username}</label>
                <div class="flex gap-2">
                  <input bind:value={resetPassword} type="password" placeholder="New password"
                    class="flex-1 min-w-0 bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-sm text-slate-200 outline-none focus:border-amber-400/40 transition"
                    onkeydown={(e) => { if (e.key === 'Enter') resetUserPassword(u); }} />
                  <button onclick={() => resetUserPassword(u)} disabled={resetBusy || !resetPassword}
                    class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-medium bg-amber-500/20 text-amber-400 hover:bg-amber-500/30 transition disabled:opacity-40 cursor-pointer border-0">
                    {#if resetBusy}<Loader2 size={12} class="animate-spin" />{:else}<RefreshCw size={12} />{/if} Reset
                  </button>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</DashboardLayout>
