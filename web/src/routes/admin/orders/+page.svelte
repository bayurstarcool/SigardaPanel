<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import Modal from '$components/Modal.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';

  import {
    ShoppingCart, Search, X, Check, Loader2, Eye, CheckCircle2,
    XCircle, ShieldOff
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

  // Approve / reject confirmation (replaces window.confirm)
  let pending = $state<{ order: Order; action: 'approve' | 'reject' } | null>(null);
  let confirmOpen = $state(false);
  function ask(order: Order, action: 'approve' | 'reject') {
    pending = { order, action };
    confirmOpen = true;
  }
  function runPending() {
    if (!pending) return;
    if (pending.action === 'approve') approveOrder(pending.order);
    else rejectOrder(pending.order);
  }

  async function approveOrder(o: Order) {
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

  function fmtDate(d: string) {
    return d ? new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }) : '—';
  }

  function openDetail(o: Order) {
    detailOrder = o;
    showDetail = true;
    noteText = o.admin_notes || '';
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
    <div class="card">
      <EmptyState icon={ShieldOff as any} title="Access denied" description="Only super admins can manage orders." />
    </div>
  {:else}
    <PageHeader title="License orders" description={loading ? 'Loading…' : `${orders.length} order${orders.length !== 1 ? 's' : ''} ${statusFilter === 'all' ? 'total' : statusFilter}`} />

    <div class="card">
      {#if orders.length > 0 || statusFilter !== 'all'}
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div class="segmented overflow-x-auto">
            {#each statusOptions as s (s)}
              <button onclick={() => { statusFilter = s; loadOrders(); }} class="segment {statusFilter === s ? 'segment-active' : ''}">
                {s === 'all' ? 'All' : s.charAt(0).toUpperCase() + s.slice(1)}
              </button>
            {/each}
          </div>
          <div class="relative w-full sm:w-64">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input bind:value={search} type="text" aria-label="Search orders" placeholder="Search by user, plan, or ID…" class="input h-8 pr-8 pl-8" />
            {#if search}
              <button onclick={() => (search = '')} class="btn btn-ghost btn-xs btn-icon absolute top-1/2 right-0.5 -translate-y-1/2" aria-label="Clear search"><X size={12} /></button>
            {/if}
          </div>
        </div>
      {/if}

      {#if loading}
        <div class="space-y-3 p-4">
          {#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}
        </div>
      {:else if orders.length === 0 && statusFilter === 'all'}
        <EmptyState icon={ShoppingCart as any} title="No orders yet" description="License orders will appear here once users purchase plans." />
      {:else if filtered.length === 0}
        <EmptyState compact icon={Search as any} title="No results" description="No orders match your filters." actionLabel="Clear filters" onaction={() => { search = ''; statusFilter = 'all'; loadOrders(); }} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Order</th>
                <th>User</th>
                <th class="max-md:hidden">Plan</th>
                <th>Status</th>
                <th class="max-sm:hidden">Amount</th>
                <th class="max-lg:hidden">Payment</th>
                <th class="max-md:hidden">Date</th>
                <th class="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as o (o.id)}
                <tr>
                  <td>
                    <button onclick={() => openDetail(o)} class="cursor-pointer font-mono text-[13px] font-medium text-fg hover:text-accent">#{o.id}</button>
                  </td>
                  <td>
                    <div class="text-fg">{o.username || `User #${o.user_id}`}</div>
                    <div class="text-xs text-subtle md:hidden">{o.plan_name || `Plan #${o.plan_id}`}</div>
                  </td>
                  <td class="text-muted max-md:hidden">{o.plan_name || `Plan #${o.plan_id}`}</td>
                  <td><StatusPill status={statusVariant(o.status)} label={o.status} /></td>
                  <td class="font-medium whitespace-nowrap text-fg tabular-nums max-sm:hidden">{formatIDR(o.amount_idr)}</td>
                  <td class="text-muted capitalize max-lg:hidden">{o.payment_method || '—'}</td>
                  <td class="text-xs whitespace-nowrap text-subtle tabular-nums max-md:hidden">{fmtDate(o.created_at)}</td>
                  <td>
                    <div class="flex items-center justify-end gap-0.5">
                      <button onclick={() => openDetail(o)} class="btn btn-ghost btn-xs btn-icon" aria-label="View order #{o.id}" title="View"><Eye size={14} /></button>
                      {#if o.status === 'pending'}
                        <button onclick={() => ask(o, 'approve')} disabled={actionBusy === o.id} class="btn btn-ghost btn-xs text-success">
                          {#if actionBusy === o.id}<Loader2 size={12} class="animate-spin" />{:else}<CheckCircle2 size={12} />{/if}<span class="max-sm:hidden">Approve</span>
                        </button>
                        <button onclick={() => ask(o, 'reject')} disabled={actionBusy === o.id} class="btn btn-danger-ghost btn-xs">
                          {#if actionBusy === o.id}<Loader2 size={12} class="animate-spin" />{:else}<XCircle size={12} />{/if}<span class="max-sm:hidden">Reject</span>
                        </button>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Detail -->
    <Modal bind:open={showDetail} title={detailOrder ? `Order #${detailOrder.id}` : 'Order detail'} maxWidth="lg">
      {#if detailOrder}
        <dl class="grid grid-cols-2 gap-x-4 gap-y-4">
          <div>
            <dt class="stat-label">User</dt>
            <dd class="mt-1 text-sm text-fg">{detailOrder.username || `User #${detailOrder.user_id}`}</dd>
          </div>
          <div>
            <dt class="stat-label">Plan</dt>
            <dd class="mt-1 text-sm text-fg">{detailOrder.plan_name || `Plan #${detailOrder.plan_id}`}</dd>
          </div>
          <div>
            <dt class="stat-label">Status</dt>
            <dd class="mt-1"><StatusPill status={statusVariant(detailOrder.status)} label={detailOrder.status} /></dd>
          </div>
          <div>
            <dt class="stat-label">Amount</dt>
            <dd class="mt-1 text-sm font-semibold text-fg tabular-nums">{formatIDR(detailOrder.amount_idr)}</dd>
          </div>
          <div>
            <dt class="stat-label">Payment method</dt>
            <dd class="mt-1 text-sm text-fg capitalize">{detailOrder.payment_method || '—'}</dd>
          </div>
          <div>
            <dt class="stat-label">Created</dt>
            <dd class="mt-1 text-sm text-fg">{detailOrder.created_at ? new Date(detailOrder.created_at).toLocaleString('id-ID') : '—'}</dd>
          </div>
        </dl>
        {#if detailOrder.notes}
          <div class="mt-4">
            <div class="stat-label">Customer notes</div>
            <p class="mt-1 text-[13px] whitespace-pre-line text-muted">{detailOrder.notes}</p>
          </div>
        {/if}
        <div class="mt-5 border-t border-line pt-4">
          <label for="order-notes" class="label">Admin notes</label>
          <textarea id="order-notes" bind:value={noteText} rows={3} class="input" placeholder="Add admin notes..."></textarea>
        </div>
      {/if}
      {#snippet footer()}
        <button onclick={() => { showDetail = false; detailOrder = null; }} class="btn btn-ghost btn-sm">Close</button>
        <button onclick={() => { if (detailOrder) saveNote(detailOrder); }} disabled={noteBusy} class="btn btn-secondary btn-sm">
          {#if noteBusy}<Loader2 size={14} class="animate-spin" />{:else}<Check size={14} />{/if} Save notes
        </button>
        {#if detailOrder?.status === 'pending'}
          <button onclick={() => { if (detailOrder) ask(detailOrder, 'reject'); showDetail = false; detailOrder = null; }} class="btn btn-danger-ghost btn-sm">
            <XCircle size={14} /> Reject
          </button>
          <button onclick={() => { if (detailOrder) ask(detailOrder, 'approve'); showDetail = false; detailOrder = null; }} class="btn btn-primary btn-sm">
            <CheckCircle2 size={14} /> Approve
          </button>
        {/if}
      {/snippet}
    </Modal>

    <ConfirmDialog
      bind:open={confirmOpen}
      title={pending?.action === 'approve' ? 'Approve order' : 'Reject order'}
      message={pending
        ? pending.action === 'approve'
          ? `Approve order #${pending.order.id} for ${pending.order.username || 'user'}? Plan will be auto-assigned.`
          : `Reject order #${pending.order.id}?`
        : ''}
      confirmLabel={pending?.action === 'approve' ? 'Approve' : 'Reject'}
      destructive={pending?.action === 'reject'}
      onconfirm={runPending}
    />
  {/if}
</DashboardLayout>
