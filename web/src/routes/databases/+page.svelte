<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { toasts } from '$lib/toast.svelte';
  import {
    Database as DatabaseIcon, Search, Trash2, RefreshCw,
    ChevronDown, ChevronRight, Globe, ExternalLink, Users,
    Plus, Loader2, Check, Pencil, X, UserPlus
  } from 'lucide-svelte';

  async function openPma(databaseId: number) {
    const t = auth.token || localStorage.getItem('sigarda_token');
    if (!t) { window.location.href = '/login'; return; }
    try {
      const res = await fetch('/pma/ticket', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${t}` },
        body: JSON.stringify({ database_id: databaseId })
      });
      if (res.status === 401) { window.location.href = '/login'; return; }
      if (!res.ok) {
        toasts.error('phpMyAdmin gagal dibuka. Database harus punya user MySQL aktif.');
        return;
      }
      const { ticket } = await res.json();
      window.open(`/pma/t/${encodeURIComponent(ticket)}`, '_blank', 'noopener,noreferrer');
    } catch {
      toasts.error('phpMyAdmin gagal dibuka. Coba lagi.');
    }
  }
  import { onMount } from 'svelte';

  let dbs = $state<any[]>([]);
  let sites = $state<any[]>([]);
  let loading = $state(true);
  let search = $state('');
  let busyId = $state<number | null>(null);
  let expandedId = $state<number | null>(null);
  let userMap = $state<Record<number, any[]>>({});
  let activeDbTab = $state<'databases' | 'users'>('databases');
  let showCreate = $state(true);
  let newDb = $state({ site_id: 0, name: '', type: 'mysql', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' });
  let newDbSiteQuery = $state('');
  let showSiteDropdown = $state(false);

  // Edit database state
  let editingDbId = $state<number | null>(null);
  let editCharset = $state('utf8mb4');
  let editCollation = $state('utf8mb4_unicode_ci');

  // Assign user state
  let showAssignFor = $state<number | null>(null);
  let availableUsersMap = $state<Record<number, any[]>>({});
  let loadingAvailable = $state<number | null>(null);
  let assignUsername = $state('');
  let assignHost = $state('%');
  let assignPrivileges = $state('ALL');
  let assigningDbId = $state<number | null>(null);

  // --- Flat "Database Users" section state ---
  let allUsers = $state<any[]>([]);
  let userSearch = $state('');
  let editingUserId = $state<number | null>(null);
  let editUserHost = $state('');
  let editUserPrivs = $state('');
  let newPwdVisible = $state('');

  // cPanel-like user management forms
  let newUser = $state({ database_id: 0, username: '', password: '', host_pattern: '%', privileges: 'ALL' });
  let addDbId = $state(0);
  let addUsername = $state('');
  let addHost = $state('%');
  let addPrivileges = $state('ALL');
  let addAvailableUsers = $state<any[]>([]);
  let addBusy = $state(false);
  let createUserBusy = $state(false);
  let newUserDbQuery = $state('');
  let addDbQuery = $state('');
  let addUserQuery = $state('');
  let showNewUserDbDropdown = $state(false);
  let showAddDbDropdown = $state(false);
  let showAddUserDropdown = $state(false);

  function dbLabel(db: any) { return `${db.name}${db.domain ? ` (${db.domain})` : ''}`; }
  function siteLabel(site: any) { return site.domain || site.name || `Site #${site.id}`; }
  const filteredSiteOptions = $derived(
    newDbSiteQuery
      ? sites.filter(s => siteLabel(s).toLowerCase().includes(newDbSiteQuery.toLowerCase())).slice(0, 12)
      : sites.slice(0, 12)
  );
  function chooseSite(site: any) {
    newDb.site_id = site.id;
    newDbSiteQuery = siteLabel(site);
    showSiteDropdown = false;
  }
  function selectSiteByQuery() {
    showSiteDropdown = true;
    const found = sites.find(s => siteLabel(s) === newDbSiteQuery);
    newDb.site_id = found?.id || 0;
  }
  const filteredNewUserDbOptions = $derived(
    newUserDbQuery ? dbs.filter(d => dbLabel(d).toLowerCase().includes(newUserDbQuery.toLowerCase())).slice(0, 12) : dbs.slice(0, 12)
  );
  const filteredAddDbOptions = $derived(
    addDbQuery ? dbs.filter(d => dbLabel(d).toLowerCase().includes(addDbQuery.toLowerCase())).slice(0, 12) : dbs.slice(0, 12)
  );
  function canonicalHost(host: string) {
    const value = (host || '%').trim().toLowerCase();
    if (value === '127.0.0.1' || value === '::1') return 'localhost';
    return value || '%';
  }
  function userHost(u: any) { return canonicalHost(u.host || u.host_pattern || '%'); }
  function userKey(u: any) { return `${u.username}@${userHost(u)}`; }
  function userCanonicalKey(u: any) { return `${u.username}@${canonicalHost(u.host || u.host_pattern || '%')}`; }
  const filteredAddUserOptions = $derived(
    addUserQuery ? addAvailableUsers.filter(u => userKey(u).toLowerCase().includes(addUserQuery.toLowerCase()) || u.username.toLowerCase().includes(addUserQuery.toLowerCase())).slice(0, 12) : addAvailableUsers.slice(0, 12)
  );
  function chooseNewUserDb(db: any) {
    newUser.database_id = db.id;
    newUserDbQuery = dbLabel(db);
    showNewUserDbDropdown = false;
  }
  function chooseAddDb(db: any) {
    addDbId = db.id;
    addDbQuery = dbLabel(db);
    showAddDbDropdown = false;
    loadAddUsers();
  }
  function chooseAddUser(u: any) {
    addUsername = u.username;
    addHost = userHost(u);
    addUserQuery = userKey(u);
    showAddUserDropdown = false;
  }
  function selectNewUserDbByQuery() {
    showNewUserDbDropdown = true;
    const found = dbs.find(d => dbLabel(d) === newUserDbQuery);
    newUser.database_id = found?.id || 0;
  }
  function selectAddDbByQuery() {
    showAddDbDropdown = true;
    const found = dbs.find(d => dbLabel(d) === addDbQuery);
    addDbId = found?.id || 0;
    if (found) loadAddUsers();
  }
  function selectAddUserByQuery() {
    showAddUserDropdown = true;
    const found = addAvailableUsers.find(u => userKey(u) === addUserQuery || u.username === addUserQuery);
    addUsername = found?.username || '';
    if (found) addHost = userHost(found);
  }

  async function loadDbs() {
    if (!auth.token) { window.location.href = '/login'; return; }
    loading = true;
    try {
      const [dbData, siteData, userData] = await Promise.all([
        apiFetch(auth.token, 'GET', 'databases'),
        apiFetch(auth.token, 'GET', 'sites'),
        apiFetch(auth.token, 'GET', 'db-users'),
      ]);
      dbs = dbData?.databases || [];
      sites = siteData?.sites || [];
      allUsers = userData?.users || [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    }
    finally { loading = false; }
  }

  async function reloadAllUsers() {
    try {
      const userData = await apiFetch(auth.token, 'GET', 'db-users');
      allUsers = userData?.users || [];
    } catch {}
  }

  const displayUsers = $derived.by(() => {
    const deduped = new Map<string, any>();
    for (const u of allUsers) {
      const key = `${u.database_id}:${u.username}@${canonicalHost(u.host_pattern || u.host || '%')}`;
      if (!deduped.has(key)) deduped.set(key, { ...u, host_pattern: canonicalHost(u.host_pattern || u.host || '%') });
    }
    return Array.from(deduped.values());
  });

  const filteredUsers = $derived(
    userSearch
      ? displayUsers.filter(u =>
          u.username.toLowerCase().includes(userSearch.toLowerCase()) ||
          canonicalHost(u.host_pattern || u.host || '%').includes(userSearch.toLowerCase()) ||
          (u.database_name || '').toLowerCase().includes(userSearch.toLowerCase()) ||
          (u.domain || '').toLowerCase().includes(userSearch.toLowerCase())
        )
      : displayUsers
  );

  function startEditUser(u: any) {
    editingUserId = u.id;
    editUserHost = u.host_pattern;
    editUserPrivs = u.privileges;
  }
  function cancelEditUser() { editingUserId = null; }

  async function saveEditUser(u: any) {
    busyId = u.id;
    try {
      const body: any = {};
      if (editUserHost !== u.host_pattern) body.host_pattern = editUserHost;
      if (editUserPrivs !== u.privileges) body.privileges = editUserPrivs;
      if (Object.keys(body).length === 0) { editingUserId = null; return; }
      await apiFetch(auth.token, 'PATCH', `db-users/${u.id}`, body);
      allUsers = allUsers.map(p => p.id === u.id ? { ...p, host_pattern: editUserHost, privileges: editUserPrivs } : p);
      editingUserId = null;
      toasts.success(`User ${u.username} updated`);
    } catch (e: any) { toasts.error(e.message); }
    finally { busyId = null; }
  }

  async function rotateUserById(u: any) {
    if (!confirm(`Reset password untuk user "${u.username}"?`)) return;
    busyId = u.id;
    try {
      const res = await apiFetch(auth.token, 'POST', `databases/${u.database_id}/users/${u.id}/rotate-password`, {});
      if (res?.password) newPwdVisible = res.password;
      toasts.success(`Password rotated for ${u.username}`);
    } catch (e: any) { toasts.error(e.message); }
    finally { busyId = null; }
  }

  async function deleteUserById(u: any) {
    if (!confirm(`Delete user "${u.username}"@${u.host_pattern}? This cannot be undone.`)) return;
    busyId = u.id;
    try {
      await apiFetch(auth.token, 'DELETE', `databases/${u.database_id}/users/${u.id}`);
      allUsers = allUsers.filter(p => p.id !== u.id);
      toasts.success(`User ${u.username} deleted`);
    } catch (e: any) { toasts.error(e.message); }
    finally { busyId = null; }
  }

  async function createDbUserGlobal() {
    if (!newUser.database_id || !newUser.username || newUser.password.length < 8) return;
    createUserBusy = true;
    try {
      const res = await apiFetch(auth.token, 'POST', `databases/${newUser.database_id}/users`, newUser);
      if (res?.password) newPwdVisible = res.password;
      toasts.success(`User ${newUser.username} created`);
      newUser = { database_id: 0, username: '', password: '', host_pattern: '%', privileges: 'ALL' };
      newUserDbQuery = '';
      reloadAllUsers();
    } catch (e: any) { toasts.error(e.message); }
    finally { createUserBusy = false; }
  }

  async function loadAddUsers() {
    addUsername = '';
    addUserQuery = '';
    addAvailableUsers = [];
    if (!addDbId) return;
    try {
      const res = await apiFetch(auth.token, 'GET', `databases/${addDbId}/available-users`);
      const apiUsers = res?.users || [];
      const alreadyAssigned = new Set(
        allUsers
          .filter((u: any) => u.database_id === addDbId)
          .map((u: any) => userCanonicalKey(u))
      );
      const merged = new Map<string, any>();
      for (const u of apiUsers) {
        const item = { username: u.username, host: userHost(u) };
        const key = userCanonicalKey(item);
        if (!alreadyAssigned.has(key)) merged.set(key, item);
      }
      for (const u of allUsers) {
        const item = { username: u.username, host: userHost(u) };
        const key = userCanonicalKey(item);
        if (!alreadyAssigned.has(key)) merged.set(key, item);
      }
      addAvailableUsers = Array.from(merged.values()).sort((a, b) => userKey(a).localeCompare(userKey(b)));
    } catch {}
  }

  async function addUserToDatabaseGlobal() {
    if (!addDbId || !addUsername) return;
    addBusy = true;
    try {
      const res = await apiFetch(auth.token, 'POST', `databases/${addDbId}/grant-user`, {
        username: addUsername,
        host_pattern: addHost,
        privileges: addPrivileges,
      });
      if (res?.password) newPwdVisible = res.password;
      toasts.success(`User ${addUsername} added to database`);
      addDbId = 0;
      addDbQuery = '';
      addUsername = '';
      addUserQuery = '';
      addHost = '%';
      addPrivileges = 'ALL';
      addAvailableUsers = [];
      reloadAllUsers();
    } catch (e: any) { toasts.error(e.message); }
    finally { addBusy = false; }
  }

  onMount(() => loadDbs());

  const filtered = $derived(
    search
      ? dbs.filter(db =>
          db.name.toLowerCase().includes(search.toLowerCase()) ||
          (db.domain || '').toLowerCase().includes(search.toLowerCase())
        )
      : dbs
  );

  function groupUsers(users: any[]): any[] {
    const groups: Record<string, any> = {};
    for (const u of users) {
      if (!groups[u.username]) {
        groups[u.username] = { ...u, hosts: [u.host_pattern] };
      } else if (!groups[u.username].hosts.includes(u.host_pattern)) {
        groups[u.username].hosts.push(u.host_pattern);
      }
    }
    return Object.values(groups);
  }

  async function toggleExpand(dbId: number) {
    if (expandedId === dbId) { expandedId = null; return; }
    expandedId = dbId;
    if (!userMap[dbId]) {
      try {
        const data = await apiFetch(auth.token, 'GET', `databases/${dbId}/users`);
        userMap[dbId] = data?.users || [];
      } catch {}
    }
  }

  async function deleteDb(db: any) {
    if (!confirm(`Delete database "${db.name}" permanently?\nThis will DROP the database from MySQL.`)) return;
    busyId = db.id;
    try {
      await apiFetch(auth.token, 'DELETE', `databases/${db.id}`);
      dbs = dbs.filter(d => d.id !== db.id);
      toasts.success(`Database ${db.name} deleted`);
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
  }

  async function createDb() {
    if (!newDb.name || !newDb.site_id) return;
    busyId = -1;
    try {
      await apiFetch(auth.token, 'POST', `sites/${newDb.site_id}/databases`, newDb);
      toasts.success(`Database ${newDb.name} created`);
      newDb = { site_id: 0, name: '', type: 'mysql', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' };
      newDbSiteQuery = '';
      loadDbs();
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
  }

  // --- Edit Database (charset/collation) ---
  function startEditDb(db: any) {
    editingDbId = db.id;
    editCharset = db.charset || 'utf8mb4';
    editCollation = db.collation || 'utf8mb4_unicode_ci';
  }

  function cancelEditDb() {
    editingDbId = null;
  }

  async function saveEditDb(dbId: number) {
    busyId = dbId;
    try {
      await apiFetch(auth.token, 'PATCH', `databases/${dbId}`, {
        charset: editCharset,
        collation: editCollation,
      });
      // Update local state
      dbs = dbs.map(d => d.id === dbId ? { ...d, charset: editCharset, collation: editCollation } : d);
      toasts.success('Database charset/collation updated');
      editingDbId = null;
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
  }

  // --- Assign Existing MySQL User ---
  async function openAssignUser(dbId: number) {
    if (showAssignFor === dbId) {
      showAssignFor = null;
      return;
    }
    showAssignFor = dbId;
    assignUsername = '';
    assignHost = '%';
    assignPrivileges = 'ALL';
    // Load available users
    if (!availableUsersMap[dbId]) {
      loadingAvailable = dbId;
      try {
        const data = await apiFetch(auth.token, 'GET', `databases/${dbId}/available-users`);
        availableUsersMap[dbId] = data?.users || [];
      } catch (e: any) {
        availableUsersMap[dbId] = [];
        toasts.error(e.message);
      } finally { loadingAvailable = null; }
    }
  }

  async function grantUser(dbId: number) {
    if (!assignUsername) return;
    assigningDbId = dbId;
    try {
      await apiFetch(auth.token, 'POST', `databases/${dbId}/grant-user`, {
        username: assignUsername,
        host_pattern: assignHost,
        privileges: assignPrivileges,
      });
      toasts.success(`User ${assignUsername} granted to database`);
      // Refresh users list
      const data = await apiFetch(auth.token, 'GET', `databases/${dbId}/users`);
      userMap[dbId] = data?.users || [];
      reloadAllUsers();
      // Clear assign form
      showAssignFor = null;
      assignUsername = '';
      assignHost = '%';
      assignPrivileges = 'ALL';
    } catch (e: any) {
      toasts.error(e.message);
    } finally { assigningDbId = null; }
  }

  async function deleteUser(username: string, dbId: number) {
    if (!confirm(`Delete user "${username}"? (all hosts)`)) return;
    try {
      const targetUsers = (userMap[dbId] || []).filter((x: any) => x.username === username);
      await Promise.all(targetUsers.map((u: any) =>
        apiFetch(auth.token, 'DELETE', `databases/${dbId}/users/${u.id}`)
      ));
      userMap[dbId] = (userMap[dbId] || []).filter((x: any) => x.username !== username);
      reloadAllUsers();
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  async function rotateUserPwd(username: string, dbId: number) {
    if (!confirm(`Reset password untuk user "${username}"?`)) return;
    busyId = -2;
    try {
      const targetUsers = (userMap[dbId] || []).filter((x: any) => x.username === username);
      if (targetUsers.length === 0) return;
      const res = await apiFetch(auth.token, 'POST', `databases/${dbId}/users/${targetUsers[0].id}/rotate-password`);
      if (res?.password) {
        toasts.success(`Password rotated! ${res.password}`);
      }
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
 }

 function fmtBytes(bytes: number) {
   if (!bytes) return '0 B';
   const units = ['B', 'KB', 'MB', 'GB'];
   let i = 0;
   let size = bytes;
   while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
   return `${size.toFixed(1)} ${units[i]}`;
 }
</script>

<DashboardLayout active="Databases">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Databases</h1>
      <p class="text-sm text-slate-500 mt-1">
        Manage databases and database users from one page.
      </p>
    </div>
    <div class="flex items-center gap-3 w-full sm:w-auto">
      <div class="relative flex-1 sm:max-w-[200px]">
        <Search size={12} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
        <input
          type="text"
          bind:value={search}
          placeholder="Cari database..."
          class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl pl-8 pr-3 py-2 text-sm text-slate-200 outline-none focus:border-sky-400 focus:ring-1 focus:ring-sky-400/20 placeholder:text-slate-600 transition"
        />
      </div>
      <div class="inline-flex items-center gap-1 rounded-xl bg-white/[0.04] border border-white/[0.07] p-1">
        <button
          onclick={() => activeDbTab = 'databases'}
          class="px-3 py-1.5 rounded-lg text-xs font-semibold transition border-0 cursor-pointer {activeDbTab === 'databases' ? 'bg-sky-400 text-[#0a1628]' : 'text-slate-400 hover:text-slate-200'}"
        >Databases</button>
        <button
          onclick={() => activeDbTab = 'users'}
          class="px-3 py-1.5 rounded-lg text-xs font-semibold transition border-0 cursor-pointer {activeDbTab === 'users' ? 'bg-sky-400 text-[#0a1628]' : 'text-slate-400 hover:text-slate-200'}"
        >Database Users</button>
      </div>
    </div>
  </div>

  <!-- New password reveal banner -->
  {#if newPwdVisible}
    <div class="mb-4 px-4 py-3 rounded-xl bg-amber-500/10 border border-amber-500/20">
      <p class="text-[0.6rem] text-amber-400 uppercase tracking-wider mb-1">New Password — Copy it now</p>
      <div class="flex items-center gap-3 flex-wrap">
        <code class="text-sm text-amber-300 font-mono break-all">{newPwdVisible}</code>
        <button
          onclick={() => { navigator.clipboard.writeText(newPwdVisible); newPwdVisible = ''; }}
          class="inline-flex items-center gap-1 text-xs text-amber-500 hover:text-amber-400 transition border-0 cursor-pointer"
        >
          <Check size={12} /> Copy & Dismiss
        </button>
        <button
          onclick={() => { newPwdVisible = ''; }}
          class="p-1 rounded text-amber-500/50 hover:text-amber-400 transition border-0 cursor-pointer"
        >
          <X size={12} />
        </button>
      </div>
    </div>
  {/if}

  {#if activeDbTab !== 'users'}
  <!-- Create form -->
  {#if showCreate}
    <div class="mb-6 bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <h2 class="text-xl font-bold text-slate-100 mb-1">Create New Database</h2>
      <p class="text-xs text-slate-500 mb-4">Create a database for a site.</p>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="relative">
          <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Site</label>
          <input
            bind:value={newDbSiteQuery}
            oninput={selectSiteByQuery}
            onfocus={() => showSiteDropdown = true}
            onblur={() => setTimeout(() => showSiteDropdown = false, 150)}
            placeholder="Search / pilih site..."
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600"
          />
          {#if showSiteDropdown}
            <div class="absolute z-30 mt-1 w-full max-h-56 overflow-y-auto rounded-lg border border-white/[0.1] bg-[#0c1524] shadow-xl">
              {#if filteredSiteOptions.length === 0}
                <div class="px-3 py-2 text-xs text-slate-500">No site found</div>
              {:else}
                {#each filteredSiteOptions as s}
                  <button
                    type="button"
                    onclick={() => chooseSite(s)}
                    class="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-sky-300 border-0 cursor-pointer"
                  >
                    {siteLabel(s)}
                  </button>
                {/each}
              {/if}
            </div>
          {/if}
        </div>
        <div>
          <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">DB Name</label>
          <input
            bind:value={newDb.name}
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600"
          />
        </div>
        <div>
          <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Type</label>
          <select
            bind:value={newDb.type}
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 appearance-none cursor-pointer"
          >
            <option value="mysql">MySQL/MariaDB</option>
            <option value="postgres">PostgreSQL</option>
          </select>
        </div>
        <div class="flex items-end gap-2">
          <button
            onclick={createDb}
            disabled={busyId === -1 || !newDb.name || !newDb.site_id}
            class="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer"
          >
            {#if busyId === -1}
              <Loader2 size={11} class="animate-spin" />
            {:else}
              <Plus size={11} />
            {/if}
            Create
          </button>

        </div>
      </div>
    </div>
  {/if}

  <div class="mt-8 mb-4 flex flex-col sm:flex-row sm:items-end justify-between gap-3">
    <div>
      <h2 class="text-xl font-bold text-slate-100">Current Databases</h2>
      <p class="text-xs text-slate-500 mt-1">{loading ? 'Loading...' : `${dbs.length} database(s) across all sites`}</p>
    </div>
  </div>

  <!-- Content -->
  {#if loading}
    <!-- Loading skeleton -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-10 space-y-4">
      {#each [1, 2, 3, 4, 5] as i}
        <div class="h-4 rounded-md bg-white/[0.04] animate-pulse" style="width: {60 + i * 8}%"></div>
      {/each}
    </div>
  {:else if dbs.length === 0}
    <!-- Empty state -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-10 text-center py-20">
      <DatabaseIcon size={48} class="mb-4 opacity-30 mx-auto" />
      <h2 class="text-lg font-semibold mb-2">No Databases</h2>
      <p class="text-sm text-slate-500 mb-4">Create a database for a site.</p>
      <button
        onclick={() => showCreate = true}
        class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition border-0 cursor-pointer"
      >
        <Plus size={13} /> Create Database
      </button>
    </div>
  {:else}
    <!-- Table -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full border-separate border-spacing-0">
          <thead>
            <tr class="text-left">
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap w-8">Users</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Database</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Engine</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Site</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Charset</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Size</th>
              <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07] whitespace-nowrap">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each filtered as db (db.id)}
              <tr
                class="hover:bg-white/[0.02] transition cursor-pointer"
                onclick={() => toggleExpand(db.id)}
              >
                <!-- Expand arrow -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="text-slate-600">
                    {#if expandedId === db.id}
                      <ChevronDown size={12} />
                    {:else}
                      <ChevronRight size={12} />
                    {/if}
                  </span>
                </td>
                <!-- Name with emoji -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="inline-flex items-center gap-2 font-medium text-slate-200">
                    <span class="w-8 h-8 rounded-xl bg-sky-400/10 text-sky-400 border border-sky-400/15 flex items-center justify-center">
                      <DatabaseIcon size={15} />
                    </span>
                    <span class="min-w-0">
                      <span class="block truncate">{db.name}</span>
                      <span class="sm:hidden text-[0.65rem] text-slate-500 font-normal">{db.domain || 'No site'}</span>
                    </span>
                  </span>
                </td>
                <!-- Type badge -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="inline-flex items-center gap-1.5 px-3 py-0.5 rounded-full text-[0.72rem] font-medium {db.type === 'mysql' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-sky-500/10 text-sky-400'}">
                    <span class="w-1.5 h-1.5 rounded-full bg-current"></span>
                    {db.type}
                  </span>
                </td>
                <!-- Site link -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  {#if db.domain}
                    <a
                      href="/sites/{db.site_id}"
                      onclick={(e) => e.stopPropagation()}
                      class="inline-flex items-center gap-1 text-xs text-sky-400 hover:text-sky-300 transition no-underline"
                    >
                      <Globe size={11} />
                      {db.domain}
                    </a>
                  {:else}
                    <span class="text-xs text-slate-600">—</span>
                  {/if}
                </td>
                <!-- Charset -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="text-xs text-slate-500 font-mono">{db.charset || '—'}</span>
                </td>
                <!-- Size -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <span class="text-xs text-slate-500 font-mono">{db.size_bytes ? fmtBytes(db.size_bytes) : '—'}</span>
                </td>
                <!-- Actions -->
                <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                  <div class="flex gap-2">
                    {#if db.site_id}
                      <a
                        href="/sites/{db.site_id}"
                        onclick={(e) => e.stopPropagation()}
                        class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition-all duration-150 cursor-pointer whitespace-nowrap no-underline"
                      >
                        <ExternalLink size={11} />
                        View Site
                      </a>
                    {/if}
                    <!-- Open phpMyAdmin -->
                    <a
                      target="_blank"
                      onclick={(e) => { e.preventDefault(); e.stopPropagation(); openPma(db.id); }}
                      class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-sky-400 hover:bg-sky-500/10 transition-all duration-150 cursor-pointer whitespace-nowrap no-underline"
                    >
                      <DatabaseIcon size={11} />
                      phpMyAdmin
                    </a>
                    <!-- Edit charset/collation -->
                    <button
                      onclick={(e) => { e.stopPropagation(); startEditDb(db); }}
                      class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:bg-sky-500/10 hover:text-sky-400 transition border-0 cursor-pointer"
                      title="Edit charset/collation"
                    >
                      <Pencil size={11} />
                    </button>
                    <button
                      onclick={(e) => { e.stopPropagation(); deleteDb(db); }}
                      disabled={busyId === db.id}
                      class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 transition border-0 cursor-pointer"
                      title="Delete database"
                    >
                      {#if busyId === db.id}
                        <Loader2 size={11} class="animate-spin" />
                      {:else}
                        <Trash2 size={11} />
                      {/if}
                    </button>
                  </div>
                </td>
              </tr>

              <!-- Edit DB inline form -->
              {#if editingDbId === db.id}
                <tr>
                  <td colspan="7" class="bg-sky-500/[0.03] border-b border-white/[0.03]">
                    <div class="px-12 py-4">
                      <div class="flex items-center gap-2 mb-3">
                        <Pencil size={13} class="text-sky-400" />
                        <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Edit Charset & Collation</span>
                      </div>
                      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 items-end">
                        <div>
                          <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Charset</label>
                          <input
                            bind:value={editCharset}
                            placeholder="utf8mb4"
                            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 font-mono outline-none focus:border-sky-400 placeholder:text-slate-600"
                          />
                        </div>
                        <div>
                          <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Collation</label>
                          <input
                            bind:value={editCollation}
                            placeholder="utf8mb4_unicode_ci"
                            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 font-mono outline-none focus:border-sky-400 placeholder:text-slate-600"
                          />
                        </div>
                        <div class="flex gap-2">
                          <button
                            onclick={() => saveEditDb(db.id)}
                            disabled={busyId === db.id}
                            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer"
                          >
                            {#if busyId === db.id}
                              <Loader2 size={11} class="animate-spin" />
                            {:else}
                              <Check size={11} />
                            {/if}
                            Save
                          </button>
                          <button
                            onclick={cancelEditDb}
                            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs text-slate-500 hover:bg-white/[0.05] transition border-0 cursor-pointer"
                          >
                            <X size={11} /> Cancel
                          </button>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              {/if}

              <!-- Expanded users row -->
              {#if expandedId === db.id}
                <tr>
                  <td colspan="7" class="bg-white/[0.01] border-b border-white/[0.03]">
                    <div class="px-12 py-4">
                      <div class="flex items-center gap-2 mb-3">
                        <Users size={13} class="text-slate-500" />
                        <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Users</span>
                        <button
                          onclick={() => openAssignUser(db.id)}
                          class="inline-flex items-center gap-1 ml-2 px-2.5 py-1 rounded-lg text-[0.65rem] font-medium text-indigo-400 hover:bg-indigo-500/10 transition border-0 cursor-pointer"
                        >
                          <UserPlus size={10} /> Assign User
                        </button>
                        <a
                          href="/database-users"
                          class="text-[0.65rem] text-sky-500 hover:text-sky-400 transition ml-2 no-underline"
                        >
                          Manage all users →
                        </a>
                      </div>

                      {#if userMap[db.id] === undefined}
                        <div class="flex items-center gap-2 text-xs text-slate-500">
                          <Loader2 size={11} class="animate-spin" /> Loading users...
                        </div>
                      {:else if (userMap[db.id] || []).length === 0}
                        <p class="text-xs text-slate-500 italic">No users</p>
                      {:else}
                        <div class="space-y-1">
                          {#each groupUsers(userMap[db.id] || []) as g}
                            <div class="flex items-center gap-3 px-3 py-2 bg-white/[0.02] rounded-lg text-xs">
                              <Users size={11} class="text-slate-600" />
                              <code class="text-slate-200 font-mono">
                                {g.username}@<span class="text-slate-500">{g.hosts.join(', ')}</span>
                              </code>
                              <span class="text-[0.6rem] text-slate-600 uppercase">{g.privileges}</span>
                              <span class="flex-1"></span>
                              <button
                                onclick={() => rotateUserPwd(g.username, db.id)}
                                class="p-1 rounded text-slate-600 hover:text-amber-400 hover:bg-amber-500/10 transition border-0 cursor-pointer"
                                title="Rotate password"
                              >
                                <RefreshCw size={10} />
                              </button>
                              <button
                                onclick={() => deleteUser(g.username, db.id)}
                                class="p-1 rounded text-slate-600 hover:text-red-400 hover:bg-red-500/10 transition border-0 cursor-pointer"
                                title="Delete user"
                              >
                                <Trash2 size={10} />
                              </button>
                            </div>
                          {/each}
                        </div>
                      {/if}

                      <!-- Assign User panel -->
                      {#if showAssignFor === db.id}
                        <div class="mt-3 bg-indigo-500/[0.05] border border-indigo-500/10 rounded-xl p-4">
                          <div class="flex items-center gap-2 mb-3">
                            <UserPlus size={12} class="text-indigo-400" />
                            <span class="text-xs font-semibold text-indigo-400 uppercase tracking-wider">Assign Existing MySQL User</span>
                          </div>

                          {#if loadingAvailable === db.id}
                            <div class="flex items-center gap-2 text-xs text-slate-500">
                              <Loader2 size={11} class="animate-spin" /> Loading available users...
                            </div>
                          {:else if (availableUsersMap[db.id] || []).length === 0}
                            <p class="text-xs text-slate-500 italic">All MySQL users already assigned to this database</p>
                          {:else}
                            <div class="grid grid-cols-1 sm:grid-cols-4 gap-3 items-end">
                              <div>
                                <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Username</label>
                                <select
                                  bind:value={assignUsername}
                                  class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer"
                                >
                                  <option value="">— Select user —</option>
                                  {#each availableUsersMap[db.id] || [] as u}
                                    <option value={u.username}>{u.username}</option>
                                  {/each}
                                </select>
                              </div>
                              <div>
                                <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Host Pattern</label>
                                <input
                                  bind:value={assignHost}
                                  placeholder="%"
                                  class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 font-mono outline-none focus:border-indigo-400 placeholder:text-slate-600"
                                />
                              </div>
                              <div>
                                <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Privileges</label>
                                <select
                                  bind:value={assignPrivileges}
                                  class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer"
                                >
                                  <option value="ALL">ALL</option>
                                  <option value="READ_WRITE">Read + Write</option>
                                  <option value="READ_ONLY">Read Only</option>
                                </select>
                              </div>
                              <div class="flex gap-2">
                                <button
                                  onclick={() => grantUser(db.id)}
                                  disabled={!assignUsername || assigningDbId === db.id}
                                  class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-semibold bg-gradient-to-r from-indigo-400 to-sky-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer"
                                >
                                  {#if assigningDbId === db.id}
                                    <Loader2 size={11} class="animate-spin" />
                                  {:else}
                                    <UserPlus size={11} />
                                  {/if}
                                  Grant
                                </button>
                                <button
                                  onclick={() => { showAssignFor = null; }}
                                  class="inline-flex items-center gap-1 px-3 py-2 rounded-lg text-xs text-slate-500 hover:bg-white/[0.05] transition border-0 cursor-pointer"
                                >
                                  <X size={11} /> Cancel
                                </button>
                              </div>
                            </div>
                          {/if}
                        </div>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      {#if filtered.length !== dbs.length}
        <div class="px-5 py-2.5 border-t border-white/5 bg-white/[0.01] text-[0.65rem] text-amber-400/60">
          Showing {filtered.length} of {dbs.length} databases
        </div>
      {/if}
    </div>
  {/if}
  {/if}

  <div class="{activeDbTab === 'users' ? '' : 'hidden'}">
    <!-- ═══════════════ SECTION: CREATE DB USER ═══════════════ -->
    <div id="database-users" class="space-y-6">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <h2 class="text-xl font-bold text-slate-100 mb-1">Create New User</h2>
        <p class="text-xs text-slate-500 mb-4">Create database user and grant access to selected database.</p>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4 items-end">
          <div class="relative">
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Database</label>
            <input bind:value={newUserDbQuery} oninput={selectNewUserDbByQuery} onfocus={() => showNewUserDbDropdown = true} onblur={() => setTimeout(() => showNewUserDbDropdown = false, 150)} placeholder="Search database..." class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600" />
            {#if showNewUserDbDropdown}
              <div class="absolute z-30 mt-1 w-full max-h-56 overflow-y-auto rounded-lg border border-white/[0.1] bg-[#0c1524] shadow-xl">
                {#if filteredNewUserDbOptions.length === 0}
                  <div class="px-3 py-2 text-xs text-slate-500">No database found</div>
                {:else}
                  {#each filteredNewUserDbOptions as db}
                    <button type="button" onclick={() => chooseNewUserDb(db)} class="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-sky-300 border-0 cursor-pointer">{dbLabel(db)}</button>
                  {/each}
                {/if}
              </div>
            {/if}
          </div>
          <div>
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Username</label>
            <input bind:value={newUser.username} placeholder="db_user" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600" />
          </div>
          <div>
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Password</label>
            <input bind:value={newUser.password} type="text" placeholder="min 8 chars" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600 font-mono" />
          </div>
          <div>
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Host</label>
            <input bind:value={newUser.host_pattern} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-sky-400 placeholder:text-slate-600 font-mono" />
          </div>
          <button onclick={createDbUserGlobal} disabled={createUserBusy || !newUser.database_id || !newUser.username || newUser.password.length < 8} class="inline-flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer">
            {#if createUserBusy}<Loader2 size={11} class="animate-spin" />{:else}<Plus size={11} />{/if}
            Create User
          </button>
        </div>
      </div>

      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <h2 class="text-xl font-bold text-slate-100 mb-1">Add User To Database</h2>
        <p class="text-xs text-slate-500 mb-4">Grant existing MySQL user to selected database.</p>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4 items-end">
          <div class="relative">
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Database</label>
            <input bind:value={addDbQuery} oninput={selectAddDbByQuery} onfocus={() => showAddDbDropdown = true} onblur={() => setTimeout(() => showAddDbDropdown = false, 150)} placeholder="Search database..." class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 placeholder:text-slate-600" />
            {#if showAddDbDropdown}
              <div class="absolute z-30 mt-1 w-full max-h-56 overflow-y-auto rounded-lg border border-white/[0.1] bg-[#0c1524] shadow-xl">
                {#if filteredAddDbOptions.length === 0}
                  <div class="px-3 py-2 text-xs text-slate-500">No database found</div>
                {:else}
                  {#each filteredAddDbOptions as db}
                    <button type="button" onclick={() => chooseAddDb(db)} class="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-indigo-300 border-0 cursor-pointer">{dbLabel(db)}</button>
                  {/each}
                {/if}
              </div>
            {/if}
          </div>
          <div class="relative">
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">User</label>
            <input bind:value={addUserQuery} oninput={selectAddUserByQuery} onfocus={() => showAddUserDropdown = true} onblur={() => setTimeout(() => showAddUserDropdown = false, 150)} disabled={!addDbId} placeholder="Search user..." class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 placeholder:text-slate-600 disabled:opacity-50" />
            {#if showAddUserDropdown && addDbId}
              <div class="absolute z-30 mt-1 w-full max-h-56 overflow-y-auto rounded-lg border border-white/[0.1] bg-[#0c1524] shadow-xl">
                {#if filteredAddUserOptions.length === 0}
                  <div class="px-3 py-2 text-xs text-slate-500">No available user</div>
                {:else}
                  {#each filteredAddUserOptions as u}
                    <button type="button" onclick={() => chooseAddUser(u)} class="w-full text-left px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-indigo-300 border-0 cursor-pointer">{userKey(u)}</button>
                  {/each}
                {/if}
              </div>
            {/if}
          </div>
          <div>
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Host</label>
            <input bind:value={addHost} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 placeholder:text-slate-600 font-mono" />
          </div>
          <div>
            <label class="text-[0.6rem] text-slate-600 block mb-1 uppercase tracking-wider">Privileges</label>
            <select bind:value={addPrivileges} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-2 text-xs text-slate-200 outline-none focus:border-indigo-400 appearance-none cursor-pointer">
              <option value="ALL">ALL PRIVILEGES</option>
              <option value="SELECT,INSERT,UPDATE,DELETE">Read/Write</option>
              <option value="SELECT">Read Only</option>
            </select>
          </div>
          <button onclick={addUserToDatabaseGlobal} disabled={addBusy || !addDbId || !addUsername} class="inline-flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-indigo-400 to-sky-400 text-[#0a1628] hover:-translate-y-0.5 transition disabled:opacity-40 border-0 cursor-pointer">
            {#if addBusy}<Loader2 size={11} class="animate-spin" />{:else}<UserPlus size={11} />{/if}
            Add
          </button>
        </div>
      </div>

    <!-- ═══════════════ SECTION: DATABASE USERS ═══════════════ -->
    <div class="pt-2">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h2 class="text-xl font-bold flex items-center gap-2 bg-gradient-to-r from-white to-indigo-100 bg-clip-text text-transparent">
            <Users size={20} class="text-indigo-400" /> Database Users
          </h2>
          <p class="text-sm text-slate-500 mt-1">
            {displayUsers.length} user(s) across all databases · localhost and 127.0.0.1 are merged
          </p>
        </div>
        <div class="relative flex-1 sm:max-w-[220px]">
          <Search size={12} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
          <input
            type="text"
            bind:value={userSearch}
            placeholder="Cari user..."
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl pl-8 pr-3 py-2 text-sm text-slate-200 outline-none focus:border-indigo-400 focus:ring-1 focus:ring-indigo-400/20 placeholder:text-slate-600 transition"
          />
        </div>
      </div>

      {#if allUsers.length === 0}
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-10 text-center">
          <Users size={40} class="mb-3 opacity-30 mx-auto" />
          <p class="text-sm text-slate-500">No database users yet.</p>
          <p class="text-xs text-slate-600 mt-1">Expand a database above and click "Assign User" to add one.</p>
        </div>
      {:else}
        <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
          <div class="overflow-x-auto">
            <table class="w-full border-separate border-spacing-0">
              <thead>
                <tr class="text-left">
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Username</th>
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Host</th>
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Privileges</th>
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Database</th>
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Site</th>
                  <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
                </tr>
              </thead>
              <tbody>
                {#each filteredUsers as u (u.id)}
                  <tr class="hover:bg-white/[0.02] transition">
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      <span class="inline-flex items-center gap-2 font-mono text-sm text-slate-200">
                        <Users size={13} class="text-slate-500" />
                        {u.username}
                      </span>
                    </td>
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      {#if editingUserId === u.id}
                        <input
                          bind:value={editUserHost}
                          class="w-24 bg-white/[0.04] border border-white/[0.07] rounded px-2 py-1 text-xs text-slate-200 outline-none focus:border-indigo-400 font-mono"
                        />
                      {:else}
                        <code class="text-xs text-slate-400 font-mono bg-white/[0.03] px-2 py-0.5 rounded">{u.host_pattern}</code>
                      {/if}
                    </td>
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      {#if editingUserId === u.id}
                        <select
                          bind:value={editUserPrivs}
                          class="bg-white/[0.04] border border-white/[0.07] rounded px-2 py-1 text-xs text-slate-200 outline-none focus:border-indigo-400 cursor-pointer"
                        >
                          <option value="ALL">ALL</option>
                          <option value="SELECT,INSERT,UPDATE,DELETE">Read/Write</option>
                          <option value="SELECT">Read Only</option>
                        </select>
                      {:else}
                        <span class="text-xs text-slate-400">{u.privileges}</span>
                      {/if}
                    </td>
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      <span class="inline-flex items-center gap-1.5 text-xs text-sky-400">
                        <DatabaseIcon size={11} />
                        {u.database_name}
                      </span>
                    </td>
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      {#if u.domain}
                        <span class="inline-flex items-center gap-1 text-xs text-slate-500">
                          <Globe size={10} />
                          {u.domain}
                        </span>
                      {:else}
                        <span class="text-xs text-slate-600">—</span>
                      {/if}
                    </td>
                    <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                      <div class="flex gap-1.5">
                        {#if editingUserId === u.id}
                          <button
                            onclick={() => saveEditUser(u)}
                            disabled={busyId === u.id}
                            class="p-1.5 rounded-lg text-emerald-400 hover:bg-emerald-500/10 transition border-0 cursor-pointer"
                            title="Save"
                          >
                            <Check size={13} />
                          </button>
                          <button
                            onclick={cancelEditUser}
                            class="p-1.5 rounded-lg text-slate-500 hover:bg-white/10 transition border-0 cursor-pointer"
                            title="Cancel"
                          >
                            <X size={13} />
                          </button>
                        {:else}
                          <button
                            onclick={() => startEditUser(u)}
                            class="p-1.5 rounded-lg text-slate-500 hover:bg-white/10 hover:text-sky-400 transition border-0 cursor-pointer"
                            title="Edit host/privileges"
                          >
                            <Pencil size={12} />
                          </button>
                          <button
                            onclick={() => rotateUserById(u)}
                            disabled={busyId === u.id}
                            class="p-1.5 rounded-lg text-slate-500 hover:bg-white/10 hover:text-amber-400 transition border-0 cursor-pointer"
                            title="Rotate password"
                          >
                            {#if busyId === u.id}
                              <Loader2 size={12} class="animate-spin" />
                            {:else}
                              <RefreshCw size={12} />
                            {/if}
                          </button>
                          <!-- Open phpMyAdmin -->
                          <a
                            target="_blank"
                            onclick={(e) => e.stopPropagation()}
                            class="p-1.5 rounded-lg text-slate-500 hover:bg-white/10 hover:text-sky-400 transition"
                            title="Open phpMyAdmin"
                          >
                            <DatabaseIcon size={12} />
                          </a>
                          <button
                            onclick={() => deleteUserById(u)}
                            disabled={busyId === u.id}
                            class="p-1.5 rounded-lg text-slate-500 hover:bg-white/10 hover:text-red-400 transition border-0 cursor-pointer"
                            title="Delete user"
                          >
                            {#if busyId === u.id}
                              <Loader2 size={12} class="animate-spin" />
                            {:else}
                              <Trash2 size={12} />
                            {/if}
                          </button>
                        {/if}
                      </div>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          {#if filteredUsers.length !== allUsers.length}
            <div class="px-5 py-2.5 border-t border-white/5 bg-white/[0.01] text-[0.65rem] text-amber-400/60">
              Showing {filteredUsers.length} of {allUsers.length} users
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</DashboardLayout>
