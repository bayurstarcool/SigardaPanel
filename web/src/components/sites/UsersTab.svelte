<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { Users, Loader2, DollarSign, X, Check } from 'lucide-svelte';
  import Modal from '$components/Modal.svelte';

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
</script>

<div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
  <div class="flex items-center justify-between px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
    <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Users size={13} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">Users with Access</span><span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">{siteUsers.length}</span></div>
  </div>
  {#if usersLoading}
    <div class="divide-y divide-white/5">{#each Array(3) as _}<div class="px-5 py-4 flex items-center gap-3"><div class="w-8 h-8 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="space-y-1"><div class="h-3 w-24 bg-white/[0.04] rounded animate-pulse"></div></div></div>{/each}</div>
  {:else if siteUsers.length === 0}
    <div class="flex flex-col items-center justify-center py-14"><Users size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">No users assigned</p><p class="text-xs text-slate-600">No users have access to this site</p></div>
  {:else}
    <div class="divide-y divide-white/[0.04]">
      {#each siteUsers as u}
        <div class="flex items-center justify-between px-5 py-3.5 hover:bg-white/[0.02] transition">
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-sky-400/20 to-indigo-500/20 flex items-center justify-center text-xs font-bold text-sky-300">{(u.username || 'U').charAt(0).toUpperCase()}</div>
            <div><div class="text-sm text-slate-200 font-medium">{u.username}</div><div class="text-xs text-slate-500">{u.email || '—'}</div></div>
          </div>
          <div class="flex items-center gap-3">
            <!-- Plan cell -->
            <button onclick={() => openAssign(u)}
              class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full transition cursor-pointer border-0
              {u.plan_id ? 'bg-sky-500/10 text-sky-400 hover:bg-sky-500/20' : 'bg-white/[0.04] text-slate-500 hover:bg-white/[0.06] hover:text-slate-400'}"
              title="Click to change plan">
              <DollarSign size={10} />
              {u.plan_name || 'Free'}
            </button>
            <span class="text-xs px-2 py-0.5 rounded-full font-medium {u.permission === 'admin' ? 'bg-amber-400/10 text-amber-400' : u.permission === 'write' ? 'bg-sky-400/10 text-sky-400' : 'bg-slate-400/10 text-slate-400'}">{u.permission}</span>
            <button onclick={() => unassignUser(u.user_id)} class="text-xs text-red-400/60 hover:text-red-400 transition p-1 rounded-lg hover:bg-red-400/10">Revoke</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Assign Plan Modal -->
{#if assignTarget}
  <Modal open={true} title="Assign Plan — {assignTarget.username}" onclose={() => assignTarget = null} maxWidth="md">
    <div class="space-y-4">
      <p class="text-sm text-slate-400">Select a plan for <strong class="text-slate-200">{assignTarget.username}</strong>.</p>
      {#if plans.length === 0}
        <div class="text-sm text-slate-500 py-4 text-center">No plans available. Create plans in Admin → Plans.</div>
      {:else}
        <div class="space-y-2 max-h-60 overflow-y-auto">
          <button onclick={() => assignPlanId = null}
            class="w-full flex items-center justify-between p-3 rounded-xl border transition cursor-pointer text-left
            {assignPlanId === null ? 'bg-sky-500/10 border-sky-500/30 text-sky-400' : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:border-white/[0.12]'}">
            <div class="flex items-center gap-2">
              <DollarSign size={14} />
              <span class="text-sm font-medium">Free (No Plan)</span>
            </div>
            {#if assignPlanId === null}<Check size={14} class="text-sky-400" />{/if}
          </button>
          {#each plans as plan}
            <button onclick={() => assignPlanId = plan.id}
              class="w-full flex items-center justify-between p-3 rounded-xl border transition cursor-pointer text-left
              {assignPlanId === plan.id ? 'bg-sky-500/10 border-sky-500/30 text-sky-400' : 'bg-white/[0.03] border-white/[0.07] text-slate-400 hover:border-white/[0.12]'}">
              <div class="flex items-center gap-3">
                <DollarSign size={14} />
                <div>
                  <span class="text-sm font-medium">{plan.name}</span>
                  <div class="text-xs text-slate-500">{formatIDR(plan.price_idr)} · {plan.billing_cycle}</div>
                </div>
              </div>
              {#if assignPlanId === plan.id}<Check size={14} class="text-sky-400 shrink-0" />{/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>
    {#snippet footer()}
      <button onclick={() => assignTarget = null} class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
      <button onclick={savePlan} disabled={assignBusy}
        class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
        {#if assignBusy}<Loader2 size={14} class="animate-spin" /> Saving...{:else}<Check size={14} /> Assign Plan{/if}
      </button>
    {/snippet}
  </Modal>
{/if}
