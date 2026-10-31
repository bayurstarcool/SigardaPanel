<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import Combobox from './Combobox.svelte';
  import { openPma, fmtSize, siteLabel } from './pma';
  import {
    Database as DatabaseIcon, Search, Trash2, ChevronRight, ExternalLink, Users,
    LoaderCircle, Pencil, UserPlus, KeyRound,
  } from 'lucide-svelte';

  let {
    dbs = $bindable(),
    sites,
    loading,
    createOpen = $bindable(false),
    loadDbs,
    reloadAllUsers,
    onpassword,
    onmanageusers,
  }: {
    dbs: any[];
    sites: any[];
    loading: boolean;
    createOpen?: boolean;
    loadDbs: () => Promise<void>;
    reloadAllUsers: () => Promise<void>;
    onpassword: (pwd: string) => void;
    onmanageusers: () => void;
  } = $props();

  let search = $state('');
  let busyId = $state<number | null>(null);
  let expandedId = $state<number | null>(null);
  let userMap = $state<Record<number, any[]>>({});

  // Create database
  const emptyDb = () => ({ site_id: 0, name: '', type: 'mysql', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' });
  let newDb = $state(emptyDb());
  let newDbSiteQuery = $state('');

  // Edit database (charset/collation)
  let editOpen = $state(false);
  let editingDb = $state<any>(null);
  let editCharset = $state('utf8mb4');
  let editCollation = $state('utf8mb4_unicode_ci');

  // Assign existing MySQL user
  let showAssignFor = $state<number | null>(null);
  let availableUsersMap = $state<Record<number, any[]>>({});
  let loadingAvailable = $state<number | null>(null);
  let assignUsername = $state('');
  let assignHost = $state('%');
  let assignPrivileges = $state('ALL');
  let assigningDbId = $state<number | null>(null);

  // Generic confirmation
  let confirm = $state({ open: false, title: '', message: '', label: 'Confirm', destructive: true, run: () => {} });
  function ask(title: string, message: string, label: string, run: () => void, destructive = true) {
    confirm = { open: true, title, message, label, destructive, run };
  }

  const filteredSiteOptions = $derived(
    newDbSiteQuery
      ? sites.filter((s) => siteLabel(s).toLowerCase().includes(newDbSiteQuery.toLowerCase())).slice(0, 12)
      : sites.slice(0, 12),
  );
  function chooseSite(site: any) {
    newDb.site_id = site.id;
    newDbSiteQuery = siteLabel(site);
  }
  function selectSiteByQuery() {
    const found = sites.find((s) => siteLabel(s) === newDbSiteQuery);
    newDb.site_id = found?.id || 0;
  }

  const filtered = $derived(
    search
      ? dbs.filter((db) => db.name.toLowerCase().includes(search.toLowerCase()) || (db.domain || '').toLowerCase().includes(search.toLowerCase()))
      : dbs,
  );

  function groupUsers(users: any[]): any[] {
    const groups: Record<string, any> = {};
    for (const u of users) {
      if (!groups[u.username]) groups[u.username] = { ...u, hosts: [u.host_pattern] };
      else if (!groups[u.username].hosts.includes(u.host_pattern)) groups[u.username].hosts.push(u.host_pattern);
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

  function deleteDb(db: any) {
    ask(`Delete database "${db.name}"?`, 'This will DROP the database from MySQL permanently.', 'Delete database', async () => {
      busyId = db.id;
      try {
        await apiFetch(auth.token, 'DELETE', `databases/${db.id}`);
        dbs = dbs.filter((d) => d.id !== db.id);
        toasts.success(`Database ${db.name} deleted`);
      } catch (e: any) {
        toasts.error(e.message);
      } finally { busyId = null; }
    });
  }

  async function createDb() {
    if (!newDb.name || !newDb.site_id) return;
    busyId = -1;
    try {
      await apiFetch(auth.token, 'POST', `sites/${newDb.site_id}/databases`, newDb);
      toasts.success(`Database ${newDb.name} created`);
      newDb = emptyDb();
      newDbSiteQuery = '';
      createOpen = false;
      loadDbs();
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
  }

  function startEditDb(db: any) {
    editingDb = db;
    editCharset = db.charset || 'utf8mb4';
    editCollation = db.collation || 'utf8mb4_unicode_ci';
    editOpen = true;
  }

  async function saveEditDb() {
    if (!editingDb) return;
    const dbId = editingDb.id;
    busyId = dbId;
    try {
      await apiFetch(auth.token, 'PATCH', `databases/${dbId}`, { charset: editCharset, collation: editCollation });
      dbs = dbs.map((d) => (d.id === dbId ? { ...d, charset: editCharset, collation: editCollation } : d));
      toasts.success('Database charset/collation updated');
      editOpen = false;
      editingDb = null;
    } catch (e: any) {
      toasts.error(e.message);
    } finally { busyId = null; }
  }

  async function openAssignUser(dbId: number) {
    if (showAssignFor === dbId) { showAssignFor = null; return; }
    showAssignFor = dbId;
    assignUsername = '';
    assignHost = '%';
    assignPrivileges = 'ALL';
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
      const data = await apiFetch(auth.token, 'GET', `databases/${dbId}/users`);
      userMap[dbId] = data?.users || [];
      reloadAllUsers();
      showAssignFor = null;
      assignUsername = '';
      assignHost = '%';
      assignPrivileges = 'ALL';
    } catch (e: any) {
      toasts.error(e.message);
    } finally { assigningDbId = null; }
  }

  function deleteUser(username: string, dbId: number) {
    ask(`Delete user "${username}"?`, 'The user is removed for all hosts on this database.', 'Delete user', async () => {
      try {
        const targetUsers = (userMap[dbId] || []).filter((x: any) => x.username === username);
        await Promise.all(targetUsers.map((u: any) => apiFetch(auth.token, 'DELETE', `databases/${dbId}/users/${u.id}`)));
        userMap[dbId] = (userMap[dbId] || []).filter((x: any) => x.username !== username);
        reloadAllUsers();
      } catch (e: any) {
        toasts.error(e.message);
      }
    });
  }

  function rotateUserPwd(username: string, dbId: number) {
    ask(`Reset password for "${username}"?`, 'A new random password is generated. Apps using the old password will lose access.', 'Reset password', async () => {
      busyId = -2;
      try {
        const targetUsers = (userMap[dbId] || []).filter((x: any) => x.username === username);
        if (targetUsers.length === 0) return;
        const res = await apiFetch(auth.token, 'POST', `databases/${dbId}/users/${targetUsers[0].id}/rotate-password`);
        if (res?.password) {
          onpassword(res.password);
          toasts.success(`Password rotated for ${username}`);
        }
      } catch (e: any) {
        toasts.error(e.message);
      } finally { busyId = null; }
    }, false);
  }
</script>

<div class="card">
  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
    <p class="text-[13px] text-muted">{loading ? 'Loading…' : `${dbs.length} database(s) across all sites`}</p>
    <div class="relative w-full sm:w-64">
      <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
      <input bind:value={search} class="input h-8 pl-8" placeholder="Search databases…" aria-label="Search databases" />
    </div>
  </div>

  {#if loading}
    <div class="space-y-3 p-4">
      {#each Array(5) as _, i (i)}<div class="skeleton h-10"></div>{/each}
    </div>
  {:else if dbs.length === 0}
    <EmptyState icon={DatabaseIcon as any} title="No databases" description="Create a database for a site." actionLabel="Create database" onaction={() => (createOpen = true)} />
  {:else if filtered.length === 0}
    <EmptyState compact icon={Search as any} title="No databases match" description="Try a different search." />
  {:else}
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th class="w-8 pr-0"><span class="sr-only">Expand</span></th>
            <th>Database</th>
            <th class="max-sm:hidden">Engine</th>
            <th class="max-sm:hidden">Site</th>
            <th class="max-md:hidden">Charset</th>
            <th class="max-sm:hidden">Size</th>
            <th class="text-right"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as db (db.id)}
            {@const open = expandedId === db.id}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
            <tr class="cursor-pointer" onclick={() => toggleExpand(db.id)}>
              <td class="w-8 pr-0">
                <button
                  class="btn btn-ghost btn-icon btn-xs"
                  aria-label={open ? 'Hide users' : 'Show users'}
                  aria-expanded={open}
                  onclick={(e) => { e.stopPropagation(); toggleExpand(db.id); }}>
                  <ChevronRight size={14} class="transition-transform duration-150 {open ? 'rotate-90' : ''}" />
                </button>
              </td>
              <td>
                <div class="font-medium text-fg">{db.name}</div>
                <div class="text-xs text-subtle sm:hidden">{db.domain || 'No site'} · {db.type}</div>
              </td>
              <td class="max-sm:hidden">
                <span class="badge {db.type === 'mysql' ? 'badge-success' : 'badge-info'}"><span class="dot"></span>{db.type}</span>
              </td>
              <td class="max-sm:hidden">
                {#if db.domain}
                  <a href="/sites/{db.site_id}" onclick={(e) => e.stopPropagation()} class="text-[13px] text-muted hover:text-accent">{db.domain}</a>
                {:else}
                  <span class="text-subtle">—</span>
                {/if}
              </td>
              <td class="font-mono text-[13px] text-muted max-md:hidden">{db.charset || '—'}</td>
              <td class="text-muted tabular-nums max-sm:hidden">{db.size_bytes ? fmtSize(db.size_bytes) : '—'}</td>
              <td class="text-right">
                <div class="flex items-center justify-end gap-0.5">
                  {#if db.site_id}
                    <a href="/sites/{db.site_id}" onclick={(e) => e.stopPropagation()} class="btn btn-ghost btn-xs max-lg:hidden">
                      <ExternalLink size={14} /> View site
                    </a>
                  {/if}
                  <button onclick={(e) => { e.stopPropagation(); openPma(db.id); }} class="btn btn-ghost btn-xs text-accent" title="Open phpMyAdmin">
                    <DatabaseIcon size={14} /><span class="max-sm:hidden">phpMyAdmin</span>
                  </button>
                  <button
                    onclick={(e) => { e.stopPropagation(); startEditDb(db); }}
                    class="btn btn-ghost btn-icon btn-xs"
                    title="Edit charset/collation"
                    aria-label="Edit charset and collation of {db.name}">
                    <Pencil size={14} />
                  </button>
                  <button
                    onclick={(e) => { e.stopPropagation(); deleteDb(db); }}
                    disabled={busyId === db.id}
                    class="btn btn-danger-ghost btn-icon btn-xs"
                    title="Delete database"
                    aria-label="Delete database {db.name}">
                    {#if busyId === db.id}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                  </button>
                </div>
              </td>
            </tr>

            {#if open}
              <tr class="hover:bg-transparent">
                <td colspan="7" class="bg-surface-2/40 px-4 py-4 sm:pl-12">
                  <div class="mb-3 flex flex-wrap items-center gap-2">
                    <Users size={14} class="text-subtle" />
                    <span class="text-xs font-medium text-muted">Users</span>
                    <button onclick={() => openAssignUser(db.id)} class="btn btn-ghost btn-xs text-accent">
                      <UserPlus size={14} /> Assign user
                    </button>
                    <button onclick={onmanageusers} class="link ml-auto text-xs">Manage all users →</button>
                  </div>

                  {#if userMap[db.id] === undefined}
                    <div class="space-y-2">
                      <div class="skeleton h-8"></div>
                      <div class="skeleton h-8 w-3/4"></div>
                    </div>
                  {:else if (userMap[db.id] || []).length === 0}
                    <p class="text-[13px] text-subtle">No users</p>
                  {:else}
                    <ul class="divide-y divide-line overflow-hidden rounded-lg border border-line bg-surface">
                      {#each groupUsers(userMap[db.id] || []) as g (g.username)}
                        <li class="flex items-center gap-3 px-3 py-2">
                          <code class="min-w-0 truncate font-mono text-[13px] text-fg">
                            {g.username}<span class="text-subtle">@{g.hosts.join(', ')}</span>
                          </code>
                          <span class="badge max-sm:hidden">{g.privileges}</span>
                          <span class="flex-1"></span>
                          <button
                            onclick={() => rotateUserPwd(g.username, db.id)}
                            disabled={busyId === -2}
                            class="btn btn-ghost btn-icon btn-xs"
                            title="Rotate password"
                            aria-label="Rotate password for {g.username}">
                            <KeyRound size={14} />
                          </button>
                          <button
                            onclick={() => deleteUser(g.username, db.id)}
                            class="btn btn-danger-ghost btn-icon btn-xs"
                            title="Delete user"
                            aria-label="Delete user {g.username}">
                            <Trash2 size={14} />
                          </button>
                        </li>
                      {/each}
                    </ul>
                  {/if}

                  {#if showAssignFor === db.id}
                    <div class="mt-3 rounded-lg border border-line bg-surface p-4">
                      <p class="mb-3 text-sm font-semibold text-fg">Assign existing MySQL user</p>
                      {#if loadingAvailable === db.id}
                        <div class="skeleton h-9"></div>
                      {:else if (availableUsersMap[db.id] || []).length === 0}
                        <p class="text-[13px] text-subtle">All MySQL users are already assigned to this database.</p>
                      {:else}
                        <div class="grid grid-cols-1 items-end gap-3 sm:grid-cols-4">
                          <div>
                            <label for="assign-user-{db.id}" class="label">Username</label>
                            <select id="assign-user-{db.id}" bind:value={assignUsername} class="input">
                              <option value="">Select user…</option>
                              {#each availableUsersMap[db.id] || [] as u, i (i)}
                                <option value={u.username}>{u.username}</option>
                              {/each}
                            </select>
                          </div>
                          <div>
                            <label for="assign-host-{db.id}" class="label">Host pattern</label>
                            <input id="assign-host-{db.id}" bind:value={assignHost} placeholder="%" class="input font-mono" />
                          </div>
                          <div>
                            <label for="assign-priv-{db.id}" class="label">Privileges</label>
                            <select id="assign-priv-{db.id}" bind:value={assignPrivileges} class="input">
                              <option value="ALL">ALL</option>
                              <option value="READ_WRITE">Read + Write</option>
                              <option value="READ_ONLY">Read only</option>
                            </select>
                          </div>
                          <div class="flex gap-2">
                            <button onclick={() => grantUser(db.id)} disabled={!assignUsername || assigningDbId === db.id} class="btn btn-primary">
                              {#if assigningDbId === db.id}<LoaderCircle size={14} class="animate-spin" />{:else}<UserPlus size={14} />{/if}
                              Grant
                            </button>
                            <button onclick={() => (showAssignFor = null)} class="btn btn-ghost">Cancel</button>
                          </div>
                        </div>
                      {/if}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
    {#if filtered.length !== dbs.length}
      <div class="border-t border-line px-4 py-2.5 text-xs text-subtle">Showing {filtered.length} of {dbs.length} databases</div>
    {/if}
  {/if}
</div>

<Modal bind:open={createOpen} title="Create database" maxWidth="md">
  <form id="create-db-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); createDb(); }}>
    <Combobox
      id="new-db-site"
      label="Site"
      bind:value={newDbSiteQuery}
      options={filteredSiteOptions}
      optionLabel={siteLabel}
      optionKey={(s) => s.id}
      placeholder="Search a site…"
      emptyText="No site found"
      oninput={selectSiteByQuery}
      onselect={chooseSite} />
    <div>
      <label for="new-db-name" class="label">Database name</label>
      <input id="new-db-name" bind:value={newDb.name} class="input font-mono" autocomplete="off" />
    </div>
    <div>
      <label for="new-db-type" class="label">Engine</label>
      <select id="new-db-type" bind:value={newDb.type} class="input">
        <option value="mysql">MySQL/MariaDB</option>
        <option value="postgres">PostgreSQL</option>
      </select>
    </div>
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (createOpen = false)}>Cancel</button>
    <button type="submit" form="create-db-form" class="btn btn-primary btn-sm" disabled={busyId === -1 || !newDb.name || !newDb.site_id}>
      {#if busyId === -1}<LoaderCircle size={14} class="animate-spin" /> Creating…{:else}Create database{/if}
    </button>
  {/snippet}
</Modal>

<Modal bind:open={editOpen} title="Edit charset & collation" maxWidth="md" onclose={() => (editingDb = null)}>
  <form id="edit-db-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); saveEditDb(); }}>
    {#if editingDb}<p class="text-[13px] text-muted">Database <code class="code">{editingDb.name}</code></p>{/if}
    <div>
      <label for="edit-db-charset" class="label">Charset</label>
      <input id="edit-db-charset" bind:value={editCharset} placeholder="utf8mb4" class="input font-mono" />
    </div>
    <div>
      <label for="edit-db-collation" class="label">Collation</label>
      <input id="edit-db-collation" bind:value={editCollation} placeholder="utf8mb4_unicode_ci" class="input font-mono" />
    </div>
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => { editOpen = false; editingDb = null; }}>Cancel</button>
    <button type="submit" form="edit-db-form" class="btn btn-primary btn-sm" disabled={!editingDb || busyId === editingDb?.id}>
      {#if editingDb && busyId === editingDb.id}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}Save{/if}
    </button>
  {/snippet}
</Modal>

<ConfirmDialog
  bind:open={confirm.open}
  title={confirm.title}
  message={confirm.message}
  confirmLabel={confirm.label}
  destructive={confirm.destructive}
  onconfirm={() => confirm.run()} />
