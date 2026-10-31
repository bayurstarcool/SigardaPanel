<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import EmptyState from '$components/EmptyState.svelte';
  import { Database, Globe, Plus, Trash2, Pencil, Check, Search, LoaderCircle, Users, UserPlus, Key, TriangleAlert, Copy, ArrowRight, ArrowLeft } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let { siteId }: { siteId: number } = $props();
  let site = $state<any>(null);
  let loading = $state(true);

  // Databases state
  let dbs = $state<any[]>([]);
  let dbLoading = $state(false);
  let showCreateDb = $state(false);
  let newDb = $state({ name: '', type: 'mysql', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' });
  let dbBusy = $state(false);
  let dbSearch = $state('');
  let editDbId = $state<number | null>(null);
  let editCharset = $state('utf8mb4');
  let editCollation = $state('utf8mb4_unicode_ci');

  // DB Users state
  let dbUsersAll = $state<any[]>([]);
  let dbUsersLoading = $state(false);
  let showCreateDbUser = $state(false);
  let newDbUser = $state({ database_id: 0, username: '', password: '', host_pattern: '%', privileges: 'ALL' });
  let dbUserBusy = $state<number | null>(null);
  let dbUserSearch = $state('');
  let dbUserNewPwd = $state('');
  let showAssignDbUser = $state(false);
  let assignDbUserData = $state({ database_id: 0, username: '', host_pattern: '%', privileges: 'ALL' });
  let availableDbUsers = $state<any[]>([]);
  let loadingAvailableUsers = $state(false);
  let dbUserFilter = $state(0);
  let editingDbUserId = $state<number | null>(null);
  let editDbUserHost = $state('');
  let editDbUserPrivs = $state('');
  let copiedField = $state('');

  function copyText(text: string, field: string) {
    navigator.clipboard.writeText(text);
    copiedField = field;
    setTimeout(() => copiedField = '', 1500);
  }

  const dbFiltered = $derived(
    dbSearch ? dbs.filter(db => db.name.toLowerCase().includes(dbSearch.toLowerCase())) : dbs
  );

  function fmtBytes(bytes: number) {
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let i = 0; let size = bytes;
    while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
    return `${size.toFixed(1)} ${units[i]}`;
  }

  function startEditDb(db: any) { editDbId = db.id; editCharset = db.charset || 'utf8mb4'; editCollation = db.collation || 'utf8mb4_unicode_ci'; }
  function cancelEditDb() { editDbId = null; }

  async function loadDBs() {
    dbLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/databases`); dbs = d?.databases || []; }
    catch {} finally { dbLoading = false; }
  }
  async function createDB() {
    if (!newDb.name) return; dbBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/databases`, newDb); showCreateDb = false; newDb = { name: '', type: 'mysql', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' }; await loadDBs(); }
    catch (e: any) { toasts.error(e.message); } finally { dbBusy = false; }
  }
  async function deleteDB(db: any) {
    if (!confirm(`Delete database "${db.name}"?`)) return;
    try { await apiFetch(auth.token, 'DELETE', `databases/${db.id}`); dbs = dbs.filter(d => d.id !== db.id); }
    catch (e: any) { toasts.error(e.message); }
  }
  async function saveEditDb(dbId: number) {
    dbBusy = true;
    try { await apiFetch(auth.token, 'PATCH', `databases/${dbId}`, { charset: editCharset, collation: editCollation }); dbs = dbs.map(d => d.id === dbId ? { ...d, charset: editCharset, collation: editCollation } : d); editDbId = null; }
    catch (e: any) { toasts.error(e.message); } finally { dbBusy = false; }
  }

  // ─── DB Users ───
  const dbUsersFiltered = $derived((() => {
    let result = dbUsersAll;
    if (dbUserFilter) result = result.filter(u => u.database_id === dbUserFilter);
    if (dbUserSearch) { const q = dbUserSearch.toLowerCase(); result = result.filter(u => u.username.toLowerCase().includes(q) || (u.database_name || '').toLowerCase().includes(q)); }
    return result;
  })());

  async function loadDbUsers() {
    dbUsersLoading = true;
    try {
      if (dbs.length === 0) { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/databases`); dbs = d?.databases || []; }
      const allUsers: any[] = [];
      for (const db of dbs) { try { const data = await apiFetch(auth.token, 'GET', `databases/${db.id}/users`); for (const u of (data?.users || [])) allUsers.push({ ...u, database_name: db.name, database_id: db.id }); } catch {} }
      dbUsersAll = allUsers;
    } catch (e: any) { toasts.error(e.message); } finally { dbUsersLoading = false; }
  }
  async function createDbUser() {
    if (!newDbUser.database_id || !newDbUser.username) return; dbUserBusy = 0;
    try {
      const res = await apiFetch(auth.token, 'POST', `databases/${newDbUser.database_id}/users`, { username: newDbUser.username, password: newDbUser.password || undefined, host_pattern: newDbUser.host_pattern, privileges: newDbUser.privileges });
      dbUserNewPwd = res?.password || ''; showCreateDbUser = false;
      newDbUser = { database_id: 0, username: '', password: '', host_pattern: '%', privileges: 'ALL' };
      toasts.success('User created'); dbUsersAll = []; await loadDbUsers();
    } catch (e: any) { toasts.error(e.message); } finally { dbUserBusy = null; }
  }
  async function deleteDbUserFromTab(dbUserId: number, dbId: number) {
    if (!confirm('Delete this database user?')) return; dbUserBusy = dbUserId;
    try { await apiFetch(auth.token, 'DELETE', `databases/${dbId}/users/${dbUserId}`); dbUsersAll = dbUsersAll.filter(u => !(u.id === dbUserId && u.database_id === dbId)); toasts.success('User deleted'); }
    catch (e: any) { toasts.error(e.message); } finally { dbUserBusy = null; }
  }
  async function rotateDbUserPwdFromTab(dbId: number, dbUserId: number) {
    if (!confirm('Reset password for this user?')) return; dbUserBusy = dbUserId;
    try { const res = await apiFetch(auth.token, 'POST', `databases/${dbId}/users/${dbUserId}/rotate-password`); dbUserNewPwd = res?.password || ''; toasts.success('Password rotated'); }
    catch (e: any) { toasts.error(e.message); } finally { dbUserBusy = null; }
  }
  function showAssignExisting() {
    showAssignDbUser = true;
    if (dbs.length === 0) apiFetch(auth.token, 'GET', `sites/${siteId}/databases`).then((d: any) => { dbs = d?.databases || []; });
  }
  async function loadAvailableForDb(dbId: number) {
    loadingAvailableUsers = true;
    try { const data = await apiFetch(auth.token, 'GET', `databases/${dbId}/available-users`); availableDbUsers = data?.users || []; } catch { availableDbUsers = []; }
    finally { loadingAvailableUsers = false; }
  }
  async function assignDbUserSubmit() {
    if (!assignDbUserData.database_id || !assignDbUserData.username) return; dbUserBusy = 0;
    try { await apiFetch(auth.token, 'POST', `databases/${assignDbUserData.database_id}/grant-user`, { username: assignDbUserData.username, host_pattern: assignDbUserData.host_pattern, privileges: assignDbUserData.privileges }); showAssignDbUser = false; toasts.success('User assigned'); dbUsersAll = []; await loadDbUsers(); }
    catch (e: any) { toasts.error(e.message); } finally { dbUserBusy = null; }
  }
  function startEditDbUser(u: any) { editingDbUserId = u.id; editDbUserHost = u.host_pattern || '%'; editDbUserPrivs = u.privileges || 'ALL'; }
  function cancelEditDbUser() { editingDbUserId = null; editDbUserHost = ''; editDbUserPrivs = ''; }
  async function saveEditDbUser(u: any) {
    try { await apiFetch(auth.token, 'PATCH', `db-users/${u.id}`, { host_pattern: editDbUserHost, privileges: editDbUserPrivs }); const idx = dbUsersAll.findIndex((x: any) => x.id === u.id); if (idx !== -1) dbUsersAll[idx] = { ...dbUsersAll[idx], host_pattern: editDbUserHost, privileges: editDbUserPrivs }; toasts.success('User updated'); }
    catch (e: any) { toasts.error(e.message); } finally { cancelEditDbUser(); }
  }

  function privLabel(p: string): string {
    if (p === 'ALL') return 'ALL';
    if (p === 'SELECT,INSERT,UPDATE,DELETE' || p === 'READ_WRITE') return 'Read+Write';
    if (p === 'SELECT' || p === 'READ_ONLY') return 'Read Only';
    return p;
  }
  function privBadge(p: string): string {
    if (p === 'ALL') return 'badge-success';
    if (p === 'SELECT,INSERT,UPDATE,DELETE' || p === 'READ_WRITE') return 'badge-info';
    return '';
  }

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try { site = await apiFetch(auth.token, 'GET', `sites/${siteId}`); } catch (e: any) { if (e.message === 'session expired') return; toasts.error(e.message); } finally { loading = false; }
    await Promise.all([loadDBs(), loadDbUsers()]);
  });
</script>

{#if loading}
  <div class="space-y-4">
    <div class="skeleton h-48 rounded-xl"></div>
    <div class="skeleton h-64 rounded-xl"></div>
  </div>
{:else if !site}
  <div class="card"><EmptyState icon={Globe} title="Site not found" /></div>
{:else}
  <div class="space-y-4">
    <!-- Databases -->
    <div class="card">
      <div class="card-header">
        <div class="flex min-w-0 items-center gap-2">
          <Database size={16} class="shrink-0 text-subtle" />
          <h3 class="card-title truncate">Databases</h3>
          <span class="badge tabular-nums">{dbs.length}</span>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <div class="relative">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input type="text" bind:value={dbSearch} aria-label="Filter databases" placeholder="Filter…" class="input h-8 w-28 pl-8 sm:w-44" />
          </div>
          <button onclick={() => (showCreateDb = !showCreateDb)} class="btn btn-secondary btn-sm"><Plus size={14} /> Create</button>
        </div>
      </div>
      {#if showCreateDb}
        <div class="flex flex-col gap-3 border-b border-line bg-surface-2/50 px-5 py-4 sm:flex-row sm:items-end">
          <div class="min-w-0 flex-1">
            <label for="db-name" class="label">Name</label>
            <input id="db-name" bind:value={newDb.name} class="input font-mono text-[13px]" placeholder="mydb" />
          </div>
          <div class="sm:w-44">
            <label for="db-type" class="label">Type</label>
            <select id="db-type" bind:value={newDb.type} class="input"><option value="mysql">MySQL / MariaDB</option><option value="postgres">PostgreSQL</option></select>
          </div>
          <div class="flex shrink-0 gap-2">
            <button onclick={createDB} disabled={dbBusy || !newDb.name} class="btn btn-primary">{dbBusy ? 'Creating…' : 'Create'}</button>
            <button onclick={() => (showCreateDb = false)} class="btn btn-ghost">Cancel</button>
          </div>
        </div>
      {/if}
      {#if dbLoading}
        <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
      {:else if dbs.length === 0}
        <EmptyState compact icon={Database} title="No databases" description="Create a database for this site." />
      {:else}
        <ul class="divide-y divide-line">
          {#each dbFiltered as db (db.id)}
            <li>
              <div class="flex items-center justify-between gap-3 px-5 py-3 transition-colors duration-100 hover:bg-surface-2/60">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="truncate text-sm font-medium text-fg">{db.name}</span>
                  <span class="badge {db.type === 'mysql' ? 'badge-success' : 'badge-info'}"><span class="dot"></span>{db.type || 'mysql'}</span>
                  {#if db.charset}<span class="badge font-mono max-sm:hidden">{db.charset}</span>{/if}
                  {#if db.size_bytes}<span class="font-mono text-xs text-subtle tabular-nums">{fmtBytes(db.size_bytes)}</span>{/if}
                </div>
                <div class="flex shrink-0 items-center gap-0.5">
                  <button onclick={() => startEditDb(db)} class="btn btn-ghost btn-icon btn-xs" aria-label="Edit charset and collation of {db.name}" title="Edit charset/collation"><Pencil size={13} /></button>
                  <button onclick={() => deleteDB(db)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete database {db.name}" title="Delete database"><Trash2 size={13} /></button>
                </div>
              </div>
              {#if editDbId === db.id}
                <div class="border-t border-line bg-surface-2/50 px-5 py-3">
                  <p class="mb-2 text-xs font-medium text-muted">Edit charset & collation</p>
                  <div class="flex flex-col gap-2 sm:flex-row sm:items-end">
                    <div class="min-w-0 flex-1">
                      <label for="db-charset-{db.id}" class="label">Charset</label>
                      <input id="db-charset-{db.id}" bind:value={editCharset} placeholder="utf8mb4" class="input h-8 font-mono text-[13px]" />
                    </div>
                    <div class="min-w-0 flex-1">
                      <label for="db-collation-{db.id}" class="label">Collation</label>
                      <input id="db-collation-{db.id}" bind:value={editCollation} placeholder="utf8mb4_unicode_ci" class="input h-8 font-mono text-[13px]" />
                    </div>
                    <div class="flex shrink-0 gap-1.5">
                      <button onclick={() => saveEditDb(db.id)} disabled={dbBusy} class="btn btn-primary btn-sm">{#if dbBusy}<LoaderCircle size={13} class="animate-spin" />{:else}<Check size={13} />{/if} Save</button>
                      <button onclick={cancelEditDb} class="btn btn-ghost btn-sm">Cancel</button>
                    </div>
                  </div>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
        {#if dbFiltered.length !== dbs.length}<div class="border-t border-line px-5 py-2 text-xs text-subtle">Showing {dbFiltered.length} of {dbs.length} databases</div>{/if}
      {/if}
    </div>

    <!-- Database users -->
    <div class="card">
      <div class="border-b border-line px-5 py-4">
        <div class="mb-3 flex items-center gap-2">
          <UserPlus size={16} class="text-subtle" />
          <h3 class="card-title">Add user to database</h3>
        </div>
        {#if !showAssignDbUser}
          <div class="space-y-3">
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label for="dbu-user" class="label">User</label>
                <input id="dbu-user" bind:value={newDbUser.username} placeholder="new_user" class="input font-mono text-[13px]" />
              </div>
              <div>
                <label for="dbu-db" class="label">Database</label>
                <select id="dbu-db" bind:value={newDbUser.database_id} class="input"><option value={0}>— Select —</option>{#each dbs as db (db.id)}<option value={db.id}>{db.name}</option>{/each}</select>
              </div>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
              <div>
                <label for="dbu-pass" class="label">Password <span class="text-subtle">(optional)</span></label>
                <input id="dbu-pass" bind:value={newDbUser.password} type="password" placeholder="Auto" autocomplete="new-password" class="input font-mono text-[13px]" />
              </div>
              <div>
                <label for="dbu-host" class="label">Host</label>
                <input id="dbu-host" bind:value={newDbUser.host_pattern} placeholder="%" class="input font-mono text-[13px]" />
              </div>
              <div class="col-span-2 sm:col-span-1">
                <label for="dbu-privs" class="label">Privileges</label>
                <select id="dbu-privs" bind:value={newDbUser.privileges} class="input"><option value="ALL">ALL</option><option value="SELECT,INSERT,UPDATE,DELETE">Read+Write</option><option value="SELECT">Read Only</option></select>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <button onclick={createDbUser} disabled={dbUserBusy !== null || !newDbUser.database_id || !newDbUser.username} class="btn btn-primary btn-sm">
                {#if dbUserBusy !== null}<LoaderCircle size={14} class="animate-spin" />{:else}<UserPlus size={14} />{/if} Add
              </button>
              <button onclick={showAssignExisting} class="link inline-flex items-center gap-1 text-[13px]">Or assign existing user <ArrowRight size={13} /></button>
            </div>
          </div>
        {:else}
          <div class="space-y-3">
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label for="dba-db" class="label">Database</label>
                <select id="dba-db" bind:value={assignDbUserData.database_id} onchange={(e: any) => { const dbId = parseInt(e.target.value); if (dbId) loadAvailableForDb(dbId); }} class="input"><option value={0}>— Select —</option>{#each dbs as db (db.id)}<option value={db.id}>{db.name}</option>{/each}</select>
              </div>
              <div>
                <label for="dba-user" class="label">User</label>
                {#if loadingAvailableUsers}
                  <div class="flex h-9 items-center gap-2 text-[13px] text-subtle"><LoaderCircle size={13} class="animate-spin" /> Loading…</div>
                {:else}
                  <select id="dba-user" bind:value={assignDbUserData.username} class="input"><option value="">— Select —</option>{#each availableDbUsers as u (u.username)}<option value={u.username}>{u.username}</option>{/each}</select>
                {/if}
              </div>
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label for="dba-host" class="label">Host</label>
                <input id="dba-host" bind:value={assignDbUserData.host_pattern} placeholder="%" class="input font-mono text-[13px]" />
              </div>
              <div>
                <label for="dba-privs" class="label">Privileges</label>
                <select id="dba-privs" bind:value={assignDbUserData.privileges} class="input"><option value="ALL">ALL</option><option value="READ_WRITE">Read+Write</option><option value="READ_ONLY">Read Only</option></select>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <button onclick={assignDbUserSubmit} disabled={dbUserBusy !== null || !assignDbUserData.database_id || !assignDbUserData.username} class="btn btn-primary btn-sm">
                {#if dbUserBusy !== null}<LoaderCircle size={14} class="animate-spin" />{:else}<UserPlus size={14} />{/if} Add
              </button>
              <button onclick={() => (showAssignDbUser = false)} class="link inline-flex items-center gap-1 text-[13px]"><ArrowLeft size={13} /> Create new user</button>
            </div>
          </div>
        {/if}
      </div>

      {#if dbUserNewPwd}
        <div class="border-b border-line bg-warning/10 px-5 py-3">
          <div class="flex items-center justify-between gap-2">
            <div class="flex min-w-0 items-center gap-2">
              <Key size={14} class="shrink-0 text-warning" />
              <span class="shrink-0 text-[13px] font-medium text-fg">New password:</span>
              <code class="code truncate">{dbUserNewPwd}</code>
            </div>
            <button onclick={() => copyText(dbUserNewPwd, 'dbpwd')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy password">
              {#if copiedField === 'dbpwd'}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}
            </button>
          </div>
          <p class="mt-1 flex items-center gap-1 text-xs text-warning"><TriangleAlert size={12} /> Save this password — it won't be shown again.</p>
        </div>
      {/if}

      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-line px-5 py-3">
        <div class="flex min-w-0 items-center gap-2">
          <Users size={16} class="shrink-0 text-subtle" />
          <h3 class="card-title truncate">Current users</h3>
          <span class="badge tabular-nums">{dbUsersAll.length}</span>
        </div>
        <div class="flex items-center gap-2">
          <select bind:value={dbUserFilter} aria-label="Filter by database" class="input h-8 w-auto text-[13px]"><option value={0}>All DBs</option>{#each dbs as db (db.id)}<option value={db.id}>{db.name}</option>{/each}</select>
          <div class="relative">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input type="text" bind:value={dbUserSearch} aria-label="Filter users" placeholder="Filter…" class="input h-8 w-32 pl-8 sm:w-40" />
          </div>
        </div>
      </div>
      {#if dbUsersLoading}
        <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-10"></div>{/each}</div>
      {:else if dbUsersFiltered.length === 0}
        <EmptyState compact icon={Users} title={dbUsersAll.length === 0 ? 'No users yet' : 'No matching users'} description={dbUsersAll.length === 0 ? 'Add a user above.' : 'Try a different search.'} />
      {:else}
        <ul class="divide-y divide-line">
          {#each dbUsersFiltered as u (u.id + '-' + u.database_id)}
            {#if editingDbUserId === u.id}
              <li class="bg-surface-2/50 px-5 py-3">
                <div class="mb-2 flex items-center gap-2">
                  <code class="font-mono text-[13px] text-fg">{u.username}@<span class="text-subtle">{u.host_pattern || '%'}</span></code>
                  <span class="badge">{u.database_name}</span>
                </div>
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
                  <label for="dbu-edit-host-{u.id}" class="sr-only">Host pattern</label>
                  <input id="dbu-edit-host-{u.id}" bind:value={editDbUserHost} placeholder="Host pattern" class="input h-8 flex-1 font-mono text-[13px]" />
                  <select bind:value={editDbUserPrivs} aria-label="Privileges" class="input h-8 w-auto text-[13px]"><option value="ALL">ALL</option><option value="SELECT,INSERT,UPDATE,DELETE">Read+Write</option><option value="SELECT">Read Only</option></select>
                  <div class="flex items-center gap-1">
                    <button onclick={() => saveEditDbUser(u)} class="btn btn-primary btn-sm"><Check size={13} /> Save</button>
                    <button onclick={cancelEditDbUser} class="btn btn-ghost btn-sm">Cancel</button>
                  </div>
                </div>
              </li>
            {:else}
              <li class="flex items-center justify-between gap-3 px-5 py-2.5 transition-colors duration-100 hover:bg-surface-2/60">
                <div class="min-w-0">
                  <code class="block truncate font-mono text-[13px] text-fg">{u.username}@<span class="text-subtle">{u.host_pattern || '%'}</span></code>
                  <div class="mt-1 flex items-center gap-1.5">
                    <span class="badge">{u.database_name}</span>
                    <span class="badge {privBadge(u.privileges)}">{privLabel(u.privileges)}</span>
                  </div>
                </div>
                <div class="flex shrink-0 items-center gap-0.5">
                  <button onclick={() => startEditDbUser(u)} class="btn btn-ghost btn-icon btn-xs" aria-label="Edit privileges of {u.username}" title="Edit privileges"><Pencil size={13} /></button>
                  <button onclick={() => rotateDbUserPwdFromTab(u.database_id, u.id)} disabled={dbUserBusy === u.id} class="btn btn-ghost btn-icon btn-xs" aria-label="Reset password of {u.username}" title="Change password">
                    {#if dbUserBusy === u.id}<LoaderCircle size={13} class="animate-spin" />{:else}<Key size={13} />{/if}
                  </button>
                  <button onclick={() => deleteDbUserFromTab(u.id, u.database_id)} disabled={dbUserBusy === u.id} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete user {u.username}" title="Delete user"><Trash2 size={13} /></button>
                </div>
              </li>
            {/if}
          {/each}
        </ul>
        {#if dbUsersFiltered.length !== dbUsersAll.length}<div class="border-t border-line px-5 py-2 text-xs text-subtle">Showing {dbUsersFiltered.length} of {dbUsersAll.length} users</div>{/if}
      {/if}
    </div>
  </div>
{/if}
