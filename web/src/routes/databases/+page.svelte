<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { Plus, RefreshCw, UserPlus, Copy, X, KeyRound } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import DatabasesPanel from './DatabasesPanel.svelte';
  import UsersPanel from './UsersPanel.svelte';

  let dbs = $state<any[]>([]);
  let sites = $state<any[]>([]);
  let allUsers = $state<any[]>([]);
  let loading = $state(true);
  let activeDbTab = $state<'databases' | 'users'>('databases');
  let newPwdVisible = $state('');

  let showCreateDb = $state(false);
  let showCreateUser = $state(false);
  let showAddUser = $state(false);

  async function loadDbs() {
    if (!auth.user) { window.location.href = '/login'; return; }
    loading = true;
    try {
      const [dbData, siteData, userData] = await Promise.all([
        apiFetch(auth.token, 'GET', 'databases'),
        apiFetch(auth.token, 'GET', 'sites'),
        apiFetch(auth.token, 'GET', 'db-users'),
      ]);
      dbs = dbData?.databases || [];
      sites = siteData?.sites || [];
      allUsers = userData?.users || [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally { loading = false; }
  }

  async function reloadAllUsers() {
    try {
      const userData = await apiFetch(auth.token, 'GET', 'db-users');
      allUsers = userData?.users || [];
    } catch {}
  }

  async function copyPassword() {
    try {
      await navigator.clipboard.writeText(newPwdVisible);
      toasts.success('Password copied');
    } catch {}
    newPwdVisible = '';
  }

  onMount(() => { loadDbs(); });
</script>

<DashboardLayout active="Databases">
  <PageHeader title="Databases" description="Manage databases and database users from one page.">
    {#snippet actions()}
      <button onclick={loadDbs} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if activeDbTab === 'databases'}
        <button onclick={() => (showCreateDb = true)} class="btn btn-primary btn-sm"><Plus size={14} /> New database</button>
      {:else}
        <button onclick={() => (showAddUser = true)} class="btn btn-secondary btn-sm"><UserPlus size={14} /> Add to database</button>
        <button onclick={() => (showCreateUser = true)} class="btn btn-primary btn-sm"><Plus size={14} /> New user</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if newPwdVisible}
    <div class="card mb-6 flex flex-wrap items-center gap-3 border-warning/30 px-4 py-3 animate-fade-in" role="status">
      <KeyRound size={16} class="shrink-0 text-warning" />
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-warning">New password — copy it now, it won't be shown again</p>
        <code class="mt-0.5 block font-mono text-sm break-all text-fg">{newPwdVisible}</code>
      </div>
      <button onclick={copyPassword} class="btn btn-secondary btn-sm"><Copy size={14} /> Copy & dismiss</button>
      <button onclick={() => (newPwdVisible = '')} class="btn btn-ghost btn-icon btn-sm" aria-label="Dismiss"><X size={14} /></button>
    </div>
  {/if}

  <div class="tabs mb-6" role="tablist">
    <button role="tab" aria-selected={activeDbTab === 'databases'} class="tab {activeDbTab === 'databases' ? 'tab-active' : ''}" onclick={() => (activeDbTab = 'databases')}>
      Databases {#if !loading}<span class="text-xs text-subtle tabular-nums">{dbs.length}</span>{/if}
    </button>
    <button role="tab" aria-selected={activeDbTab === 'users'} class="tab {activeDbTab === 'users' ? 'tab-active' : ''}" onclick={() => (activeDbTab = 'users')}>
      Database users {#if !loading}<span class="text-xs text-subtle tabular-nums">{allUsers.length}</span>{/if}
    </button>
  </div>

  <div class={activeDbTab === 'databases' ? '' : 'hidden'}>
    <DatabasesPanel
      bind:dbs
      bind:createOpen={showCreateDb}
      {sites}
      {loading}
      {loadDbs}
      {reloadAllUsers}
      onpassword={(p) => (newPwdVisible = p)}
      onmanageusers={() => (activeDbTab = 'users')} />
  </div>

  <div id="database-users" class={activeDbTab === 'users' ? '' : 'hidden'}>
    <UsersPanel
      bind:allUsers
      bind:createOpen={showCreateUser}
      bind:addOpen={showAddUser}
      {dbs}
      {loading}
      {reloadAllUsers}
      onpassword={(p) => (newPwdVisible = p)} />
  </div>
</DashboardLayout>
