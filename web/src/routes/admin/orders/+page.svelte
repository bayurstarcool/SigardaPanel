<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import Modal from '$components/Modal.svelte';
  import StatusPill from '$components/StatusPill.svelte';

  import {
    ShoppingCart, Search, X, Check, Loader2, Eye, CheckCircle2,
    XCircle, Clock, Filter
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type Order = {
    id: number;
    user_id: number;
    username?: string;
    plan_id: number;
    plan_name?: string;
    status: string;
    amount_idr: number;
    payment_method: string;
    notes?: string;
    admin_notes?: string;
    created_at: string;
    updated_at: string;
  };

  let orders = $state<Order[]>([]);
  let loading = $state(true);
  let search = $state('');
  let statusFilter = $state('all');
  let detailOrder = $state<Order | null>(null);
  let showDetail = $state(false);
  let noteText = $state('');
  let noteBusy = $state(false);
  let actionBusy = $state<number | null>(null);

  const formatIDR = (v: number) =>
    new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', minimumFractionDigits: 0 }).format(v);

  const statusOptions = ['all', 'pending', 'approved', 'rejected', 'expired'];

  const filtered = $derived.by(() => {
    let list = orders;
    if (statusFilter !== 'all') {
      list = list.filter(o => o.status === statusFilter);
    }
    const q = search.toLowerCase().trim();
    if (q) {
      list = list.filter(o =>
        (o.username || '').toLowerCase().includes(q) ||
        (o.plan_name || '').toLowerCase().includes(q) ||
        String(o.id).includes(q)
      );
    }
    return list;
  });

  async function loadOrders() {
    loading = true;
    try {
      const params = statusFilter !== 'all' ? `?status=${statusFilter}` : '';
      const d = await apiFetch(auth.token, 'GET', `orders${params}`);
      orders = d?.orders || d || [];
    } catch (e: any) {
      if (e.message !== 'session expired') toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (!hasRole(auth.user?.role, 'super_admin')) return;
    loadOrders();
  });

  async function approveOrder(o: Order) {
    if (!confirm(`Approve order #${o.id} for ${o.username || 'user'}? Plan will be auto-assigned.`)) return;
    actionBusy = o.id;
    try {
      await apiFetch(auth.token, 'PATCH', `orders/${o.id}`, { status: 'approved', admin_notes: noteText });
      toasts.success(`Order #${o.id} approved — plan assigned to user`);
      noteText = '';
      await loadOrders();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      actionBusy = null;
    }
  }

  async function rejectOrder(o: Order) {
    if (!confirm(`Reject order #${o.id}?`)) return;
    actionBusy = o.id;
    try {
      await apiFetch(auth.token, 'PATCH', `orders/${o.id}`, { status: 'rejected', admin_notes: noteText });
      toasts.success(`Order #${o.id} rejected`);
      noteText = '';
      await loadOrders();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      actionBusy = null;
    }
  }

  async function saveNote(o: Order) {
    noteBusy = true;
    try {
      await apiFetch(auth.token, 'PATCH', `orders/${o.id}`, { status: o.status, admin_notes: noteText });
      o.admin_notes = noteText;
      toasts.success('Notes saved');
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      noteBusy = false;
    }
  }

  function statusVariant(s: string) {
    switch (s) {
      case 'approved': return 'active';
      case 'pending': return 'pending';
      case 'rejected': return 'failed';
      case 'expired': return 'expired';
      default: return s;
    }
  }
</script>

<DashboardLayout active="Orders">
  {#if !hasRole(auth.user?.role, 'super_admin')}
    <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-20 px-6 text-center">
      <ShoppingCart size={48} class="mx-auto mb-4 text-slate-600 opacity-40" />
      <h2 class="text-lg font-semibold text-slate-300 mb-2">Access Denied</h2>
      <p class="text-sm text-slate-500">Only super admins can manage orders.</p>
    </div>
  {:else}
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">License Orders</h1>
        <p class="text-sm text-slate-500 mt-1">
          {loading ? 'Loading...' : `${orders.length} order${orders.length !== 1 ? 's' : ''} total`}
        </p>
      </div>
    </div>

    <!-- Filters -->
    {#if !loading && orders.length > 0}
      <div class="flex flex-col sm:flex-row gap-3 mb-5">
        <div class="relative flex-1">
          <Search size={16} class="absolute left-4 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
          <input bind:value={search} type="text" placeholder="Search by user, plan, or ID..."
            class="w-full bg-white/[0.03] border border-white/[0.07] rounded-xl pl-10 pr-10 py-2.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-sky-400/30 focus:ring-1 focus:ring-sky-400/10 transition" />
          {#if search}
            <button onclick={() => search = ''} class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1"><X size={14} /></button>
          {/if}
        </div>
        <div class="flex items-center gap-1.5 bg-white/[0.03] border border-white/[0.07] rounded-xl p-1">
          {#each statusOptions as s}
            <button onclick={() => { statusFilter = s; loadOrders(); }}
              class="px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer border-0 {statusFilter === s ? 'bg-sky-500/15 text-sky-400' : 'text-slate-500 hover:text-slate-300 hover:bg-white/[0.04]'}">
              {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Loading skeleton -->
    {#if loading}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
        <div class="p-5 space-y-0">
          {#each Array(4) as _}
            <div class="flex items-center gap-4 py-3.5 border-b border-white/[0.03] last:border-0">
              <div class="flex-1 space-y-2">
                <div class="h-3 w-40 bg-white/[0.06] rounded-lg animate-pulse"></div>
                <div class="h-2.5 w-56 bg-white/[0.04] rounded-lg animate-pulse"></div>
              </div>
              <div class="h-5 w-20 bg-white/[0.05] rounded-full animate-pulse"></div>
            </div>
          {/each}
        </div>
      </div>
    {:else if orders.length === 0}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
        <div class="w-16 h-16 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-5"><ShoppingCart size={28} class="text-slate-600" /></div>
        <h2 class="text-lg font-semibold text-slate-300 mb-2">No orders yet</h2>
        <p class="text-sm text-slate-500 mb-6 max-w-sm mx-auto">License orders will appear here once users purchase plans.</p>
      </div>
    {:else if filtered.length === 0}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
        <Search size={28} class="text-slate-600 mx-auto mb-3" />
        <h2 class="text-lg font-semibold text-slate-300 mb-2">No results</h2>
        <p class="text-sm text-slate-500">No orders match your filters.</p>
        <button onclick={() => { search = ''; statusFilter = 'all'; loadOrders(); }}
          class="mt-4 inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0"><X size={14} /> Clear filters</button>
      </div>
    {:else}
      <!-- Desktop table -->
      <div class="hidden md:block bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead>
              <tr class="text-left">
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Order</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">User</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Plan</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Status</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Amount</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Payment</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Date</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as o (o.id)}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-sm text-slate-200 font-medium">#{o.id}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-sm text-slate-200">{o.username || `User #${o.user_id}`}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-sm text-slate-300">{o.plan_name || `Plan #${o.plan_id}`}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <StatusPill status={statusVariant(o.status)} />
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-sm text-emerald-400 font-semibold">{formatIDR(o.amount_idr)}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-xs text-slate-400 bg-white/[0.04] px-2 py-1 rounded-md capitalize">{o.payment_method || '—'}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-xs text-slate-500">{o.created_at ? new Date(o.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }) : '—'}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <div class="flex items-center gap-1">
                      <button onclick={() => { detailOrder = o; showDetail = true; noteText = o.admin_notes || ''; }}
                        class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5 cursor-pointer border-0 bg-transparent"><Eye size={12} /> View</button>
                      {#if o.status === 'pending'}
                        <button onclick={() => approveOrder(o)} disabled={actionBusy === o.id}
                          class="inline-flex items-center gap-1 text-xs text-emerald-400/70 hover:text-emerald-400 transition px-2 py-1 rounded-lg hover:bg-emerald-400/5 cursor-pointer border-0 bg-transparent disabled:opacity-40">
                          {#if actionBusy === o.id}<Loader2 size={11} class="animate-spin" />{:else}<CheckCircle2 size={12} />{/if} Approve
                        </button>
                        <button onclick={() => rejectOrder(o)} disabled={actionBusy === o.id}
                          class="inline-flex items-center gap-1 text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent disabled:opacity-40">
                          {#if actionBusy === o.id}<Loader2 size={11} class="animate-spin" />{:else}<XCircle size={12} />{/if} Reject
                        </button>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile cards -->
      <div class="md:hidden space-y-3">
        {#each filtered as o (o.id)}
          <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl p-4 space-y-3">
            <div class="flex items-start justify-between">
              <div>
                <div class="text-sm font-medium text-slate-200">#{o.id} — {o.username || `User #${o.user_id}`}</div>
                <div class="text-xs text-slate-500 mt-0.5">{o.plan_name || `Plan #${o.plan_id}`}</div>
              </div>
              <StatusPill status={statusVariant(o.status)} />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-emerald-400 font-semibold text-sm">{formatIDR(o.amount_idr)}</span>
              <span class="text-[0.65rem] text-slate-500">{o.created_at ? new Date(o.created_at).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }) : '—'}</span>
            </div>
            <div class="flex items-center gap-1">
              <button onclick={() => { detailOrder = o; showDetail = true; noteText = o.admin_notes || ''; }}
                class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5 cursor-pointer border-0 bg-transparent"><Eye size={12} /></button>
              {#if o.status === 'pending'}
                <button onclick={() => approveOrder(o)} disabled={actionBusy === o.id}
                  class="inline-flex items-center gap-1 text-xs text-emerald-400/70 hover:text-emerald-400 transition px-2 py-1 rounded-lg hover:bg-emerald-400/5 cursor-pointer border-0 bg-transparent disabled:opacity-40">
                  {#if actionBusy === o.id}<Loader2 size={11} class="animate-spin" />{:else}<CheckCircle2 size={12} />{/if} Approve
                </button>
                <button onclick={() => rejectOrder(o)} disabled={actionBusy === o.id}
                  class="inline-flex items-center gap-1 text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent disabled:opacity-40">
                  {#if actionBusy === o.id}<Loader2 size={11} class="animate-spin" />{:else}<XCircle size={12} />{/if} Reject
                </button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Detail Modal -->
    <Modal bind:open={showDetail} title={detailOrder ? `Order #${detailOrder.id}` : 'Order Detail'} maxWidth="lg">
      {#if detailOrder}
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <div class="text-xs text-slate-500 mb-1">User</div>
              <div class="text-sm text-slate-200">{detailOrder.username || `User #${detailOrder.user_id}`}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Plan</div>
              <div class="text-sm text-slate-200">{detailOrder.plan_name || `Plan #${detailOrder.plan_id}`}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Status</div>
              <StatusPill status={statusVariant(detailOrder.status)} />
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Amount</div>
              <div class="text-sm text-emerald-400 font-semibold">{formatIDR(detailOrder.amount_idr)}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Payment Method</div>
              <div class="text-sm text-slate-200 capitalize">{detailOrder.payment_method || '—'}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Created</div>
              <div class="text-sm text-slate-200">{detailOrder.created_at ? new Date(detailOrder.created_at).toLocaleString('id-ID') : '—'}</div>
            </div>
          </div>

          <!-- Admin notes -->
          <div>
            <label for="order-notes" class="block text-xs text-slate-500 mb-1.5">Admin Notes</label>
            <textarea id="order-notes" bind:value={noteText} rows={3}
              class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition"
              placeholder="Add admin notes..."></textarea>
          </div>
        </div>
      {/if}
      {#snippet footer()}
        <button onclick={() => { showDetail = false; detailOrder = null; }}
          class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Close</button>
        <button onclick={() => { if (detailOrder) saveNote(detailOrder); }} disabled={noteBusy}
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500/15 text-sky-400 hover:bg-sky-500/25 transition disabled:opacity-40 cursor-pointer border-0">
          {#if noteBusy}<Loader2 size={14} class="animate-spin" />{:else}<Check size={14} />{/if} Save Notes
        </button>
        {#if detailOrder?.status === 'pending'}
          <button onclick={() => { if (detailOrder) approveOrder(detailOrder); showDetail = false; detailOrder = null; }}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-emerald-500/15 text-emerald-400 hover:bg-emerald-500/25 transition cursor-pointer border-0">
            <CheckCircle2 size={14} /> Approve
          </button>
          <button onclick={() => { if (detailOrder) rejectOrder(detailOrder); showDetail = false; detailOrder = null; }}
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-red-500/15 text-red-400 hover:bg-red-500/25 transition cursor-pointer border-0">
            <XCircle size={14} /> Reject
          </button>
        {/if}
      {/snippet}
    </Modal>
  {/if}
</DashboardLayout>
