<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import Modal from '$components/Modal.svelte';
  import StatusPill from '$components/StatusPill.svelte';

  import {
    DollarSign, Plus, Pencil, Trash2, Loader2, X, Check,
    Package, Search, Copy, ExternalLink
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

  async function deletePlan(p: Plan) {
    if (!confirm(`Delete plan "${p.name}"? This cannot be undone.`)) return;
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
    <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-20 px-6 text-center">
      <DollarSign size={48} class="mx-auto mb-4 text-slate-600 opacity-40" />
      <h2 class="text-lg font-semibold text-slate-300 mb-2">Access Denied</h2>
      <p class="text-sm text-slate-500">Only super admins can manage plans.</p>
    </div>
  {:else}
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Plans</h1>
        <p class="text-sm text-slate-500 mt-1">
          {loading ? 'Loading...' : `${plans.length} plan${plans.length !== 1 ? 's' : ''} total`}
        </p>
      </div>
      <button onclick={openCreate}
        class="inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer border-0 whitespace-nowrap">
        <Plus size={15} /> Create Plan
      </button>
    </div>

    <!-- Search -->
    {#if !loading && plans.length > 0}
      <div class="relative mb-5">
        <Search size={16} class="absolute left-4 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
        <input bind:value={search} type="text" placeholder="Search plans..."
          class="w-full bg-white/[0.03] border border-white/[0.07] rounded-xl pl-10 pr-10 py-2.5 text-sm text-slate-200 placeholder-slate-600 outline-none focus:border-sky-400/30 focus:ring-1 focus:ring-sky-400/10 transition" />
        {#if search}
          <button onclick={() => search = ''} class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300 transition cursor-pointer border-0 bg-transparent p-1"><X size={14} /></button>
        {/if}
      </div>
    {/if}

    <!-- Loading skeleton -->
    {#if loading}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
        <div class="p-5 space-y-0">
          {#each Array(4) as _}
            <div class="flex items-center gap-4 py-3.5 border-b border-white/[0.03] last:border-0">
              <div class="flex-1 space-y-2">
                <div class="h-3 w-32 bg-white/[0.06] rounded-lg animate-pulse"></div>
                <div class="h-2.5 w-48 bg-white/[0.04] rounded-lg animate-pulse"></div>
              </div>
              <div class="h-5 w-20 bg-white/[0.05] rounded-full animate-pulse"></div>
            </div>
          {/each}
        </div>
      </div>
    {:else if plans.length === 0}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
        <div class="w-16 h-16 rounded-2xl bg-white/[0.04] flex items-center justify-center mx-auto mb-5"><DollarSign size={28} class="text-slate-600" /></div>
        <h2 class="text-lg font-semibold text-slate-300 mb-2">No plans yet</h2>
        <p class="text-sm text-slate-500 mb-6 max-w-sm mx-auto">Create your first plan to start billing users.</p>
        <button onclick={openCreate} class="inline-flex items-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all duration-150 cursor-pointer border-0"><Plus size={15} /> Create Plan</button>
      </div>
    {:else if filtered.length === 0 && search}
      <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl py-16 px-6 text-center">
        <Search size={28} class="text-slate-600 mx-auto mb-3" />
        <h2 class="text-lg font-semibold text-slate-300 mb-2">No results</h2>
        <p class="text-sm text-slate-500">No plans match "<span class="text-sky-400">{search}</span>"</p>
        <button onclick={() => search = ''} class="mt-4 inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0"><X size={14} /> Clear</button>
      </div>
    {:else}
      <!-- Desktop table -->
      <div class="hidden md:block bg-[#040a14] border border-white/[0.07] rounded-2xl overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-0">
            <thead>
              <tr class="text-left">
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Name</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Slug</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Price</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Cycle</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Status</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Features</th>
                <th class="px-5 py-3.5 font-medium text-[0.72rem] uppercase tracking-wider text-slate-500 bg-white/[0.02] border-b border-white/[0.07]">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each filtered as p (p.id)}
                <tr class="hover:bg-white/[0.02] transition group">
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <div class="flex items-center gap-2.5">
                      <div class="w-8 h-8 rounded-lg bg-sky-500/10 flex items-center justify-center"><DollarSign size={14} class="text-sky-400" /></div>
                      <span class="text-slate-200 font-medium">{p.name}</span>
                    </div>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] text-slate-400 align-middle">
                    <code class="text-xs bg-white/[0.04] px-2 py-1 rounded-md">{p.slug}</code>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-emerald-400 font-semibold text-sm">{formatIDR(p.price_idr)}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-xs px-2.5 py-1 rounded-lg bg-white/[0.04] text-slate-300 capitalize">{p.billing_cycle}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <StatusPill status={p.is_active ? 'active' : 'disabled'} />
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <span class="text-xs text-slate-400">{featuresCount(p)} feature{featuresCount(p) !== 1 ? 's' : ''}</span>
                  </td>
                  <td class="px-5 py-3.5 border-b border-white/[0.03] align-middle">
                    <div class="flex items-center gap-1">
                      <button onclick={() => { detailPlan = p; showDetail = true; }} class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5 cursor-pointer border-0 bg-transparent"><ExternalLink size={12} /> View</button>
                      <button onclick={() => openEdit(p)} class="inline-flex items-center gap-1 text-xs text-sky-400/70 hover:text-sky-400 transition px-2 py-1 rounded-lg hover:bg-sky-400/5 cursor-pointer border-0 bg-transparent"><Pencil size={12} /> Edit</button>
                      <button onclick={() => deletePlan(p)} class="inline-flex items-center gap-1 text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent"><Trash2 size={12} /> Delete</button>
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
        {#each filtered as p (p.id)}
          <div class="bg-[#040a14] border border-white/[0.07] rounded-2xl p-4 space-y-3">
            <div class="flex items-start justify-between">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-xl bg-sky-500/10 flex items-center justify-center"><DollarSign size={16} class="text-sky-400" /></div>
                <div>
                  <div class="text-sm font-medium text-slate-200">{p.name}</div>
                  <code class="text-[0.65rem] text-slate-500">{p.slug}</code>
                </div>
              </div>
              <StatusPill status={p.is_active ? 'active' : 'disabled'} />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-emerald-400 font-semibold text-sm">{formatIDR(p.price_idr)}</span>
              <span class="text-xs text-slate-400 capitalize">{p.billing_cycle}</span>
            </div>
            <div class="flex items-center gap-1">
              <button onclick={() => { detailPlan = p; showDetail = true; }} class="inline-flex items-center gap-1 text-xs text-indigo-400/70 hover:text-indigo-400 transition px-2 py-1 rounded-lg hover:bg-indigo-400/5 cursor-pointer border-0 bg-transparent"><ExternalLink size={12} /></button>
              <button onclick={() => openEdit(p)} class="inline-flex items-center gap-1 text-xs text-sky-400/70 hover:text-sky-400 transition px-2 py-1 rounded-lg hover:bg-sky-400/5 cursor-pointer border-0 bg-transparent"><Pencil size={12} /> Edit</button>
              <button onclick={() => deletePlan(p)} class="inline-flex items-center gap-1 text-xs text-red-400/50 hover:text-red-400 transition px-2 py-1 rounded-lg hover:bg-red-400/5 cursor-pointer border-0 bg-transparent"><Trash2 size={12} /> Delete</button>
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Create / Edit Modal -->
    <Modal bind:open={showCreate} title={editingPlan ? 'Edit Plan' : 'Create Plan'} maxWidth="lg">
      <div class="space-y-4">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="plan-name" class="block text-xs text-slate-500 mb-1.5">Name *</label>
            <input id="plan-name" bind:value={form.name} oninput={() => { if (!editingPlan && !form.slug) form.slug = autoSlug(form.name); }}
              class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" placeholder="e.g. Pro Plan" />
          </div>
          <div>
            <label for="plan-slug" class="block text-xs text-slate-500 mb-1.5">Slug *</label>
            <div class="flex gap-2">
              <input id="plan-slug" bind:value={form.slug}
                class="flex-1 bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" placeholder="pro-plan" />
              <button onclick={() => { form.slug = autoSlug(form.name); }}
                class="shrink-0 p-2.5 rounded-xl bg-white/[0.04] border border-white/[0.07] text-slate-400 hover:text-sky-400 hover:bg-sky-400/5 transition cursor-pointer border-0"
                title="Auto-generate slug"><Copy size={14} /></button>
            </div>
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="plan-price" class="block text-xs text-slate-500 mb-1.5">Price (IDR) *</label>
            <input id="plan-price" type="number" min="0" bind:value={form.price_idr}
              class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition" />
          </div>
          <div>
            <label for="plan-cycle" class="block text-xs text-slate-500 mb-1.5">Billing Cycle</label>
            <select id="plan-cycle" bind:value={form.billing_cycle}
              class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition">
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
              <option value="one-time">One-time</option>
            </select>
          </div>
        </div>
        <div>
          <label for="plan-features" class="block text-xs text-slate-500 mb-1.5">Features (JSON array or comma-separated)</label>
          <textarea id="plan-features" bind:value={form.features} rows={4}
            class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition font-mono"
            placeholder='["5 websites", "10GB storage", "Priority support"]'></textarea>
        </div>
        <div class="flex items-center gap-3">
          <label for="plan-active" class="text-xs text-slate-500">Status</label>
          <button id="plan-active" onclick={() => form.is_active = !form.is_active}
            class="relative w-10 h-5 rounded-full transition-colors duration-200 cursor-pointer border-0 {form.is_active ? 'bg-emerald-500' : 'bg-white/[0.08]'}">
            <div class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform duration-200 {form.is_active ? 'translate-x-5' : ''}"></div>
          </button>
          <span class="text-xs text-slate-400">{form.is_active ? 'Active' : 'Inactive'}</span>
        </div>
      </div>
      {#snippet footer()}
        <button onclick={() => { showCreate = false; editingPlan = null; form = { ...formDefault }; }}
          class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
        <button onclick={savePlan} disabled={busy || !form.name || !form.slug}
          class="inline-flex items-center gap-1.5 px-5 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">
          {#if busy}<Loader2 size={14} class="animate-spin" /> Saving...{:else}<Check size={14} /> {editingPlan ? 'Update' : 'Create'}{/if}
        </button>
      {/snippet}
    </Modal>

    <!-- Detail Modal -->
    <Modal bind:open={showDetail} title={detailPlan?.name || 'Plan Detail'} maxWidth="lg">
      {#if detailPlan}
        {@const fc = (() => { try { const a = JSON.parse(detailPlan.features || '[]'); return Array.isArray(a) ? a : []; } catch { return []; } })()}
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <div class="text-xs text-slate-500 mb-1">Slug</div>
              <code class="text-sm text-slate-200 bg-white/[0.04] px-2 py-1 rounded-md">{detailPlan.slug}</code>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Price</div>
              <div class="text-sm text-emerald-400 font-semibold">{formatIDR(detailPlan.price_idr)}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Billing Cycle</div>
              <div class="text-sm text-slate-200 capitalize">{detailPlan.billing_cycle}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Status</div>
              <StatusPill status={detailPlan.is_active ? 'active' : 'disabled'} />
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Users Assigned</div>
              <div class="text-sm text-slate-200">{detailPlan.user_count ?? 0}</div>
            </div>
            <div>
              <div class="text-xs text-slate-500 mb-1">Created</div>
              <div class="text-sm text-slate-200">{detailPlan.created_at ? new Date(detailPlan.created_at).toLocaleDateString('id-ID') : '—'}</div>
            </div>
          </div>
          {#if fc.length > 0}
            <div>
              <div class="text-xs text-slate-500 mb-2">Features ({fc.length})</div>
              <div class="flex flex-wrap gap-2">
                {#each fc as feat}
                  <span class="text-xs bg-sky-500/10 text-sky-400 border border-sky-500/15 px-2.5 py-1 rounded-lg">{feat}</span>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {/if}
      {#snippet footer()}
        <button onclick={() => { showDetail = false; detailPlan = null; }}
          class="px-4 py-2 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Close</button>
      {/snippet}
    </Modal>
  {/if}
</DashboardLayout>
