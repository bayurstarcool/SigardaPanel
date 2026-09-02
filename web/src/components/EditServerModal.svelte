<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import { hasRole } from '$lib/auth.svelte';

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

  async function regenerateToken() {
    if (!confirm('Regenerate agent token? The current token will stop working immediately.')) return;
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
  <Modal bind:open title="Edit Server" maxWidth="md">
    {#snippet footer()}
      <button
        onclick={() => (open = false)}
        class="px-4 py-2 text-sm rounded-lg bg-gray-700/50 text-gray-300 hover:bg-gray-700 transition cursor-pointer border-0"
      >Cancel</button>
      <button
        onclick={save}
        disabled={saving}
        class="px-4 py-2 text-sm rounded-lg bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] font-semibold hover:-translate-y-0.5 transition-all duration-150 cursor-pointer border-0 disabled:opacity-50"
      >{saving ? 'Saving...' : 'Save Changes'}</button>
    {/snippet}

    {#if error}
      <div class="mb-4 px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm">{error}</div>
    {/if}

    <div class="flex flex-col gap-4">
      <div>
        <label class="block text-sm font-medium text-gray-400 mb-1">Name</label>
        <input
          type="text"
          bind:value={name}
          class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
          placeholder="My Server"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-400 mb-1">Hostname</label>
        <input
          type="text"
          bind:value={hostname}
          class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
          placeholder="example.com"
        />
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">IP Address</label>
          <input
            type="text"
            bind:value={ip_address}
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
            placeholder="1.2.3.4"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">SSH Port</label>
          <input
            type="number"
            bind:value={ssh_port}
            min="1"
            max="65535"
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
          />
        </div>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-400 mb-1">Agent Token</label>
        <div class="flex gap-2">
          <input
            type="text"
            value={agentToken}
            readonly
            class="flex-1 px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm font-mono text-xs focus:outline-none"
            placeholder="Loading..."
          />
          <button
            onclick={copyToken}
            class="px-3 py-2 rounded-lg bg-gray-700/50 text-gray-300 hover:bg-gray-700 transition cursor-pointer border-0 text-xs whitespace-nowrap"
          >{tokenCopied ? 'Copied!' : 'Copy'}</button>
          <button
            onclick={regenerateToken}
            disabled={regenerating}
            class="px-3 py-2 rounded-lg bg-amber-600/20 text-amber-400 hover:bg-amber-600/30 transition cursor-pointer border-0 text-xs whitespace-nowrap disabled:opacity-50"
          >{regenerating ? '...' : 'Regen'}</button>
        </div>
        <p class="text-[0.65rem] text-slate-600 mt-1">Used on the target server to authenticate with the panel</p>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-400 mb-1">Description</label>
        <textarea
          bind:value={description}
          rows="2"
          class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50 resize-none"
          placeholder="Optional notes..."
        ></textarea>
      </div>
    </div>
  </Modal>
{/if}
