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
    DollarSign, Plus, Pencil, Trash2, Loader2, X, Check,
    Search, Wand2, Eye, ShieldOff
  } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type Plan = {
    id: number;
    name: string;
    slug: string;
    price_idr: number;
    billing_cycle: string;
    is_active: boolean;
    features: string;
    created_at: string;
    user_count?: number;
  };

  let plans = $state<Plan[]>([]);
  let loading = $state(true);
  let showCreate = $state(false);
  let editingPlan = $state<Plan | null>(null);
  let busy = $state(false);
  let search = $state('');
  let detailPlan = $state<Plan | null>(null);
  let showDetail = $state(false);

  const formDefault = {
    name: '',
    slug: '',
    price_idr: 0,
    billing_cycle: 'monthly',
    is_active: true,
    features: '[]',
  };
  let form = $state({ ...formDefault });

  const filtered = $derived.by(() => {
    const q = search.toLowerCase().trim();
    if (!q) return plans;
    return plans.filter(p =>
      (p.name || '').toLowerCase().includes(q) ||
      (p.slug || '').toLowerCase().includes(q)
    );
  });

  const formatIDR = (v: number) =>
    new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', minimumFractionDigits: 0 }).format(v);

  function autoSlug(name: string) {
    return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  }

  async function loadPlans() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', 'plans');
      plans = d?.plans || d || [];
    } catch (e: any) {
      if (e.message !== 'session expired') toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (!hasRole(auth.user?.role, 'super_admin')) return;
    loadPlans();
  });

  async function savePlan() {
    busy = true;
    try {
      // Validate features JSON
      let featuresArr: string[];
      try {
        const parsed = JSON.parse(form.features);
        featuresArr = Array.isArray(parsed) ? parsed : [];
      } catch {
        featuresArr = form.features.split(',').map(s => s.trim()).filter(Boolean);
      }
      const payload = {
        ...form,
        features: JSON.stringify(featuresArr),
      };
      if (editingPlan) {
        await apiFetch(auth.token, 'PATCH', `plans/${editingPlan.id}`, payload);
        toasts.success('Plan updated');
      } else {
        await apiFetch(auth.token, 'POST', 'plans', payload);
        toasts.success('Plan created');
      }
      showCreate = false;
      editingPlan = null;
      form = { ...formDefault };
      await loadPlans();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      busy = false;
    }
  }

  let confirmDelete = $state<Plan | null>(null);
  let confirmDeleteOpen = $state(false);
  function askDelete(p: Plan) { confirmDelete = p; confirmDeleteOpen = true; }

  async function deletePlan(p: Plan) {
    try {
      await apiFetch(auth.token, 'DELETE', `plans/${p.id}`);
      plans = plans.filter(x => x.id !== p.id);
      toasts.success(`Plan "${p.name}" deleted`);
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  function openEdit(p: Plan) {
    editingPlan = p;
    form = {
      name: p.name,
      slug: p.slug,
      price_idr: p.price_idr,
      billing_cycle: p.billing_cycle,
      is_active: p.is_active,
      features: typeof p.features === 'string' ? p.features : JSON.stringify(p.features || []),
    };
    showCreate = true;
  }

  function openCreate() {
    editingPlan = null;
    form = { ...formDefault };
    showCreate = true;
  }

  function featuresList(p: Plan): string[] {
    try {
      const a = JSON.parse(p.features || '[]');
      return Array.isArray(a) ? a : [];
    } catch {
      return [];
    }
  }

  function featuresCount(p: Plan) {
    try {
      const arr = JSON.parse(p.features || '[]');
      return Array.isArray(arr) ? arr.length : 0;
    } catch {
      return 0;
    }
  }
</script>

<DashboardLayout active="Plans">
  {#if !hasRole(auth.user?.role, 'super_admin')}
    <div class="card">
      <EmptyState icon={ShieldOff as any} title="Access denied" description="Only super admins can manage plans." />
    </div>
  {:else}
    <PageHeader title="Plans" description={loading ? 'Loading…' : `${plans.length} plan${plans.length !== 1 ? 's' : ''} total`}>
      {#snippet actions()}
        <button onclick={openCreate} class="btn btn-primary btn-sm"><Plus size={14} /> Create plan</button>
      {/snippet}
    </PageHeader>

    <div class="card">
      {#if !loading && plans.length > 0}
        <div class="flex flex-wrap items-center justify-end gap-3 border-b border-line px-4 py-3">
          <div class="relative w-full sm:w-64">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input bind:value={search} type="text" aria-label="Search plans" placeholder="Search plans…" class="input h-8 pr-8 pl-8" />
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
      {:else if plans.length === 0}
        <EmptyState icon={DollarSign as any} title="No plans yet" description="Create your first plan to start billing users." actionLabel="Create plan" onaction={openCreate} />
      {:else if filtered.length === 0 && search}
        <EmptyState compact icon={Search as any} title="No results" description={`No plans match "${search}"`} actionLabel="Clear search" onaction={() => (search = '')} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Name</th>
                <th class="max-md:hidden">Slug</th>
                <th>Price</th>
                <th class="max-sm:hidden">Cycle</th>
                <th>Status</th>
                <th class="max-md:hidden">Features</th>
                <th class="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as p (p.id)}
                <tr>
                  <td>
                    <button onclick={() => { detailPlan = p; showDetail = true; }} class="cursor-pointer text-left font-medium text-fg hover:text-accent">{p.name}</button>
                  </td>
                  <td class="max-md:hidden"><code class="code text-xs">{p.slug}</code></td>
                  <td class="font-medium whitespace-nowrap text-fg tabular-nums">{formatIDR(p.price_idr)}</td>
                  <td class="text-muted capitalize max-sm:hidden">{p.billing_cycle}</td>
                  <td><StatusPill status={p.is_active ? 'active' : 'disabled'} /></td>
                  <td class="text-xs text-muted tabular-nums max-md:hidden">{featuresCount(p)} feature{featuresCount(p) !== 1 ? 's' : ''}</td>
                  <td>
                    <div class="flex items-center justify-end gap-0.5">
                      <button onclick={() => { detailPlan = p; showDetail = true; }} class="btn btn-ghost btn-xs btn-icon" aria-label="View {p.name}" title="View"><Eye size={14} /></button>
                      <button onclick={() => openEdit(p)} class="btn btn-ghost btn-xs btn-icon" aria-label="Edit {p.name}" title="Edit"><Pencil size={14} /></button>
                      <button onclick={() => askDelete(p)} class="btn btn-danger-ghost btn-xs btn-icon" aria-label="Delete {p.name}" title="Delete"><Trash2 size={14} /></button>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <!-- Create / Edit -->
    <Modal bind:open={showCreate} title={editingPlan ? 'Edit plan' : 'Create plan'} maxWidth="lg">
      <div class="space-y-4">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label for="plan-name" class="label">Name <span class="text-danger">*</span></label>
            <input
              id="plan-name"
              bind:value={form.name}
              oninput={() => { if (!editingPlan && !form.slug) form.slug = autoSlug(form.name); }}
              class="input"
              placeholder="e.g. Pro Plan"
            />
          </div>
          <div>
            <label for="plan-slug" class="label">Slug <span class="text-danger">*</span></label>
            <div class="flex gap-2">
              <input id="plan-slug" bind:value={form.slug} class="input font-mono text-[13px]" placeholder="pro-plan" />
              <button onclick={() => { form.slug = autoSlug(form.name); }} class="btn btn-secondary btn-icon" aria-label="Auto-generate slug" title="Auto-generate slug"><Wand2 size={14} /></button>
            </div>
          </div>
          <div>
            <label for="plan-price" class="label">Price (IDR) <span class="text-danger">*</span></label>
            <input id="plan-price" type="number" min="0" bind:value={form.price_idr} class="input tabular-nums" />
          </div>
          <div>
            <label for="plan-cycle" class="label">Billing cycle</label>
            <select id="plan-cycle" bind:value={form.billing_cycle} class="input">
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
              <option value="one-time">One-time</option>
            </select>
          </div>
        </div>
        <div>
          <label for="plan-features" class="label">Features</label>
          <textarea id="plan-features" bind:value={form.features} rows={4} class="input font-mono text-[13px]" placeholder={'["5 websites", "10GB storage", "Priority support"]'}></textarea>
          <p class="hint">JSON array or comma-separated list.</p>
        </div>
        <label for="plan-active" class="flex cursor-pointer items-center gap-2.5 text-sm text-fg">
          <input id="plan-active" type="checkbox" bind:checked={form.is_active} class="checkbox" />
          Active
          <span class="text-xs text-subtle">— inactive plans can't be ordered</span>
        </label>
      </div>
      {#snippet footer()}
        <button onclick={() => { showCreate = false; editingPlan = null; form = { ...formDefault }; }} class="btn btn-secondary btn-sm">Cancel</button>
        <button onclick={savePlan} disabled={busy || !form.name || !form.slug} class="btn btn-primary btn-sm">
          {#if busy}<Loader2 size={14} class="animate-spin" /> Saving...{:else}<Check size={14} /> {editingPlan ? 'Update' : 'Create'}{/if}
        </button>
      {/snippet}
    </Modal>

    <!-- Detail -->
    <Modal bind:open={showDetail} title={detailPlan?.name || 'Plan detail'} maxWidth="lg">
      {#if detailPlan}
        {@const fc = featuresList(detailPlan)}
        <dl class="grid grid-cols-2 gap-x-4 gap-y-4">
          <div>
            <dt class="stat-label">Slug</dt>
            <dd class="mt-1"><code class="code">{detailPlan.slug}</code></dd>
          </div>
          <div>
            <dt class="stat-label">Price</dt>
            <dd class="mt-1 text-sm font-semibold text-fg tabular-nums">{formatIDR(detailPlan.price_idr)}</dd>
          </div>
          <div>
            <dt class="stat-label">Billing cycle</dt>
            <dd class="mt-1 text-sm text-fg capitalize">{detailPlan.billing_cycle}</dd>
          </div>
          <div>
            <dt class="stat-label">Status</dt>
            <dd class="mt-1"><StatusPill status={detailPlan.is_active ? 'active' : 'disabled'} /></dd>
          </div>
          <div>
            <dt class="stat-label">Users assigned</dt>
            <dd class="mt-1 text-sm text-fg tabular-nums">{detailPlan.user_count ?? 0}</dd>
          </div>
          <div>
            <dt class="stat-label">Created</dt>
            <dd class="mt-1 text-sm text-fg">{detailPlan.created_at ? new Date(detailPlan.created_at).toLocaleDateString('id-ID') : '—'}</dd>
          </div>
        </dl>
        {#if fc.length > 0}
          <div class="mt-5 border-t border-line pt-4">
            <div class="stat-label mb-2">Features ({fc.length})</div>
            <div class="flex flex-wrap gap-1.5">
              {#each fc as feat, i (i)}
                <span class="badge">{feat}</span>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
      {#snippet footer()}
        <button onclick={() => { showDetail = false; detailPlan = null; }} class="btn btn-secondary btn-sm">Close</button>
      {/snippet}
    </Modal>

    <ConfirmDialog
      bind:open={confirmDeleteOpen}
      title="Delete plan"
      message={confirmDelete ? `Delete plan "${confirmDelete.name}"? This cannot be undone.` : ''}
      confirmLabel="Delete"
      destructive
      onconfirm={() => confirmDelete && deletePlan(confirmDelete)}
    />
  {/if}
</DashboardLayout>
