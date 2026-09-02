<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { Mail, Server, Plus, Trash2, RefreshCw, Download, Shield, CheckCircle, XCircle, Loader2, HardDrive, Users, Lock } from 'lucide-svelte';
  import { onMount } from 'svelte';
  import { toasts } from '$lib/toast.svelte';

  let servers = $state<any[]>([]);
  let selectedServer = $state<number>(0);
  let status = $state<any>(null);
  let accounts = $state<any[]>([]);
  let loading = $state(false);
  let statusLoading = $state(false);
  let installing = $state(false);
  let creating = $state(false);
  let deletingEmail = $state('');
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  // Create form
  let showCreate = $state(false);
  let formDomain = $state('');
  let formEmail = $state('');
  let formPassword = $state('');
  let formQuota = $state('1024');
  let formFirstName = $state('');
  let formLastName = $state('');

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }
    const d: any = await apiFetch(auth.token, 'GET', 'servers');
    servers = d?.servers ?? [];
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadStatus();
    }
  });

  function loadStatus() {
    if (!selectedServer) return;
    statusLoading = true;
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'status' })
      .then((d: any) => {
        status = d;
        if (d?.installed) loadAccounts();
      })
      .catch(() => { status = { installed: false }; })
      .finally(() => statusLoading = false);
  }

  function loadAccounts() {
    if (!selectedServer) return;
    loading = true;
    // List without domain filter — get all accounts
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'list', domain: '' })
      .then((d: any) => { accounts = d?.accounts ?? []; })
      .catch(() => {})
      .finally(() => loading = false);
  }

  async function installEmail() {
    if (!confirm('Install Postfix + Dovecot + Roundcube? This may take a few minutes.')) return;
    installing = true;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'install' });
      toasts.success('Email server installed successfully');
      loadStatus();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to install email server');
    } finally {
      installing = false;
    }
  }

  async function createAccount() {
    if (!formDomain || !formEmail || !formPassword) {
      toasts.error('Domain, email, and password are required');
      return;
    }
    creating = true;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, {
        action: 'create',
        domain: formDomain,
        email: formEmail,
        password: formPassword,
        quota: parseInt(formQuota) || 0,
        first_name: formFirstName,
        last_name: formLastName,
      });
      toasts.success(`Email account ${formEmail} created`);
      showCreate = false;
      formDomain = '';
      formEmail = '';
      formPassword = '';
      formQuota = '1024';
      formFirstName = '';
      formLastName = '';
      loadAccounts();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to create email account');
    } finally {
      creating = false;
    }
  }

  async function deleteAccount(email: string) {
    if (!confirm(`Delete email account ${email}? This cannot be undone.`)) return;
    deletingEmail = email;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, {
        action: 'delete',
        email,
      });
      toasts.success(`Account ${email} deleted`);
      loadAccounts();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to delete account');
    } finally {
      deletingEmail = '';
    }
  }

  function formatQuota(mb: number) {
    if (!mb || mb === 0) return 'Unlimited';
    if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB';
    return mb + ' MB';
  }

  function generatePassword() {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%';
    let pass = '';
    for (let i = 0; i < 16; i++) pass += chars.charAt(Math.floor(Math.random() * chars.length));
    formPassword = pass;
  }
</script>

<DashboardLayout active="Email">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
    <div>
      <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent flex items-center gap-3">
        <Mail size={28} class="text-sky-400" />
        Email Management
      </h1>
      <p class="text-sm text-slate-500 mt-1">Manage Postfix, Dovecot & email accounts</p>
    </div>
    <div class="flex gap-2">
      <select bind:value={selectedServer} onchange={() => { status = null; accounts = []; loadStatus(); }}
        class="bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-sky-500/50">
        {#each servers as s}
          <option value={s.id}>{s.name}</option>
        {/each}
      </select>
      <button onclick={() => { loadStatus(); }}
        disabled={statusLoading}
        class="p-2 bg-white/[0.05] border border-white/[0.1] rounded-lg hover:bg-white/[0.1] transition disabled:opacity-50">
        <RefreshCw size={16} class={statusLoading ? 'animate-spin' : ''} />
      </button>
    </div>
  </div>

  <!-- Status Bar -->
  {#if statusLoading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-12 mb-6">
      <Loader2 size={32} class="animate-spin mx-auto mb-3 text-sky-400 opacity-60" />
      <p class="text-sm text-slate-500">Checking email server status...</p>
    </div>
  {:else if status && !status.installed}
    <!-- Not Installed -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-8 text-center mb-6">
      <div class="w-16 h-16 rounded-full bg-sky-500/10 flex items-center justify-center mx-auto mb-4">
        <Mail size={32} class="text-sky-400" />
      </div>
      <h2 class="text-lg font-semibold text-white mb-2">Email Server Not Installed</h2>
      <p class="text-sm text-slate-500 mb-6 max-w-md mx-auto">
        Install Postfix (MTA), Dovecot (IMAP/POP3), and Roundcube (webmail) to start managing email accounts on this server.
      </p>
      {#if canManage}
        <button onclick={installEmail} disabled={installing}
          class="px-6 py-2.5 bg-sky-500 hover:bg-sky-400 text-white rounded-lg text-sm font-medium transition disabled:opacity-50 flex items-center gap-2 mx-auto">
          {#if installing}
            <Loader2 size={16} class="animate-spin" />
            Installing... (this may take a few minutes)
          {:else}
            <Download size={16} />
            Install Email Server
          {/if}
        </button>
      {/if}
    </div>
  {:else if status}
    <!-- Status Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center justify-between mb-2">
          <span class="text-xs text-slate-500">Postfix (SMTP)</span>
          {#if status.postfix_running}
            <span class="flex items-center gap-1 text-xs text-emerald-400"><CheckCircle size={12} /> Running</span>
          {:else}
            <span class="flex items-center gap-1 text-xs text-red-400"><XCircle size={12} /> Stopped</span>
          {/if}
        </div>
        <div class="text-sm text-white font-medium flex items-center gap-2">
          <Server size={14} class="text-sky-400" /> Port 25 / 587
        </div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center justify-between mb-2">
          <span class="text-xs text-slate-500">Dovecot (IMAP)</span>
          {#if status.dovecot_running}
            <span class="flex items-center gap-1 text-xs text-emerald-400"><CheckCircle size={12} /> Running</span>
          {:else}
            <span class="flex items-center gap-1 text-xs text-red-400"><XCircle size={12} /> Stopped</span>
          {/if}
        </div>
        <div class="text-sm text-white font-medium flex items-center gap-2">
          <Shield size={14} class="text-sky-400" /> Port 993 / 995
        </div>
      </div>
      <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
        <div class="flex items-center justify-between mb-2">
          <span class="text-xs text-slate-500">Accounts</span>
        </div>
        <div class="text-2xl font-bold text-white flex items-center gap-2">
          <Users size={18} class="text-sky-400" /> {accounts.length}
        </div>
      </div>
    </div>

    <!-- Create Account Button + Form -->
    <div class="mb-6">
      {#if canManage}
        <button onclick={() => showCreate = !showCreate}
          class="px-4 py-2 bg-sky-500/20 text-sky-400 border border-sky-500/30 rounded-lg text-sm font-medium hover:bg-sky-500/30 transition flex items-center gap-2">
          <Plus size={16} />
          {showCreate ? 'Close Form' : 'Create Email Account'}
        </button>
      {/if}

      {#if showCreate}
        <div class="mt-4 bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
          <h3 class="text-sm font-semibold text-white mb-4">New Email Account</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">Domain *</label>
              <input bind:value={formDomain} placeholder="example.com"
                class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">Email Address *</label>
              <input bind:value={formEmail} placeholder="user@example.com"
                class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">Password *</label>
              <div class="flex gap-2">
                <input bind:value={formPassword} type="text" placeholder="Strong password"
                  class="flex-1 bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50 font-mono" />
                <button onclick={generatePassword}
                  class="px-3 py-2 bg-white/[0.05] border border-white/[0.1] rounded-lg text-xs text-slate-400 hover:text-sky-400 hover:bg-white/[0.1] transition whitespace-nowrap">
                  Generate
                </button>
              </div>
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">Quota (MB, 0 = unlimited)</label>
              <input bind:value={formQuota} type="number" min="0" placeholder="1024"
                class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">First Name</label>
              <input bind:value={formFirstName} placeholder="John"
                class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1.5">Last Name</label>
              <input bind:value={formLastName} placeholder="Doe"
                class="w-full bg-white/[0.05] border border-white/[0.1] rounded-lg px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-sky-500/50" />
            </div>
          </div>
          <div class="mt-4 flex justify-end">
            <button onclick={createAccount} disabled={creating}
              class="px-5 py-2 bg-sky-500 hover:bg-sky-400 text-white rounded-lg text-sm font-medium transition disabled:opacity-50 flex items-center gap-2">
              {#if creating}
                <Loader2 size={14} class="animate-spin" /> Creating...
              {:else}
                <Plus size={14} /> Create Account
              {/if}
            </button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Accounts List -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="px-5 py-3 border-b border-white/[0.06] flex items-center justify-between">
        <h3 class="text-sm font-semibold text-white">Email Accounts</h3>
        <button onclick={loadAccounts} class="text-slate-500 hover:text-sky-400 transition">
          <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
        </button>
      </div>

      {#if loading}
        <div class="px-5 py-12 text-center">
          <Loader2 size={24} class="animate-spin mx-auto mb-2 text-sky-400 opacity-60" />
          <p class="text-sm text-slate-500">Loading accounts...</p>
        </div>
      {:else if accounts.length === 0}
        <div class="px-5 py-12 text-center">
          <Mail size={32} class="mx-auto mb-3 text-slate-600" />
          <p class="text-sm text-slate-500">No email accounts yet</p>
          <p class="text-xs text-slate-600 mt-1">Create your first account to get started</p>
        </div>
      {:else}
        <!-- Desktop table header -->
        <div class="hidden sm:grid grid-cols-12 gap-4 px-5 py-2.5 border-b border-white/[0.04] text-xs text-slate-500 uppercase tracking-wider">
          <div class="col-span-4">Email</div>
          <div class="col-span-2">Domain</div>
          <div class="col-span-2">Name</div>
          <div class="col-span-2">Quota</div>
          <div class="col-span-2 text-right">Actions</div>
        </div>

        <div class="divide-y divide-white/[0.04]">
          {#each accounts as acct}
            <div class="grid grid-cols-1 sm:grid-cols-12 gap-2 sm:gap-4 px-5 py-3 hover:bg-white/[0.02] transition items-center">
              <div class="sm:col-span-4 flex items-center gap-2">
                <div class="w-8 h-8 rounded-full bg-sky-500/10 flex items-center justify-center shrink-0">
                  <Mail size={14} class="text-sky-400" />
                </div>
                <div>
                  <div class="text-sm text-white font-medium">{acct.email}</div>
                </div>
              </div>
              <div class="sm:col-span-2 text-sm text-slate-400">{acct.domain || '—'}</div>
              <div class="sm:col-span-2 text-sm text-slate-400">
                {#if acct.first_name || acct.last_name}
                  {acct.first_name} {acct.last_name}
                {:else}
                  —
                {/if}
              </div>
              <div class="sm:col-span-2 text-sm text-slate-400 flex items-center gap-1">
                <HardDrive size={12} class="text-slate-600" />
                {formatQuota(acct.quota)}
              </div>
              <div class="sm:col-span-2 flex justify-end">
                {#if canManage}
                  <button onclick={() => deleteAccount(acct.email)} disabled={deletingEmail === acct.email}
                    class="p-2 text-slate-500 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition disabled:opacity-50"
                    title="Delete account">
                    {#if deletingEmail === acct.email}
                      <Loader2 size={14} class="animate-spin" />
                    {:else}
                      <Trash2 size={14} />
                    {/if}
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Connection Info -->
    <div class="mt-6 bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
      <h3 class="text-sm font-semibold text-white mb-3 flex items-center gap-2">
        <Lock size={14} class="text-sky-400" /> Connection Settings
      </h3>
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div class="bg-white/[0.02] rounded-lg p-3">
          <div class="text-xs text-slate-500 mb-1">Incoming (IMAP)</div>
          <div class="text-sm text-white font-mono">{status?.hostname || 'mail.yourdomain.com'}</div>
          <div class="text-xs text-slate-600 mt-1">Port 993 (SSL/TLS)</div>
        </div>
        <div class="bg-white/[0.02] rounded-lg p-3">
          <div class="text-xs text-slate-500 mb-1">Outgoing (SMTP)</div>
          <div class="text-sm text-white font-mono">{status?.hostname || 'mail.yourdomain.com'}</div>
          <div class="text-xs text-slate-600 mt-1">Port 587 (STARTTLS)</div>
        </div>
        <div class="bg-white/[0.02] rounded-lg p-3">
          <div class="text-xs text-slate-500 mb-1">Webmail</div>
          <div class="text-sm text-white font-mono">https://{status?.hostname || 'mail.yourdomain.com'}/roundcube</div>
          <div class="text-xs text-slate-600 mt-1">Roundcube Web UI</div>
        </div>
      </div>
    </div>
  {/if}
</DashboardLayout>
