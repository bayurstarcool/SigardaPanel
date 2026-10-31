<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { Users, LoaderCircle, Check, CreditCard } from 'lucide-svelte';
  import Modal from '$components/Modal.svelte';
  import EmptyState from '$components/EmptyState.svelte';

  let { siteId }: { siteId: number } = $props();

  let siteUsers = $state<any[]>([]);
  let usersLoading = $state(false);
  let usersLoaded = $state(false);

  // Plan assignment state
  let plans = $state<any[]>([]);
  let assignTarget = $state<any>(null);
  let assignPlanId = $state<number | null>(null);
  let assignBusy = $state(false);

  $effect(() => { if (!usersLoading && siteUsers.length === 0 && usersLoaded === false) { usersLoaded = true; loadUsers(); } });

  const formatIDR = (v: number) =>
    new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', minimumFractionDigits: 0 }).format(v);

  async function loadUsers() {
    usersLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/users`); siteUsers = d?.users || []; }
    catch {} finally { usersLoading = false; }
  }

  async function loadPlans() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'plans');
      plans = d?.plans || d || [];
    } catch {}
  }

  async function unassignUser(userId: number) {
    if (!confirm('Revoke this user access?')) return;
    try { await apiFetch(auth.token, 'DELETE', `users/${userId}/sites/${siteId}`); await loadUsers(); }
    catch (e: any) { toasts.error(e.message); }
  }

  function openAssign(u: any) {
    assignTarget = u;
    assignPlanId = u.plan_id || null;
    if (plans.length === 0) loadPlans();
  }

  async function savePlan() {
    if (!assignTarget) return;
    assignBusy = true;
    try {
      await apiFetch(auth.token, 'PATCH', `users/${assignTarget.user_id}`, {
        plan_id: assignPlanId,
      });
      assignTarget.plan_id = assignPlanId;
      const plan = plans.find(p => p.id === assignPlanId);
      assignTarget.plan_name = plan?.name || (assignPlanId ? `Plan #${assignPlanId}` : null);
      toasts.success(`Plan updated for ${assignTarget.username}`);
      assignTarget = null;
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      assignBusy = false;
    }
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
            <button onclick={() => openAssign(u)} class="badge cursor-pointer transition-colors hover:ring-line-strong {u.plan_id ? 'badge-accent' : ''}" title="Change plan">
              <CreditCard size={11} /> {u.plan_name || 'Free'}
            </button>
            <span class="badge {permBadge[u.permission] ?? ''}">{u.permission}</span>
            <button onclick={() => unassignUser(u.user_id)} class="btn btn-danger-ghost btn-xs">Revoke</button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<!-- Assign plan modal -->
{#if assignTarget}
  <Modal open={true} title="Assign plan — {assignTarget.username}" onclose={() => (assignTarget = null)} maxWidth="md">
    <p class="mb-4 text-[13px] text-muted">Select a plan for <strong class="font-medium text-fg">{assignTarget.username}</strong>.</p>
    {#if plans.length === 0}
      <p class="py-4 text-center text-[13px] text-subtle">No plans available. Create plans in Admin → Plans.</p>
    {:else}
      <div class="max-h-60 space-y-1.5 overflow-y-auto">
        <button onclick={() => (assignPlanId = null)}
          class="flex w-full cursor-pointer items-center justify-between rounded-lg border p-3 text-left transition-colors {assignPlanId === null ? 'border-accent bg-accent/10' : 'border-line hover:border-line-strong hover:bg-surface-2'}">
          <span class="text-sm font-medium {assignPlanId === null ? 'text-accent' : 'text-fg'}">Free (no plan)</span>
          {#if assignPlanId === null}<Check size={14} class="text-accent" />{/if}
        </button>
        {#each plans as plan (plan.id)}
          <button onclick={() => (assignPlanId = plan.id)}
            class="flex w-full cursor-pointer items-center justify-between rounded-lg border p-3 text-left transition-colors {assignPlanId === plan.id ? 'border-accent bg-accent/10' : 'border-line hover:border-line-strong hover:bg-surface-2'}">
            <div>
              <div class="text-sm font-medium {assignPlanId === plan.id ? 'text-accent' : 'text-fg'}">{plan.name}</div>
              <div class="text-xs text-subtle tabular-nums">{formatIDR(plan.price_idr)} · {plan.billing_cycle}</div>
            </div>
            {#if assignPlanId === plan.id}<Check size={14} class="shrink-0 text-accent" />{/if}
          </button>
        {/each}
      </div>
    {/if}
    {#snippet footer()}
      <button onclick={() => (assignTarget = null)} class="btn btn-secondary btn-sm">Cancel</button>
      <button onclick={savePlan} disabled={assignBusy} class="btn btn-primary btn-sm">
        {#if assignBusy}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}<Check size={14} /> Assign plan{/if}
      </button>
    {/snippet}
  </Modal>
{/if}
