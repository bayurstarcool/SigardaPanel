<script lang="ts">
    import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
    import {
    Database, Globe, ArrowLeft, Plus, Trash2, Pencil, Check, Search,
    Loader2, Users, UserPlus, Key, AlertTriangle, Copy,
  } from 'lucide-svelte';
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
  async function openAssignDbUser() {
    showAssignDbUser = true; showCreateDbUser = false;
    assignDbUserData = { database_id: 0, username: '', host_pattern: '%', privileges: 'ALL' };
    if (dbs.length === 0) { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/databases`); dbs = d?.databases || []; }
    if (dbs.length > 0 && !availableDbUsers.length) {
      loadingAvailableUsers = true;
      try { const data = await apiFetch(auth.token, 'GET', `databases/${dbs[0].id}/available-users`); availableDbUsers = data?.users || []; } catch { availableDbUsers = []; }
      finally { loadingAvailableUsers = false; }
    }
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

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try { site = await apiFetch(auth.token, 'GET', `sites/${siteId}`); } catch (e: any) { if (e.message === 'session expired') return; toasts.error(e.message); } finally { loading = false; }
    await Promise.all([loadDBs(), loadDbUsers()]);
  });
</script>


  {#if loading}
    <div class="space-y-4"><div class="h-8 w-32 bg-white/[0.04] rounded-lg animate-pulse"></div></div>
  {:else if !site}
    <div class="flex flex-col items-center justify-center py-32"><Globe size={36} class="text-white/[0.08]" /><h2 class="text-lg font-semibold text-slate-300 mt-4">Site not found</h2></div>
  {:else}
    <div class="flex items-center gap-3 mb-6">
      <div class="w-10 h-10 rounded-xl bg-violet-400/10 flex items-center justify-center"><Database size={18} class="text-violet-400" /></div>
      <div><h1 class="text-lg font-bold text-slate-200">Databases</h1><p class="text-xs text-slate-500">{site.domain}</p></div>
    </div>

    <div class="space-y-0">
      <!-- ═══ Card: Databases ═══ -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
        <div class="flex items-center justify-between px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 lg:py-3 border-b border-white/[0.05]">
          <div class="flex items-center gap-2 min-w-0">
            <div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center shrink-0"><Database size={13} class="text-sky-400" /></div>
            <span class="text-xs sm:text-sm font-semibold text-slate-200 truncate">Databases</span>
            <span class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 bg-white/[0.04] px-1.5 sm:px-2 py-0.5 rounded-full border border-white/[0.06] shrink-0">{dbs.length}</span>
          </div>
          <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
            <div class="relative"><Search size={11} class="absolute left-2 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
              <input type="text" bind:value={dbSearch} placeholder="Search..." class="w-24 sm:w-44 bg-white/[0.04] border border-white/[0.07] rounded-lg pl-6 sm:pl-7 pr-2 sm:pr-3 py-1 sm:py-1.5 text-[0.65rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400/50 placeholder:text-slate-600 transition" />
            </div>
            <button onclick={() => showCreateDb = !showCreateDb} class="inline-flex items-center gap-1 px-2 sm:px-3 py-1 sm:py-1.5 rounded-xl text-[0.65rem] sm:text-xs font-medium bg-sky-400/[0.08] text-sky-400 border border-sky-400/15 hover:bg-sky-400/[0.12] hover:border-sky-400/25 transition-all duration-200"><Plus size={11} /> <span class="hidden sm:inline">Create</span><span class="sm:hidden">New</span></button>
          </div>
        </div>
        {#if showCreateDb}
          <div class="px-3 sm:px-4 lg:px-5 py-3 border-b border-white/[0.05] bg-white/[0.01]">
            <div class="flex flex-col sm:flex-row sm:items-end gap-2 sm:gap-3">
              <div class="flex-1 min-w-0"><label class="block text-[0.6rem] text-slate-500 mb-0.5 uppercase tracking-wider">Name</label><input bind:value={newDb.name} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400/50 font-mono placeholder:text-slate-600" placeholder="mydb" /></div>
              <div class="w-full sm:w-36"><label class="block text-[0.6rem] text-slate-500 mb-0.5 uppercase tracking-wider">Type</label>
                <select bind:value={newDb.type} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400 appearance-none cursor-pointer"><option value="mysql">MySQL / MariaDB</option><option value="postgres">PostgreSQL</option></select>
              </div>
              <div class="flex gap-1.5 sm:gap-2 shrink-0">
                <button onclick={createDB} disabled={dbBusy || !newDb.name} class="px-3 sm:px-4 py-1.5 sm:py-2 rounded-lg text-[0.7rem] sm:text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{dbBusy ? '...' : 'Create'}</button>
                <button onclick={() => showCreateDb = false} class="px-2.5 sm:px-3 py-1.5 sm:py-2 rounded-lg text-[0.7rem] sm:text-xs text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
              </div>
            </div>
          </div>
        {/if}
        {#if dbLoading}
          <div class="divide-y divide-white/5">{#each Array(3) as _}<div class="px-3 sm:px-5 py-2.5 sm:py-3 flex items-center gap-3"><div class="h-3.5 w-32 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="h-4 w-16 bg-white/[0.03] rounded-full animate-pulse"></div></div>{/each}</div>
        {:else if dbs.length === 0}
          <div class="flex flex-col items-center justify-center py-10 sm:py-14"><Database size={20} class="text-white/[0.08] mb-3" /><p class="text-xs sm:text-sm font-medium text-slate-400 mb-0.5">No databases</p><p class="text-[0.65rem] sm:text-xs text-slate-600">Create a database for this site</p></div>
        {:else}
          <div class="divide-y divide-white/[0.04]">
            {#each dbFiltered as db (db.id)}
              <div>
                <div class="flex items-center justify-between px-3 sm:px-4 lg:px-5 py-2.5 sm:py-3 hover:bg-white/[0.02] transition">
                  <div class="flex items-center gap-2 sm:gap-3 min-w-0">
                    <span class="text-[0.8rem] sm:text-sm font-medium text-slate-200 truncate">{db.name}</span>
                    <span class="inline-flex items-center gap-1 px-1.5 sm:px-2 py-0.5 rounded-full text-[0.55rem] sm:text-[0.65rem] font-medium shrink-0 {db.type === 'mysql' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-sky-500/10 text-sky-400'}"><span class="w-1 h-1 rounded-full bg-current"></span>{db.type || 'mysql'}</span>
                    {#if db.charset}<span class="hidden sm:inline text-[0.6rem] sm:text-[0.65rem] text-slate-500 font-mono bg-white/[0.04] px-1.5 py-0.5 rounded border border-white/[0.06] shrink-0">{db.charset}</span>{/if}
                    {#if db.size_bytes}<span class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 font-mono shrink-0">{fmtBytes(db.size_bytes)}</span>{/if}
                  </div>
                  <div class="flex items-center gap-0.5 sm:gap-1 shrink-0">
                    <button onclick={() => startEditDb(db)} class="p-1 sm:p-1.5 rounded-lg text-slate-500 hover:text-sky-400 hover:bg-sky-400/10 transition border-0 cursor-pointer" title="Edit charset/collation"><Pencil size={11} /></button>
                    <button onclick={() => deleteDB(db)} class="p-1 sm:p-1.5 rounded-lg text-slate-500 hover:text-red-400 hover:bg-red-400/10 transition border-0 cursor-pointer" title="Delete database"><Trash2 size={11} /></button>
                  </div>
                </div>
                {#if editDbId === db.id}
                  <div class="px-3 sm:px-4 lg:px-5 py-2.5 sm:py-3 bg-sky-500/[0.03] border-t border-white/[0.03]">
                    <div class="flex items-center gap-1.5 mb-1.5 sm:mb-2"><Pencil size={11} class="text-sky-400" /><span class="text-[0.6rem] sm:text-[0.65rem] font-semibold text-slate-400 uppercase tracking-wider">Edit Charset & Collation</span></div>
                    <div class="flex flex-col sm:flex-row sm:items-end gap-2 sm:gap-3">
                      <div class="flex-1 min-w-0"><label class="text-[0.55rem] sm:text-[0.6rem] text-slate-600 block mb-0.5 uppercase tracking-wider">Charset</label><input bind:value={editCharset} placeholder="utf8mb4" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400 placeholder:text-slate-600" /></div>
                      <div class="flex-1 min-w-0"><label class="text-[0.55rem] sm:text-[0.6rem] text-slate-600 block mb-0.5 uppercase tracking-wider">Collation</label><input bind:value={editCollation} placeholder="utf8mb4_unicode_ci" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400 placeholder:text-slate-600" /></div>
                      <div class="flex gap-1.5 shrink-0">
                        <button onclick={() => saveEditDb(db.id)} disabled={dbBusy} class="inline-flex items-center gap-1 px-3 sm:px-4 py-1.5 sm:py-2 rounded-lg text-[0.7rem] sm:text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer">{#if dbBusy}<Loader2 size={10} class="animate-spin" />{:else}<Check size={10} />{/if} Save</button>
                        <button onclick={cancelEditDb} class="px-2.5 sm:px-3 py-1.5 sm:py-2 rounded-lg text-[0.7rem] sm:text-xs text-slate-500 hover:bg-white/[0.05] transition border-0 cursor-pointer">Cancel</button>
                      </div>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
          {#if dbFiltered.length !== dbs.length}<div class="px-3 sm:px-5 py-1.5 border-t border-white/5 bg-white/[0.01] text-[0.6rem] sm:text-[0.65rem] text-slate-500">Showing {dbFiltered.length} of {dbs.length} databases</div>{/if}
        {/if}
      </div>

      <!-- ═══ Card: Database Users ═══ -->
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300 mt-3 sm:mt-4">
        <!-- Add User To Database -->
        <div class="px-3 sm:px-4 lg:px-5 py-2.5 sm:py-3 lg:py-4 border-b border-white/[0.05]">
          <div class="flex items-center gap-2 mb-2.5 sm:mb-3"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center shrink-0"><UserPlus size={13} class="text-sky-400" /></div><span class="text-xs sm:text-sm font-semibold text-slate-200">Add User To Database</span></div>
          {#if !showAssignDbUser}
            <div class="space-y-2.5">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 sm:gap-3">
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">User</label><input bind:value={newDbUser.username} placeholder="new_user" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400/50 placeholder:text-slate-600" /></div>
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Database</label>
                  <select bind:value={newDbUser.database_id} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400 appearance-none cursor-pointer"><option value={0}>— Select —</option>{#each dbs as db}<option value={db.id}>{db.name}</option>{/each}</select>
                </div>
              </div>
              <div class="grid grid-cols-2 sm:grid-cols-3 gap-2 sm:gap-3">
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Password <span class="text-slate-700">(opt)</span></label><input bind:value={newDbUser.password} type="password" placeholder="Auto" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400/50 placeholder:text-slate-600" /></div>
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Host</label><input bind:value={newDbUser.host_pattern} placeholder="%" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400/50 placeholder:text-slate-600" /></div>
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Privileges</label>
                  <select bind:value={newDbUser.privileges} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400 appearance-none cursor-pointer"><option value="ALL">ALL</option><option value="SELECT,INSERT,UPDATE,DELETE">Read+Write</option><option value="SELECT">Read Only</option></select>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <button onclick={createDbUser} disabled={dbUserBusy !== null || !newDbUser.database_id || !newDbUser.username} class="inline-flex items-center gap-1 px-3 sm:px-5 py-1.5 sm:py-2 rounded-xl text-[0.75rem] sm:text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.15)] hover:-translate-y-0.5 transition-all disabled:opacity-40 border-0 cursor-pointer">{#if dbUserBusy !== null}<Loader2 size={11} class="animate-spin" />{:else}<UserPlus size={11} />{/if} Add</button>
                <button onclick={() => { showAssignDbUser = true; if (dbs.length === 0) apiFetch(auth.token, 'GET', `sites/${siteId}/databases`).then((d: any) => { dbs = d?.databases || []; }); }} class="text-[0.65rem] sm:text-xs text-indigo-400/70 hover:text-indigo-300 transition cursor-pointer bg-transparent border-0 p-0">Or assign existing user →</button>
              </div>
            </div>
          {:else}
            <div class="space-y-2.5">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 sm:gap-3">
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Database</label>
                  <select bind:value={assignDbUserData.database_id} onchange={(e: any) => { const dbId = parseInt(e.target.value); if (dbId) loadAvailableForDb(dbId); }} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer"><option value={0}>— Select —</option>{#each dbs as db}<option value={db.id}>{db.name}</option>{/each}</select>
                </div>
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">User</label>
                  {#if loadingAvailableUsers}<div class="flex items-center gap-2 text-[0.65rem] sm:text-xs text-slate-500 py-1.5 sm:py-2"><Loader2 size={10} class="animate-spin" /> Loading...</div>
                  {:else}<select bind:value={assignDbUserData.username} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer"><option value="">— Select —</option>{#each availableDbUsers as u}<option value={u.username}>{u.username}</option>{/each}</select>{/if}
                </div>
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 sm:gap-3">
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Host</label><input bind:value={assignDbUserData.host_pattern} placeholder="%" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-indigo-400/50 placeholder:text-slate-600" /></div>
                <div><label class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 block mb-0.5">Privileges</label>
                  <select bind:value={assignDbUserData.privileges} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1.5 sm:py-2 text-[0.75rem] sm:text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer"><option value="ALL">ALL</option><option value="READ_WRITE">Read+Write</option><option value="READ_ONLY">Read Only</option></select>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <button onclick={assignDbUserSubmit} disabled={dbUserBusy !== null || !assignDbUserData.database_id || !assignDbUserData.username} class="inline-flex items-center gap-1 px-3 sm:px-5 py-1.5 sm:py-2 rounded-xl text-[0.75rem] sm:text-sm font-semibold bg-gradient-to-r from-indigo-400 to-sky-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.15)] hover:-translate-y-0.5 transition-all disabled:opacity-40 border-0 cursor-pointer">{#if dbUserBusy !== null}<Loader2 size={11} class="animate-spin" />{:else}<UserPlus size={11} />{/if} Add</button>
                <button onclick={() => showAssignDbUser = false} class="text-[0.65rem] sm:text-xs text-slate-400 hover:text-slate-200 transition cursor-pointer bg-transparent border-0 p-0">← Create new user</button>
              </div>
            </div>
          {/if}
        </div>

        <!-- Password display -->
        {#if dbUserNewPwd}
          <div class="px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 bg-amber-500/[0.06] border-b border-amber-500/10">
            <div class="flex items-center justify-between gap-2"><div class="flex items-center gap-1.5 sm:gap-2 min-w-0"><Key size={11} class="text-amber-400 shrink-0" /><span class="text-[0.65rem] sm:text-xs text-amber-300 font-medium shrink-0">New password:</span><code class="text-[0.65rem] sm:text-xs text-amber-200 font-mono bg-amber-500/10 px-1.5 sm:px-2 py-0.5 rounded truncate">{dbUserNewPwd}</code></div><button onclick={() => copyText(dbUserNewPwd, 'dbpwd')} class="text-[0.65rem] sm:text-xs text-amber-400/60 hover:text-amber-300 transition shrink-0">{#if copiedField === 'dbpwd'}<Check size={11} />{:else}<Copy size={11} />{/if}</button></div>
            <p class="text-[0.55rem] sm:text-[0.6rem] text-amber-400/50 mt-0.5 sm:mt-1 flex items-center gap-1"><AlertTriangle size={9} /> Save this password — it won't be shown again.</p>
          </div>
        {/if}

        <!-- Current Users -->
        <div class="px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 lg:py-3 border-b border-white/[0.05]">
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center shrink-0"><Users size={13} class="text-sky-400" /></div><span class="text-xs sm:text-sm font-semibold text-slate-200 truncate">Current Users</span><span class="text-[0.6rem] sm:text-[0.65rem] text-slate-500 bg-white/[0.04] px-1.5 sm:px-2 py-0.5 rounded-full border border-white/[0.06] shrink-0">{dbUsersAll.length}</span></div>
            <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
              <select bind:value={dbUserFilter} class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-2 sm:px-3 py-1 sm:py-1.5 text-[0.6rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400 appearance-none cursor-pointer shrink-0"><option value={0}>All DBs</option>{#each dbs as db}<option value={db.id}>{db.name}</option>{/each}</select>
              <div class="relative"><Search size={10} class="absolute left-2 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" /><input type="text" bind:value={dbUserSearch} placeholder="Search..." class="w-20 sm:w-32 bg-white/[0.04] border border-white/[0.07] rounded-lg pl-5 sm:pl-7 pr-2 sm:pr-3 py-1 sm:py-1.5 text-[0.6rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400/50 placeholder:text-slate-600 transition" /></div>
            </div>
          </div>
        </div>
        {#if dbUsersLoading}
          <div class="divide-y divide-white/5">{#each Array(3) as _}<div class="px-3 sm:px-5 py-2 sm:py-3 flex items-center gap-3"><div class="h-3 w-24 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="h-4 w-16 bg-white/[0.03] rounded-full animate-pulse"></div></div>{/each}</div>
        {:else if dbUsersFiltered.length === 0}
          <div class="flex flex-col items-center justify-center py-10 sm:py-14"><Users size={20} class="text-white/[0.08] mb-3" /><p class="text-xs sm:text-sm font-medium text-slate-400 mb-0.5">{dbUsersAll.length === 0 ? 'No users yet' : 'No matching users'}</p><p class="text-[0.6rem] sm:text-[0.65rem] sm:text-xs text-slate-600">{dbUsersAll.length === 0 ? 'Add a user above' : 'Try different search'}</p></div>
        {:else}
          <div class="divide-y divide-white/[0.04]">
            {#each dbUsersFiltered as u (u.id + '-' + u.database_id)}
              {#if editingDbUserId === u.id}
                <div class="px-3 sm:px-4 lg:px-5 py-2.5 sm:py-3 bg-sky-500/[0.03]">
                  <div class="flex items-center gap-1.5 mb-1.5 sm:mb-2"><code class="text-[0.7rem] sm:text-xs font-mono text-slate-300">{u.username}@<span class="text-slate-500">{u.host_pattern || '%'}</span></code><span class="text-[0.55rem] sm:text-[0.6rem] text-sky-400/70 bg-sky-400/[0.06] px-1 sm:px-1.5 py-0.5 rounded">{u.database_name}</span></div>
                  <div class="flex flex-col sm:flex-row sm:items-center gap-1.5 sm:gap-2">
                    <input bind:value={editDbUserHost} placeholder="Host pattern" class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1 sm:py-1.5 text-[0.7rem] sm:text-xs text-slate-200 font-mono outline-none focus:border-sky-400" />
                    <select bind:value={editDbUserPrivs} class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-2.5 sm:px-3 py-1 sm:py-1.5 text-[0.7rem] sm:text-xs text-slate-200 outline-none focus:border-sky-400 cursor-pointer"><option value="ALL">ALL</option><option value="SELECT,INSERT,UPDATE,DELETE">Read+Write</option><option value="SELECT">Read Only</option></select>
                    <div class="flex items-center gap-1">
                      <button onclick={() => saveEditDbUser(u)} class="inline-flex items-center gap-1 px-2.5 sm:px-3 py-1 sm:py-1.5 rounded-lg text-[0.65rem] sm:text-xs font-medium bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 transition border-0 cursor-pointer"><Check size={10} /> Save</button>
                      <button onclick={cancelEditDbUser} class="px-2.5 sm:px-3 py-1 sm:py-1.5 rounded-lg text-[0.65rem] sm:text-xs text-slate-500 hover:bg-white/[0.05] transition border-0 cursor-pointer">Cancel</button>
                    </div>
                  </div>
                </div>
              {:else}
                <div class="px-3 sm:px-4 lg:px-5 py-2 sm:py-2.5 hover:bg-white/[0.02] transition">
                  <div class="flex items-center justify-between gap-2 sm:gap-3">
                    <div class="flex items-center gap-2 sm:gap-3 min-w-0">
                      <Users size={12} class="text-slate-500/50 shrink-0" />
                      <div class="min-w-0">
                        <code class="text-[0.75rem] sm:text-sm font-mono text-slate-200 block truncate">{u.username}@<span class="text-slate-500">{u.host_pattern || '%'}</span></code>
                        <div class="flex items-center gap-1.5 mt-0.5">
                          <span class="text-[0.55rem] sm:text-[0.6rem] text-sky-400/70 bg-sky-400/[0.06] px-1 sm:px-1.5 py-0.5 rounded">{u.database_name}</span>
                          <span class="inline-flex items-center px-1 sm:px-1.5 py-0.5 rounded text-[0.55rem] sm:text-[0.6rem] font-medium {u.privileges === 'ALL' ? 'bg-emerald-500/10 text-emerald-400' : u.privileges === 'SELECT,INSERT,UPDATE,DELETE' || u.privileges === 'READ_WRITE' ? 'bg-sky-500/10 text-sky-400' : 'bg-slate-400/10 text-slate-400'}">{u.privileges === 'ALL' ? 'ALL' : u.privileges === 'SELECT,INSERT,UPDATE,DELETE' || u.privileges === 'READ_WRITE' ? 'Read+Write' : u.privileges === 'SELECT' || u.privileges === 'READ_ONLY' ? 'Read Only' : u.privileges}</span>
                        </div>
                      </div>
                    </div>
                    <div class="flex items-center gap-0.5 sm:gap-1 shrink-0">
                      <button onclick={() => startEditDbUser(u)} class="p-1 sm:p-1.5 rounded-lg text-slate-500 hover:text-sky-400 hover:bg-sky-400/10 transition border-0 cursor-pointer" title="Edit privileges"><Pencil size={11} /></button>
                      <button onclick={() => rotateDbUserPwdFromTab(u.database_id, u.id)} disabled={dbUserBusy === u.id} class="p-1 sm:p-1.5 rounded-lg text-slate-500 hover:text-amber-400 hover:bg-amber-400/10 transition border-0 cursor-pointer" title="Change password">{#if dbUserBusy === u.id}<Loader2 size={11} class="animate-spin" />{:else}<Key size={11} />{/if}</button>
                      <button onclick={() => deleteDbUserFromTab(u.id, u.database_id)} disabled={dbUserBusy === u.id} class="p-1 sm:p-1.5 rounded-lg text-slate-500 hover:text-red-400 hover:bg-red-400/10 transition border-0 cursor-pointer" title="Delete user"><Trash2 size={11} /></button>
                    </div>
                  </div>
                </div>
              {/if}
            {/each}
          </div>
          {#if dbUsersFiltered.length !== dbUsersAll.length}<div class="px-3 sm:px-5 py-1 border-t border-white/5 bg-white/[0.01] text-[0.55rem] sm:text-[0.6rem] text-slate-500">Showing {dbUsersFiltered.length} of {dbUsersAll.length} users</div>{/if}
        {/if}
      </div>
    </div>
  {/if}

