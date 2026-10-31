<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { hasRole } from '$lib/auth.svelte';
  import { Copy, Check, RefreshCw } from 'lucide-svelte';

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
  let ssh_port = $state(22);
  let description = $state('');
  let saving = $state(false);
  let error = $state('');
  let agentToken = $state('');
  let tokenCopied = $state(false);
  let regenerating = $state(false);

  async function loadServer() {
    if (!server?.id) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${server.id}`);
      if (data) {
        agentToken = data.agent_token || '';
        name = data.name || '';
        hostname = data.hostname || '';
        ip_address = data.ip_address || '';
        ssh_port = data.ssh_port || 22;
        description = data.description || '';
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
      if (ssh_port !== (server.ssh_port || 22)) body.ssh_port = ssh_port;
      if (description !== (server.description || '')) body.description = description;

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
      await navigator.clipboard.writeText(agentToken);
      tokenCopied = true;
      setTimeout(() => (tokenCopied = false), 2000);
    } catch {
      // Fallback
      const el = document.createElement('textarea');
      el.value = agentToken;
      document.body.appendChild(el);
      el.select();
      document.execCommand('copy');
      document.body.removeChild(el);
      tokenCopied = true;
      setTimeout(() => (tokenCopied = false), 2000);
    }
  }

  let confirmRegen = $state(false);

  async function regenerateToken() {
    regenerating = true;
    error = '';
    try {
      const data = await apiFetch(auth.token, 'POST', `servers/${server.id}/token`);
      agentToken = data.agent_token || '';
    } catch (e: any) {
      error = e.message || 'Failed to regenerate token';
    } finally {
      regenerating = false;
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
      <div class="mb-4 rounded-lg bg-danger/10 px-3 py-2 text-[13px] text-danger">{error}</div>
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
        <label class="label" for="es-token">Agent token</label>
        <div class="flex gap-2">
          <input id="es-token" type="text" value={agentToken} readonly class="input flex-1 font-mono text-xs" placeholder="Loading…" />
          <button onclick={copyToken} class="btn btn-secondary btn-icon" aria-label="Copy token" title="Copy token">
            {#if tokenCopied}<Check size={14} class="text-success" />{:else}<Copy size={14} />{/if}
          </button>
          <button onclick={() => (confirmRegen = true)} disabled={regenerating} class="btn btn-secondary">
            <RefreshCw size={14} class={regenerating ? 'animate-spin' : ''} /> Regenerate
          </button>
        </div>
        <p class="hint">Used on the target server to authenticate with the panel.</p>
      </div>
      <div>
        <label class="label" for="es-desc">Description</label>
        <textarea id="es-desc" bind:value={description} rows="2" class="input resize-none" placeholder="Optional notes…"></textarea>
      </div>
    </div>
  </Modal>

  <ConfirmDialog
    bind:open={confirmRegen}
    title="Regenerate agent token?"
    message="The current token will stop working immediately. Update the agent on the server with the new token."
    confirmLabel="Regenerate"
    destructive
    onconfirm={regenerateToken}
  />
{/if}
