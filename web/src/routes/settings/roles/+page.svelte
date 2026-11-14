<script lang="ts">
  import { onMount } from 'svelte';
  import { auth, hasPermission } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { ShieldCheck, Plus, Pencil, Trash2, TriangleAlert, X, Save, Lock } from 'lucide-svelte';

  type Perm = { name: string; category: string; description: string; dangerous: boolean; min_role: string };
  type Role = { id: number; name: string; description: string; built_in: boolean; permissions: string[]; dangerous: boolean; users: number };

  let perms = $state<Perm[]>([]);
  let roles = $state<Role[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  const canManage = $derived(hasPermission('role.manage'));

  // Editor: null = closed, id 0 = new role.
  let editing = $state<{ id: number; name: string; description: string; selected: Record<string, boolean> } | null>(null);
  let saving = $state(false);
  let toDelete = $state<Role | null>(null);
  let confirmDelete = $state(false);

  const categories = $derived([...new Set(perms.map((p) => p.category))]);
  const byName = $derived(Object.fromEntries(perms.map((p) => [p.name, p])));
  const builtinLabel: Record<string, string> = { super_admin: 'Super Admin', admin: 'Admin', user: 'User' };

  async function load() {
    loading = true;
    loadError = '';
    try {
      const [p, r] = await Promise.all([apiFetch(auth.token, 'GET', 'rbac/permissions'), apiFetch(auth.token, 'GET', 'roles')]);
      perms = p?.permissions || [];
      roles = r?.roles || [];
    } catch (e: any) {
      loadError = e.message || 'Failed to load roles';
    } finally {
      loading = false;
    }
  }
  onMount(load);

  function startNew() {
    editing = { id: 0, name: '', description: '', selected: {} };
  }
  function startEdit(r: Role) {
    editing = { id: r.id, name: r.name, description: r.description, selected: Object.fromEntries(r.permissions.map((p) => [p, true])) };
  }

  async function save() {
    if (!editing) return;
    saving = true;
    const body = {
      name: editing.name.trim(),
      description: editing.description.trim(),
      permissions: Object.keys(editing.selected).filter((k) => editing!.selected[k]),
    };
    try {
      if (editing.id === 0) await apiFetch(auth.token, 'POST', 'roles', body);
      else await apiFetch(auth.token, 'PATCH', `roles/${editing.id}`, body);
      toasts.success(editing.id === 0 ? `Role ${body.name} created` : `Role ${body.name} saved`);
      editing = null;
      await load();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to save the role');
    } finally {
      saving = false;
    }
  }

  async function remove() {
    const r = toDelete;
    toDelete = null;
    if (!r) return;
    try {
      await apiFetch(auth.token, 'DELETE', `roles/${r.id}`);
      toasts.success(`Role ${r.name} deleted`);
      await load();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to delete the role');
    }
  }

  const selectedDangerous = $derived(editing ? Object.keys(editing.selected).some((k) => editing!.selected[k] && byName[k]?.dangerous) : false);
</script>

<DashboardLayout active="Settings">
  <PageHeader title="Roles & permissions" description="Built-in roles are fixed; custom roles combine permissions. Ownership still applies: a permission never opens other tenants' sites unless it says so.">
    {#snippet actions()}
      {#if canManage && !editing}
        <button onclick={startNew} class="btn btn-primary btn-sm"><Plus size={14} /> New role</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="card p-5"><div class="skeleton h-4 w-1/3"></div></div>
  {:else if loadError}
    <div class="card p-5 text-sm text-danger">{loadError}</div>
  {:else}
    {#if editing}
      <div class="card mb-6">
        <div class="card-header">
          <h2 class="card-title">{editing.id === 0 ? 'New role' : `Edit ${editing.name}`}</h2>
          <button onclick={() => (editing = null)} class="btn btn-ghost btn-icon btn-sm" aria-label="Close"><X size={14} /></button>
        </div>
        <div class="card-body space-y-4">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label for="role-name" class="label">Name</label>
              <input id="role-name" bind:value={editing.name} class="input font-mono" placeholder="support" readonly={editing.id !== 0} />
              <p class="mt-1 text-xs text-subtle">Lowercase letters, digits, _ or -.</p>
            </div>
            <div>
              <label for="role-desc" class="label">Description</label>
              <input id="role-desc" bind:value={editing.description} class="input" placeholder="Helpdesk: reads every site, signs in as customers" />
            </div>
          </div>
          {#if selectedDangerous}
            <p class="flex items-center gap-2 rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-[13px] text-warning">
              <TriangleAlert size={14} /> This role holds dangerous permissions (root on servers or access across every tenant).
            </p>
          {/if}
          {#each categories as cat (cat)}
            <fieldset>
              <legend class="mb-2 text-xs font-semibold uppercase tracking-wide text-subtle">{cat}</legend>
              <div class="grid gap-2 sm:grid-cols-2">
                {#each perms.filter((p) => p.category === cat) as p (p.name)}
                  <label class="flex cursor-pointer items-start gap-2 rounded-md border border-line p-2.5 hover:bg-surface-2">
                    <input type="checkbox" class="checkbox mt-0.5" bind:checked={editing.selected[p.name]} />
                    <span class="min-w-0">
                      <span class="flex flex-wrap items-center gap-1.5 font-mono text-[12px] text-fg">
                        {p.name}
                        {#if p.dangerous}<span class="badge badge-warning"><TriangleAlert size={10} /> dangerous</span>{/if}
                      </span>
                      <span class="block text-xs text-muted">{p.description}</span>
                    </span>
                  </label>
                {/each}
              </div>
            </fieldset>
          {/each}
          <div class="flex gap-2">
            <button onclick={save} disabled={saving || !editing.name.trim()} class="btn btn-primary btn-sm"><Save size={14} /> {saving ? 'Saving…' : 'Save role'}</button>
            <button onclick={() => (editing = null)} class="btn btn-ghost btn-sm">Cancel</button>
          </div>
        </div>
      </div>
    {/if}

    <div class="space-y-4">
      {#each roles as r (r.id)}
        <div class="card">
          <div class="card-header">
            <div class="flex min-w-0 items-center gap-2">
              {#if r.built_in}<Lock size={14} class="text-subtle" />{:else}<ShieldCheck size={14} class="text-accent" />{/if}
              <h3 class="card-title">{r.built_in ? builtinLabel[r.name] || r.name : r.name}</h3>
              <span class="badge">{r.built_in ? 'built-in' : 'custom'}</span>
              {#if r.dangerous}<span class="badge badge-warning"><TriangleAlert size={10} /> dangerous</span>{/if}
              <span class="text-xs text-subtle tabular-nums">{r.users} user{r.users === 1 ? '' : 's'}</span>
            </div>
            {#if canManage && !r.built_in}
              <div class="flex gap-1">
                <button onclick={() => startEdit(r)} class="btn btn-ghost btn-icon btn-sm" aria-label="Edit {r.name}"><Pencil size={14} /></button>
                <button onclick={() => { toDelete = r; confirmDelete = true; }} class="btn btn-ghost btn-icon btn-sm text-danger" aria-label="Delete {r.name}" disabled={r.users > 0} title={r.users > 0 ? 'Assigned to users' : 'Delete'}><Trash2 size={14} /></button>
              </div>
            {/if}
          </div>
          <div class="card-body">
            {#if r.description}<p class="mb-3 text-[13px] text-muted">{r.description}</p>{/if}
            {#if r.permissions.length === 0}
              <p class="text-xs text-subtle">Only the baseline every user has: their own and assigned sites.</p>
            {:else}
              <div class="flex flex-wrap gap-1.5">
                {#each r.permissions as p (p)}
                  <span class="badge font-mono {byName[p]?.dangerous ? 'badge-warning' : ''}" title={byName[p]?.description || ''}>{p}</span>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</DashboardLayout>

<ConfirmDialog
  bind:open={confirmDelete}
  title="Delete role"
  message={toDelete ? `Delete the role ${toDelete.name}? This cannot be undone.` : ''}
  confirmLabel="Delete"
  destructive
  onconfirm={remove}
/>
