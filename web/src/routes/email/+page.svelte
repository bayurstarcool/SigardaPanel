<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Mail, Server, Plus, Trash2, RefreshCw, Download, LoaderCircle, Users, Lock, WifiOff, Search, KeyRound } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let servers = $state<any[]>([]);
  let serversLoaded = $state(false);
  let selectedServer = $state<number>(0);
  let status = $state<any>(null);
  let statusError = $state('');
  let accounts = $state<any[]>([]);
  let accountsError = $state('');
  let loading = $state(false);
  let statusLoading = $state(false);
  let installing = $state(false);
  let creating = $state(false);
  let deletingEmail = $state('');
  let query = $state('');
  const canManage = $derived(hasRole(auth.user?.role, 'admin'));

  let confirmInstall = $state(false);
  let confirmDelete = $state(false);
  let deleteTarget = $state('');

  // Create form
  let showCreate = $state(false);
  let formDomain = $state('');
  let formEmail = $state('');
  let formPassword = $state('');
  let formQuota = $state('1024');
  let formFirstName = $state('');
  let formLastName = $state('');

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e?.message || 'Failed to load servers');
    }
    serversLoaded = true;
    if (servers.length > 0) {
      selectedServer = servers[0].id;
      loadStatus();
    }
  });

  function friendlyError(e: any, fallback: string): string {
    const msg = String(e?.message || '');
    if (/not found|404/i.test(msg)) return 'Email management is not available on this panel version yet.';
    if (/unreachable|unavailable/i.test(msg)) return 'The server agent is unreachable. Check that it is running, then retry.';
    return msg || fallback;
  }

  function loadStatus() {
    if (!selectedServer) return;
    statusLoading = true;
    statusError = '';
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'status' })
      .then((d: any) => {
        status = d?.installed === undefined && d?.data ? d.data : d;
        if (status?.installed) loadAccounts();
      })
      .catch((e: any) => {
        status = null;
        if (e?.message !== 'session expired') statusError = friendlyError(e, 'Could not check the email server');
      })
      .finally(() => (statusLoading = false));
  }

  function loadAccounts() {
    if (!selectedServer) return;
    loading = true;
    accountsError = '';
    // List without a domain filter — all accounts
    apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'list', domain: '' })
      .then((d: any) => { accounts = d?.accounts ?? d?.data?.accounts ?? []; })
      .catch((e: any) => { if (e?.message !== 'session expired') accountsError = friendlyError(e, 'Could not load accounts'); })
      .finally(() => (loading = false));
  }

  function changeServer() {
    status = null;
    accounts = [];
    loadStatus();
  }

  async function installEmail() {
    installing = true;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'install' });
      toasts.success('Email server installed successfully');
      loadStatus();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(friendlyError(e, 'Failed to install email server'));
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
      if (e?.message !== 'session expired') toasts.error(friendlyError(e, 'Failed to create email account'));
    } finally {
      creating = false;
    }
  }

  function askDelete(email: string) {
    deleteTarget = email;
    confirmDelete = true;
  }

  async function deleteAccount(email: string) {
    deletingEmail = email;
    try {
      await apiFetch(auth.token, 'POST', `servers/${selectedServer}/email/manage`, { action: 'delete', email });
      toasts.success(`Account ${email} deleted`);
      loadAccounts();
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(friendlyError(e, 'Failed to delete account'));
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
    const buf = new Uint32Array(16);
    crypto.getRandomValues(buf);
    formPassword = Array.from(buf, (n) => chars.charAt(n % chars.length)).join('');
  }

  const host = $derived(status?.hostname || 'mail.yourdomain.com');
  const visible = $derived(
    accounts.filter((a) => !query || `${a.email} ${a.domain} ${a.first_name ?? ''} ${a.last_name ?? ''}`.toLowerCase().includes(query.toLowerCase())),
  );
</script>

<DashboardLayout active="Email">
  <PageHeader title="Email" description="Postfix, Dovecot and mail accounts">
    {#snippet actions()}
      {#if servers.length > 1}
        <select bind:value={selectedServer} onchange={changeServer} class="input h-8 w-auto min-w-40 text-[13px]" aria-label="Server">
          {#each servers as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
        </select>
      {/if}
      <button onclick={loadStatus} disabled={statusLoading || !selectedServer} class="btn btn-secondary btn-sm">
        <RefreshCw size={14} class={statusLoading ? 'animate-spin' : ''} /> Refresh
      </button>
      {#if canManage && status?.installed}
        <button onclick={() => (showCreate = true)} class="btn btn-primary btn-sm"><Plus size={14} /> New account</button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if serversLoaded && servers.length === 0}
    <div class="card">
      <EmptyState icon={Server} title="No servers" description="Add a server first — email is managed through its agent.">
        <a href="/servers" class="btn btn-primary btn-sm">Go to servers</a>
      </EmptyState>
    </div>
  {:else if statusLoading && !status}
    <div class="mb-6 grid gap-3 sm:grid-cols-3">{#each Array(3) as _, i (i)}<div class="skeleton h-[88px]"></div>{/each}</div>
    <div class="card space-y-3 p-4">{#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
  {:else if statusError}
    <div class="card">
      <EmptyState icon={WifiOff} title="Couldn't check the email server" description={statusError} actionLabel="Retry" onaction={loadStatus} />
    </div>
  {:else if status && !status.installed}
    <div class="card">
      <EmptyState icon={Mail} title="Email server not installed" description="Install Postfix (SMTP), Dovecot (IMAP/POP3) and Roundcube (webmail) to manage mail accounts on this server.">
        {#if canManage}
          <button onclick={() => (confirmInstall = true)} disabled={installing} class="btn btn-primary btn-sm">
            {#if installing}<LoaderCircle size={14} class="animate-spin" /> Installing — this takes a few minutes…{:else}<Download size={14} /> Install email server{/if}
          </button>
        {/if}
      </EmptyState>
    </div>
  {:else if status}
    <div class="mb-6 grid gap-3 sm:grid-cols-3">
      <div class="card p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="stat-label">Postfix (SMTP)</span>
          <span class="badge {status.postfix_running ? 'badge-success' : 'badge-danger'}"><span class="dot"></span>{status.postfix_running ? 'Running' : 'Stopped'}</span>
        </div>
        <div class="mt-2 text-sm text-muted tabular-nums">Ports 25 / 587</div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="stat-label">Dovecot (IMAP)</span>
          <span class="badge {status.dovecot_running ? 'badge-success' : 'badge-danger'}"><span class="dot"></span>{status.dovecot_running ? 'Running' : 'Stopped'}</span>
        </div>
        <div class="mt-2 text-sm text-muted tabular-nums">Ports 993 / 995</div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between gap-2">
          <span class="stat-label">Accounts</span>
          <Users size={16} strokeWidth={1.75} class="text-subtle" />
        </div>
        <div class="stat-value">{loading ? '–' : accounts.length}</div>
      </div>
    </div>

    <div class="card mb-6">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 class="text-sm font-semibold text-fg">Accounts</h2>
        <div class="flex w-full items-center gap-2 sm:w-auto">
          <div class="relative flex-1 sm:w-64">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
            <input bind:value={query} class="input h-8 pl-8" placeholder="Filter accounts…" aria-label="Filter accounts" />
          </div>
          <button onclick={loadAccounts} class="btn btn-ghost btn-icon btn-sm" aria-label="Reload accounts" disabled={loading}>
            <RefreshCw size={14} class={loading ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      {#if loading && accounts.length === 0}
        <div class="space-y-3 p-4">{#each Array(4) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
      {:else if accountsError}
        <EmptyState compact icon={WifiOff} title="Couldn't load accounts" description={accountsError} actionLabel="Retry" onaction={loadAccounts} />
      {:else if visible.length === 0}
        <EmptyState
          compact
          icon={Mail}
          title={accounts.length ? 'No accounts match' : 'No email accounts yet'}
          description={accounts.length ? 'Try another filter.' : 'Create the first mailbox for one of your domains.'}
          actionLabel={!accounts.length && canManage ? 'New account' : ''}
          onaction={() => (showCreate = true)}
        />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Email</th>
                <th class="max-sm:hidden">Domain</th>
                <th class="max-md:hidden">Name</th>
                <th>Quota</th>
                {#if canManage}<th class="text-right">Actions</th>{/if}
              </tr>
            </thead>
            <tbody>
              {#each visible as acct (acct.email)}
                <tr>
                  <td class="font-medium text-fg">{acct.email}</td>
                  <td class="text-muted max-sm:hidden">{acct.domain || '—'}</td>
                  <td class="text-muted max-md:hidden">{acct.first_name || acct.last_name ? `${acct.first_name ?? ''} ${acct.last_name ?? ''}`.trim() : '—'}</td>
                  <td class="text-muted tabular-nums">{formatQuota(acct.quota)}</td>
                  {#if canManage}
                    <td class="text-right">
                      <button onclick={() => askDelete(acct.email)} disabled={deletingEmail === acct.email} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete account" title="Delete account">
                        {#if deletingEmail === acct.email}<LoaderCircle size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
                      </button>
                    </td>
                  {/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <div class="card">
      <div class="card-header"><h2 class="card-title flex items-center gap-2"><Lock size={16} class="text-subtle" /> Connection settings</h2></div>
      <div class="grid divide-y divide-line sm:grid-cols-3 sm:divide-x sm:divide-y-0">
        {#each [
          { label: 'Incoming (IMAP)', value: host, meta: 'Port 993 · SSL/TLS' },
          { label: 'Outgoing (SMTP)', value: host, meta: 'Port 587 · STARTTLS' },
          { label: 'Webmail', value: `https://${host}/roundcube`, meta: 'Roundcube' },
        ] as row (row.label)}
          <div class="min-w-0 px-5 py-4">
            <div class="stat-label">{row.label}</div>
            <div class="mt-1 truncate font-mono text-[13px] text-fg" title={row.value}>{row.value}</div>
            <div class="mt-0.5 text-xs text-subtle">{row.meta}</div>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <Modal bind:open={showCreate} title="New email account" maxWidth="lg">
    <form id="email-create" class="grid gap-4 sm:grid-cols-2" onsubmit={(e) => { e.preventDefault(); createAccount(); }}>
      <div>
        <label class="label" for="em-domain">Domain <span class="text-danger">*</span></label>
        <input id="em-domain" bind:value={formDomain} placeholder="example.com" class="input" />
      </div>
      <div>
        <label class="label" for="em-email">Email address <span class="text-danger">*</span></label>
        <input id="em-email" bind:value={formEmail} placeholder="user@example.com" class="input" />
      </div>
      <div class="sm:col-span-2">
        <label class="label" for="em-pass">Password <span class="text-danger">*</span></label>
        <div class="flex gap-2">
          <input id="em-pass" bind:value={formPassword} type="text" autocomplete="new-password" placeholder="Strong password" class="input flex-1 font-mono text-[13px]" />
          <button type="button" onclick={generatePassword} class="btn btn-secondary"><KeyRound size={14} /> Generate</button>
        </div>
      </div>
      <div>
        <label class="label" for="em-first">First name</label>
        <input id="em-first" bind:value={formFirstName} placeholder="John" class="input" />
      </div>
      <div>
        <label class="label" for="em-last">Last name</label>
        <input id="em-last" bind:value={formLastName} placeholder="Doe" class="input" />
      </div>
      <div>
        <label class="label" for="em-quota">Quota (MB)</label>
        <input id="em-quota" bind:value={formQuota} type="number" min="0" placeholder="1024" class="input tabular-nums" />
        <p class="hint">0 = unlimited</p>
      </div>
    </form>
    {#snippet footer()}
      <button class="btn btn-secondary btn-sm" onclick={() => (showCreate = false)}>Cancel</button>
      <button type="submit" form="email-create" class="btn btn-primary btn-sm" disabled={creating}>
        {#if creating}<LoaderCircle size={14} class="animate-spin" /> Creating…{:else}<Plus size={14} /> Create account{/if}
      </button>
    {/snippet}
  </Modal>

  <ConfirmDialog
    bind:open={confirmInstall}
    title="Install email server?"
    message="Postfix, Dovecot and Roundcube will be installed on this server. This may take a few minutes."
    confirmLabel="Install"
    onconfirm={installEmail}
  />

  <ConfirmDialog
    bind:open={confirmDelete}
    title="Delete email account?"
    message={`${deleteTarget} and its mailbox will be deleted. This cannot be undone.`}
    confirmLabel="Delete"
    destructive
    onconfirm={() => deleteAccount(deleteTarget)}
  />
</DashboardLayout>
