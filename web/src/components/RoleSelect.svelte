<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';

  // Role picker for user create/edit: built-in and custom roles from
  // GET /roles. Roles the caller may not assign (more permissions than its
  // own, dangerous ones, user management for non-super admins) are shown
  // disabled; the API refuses them anyway.
  let { id, value = $bindable('user'), current = '' }: { id: string; value?: string; current?: string } = $props();

  type RoleRow = { name: string; description: string; built_in: boolean; assignable: boolean };
  let roles = $state<RoleRow[]>([]);

  const builtinLabel: Record<string, string> = { super_admin: 'Super Admin', admin: 'Admin', user: 'User' };
  const label = (r: RoleRow) => (r.built_in ? builtinLabel[r.name] || r.name : r.name);

  onMount(async () => {
    try {
      const d = await apiFetch(auth.token, 'GET', 'roles');
      roles = d?.roles || [];
    } catch {
      // Older API or no permission: offer the built-ins the caller had before.
      roles = [
        { name: 'user', description: '', built_in: true, assignable: true },
        { name: 'admin', description: '', built_in: true, assignable: auth.user?.role === 'super_admin' },
        { name: 'super_admin', description: '', built_in: true, assignable: auth.user?.role === 'super_admin' },
      ];
    }
  });
</script>

<select {id} bind:value class="input">
  {#each roles as r (r.name)}
    {#if r.assignable || r.name === current || r.name === value}
      <option value={r.name} disabled={!r.assignable && r.name !== current}>
        {label(r)}{r.built_in ? '' : ' (custom)'}
      </option>
    {/if}
  {/each}
</select>
