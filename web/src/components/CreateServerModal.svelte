<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import { Copy, Check, TriangleAlert } from 'lucide-svelte';

  let {
    open = $bindable(false),
    oncreated,
  }: {
    open: boolean;
    oncreated?: () => void;
  } = $props();

  let name = $state('');
  let hostname = $state('');
  let ip_address = $state('');
  let agent_url = $state('');
  let ssh_port = $state(22);
  let description = $state('');
  let saving = $state(false);
  let error = $state('');
  let createdToken = $state('');
  let createdServer = $state<any>(null);

  let canManage = $derived(hasRole(auth.user?.role, 'admin'));

  $effect(() => {
    if (open) {
      name = '';
      hostname = '';
      ip_address = '';
      agent_url = '';
      ssh_port = 22;
      description = '';
      error = '';
      createdToken = '';
      createdServer = null;
    }
  });

  // Auto-fill agent_url from IP when IP changes and agent_url is empty
  $effect(() => {
    if (ip_address && !agent_url) {
      agent_url = 'http://' + ip_address + ':9090';
    }
  });

  async function create() {
    saving = true;
    error = '';
    try {
      const body: any = { name, hostname };
      if (ip_address) body.ip_address = ip_address;
      if (agent_url) body.agent_url = agent_url;
      if (ssh_port) body.ssh_port = ssh_port;
      if (description) body.description = description;

      const data: any = await apiFetch(auth.token, 'POST', 'servers', body);
      createdServer = data;
      createdToken = data.agent_token || '';
      oncreated?.();
    } catch (e: any) {
      error = e.message || 'Failed to create server';
    } finally {
      saving = false;
    }
  }

  let copied = $state(false);

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(createdToken);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      // clipboard unavailable (non-HTTPS) — the token stays visible for manual copy
    }
  }

  function close() {
    open = false;
  }
</script>

{#if canManage}
  <Modal bind:open title={createdServer ? 'Server created' : 'Add server'} maxWidth="md">
    {#snippet footer()}
      {#if createdServer}
        <button onclick={close} class="btn btn-primary btn-sm">Done</button>
      {:else}
        <button onclick={close} class="btn btn-secondary btn-sm">Cancel</button>
        <button onclick={create} disabled={saving || !name || !hostname || !agent_url} class="btn btn-primary btn-sm">
          {saving ? 'Creating…' : 'Create server'}
        </button>
      {/if}
    {/snippet}

    {#if error}
      <div class="mb-4 rounded-lg bg-danger/10 px-3 py-2 text-[13px] text-danger">{error}</div>
    {/if}

    {#if createdServer}
      <div class="space-y-4">
        <div class="rounded-lg bg-success/10 px-3 py-2 text-[13px] text-success">
          Server <strong class="font-semibold">{createdServer.name}</strong> created.
        </div>

        <div>
          <p class="label">Agent token</p>
          <p class="mb-2 flex items-center gap-1.5 text-xs text-warning">
            <TriangleAlert size={12} /> Copy this token now — it won't be shown again.
          </p>
          <div class="flex items-start gap-2">
            <code class="flex-1 rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs break-all text-fg">{createdToken}</code>
            <button onclick={copyToken} class="btn btn-secondary btn-sm">
              {#if copied}<Check size={14} /> Copied{:else}<Copy size={14} /> Copy{/if}
            </button>
          </div>
        </div>

        <div class="border-t border-line pt-4">
          <p class="text-xs text-subtle">Install the agent on your server with this token:</p>
          <code class="mt-2 block rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs break-all text-muted">
            curl -sL https://panel.sigarda.com/api/install-agent.sh | bash -s -- --token {createdToken}
          </code>
        </div>
      </div>
    {:else}
      <div class="space-y-4">
        <div>
          <label class="label" for="cs-name">Name <span class="text-danger">*</span></label>
          <input id="cs-name" type="text" bind:value={name} class="input" placeholder="My server" />
        </div>
        <div>
          <label class="label" for="cs-hostname">Hostname <span class="text-danger">*</span></label>
          <input id="cs-hostname" type="text" bind:value={hostname} class="input" placeholder="server.example.com" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="label" for="cs-ip">IP address</label>
            <input id="cs-ip" type="text" bind:value={ip_address} class="input font-mono text-[13px]" placeholder="1.2.3.4" />
          </div>
          <div>
            <label class="label" for="cs-port">SSH port</label>
            <input id="cs-port" type="number" bind:value={ssh_port} min="1" max="65535" class="input tabular-nums" />
          </div>
        </div>
        <div>
          <label class="label" for="cs-agent">Agent URL <span class="text-danger">*</span></label>
          <input id="cs-agent" type="text" bind:value={agent_url} class="input font-mono text-[13px]" placeholder="http://1.2.3.4:9090" />
          <p class="hint">Auto-filled from IP. Must be reachable from this panel server.</p>
        </div>
        <div>
          <label class="label" for="cs-desc">Description</label>
          <textarea id="cs-desc" bind:value={description} rows="2" class="input resize-none" placeholder="Optional notes about this server…"></textarea>
        </div>
      </div>
    {/if}
  </Modal>
{/if}
