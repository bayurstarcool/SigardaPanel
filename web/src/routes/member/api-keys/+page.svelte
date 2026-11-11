<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDate, fmtRelative, fmtDateTime } from '$lib/format';
  import { toasts } from '$lib/toast.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Key, Copy, Check, Plus, Trash2, AlertTriangle, LogIn } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type ApiKey = {
    id: number;
    name: string;
    prefix: string;
    created_at: string;
    expires_at?: string;
    last_used_at?: string;
    expired: boolean;
  };

  let keys = $state<ApiKey[]>([]);
  let limit = $state(20);
  let loading = $state(true);
  let loadError = $state('');

  let showCreate = $state(false);
  let newName = $state('');
  let newExpiry = $state('0');
  let creating = $state(false);
  let createError = $state('');

  // The raw key lives only in this component state, only until the dialog closes.
  let showToken = $state(false);
  let createdToken = $state('');
  let createdName = $state('');
  let copied = $state(false);

  let revokeTarget = $state<ApiKey | null>(null);
  let confirmRevoke = $state(false);
  let revoking = $state<number | null>(null);

  let origin = $state('');
  const usage = $derived([
    { label: 'API request', cmd: `curl -H "Authorization: Bearer YOUR_API_KEY" ${origin}/api/v1/sites` },
    { label: 'CLI (use the key instead of a password login)', cmd: `mkdir -p ~/.config/sigardapanel && printf '%s' YOUR_API_KEY > ~/.config/sigardapanel/token && chmod 600 ~/.config/sigardapanel/token` },
  ]);

  onMount(() => {
    origin = window.location.origin;
    if (!auth.user) { loading = false; return; }
    load();
  });

  async function load() {
    loading = true;
    loadError = '';
    try {
      const d = await apiFetch(auth.token, 'GET', 'api-keys');
      keys = d?.keys ?? [];
      limit = d?.limit ?? 20;
    } catch (e: any) {
      if (e?.message !== 'session expired') loadError = e?.message || 'Failed to load API keys';
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    newName = '';
    newExpiry = '0';
    createError = '';
    showCreate = true;
  }

  async function createKey() {
    const name = newName.trim();
    if (!name) { createError = 'Give the key a name so you can recognise it later.'; return; }
    creating = true;
    createError = '';
    try {
      const d = await apiFetch(auth.token, 'POST', 'api-keys', { name, expires_in_days: Number(newExpiry) });
      createdToken = d?.token ?? '';
      createdName = d?.key?.name ?? name;
      copied = false;
      showCreate = false;
      showToken = true;
      await load();
    } catch (e: any) {
      createError = e?.message || 'Failed to create key';
    } finally {
      creating = false;
    }
  }

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(createdToken);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      toasts.error('Copy failed — select the key and copy it manually');
    }
  }

  function forgetToken() {
    showToken = false;
    createdToken = '';
    createdName = '';
  }

  function askRevoke(k: ApiKey) {
    revokeTarget = k;
    confirmRevoke = true;
  }

  async function revoke() {
    const k = revokeTarget;
    if (!k) return;
    revoking = k.id;
    try {
      await apiFetch(auth.token, 'DELETE', `api-keys/${k.id}`);
      toasts.success(`Revoked "${k.name}"`);
      keys = keys.filter((x) => x.id !== k.id);
    } catch (e: any) {
      toasts.error(e?.message || 'Failed to revoke key');
    } finally {
      revoking = null;
      revokeTarget = null;
    }
  }

  const atLimit = $derived(keys.length >= limit);
</script>

<svelte:head>
  <title>API Keys — SigardaPanel</title>
</svelte:head>

<PageHeader title="API keys" description="Personal keys for the CLI and integrations. A key acts as you, with your permissions.">
  {#snippet actions()}
    {#if auth.user}
      <button class="btn btn-primary btn-sm" onclick={openCreate} disabled={loading || atLimit} title={atLimit ? `Limit of ${limit} keys reached` : undefined}>
        <Plus size={14} /> New key
      </button>
    {/if}
  {/snippet}
</PageHeader>

<div class="space-y-6">
  {#if !auth.user}
    <div class="card">
      <EmptyState icon={LogIn} title="Sign in to manage API keys" description="API keys belong to your panel account.">
        <a href="/login" class="btn btn-primary btn-sm mt-4">Sign in</a>
      </EmptyState>
    </div>
  {:else}
    <section class="card">
      <div class="card-header">
        <h2 class="card-title">Your keys</h2>
        {#if !loading && !loadError}<span class="text-xs text-subtle tabular-nums">{keys.length} / {limit}</span>{/if}
      </div>
      {#if loading}
        <div class="space-y-3 p-4">{#each [1, 2, 3] as i (i)}<div class="skeleton h-9"></div>{/each}</div>
      {:else if loadError}
        <EmptyState compact icon={AlertTriangle} title="Could not load API keys" description={loadError} actionLabel="Try again" onaction={load} />
      {:else if keys.length === 0}
        <EmptyState compact icon={Key} title="No API keys yet" description="Create a key to use the CLI or call the API from scripts." actionLabel="New key" onaction={openCreate} />
      {:else}
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Name</th>
                <th class="max-sm:hidden">Created</th>
                <th class="max-md:hidden">Last used</th>
                <th>Status</th>
                <th class="text-right"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {#each keys as k (k.id)}
                <tr>
                  <td class="max-w-0 min-w-32">
                    <div class="truncate font-medium text-fg" title={k.name}>{k.name}</div>
                    <div class="font-mono text-xs text-subtle">{k.prefix}…</div>
                  </td>
                  <td class="whitespace-nowrap text-muted tabular-nums max-sm:hidden">{fmtDate(k.created_at)}</td>
                  <td class="whitespace-nowrap text-muted max-md:hidden" title={k.last_used_at ? fmtDateTime(k.last_used_at) : ''}>
                    {k.last_used_at ? fmtRelative(k.last_used_at) : 'Never'}
                  </td>
                  <td class="whitespace-nowrap">
                    {#if k.expired}
                      <span class="badge badge-danger"><span class="dot"></span>Expired</span>
                    {:else}
                      <span class="badge badge-success"><span class="dot"></span>Active</span>
                    {/if}
                    {#if k.expires_at && !k.expired}
                      <div class="mt-0.5 text-xs text-subtle tabular-nums">until {fmtDate(k.expires_at)}</div>
                    {/if}
                  </td>
                  <td class="text-right">
                    <button class="btn btn-danger-ghost btn-xs" onclick={() => askRevoke(k)} disabled={revoking === k.id}>
                      <Trash2 size={12} /> {revoking === k.id ? 'Revoking…' : 'Revoke'}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  {/if}

  <section class="card">
    <div class="card-header">
      <h2 class="card-title">How to use</h2>
    </div>
    <div class="card-body space-y-3">
      {#each usage as u (u.label)}
        <div class="min-w-0">
          <p class="mb-1 text-xs text-subtle">{u.label}</p>
          <pre class="overflow-x-auto rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] text-fg">{u.cmd}</pre>
        </div>
      {/each}
      <p class="hint">Keys cannot manage users or other API keys, log out, or open terminals — those need a signed-in session.</p>
    </div>
  </section>
</div>

<Modal bind:open={showCreate} title="New API key" maxWidth="md">
  <form id="create-key-form" onsubmit={(e) => { e.preventDefault(); createKey(); }} class="space-y-4">
    <div>
      <label class="label" for="key-name">Name</label>
      <input id="key-name" class="input" bind:value={newName} maxlength="64" placeholder="e.g. CI deploy, laptop CLI" autocomplete="off" />
    </div>
    <div>
      <label class="label" for="key-expiry">Expires</label>
      <select id="key-expiry" class="input" bind:value={newExpiry}>
        <option value="0">Never</option>
        <option value="7">In 7 days</option>
        <option value="30">In 30 days</option>
        <option value="90">In 90 days</option>
        <option value="365">In 1 year</option>
      </select>
    </div>
    {#if createError}
      <p class="rounded-lg bg-danger/10 px-3 py-2 text-[13px] text-danger" role="alert">{createError}</p>
    {/if}
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (showCreate = false)}>Cancel</button>
    <button class="btn btn-primary btn-sm" type="submit" form="create-key-form" disabled={creating}>
      {creating ? 'Creating…' : 'Create key'}
    </button>
  {/snippet}
</Modal>

<Modal bind:open={showToken} title="Copy your new API key" maxWidth="md" onclose={forgetToken}>
  <p class="text-[13px] text-muted">
    This is the only time <span class="font-medium text-fg">{createdName}</span> is shown. Store it somewhere safe — if you lose it, revoke it and create a new one.
  </p>
  <div class="mt-3 flex items-center gap-2">
    <code class="min-w-0 flex-1 rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-[13px] break-all text-fg select-all">{createdToken}</code>
    <button class="btn btn-secondary btn-icon" onclick={copyToken} aria-label="Copy API key" title="Copy API key">
      {#if copied}<Check size={14} class="text-success" />{:else}<Copy size={14} />{/if}
    </button>
  </div>
  {#snippet footer()}
    <button class="btn btn-primary btn-sm" onclick={forgetToken}>Done</button>
  {/snippet}
</Modal>

<ConfirmDialog
  bind:open={confirmRevoke}
  title="Revoke API key?"
  message={revokeTarget ? `"${revokeTarget.name}" (${revokeTarget.prefix}…) stops working immediately. Scripts using it will get 401 errors.` : ''}
  confirmLabel="Revoke"
  destructive
  onconfirm={revoke}
  oncancel={() => (revokeTarget = null)} />
