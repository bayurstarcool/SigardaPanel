<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import Combobox from './Combobox.svelte';
  import { openPma, canonicalHost, dbLabel } from './pma';
  import { Database as DatabaseIcon, Search, Trash2, Users, LoaderCircle, Check, Pencil, X, KeyRound } from 'lucide-svelte';

  let {
    allUsers = $bindable(),
    dbs,
    loading,
    createOpen = $bindable(false),
    addOpen = $bindable(false),
    reloadAllUsers,
    onpassword,
  }: {
    allUsers: any[];
    dbs: any[];
    loading: boolean;
    createOpen?: boolean;
    addOpen?: boolean;
    reloadAllUsers: () => Promise<void>;
    onpassword: (pwd: string) => void;
  } = $props();

  let userSearch = $state('');
  let busyId = $state<number | null>(null);
  let editingUserId = $state<number | null>(null);
  let editUserHost = $state('');
  let editUserPrivs = $state('');

  // Create user form
  const emptyUser = () => ({ database_id: 0, username: '', password: '', host_pattern: '%', privileges: 'ALL' });
  let newUser = $state(emptyUser());
  let newUserDbQuery = $state('');
  let createUserBusy = $state(false);

  // Add existing user to database form
  let addDbId = $state(0);
  let addUsername = $state('');
  let addHost = $state('%');
  let addPrivileges = $state('ALL');
  let addAvailableUsers = $state<any[]>([]);
  let addBusy = $state(false);
  let addDbQuery = $state('');
  let addUserQuery = $state('');

  // Generic confirmation
  let confirm = $state({ open: false, title: '', message: '', label: 'Confirm', destructive: true, run: () => {} });
  function ask(title: string, message: string, label: string, run: () => void, destructive = true) {
    confirm = { open: true, title, message, label, destructive, run };
  }

  function userHost(u: any) { return canonicalHost(u.host || u.host_pattern || '%'); }
  function userKey(u: any) { return `${u.username}@${userHost(u)}`; }

  const filteredNewUserDbOptions = $derived(
    newUserDbQuery ? dbs.filter((d) => dbLabel(d).toLowerCase().includes(newUserDbQuery.toLowerCase())).slice(0, 12) : dbs.slice(0, 12),
  );
  const filteredAddDbOptions = $derived(
    addDbQuery ? dbs.filter((d) => dbLabel(d).toLowerCase().includes(addDbQuery.toLowerCase())).slice(0, 12) : dbs.slice(0, 12),
  );
  const filteredAddUserOptions = $derived(
    addUserQuery
      ? addAvailableUsers
          .filter((u) => userKey(u).toLowerCase().includes(addUserQuery.toLowerCase()) || u.username.toLowerCase().includes(addUserQuery.toLowerCase()))
          .slice(0, 12)
      : addAvailableUsers.slice(0, 12),
  );

  function chooseNewUserDb(db: any) {
    newUser.database_id = db.id;
    newUserDbQuery = dbLabel(db);
  }
  function chooseAddDb(db: any) {
    addDbId = db.id;
    addDbQuery = dbLabel(db);
    loadAddUsers();
  }
  function chooseAddUser(u: any) {
    addUsername = u.username;
    addHost = userHost(u);
    addUserQuery = userKey(u);
  }
  function selectNewUserDbByQuery() {
    const found = dbs.find((d) => dbLabel(d) === newUserDbQuery);
    newUser.database_id = found?.id || 0;
  }
  function selectAddDbByQuery() {
    const found = dbs.find((d) => dbLabel(d) === addDbQuery);
    addDbId = found?.id || 0;
    if (found) loadAddUsers();
  }
  function selectAddUserByQuery() {
    const found = addAvailableUsers.find((u) => userKey(u) === addUserQuery || u.username === addUserQuery);
    addUsername = found?.username || '';
    if (found) addHost = userHost(found);
  }

  const displayUsers = $derived.by(() => {
    const deduped = new Map<string, any>();
    for (const u of allUsers) {
      const key = `${u.database_id}:${u.username}@${canonicalHost(u.host_pattern || u.host || '%')}`;
      if (!deduped.has(key)) deduped.set(key, { ...u, host_pattern: canonicalHost(u.host_pattern || u.host || '%') });
    }
    return Array.from(deduped.values());
  });

  const filteredUsers = $derived.by(() => {
    const q = userSearch.toLowerCase();
    if (!q) return displayUsers;
    return displayUsers.filter(
      (u) =>
        u.username.toLowerCase().includes(q) ||
        canonicalHost(u.host_pattern || u.host || '%').includes(q) ||
        (u.database_name || '').toLowerCase().includes(q) ||
        (u.domain || '').toLowerCase().includes(q),
    );
  });

  function startEditUser(u: any) {
    editingUserId = u.id;
    editUserHost = u.host_pattern;
    editUserPrivs = u.privileges;
  }

  async function saveEditUser(u: any) {
    busyId = u.id;
    try {
      const body: any = {};
      if (editUserHost !== u.host_pattern) body.host_pattern = editUserHost;
      if (editUserPrivs !== u.privileges) body.privileges = editUserPrivs;
      if (Object.keys(body).length === 0) { editingUserId = null; return; }
      await apiFetch(auth.token, 'PATCH', `db-users/${u.id}`, body);
      allUsers = allUsers.map((p) => (p.id === u.id ? { ...p, host_pattern: editUserHost, privileges: editUserPrivs } : p));
      editingUserId = null;
      toasts.success(`User ${u.username} updated`);
    } catch (e: any) { toasts.error(e.message); }
    finally { busyId = null; }
  }

  function rotateUserById(u: any) {
    ask(`Reset password for "${u.username}"?`, 'A new random password is generated. Apps using the old password will lose access.', 'Reset password', async () => {
      busyId = u.id;
      try {
        const res = await apiFetch(auth.token, 'POST', `databases/${u.database_id}/users/${u.id}/rotate-password`, {});
        if (res?.password) onpassword(res.password);
        toasts.success(`Password rotated for ${u.username}`);
      } catch (e: any) { toasts.error(e.message); }
      finally { busyId = null; }
    }, false);
  }

  function deleteUserById(u: any) {
    ask(`Delete user "${u.username}"@${u.host_pattern}?`, 'This cannot be undone.', 'Delete user', async () => {
      busyId = u.id;
      try {
        await apiFetch(auth.token, 'DELETE', `databases/${u.database_id}/users/${u.id}`);
        allUsers = allUsers.filter((p) => p.id !== u.id);
        toasts.success(`User ${u.username} deleted`);
      } catch (e: any) { toasts.error(e.message); }
      finally { busyId = null; }
    });
  }

  async function createDbUserGlobal() {
    if (!newUser.database_id || !newUser.username || newUser.password.length < 8) return;
    createUserBusy = true;
    try {
      const res = await apiFetch(auth.token, 'POST', `databases/${newUser.database_id}/users`, newUser);
      if (res?.password) onpassword(res.password);
      toasts.success(`User ${newUser.username} created`);
      newUser = emptyUser();
      newUserDbQuery = '';
      createOpen = false;
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
      const alreadyAssigned = new Set(allUsers.filter((u: any) => u.database_id === addDbId).map((u: any) => userKey(u)));
      const merged = new Map<string, any>();
      for (const u of [...apiUsers, ...allUsers]) {
        const item = { username: u.username, host: userHost(u) };
        const key = userKey(item);
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
      if (res?.password) onpassword(res.password);
      toasts.success(`User ${addUsername} added to database`);
      addDbId = 0;
      addDbQuery = '';
      addUsername = '';
      addUserQuery = '';
      addHost = '%';
      addPrivileges = 'ALL';
      addAvailableUsers = [];
      addOpen = false;
      reloadAllUsers();
    } catch (e: any) { toasts.error(e.message); }
    finally { addBusy = false; }
  }
</script>

<div class="card">
  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
    <p class="text-[13px] text-muted">{displayUsers.length} user(s) across all databases · localhost and 127.0.0.1 are merged</p>
    <div class="relative w-full sm:w-64">
      <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
      <input bind:value={userSearch} class="input h-8 pl-8" placeholder="Search users…" aria-label="Search users" />
    </div>
  </div>

  {#if loading}
    <div class="space-y-3 p-4">
      {#each Array(4) as _, i (i)}<div class="skeleton h-10"></div>{/each}
    </div>
  {:else if allUsers.length === 0}
    <EmptyState
      icon={Users as any}
      title="No database users yet"
      description="Create a user, or expand a database and assign an existing one."
      actionLabel="Create user"
      onaction={() => (createOpen = true)} />
  {:else if filteredUsers.length === 0}
    <EmptyState compact icon={Search as any} title="No users match" description="Try a different search." />
  {:else}
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Host</th>
            <th class="max-sm:hidden">Privileges</th>
            <th class="max-sm:hidden">Database</th>
            <th class="max-md:hidden">Site</th>
            <th class="text-right"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {#each filteredUsers as u (u.id)}
            {@const editing = editingUserId === u.id}
            <tr>
              <td>
                <div class="font-mono text-[13px] text-fg">{u.username}</div>
                <div class="text-xs text-subtle sm:hidden">{u.database_name}</div>
              </td>
              <td>
                {#if editing}
                  <label for="edit-host-{u.id}" class="sr-only">Host</label>
                  <input id="edit-host-{u.id}" bind:value={editUserHost} class="input h-8 w-28 font-mono text-[13px]" />
                {:else}
                  <code class="code">{u.host_pattern}</code>
                {/if}
              </td>
              <td class={editing ? '' : 'max-sm:hidden'}>
                {#if editing}
                  <label for="edit-privs-{u.id}" class="sr-only">Privileges</label>
                  <select id="edit-privs-{u.id}" bind:value={editUserPrivs} class="input h-8 w-36 text-[13px]">
                    <option value="ALL">ALL</option>
                    <option value="SELECT,INSERT,UPDATE,DELETE">Read/Write</option>
                    <option value="SELECT">Read only</option>
                  </select>
                {:else}
                  <span class="text-[13px] text-muted">{u.privileges}</span>
                {/if}
              </td>
              <td class="max-sm:hidden">
                <span class="inline-flex items-center gap-1.5 text-[13px] text-fg"><DatabaseIcon size={14} class="text-subtle" />{u.database_name}</span>
              </td>
              <td class="text-[13px] text-muted max-md:hidden">{u.domain || '—'}</td>
              <td class="text-right">
                <div class="flex items-center justify-end gap-0.5">
                  {#if editing}
                    <button onclick={() => saveEditUser(u)} disabled={busyId === u.id} class="btn btn-ghost btn-icon btn-xs text-success" title="Save" aria-label="Save changes">
                      {#if busyId === u.id}<LoaderCircle size={14} class="animate-spin" />{:else}<Check size={14} />{/if}
                    </button>
                    <button onclick={() => (editingUserId = null)} class="btn btn-ghost btn-icon btn-xs" title="Cancel" aria-label="Cancel editing">
                      <X size={14} />
                    </button>
                  {:else}
                    <button onclick={() => startEditUser(u)} class="btn btn-ghost btn-icon btn-xs" title="Edit host/privileges" aria-label="Edit {u.username}">
                      <Pencil size={14} />
                    </button>
                    <button onclick={() => rotateUserById(u)} disabled={busyId === u.id} class="btn btn-ghost btn-icon btn-xs" title="Rotate password" aria-label="Rotate password for {u.username}">
                      {#if busyId === u.id}<LoaderCircle size={14} class="animate-spin" />{:else}<KeyRound size={14} />{/if}
                    </button>
                    <button onclick={() => openPma(u.database_id)} class="btn btn-ghost btn-icon btn-xs max-sm:hidden" title="Open phpMyAdmin" aria-label="Open phpMyAdmin for {u.database_name}">
                      <DatabaseIcon size={14} />
                    </button>
                    <button onclick={() => deleteUserById(u)} disabled={busyId === u.id} class="btn btn-danger-ghost btn-icon btn-xs" title="Delete user" aria-label="Delete user {u.username}">
                      <Trash2 size={14} />
                    </button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if filteredUsers.length !== displayUsers.length}
      <div class="border-t border-line px-4 py-2.5 text-xs text-subtle">Showing {filteredUsers.length} of {displayUsers.length} users</div>
    {/if}
  {/if}
</div>

<Modal bind:open={createOpen} title="Create database user" maxWidth="md">
  <form id="create-user-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); createDbUserGlobal(); }}>
    <p class="text-[13px] text-muted">Creates a user and grants it access to the selected database.</p>
    <Combobox
      id="new-user-db"
      label="Database"
      bind:value={newUserDbQuery}
      options={filteredNewUserDbOptions}
      optionLabel={dbLabel}
      optionKey={(d) => d.id}
      placeholder="Search a database…"
      emptyText="No database found"
      oninput={selectNewUserDbByQuery}
      onselect={chooseNewUserDb} />
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label for="new-user-name" class="label">Username</label>
        <input id="new-user-name" bind:value={newUser.username} placeholder="db_user" autocomplete="off" class="input font-mono" />
      </div>
      <div>
        <label for="new-user-host" class="label">Host</label>
        <input id="new-user-host" bind:value={newUser.host_pattern} class="input font-mono" />
      </div>
    </div>
    <div>
      <label for="new-user-pass" class="label">Password</label>
      <input id="new-user-pass" bind:value={newUser.password} type="text" placeholder="Minimum 8 characters" autocomplete="off" class="input font-mono" />
      {#if newUser.password && newUser.password.length < 8}<p class="hint text-warning">At least 8 characters.</p>{/if}
    </div>
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (createOpen = false)}>Cancel</button>
    <button
      type="submit"
      form="create-user-form"
      class="btn btn-primary btn-sm"
      disabled={createUserBusy || !newUser.database_id || !newUser.username || newUser.password.length < 8}>
      {#if createUserBusy}<LoaderCircle size={14} class="animate-spin" /> Creating…{:else}Create user{/if}
    </button>
  {/snippet}
</Modal>

<Modal bind:open={addOpen} title="Add user to database" maxWidth="md">
  <form id="add-user-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); addUserToDatabaseGlobal(); }}>
    <p class="text-[13px] text-muted">Grants an existing MySQL user access to the selected database.</p>
    <Combobox
      id="add-user-db"
      label="Database"
      bind:value={addDbQuery}
      options={filteredAddDbOptions}
      optionLabel={dbLabel}
      optionKey={(d) => d.id}
      placeholder="Search a database…"
      emptyText="No database found"
      oninput={selectAddDbByQuery}
      onselect={chooseAddDb} />
    <Combobox
      id="add-user-user"
      label="User"
      bind:value={addUserQuery}
      options={filteredAddUserOptions}
      optionLabel={userKey}
      optionKey={userKey}
      placeholder={addDbId ? 'Search a user…' : 'Pick a database first'}
      emptyText="No available user"
      disabled={!addDbId}
      mono
      oninput={selectAddUserByQuery}
      onselect={chooseAddUser} />
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label for="add-user-host" class="label">Host</label>
        <input id="add-user-host" bind:value={addHost} class="input font-mono" />
      </div>
      <div>
        <label for="add-user-privs" class="label">Privileges</label>
        <select id="add-user-privs" bind:value={addPrivileges} class="input">
          <option value="ALL">ALL PRIVILEGES</option>
          <option value="SELECT,INSERT,UPDATE,DELETE">Read/Write</option>
          <option value="SELECT">Read only</option>
        </select>
      </div>
    </div>
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (addOpen = false)}>Cancel</button>
    <button type="submit" form="add-user-form" class="btn btn-primary btn-sm" disabled={addBusy || !addDbId || !addUsername}>
      {#if addBusy}<LoaderCircle size={14} class="animate-spin" /> Adding…{:else}Add to database{/if}
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
