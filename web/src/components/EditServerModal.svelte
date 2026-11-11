<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { hasRole } from '$lib/auth.svelte';
  import { Copy, Check, RefreshCw, Trash2, TriangleAlert } from 'lucide-svelte';

  let {
    open = $bindable(false),
    server = null,
    onsaved,
  }: {
    open: boolean;
    server: any;
    onsaved?: () => void;
  } = $props();

  let name = $state('');
  let hostname = $state('');
  let ip_address = $state('');
  let agent_url = $state('');
  let ssh_port = $state(22);
  let description = $state('');
  let isLocal = $state(false);
  let saving = $state(false);
  let error = $state('');
  // The API never returns an existing token; only a regenerated one, once.
  let newToken = $state('');
  let tokenCopied = $state(false);
  let regenerating = $state(false);
  let deleting = $state(false);

  async function loadServer() {
    if (!server?.id) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${server.id}`);
      if (data) {
        name = data.name || '';
        hostname = data.hostname || '';
        ip_address = data.ip_address || '';
        agent_url = data.agent_url || '';
        ssh_port = data.ssh_port || 22;
        description = data.description || '';
        isLocal = !!data.is_local;
      }
    } catch (e: any) {
      if (e.message !== 'session expired') {
        error = 'Failed to load server details';
      }
    }
  }

  $effect(() => {
    if (open && server) {
      error = '';
      newToken = '';
      tokenCopied = false;
      loadServer();
    }
  });

  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  async function save() {
    saving = true;
    error = '';
    try {
      const body: any = {};
      if (name !== server.name) body.name = name;
      if (hostname !== server.hostname) body.hostname = hostname;
      if (ip_address !== (server.ip_address || '')) body.ip_address = ip_address;
      if (!isLocal && agent_url.trim() !== (server.agent_url || '')) body.agent_url = agent_url.trim();
      if (ssh_port !== (server.ssh_port || 22)) body.ssh_port = ssh_port;
      if (description !== (server.description || '')) body.description = description;
      if (Object.keys(body).length === 0) {
        open = false;
        return;
      }

      await apiFetch(auth.token, 'PATCH', `servers/${server.id}`, body);
      open = false;
      onsaved?.();
    } catch (e: any) {
      error = e.message || 'Failed to update server';
    } finally {
      saving = false;
    }
  }

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(newToken);
      tokenCopied = true;
      setTimeout(() => (tokenCopied = false), 2000);
    } catch {
      // clipboard unavailable (non-HTTPS): the token stays visible for manual copy
    }
  }

  let confirmRegen = $state(false);
  let confirmDelete = $state(false);

  async function regenerateToken() {
    regenerating = true;
    error = '';
    try {
      const data = await apiFetch(auth.token, 'POST', `servers/${server.id}/token`);
      newToken = data.agent_token || '';
    } catch (e: any) {
      error = e.message || 'Failed to regenerate token';
    } finally {
      regenerating = false;
    }
  }

  async function removeServer() {
    deleting = true;
    error = '';
    try {
      await apiFetch(auth.token, 'DELETE', `servers/${server.id}`);
      open = false;
      onsaved?.();
    } catch (e: any) {
      error = e.message || 'Failed to remove server';
    } finally {
      deleting = false;
    }
  }
</script>

{#if canManage}
  <Modal bind:open title="Edit server" maxWidth="md">
    {#snippet footer()}
      <button onclick={() => (open = false)} class="btn btn-secondary btn-sm">Cancel</button>
      <button onclick={save} disabled={saving} class="btn btn-primary btn-sm">{saving ? 'Saving…' : 'Save changes'}</button>
    {/snippet}

    {#if error}
      <div class="mb-4 rounded-lg bg-danger/10 px-3 py-2 text-[13px] text-danger" role="alert">{error}</div>
    {/if}

    <div class="space-y-4">
      <div>
        <label class="label" for="es-name">Name</label>
        <input id="es-name" type="text" bind:value={name} class="input" placeholder="My server" />
      </div>
      <div>
        <label class="label" for="es-hostname">Hostname</label>
        <input id="es-hostname" type="text" bind:value={hostname} class="input" placeholder="example.com" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="label" for="es-ip">IP address</label>
          <input id="es-ip" type="text" bind:value={ip_address} class="input font-mono text-[13px]" placeholder="1.2.3.4" />
        </div>
        <div>
          <label class="label" for="es-port">SSH port</label>
          <input id="es-port" type="number" bind:value={ssh_port} min="1" max="65535" class="input tabular-nums" />
        </div>
      </div>
      <div>
        <label class="label" for="es-agent">Agent URL</label>
        <input id="es-agent" type="text" bind:value={agent_url} disabled={isLocal} class="input font-mono text-[13px]" placeholder="http://1.2.3.4:7710" />
        <p class="hint">{isLocal ? "The panel's own agent, reached locally." : "Where the panel reaches this server's agent."}</p>
      </div>
      <div>
        <label class="label" for="es-desc">Description</label>
        <textarea id="es-desc" bind:value={description} rows="2" class="input resize-none" placeholder="Optional notes…"></textarea>
      </div>

      <div class="border-t border-line pt-4">
        <p class="label">Agent token</p>
        {#if isLocal}
          <p class="hint">
            The panel's own agent token is kept in <code class="code">/etc/sigardapanel/panel.env</code> and
            <code class="code">agent.env</code>. Rotate it on the panel host with <code class="code">sudo sigardapanel server token</code>.
          </p>
        {:else if newToken}
          <p class="mb-2 flex items-center gap-1.5 text-xs text-warning">
            <TriangleAlert size={12} /> New token, shown once. Put it into the server's <code class="code">/etc/sigardapanel/agent.env</code> and restart <code class="code">sigardapanel-agent</code>.
          </p>
          <div class="flex gap-2">
            <input id="es-token" type="text" value={newToken} readonly class="input flex-1 font-mono text-xs" />
            <button onclick={copyToken} class="btn btn-secondary btn-icon" aria-label="Copy token" title="Copy token">
              {#if tokenCopied}<Check size={14} class="text-success" />{:else}<Copy size={14} />{/if}
            </button>
          </div>
        {:else}
          <div class="flex items-center justify-between gap-3">
            <p class="hint">Not displayed for security. Regenerate to issue a new one (the old one stops working).</p>
            <button onclick={() => (confirmRegen = true)} disabled={regenerating} class="btn btn-secondary btn-sm shrink-0">
              <RefreshCw size={14} class={regenerating ? 'animate-spin' : ''} /> Regenerate
            </button>
          </div>
        {/if}
      </div>

      {#if !isLocal}
        <div class="border-t border-line pt-4">
          <div class="flex items-center justify-between gap-3">
            <p class="hint">Remove the server from the panel. Delete or transfer its sites first; nothing on the machine is changed.</p>
            <button onclick={() => (confirmDelete = true)} disabled={deleting} class="btn btn-danger btn-sm shrink-0">
              <Trash2 size={14} /> Remove
            </button>
          </div>
        </div>
      {/if}
    </div>
  </Modal>

  <ConfirmDialog
    bind:open={confirmRegen}
    title="Regenerate agent token?"
    message="The current token stops working immediately. The server shows as unauthorized until its agent.env has the new token and the agent was restarted."
    confirmLabel="Regenerate"
    destructive
    onconfirm={regenerateToken}
  />
  <ConfirmDialog
    bind:open={confirmDelete}
    title="Remove server?"
    message="The panel stops managing this server. The agent keeps running on the machine until you uninstall it."
    confirmLabel="Remove"
    destructive
    onconfirm={removeServer}
  />
{/if}
