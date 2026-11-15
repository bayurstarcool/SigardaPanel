<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { Users } from 'lucide-svelte';
  import EmptyState from '$components/EmptyState.svelte';

  let { siteId }: { siteId: number } = $props();

  let siteUsers = $state<any[]>([]);
  let usersLoading = $state(false);
  let usersLoaded = $state(false);

  $effect(() => { if (!usersLoading && siteUsers.length === 0 && usersLoaded === false) { usersLoaded = true; loadUsers(); } });

  async function loadUsers() {
    usersLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/users`); siteUsers = d?.users || []; }
    catch (e: any) { if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load'); } finally { usersLoading = false; }
  }

  async function unassignUser(userId: number) {
    if (!confirm('Revoke this user access?')) return;
    try { await apiFetch(auth.token, 'DELETE', `users/${userId}/sites/${siteId}`); await loadUsers(); }
    catch (e: any) { toasts.error(e.message); }
  }

  const permBadge: Record<string, string> = { admin: 'badge-warning', write: 'badge-info' };
</script>

<div class="card">
  <div class="card-header">
    <div class="flex items-center gap-2">
      <Users size={16} class="text-subtle" />
      <h3 class="card-title">Users with access</h3>
      <span class="badge tabular-nums">{siteUsers.length}</span>
    </div>
  </div>
  {#if usersLoading}
    <div class="space-y-3 p-4">{#each Array(3) as _, i (i)}<div class="skeleton h-10"></div>{/each}</div>
  {:else if siteUsers.length === 0}
    <EmptyState compact icon={Users} title="No users assigned" description="No users have access to this site." />
  {:else}
    <ul class="divide-y divide-line">
      {#each siteUsers as u (u.user_id)}
        <li class="flex flex-wrap items-center justify-between gap-3 px-5 py-3 transition-colors duration-100 hover:bg-surface-2/60">
          <div class="flex min-w-0 items-center gap-3">
            <span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent/10 text-xs font-semibold text-accent">{(u.username || 'U').charAt(0).toUpperCase()}</span>
            <div class="min-w-0">
              <div class="truncate text-sm font-medium text-fg">{u.username}</div>
              <div class="truncate text-xs text-subtle">{u.email || '—'}</div>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span class="badge {permBadge[u.permission] ?? ''}">{u.permission}</span>
            <button onclick={() => unassignUser(u.user_id)} class="btn btn-danger-ghost btn-xs">Revoke</button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
