<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';

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

  function close() {
    open = false;
  }
</script>

{#if canManage}
  <Modal bind:open title={createdServer ? 'Server Created' : 'Add Server'} maxWidth="md">
    {#snippet footer()}
      {#if createdServer}
        <button
          onclick={close}
          class="px-4 py-2 text-sm rounded-lg bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] font-semibold hover:-translate-y-0.5 transition-all duration-150 cursor-pointer border-0"
        >Done</button>
      {:else}
        <button
          onclick={close}
          class="px-4 py-2 text-sm rounded-lg bg-gray-700/50 text-gray-300 hover:bg-gray-700 transition cursor-pointer border-0"
        >Cancel</button>
        <button
          onclick={create}
          disabled={saving || !name || !hostname || !agent_url}
          class="px-4 py-2 text-sm rounded-lg bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] font-semibold hover:-translate-y-0.5 transition-all duration-150 cursor-pointer border-0 disabled:opacity-50"
        >{saving ? 'Creating...' : 'Create Server'}</button>
      {/if}
    {/snippet}

    {#if error}
      <div class="mb-4 px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm">{error}</div>
    {/if}

    {#if createdServer}
      <div class="flex flex-col gap-4">
        <div class="px-3 py-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-sm">
          Server <strong>{createdServer.name}</strong> created successfully.
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">Agent Token</label>
          <p class="text-xs text-yellow-400 mb-2">⚠ Copy this token now — it won't be shown again.</p>
          <div class="flex items-center gap-2">
            <code class="flex-1 px-3 py-2 bg-gray-800/80 border border-gray-700/50 rounded-lg text-emerald-300 text-xs font-mono break-all">{createdToken}</code>
            <button
              onclick={() => navigator.clipboard.writeText(createdToken)}
              class="px-3 py-2 text-xs rounded-lg bg-gray-700/50 text-gray-300 hover:bg-gray-700 hover:text-white transition cursor-pointer border-0 whitespace-nowrap"
            >Copy</button>
          </div>
        </div>

        <div class="border-t border-gray-700/50 pt-4">
          <p class="text-xs text-slate-500">Install the agent on your server with this token:</p>
          <code class="block mt-2 px-3 py-2 bg-gray-800/80 border border-gray-700/50 rounded-lg text-slate-300 text-xs font-mono">
            curl -sL https://panel.sigarda.com/api/install-agent.sh | bash -s -- --token {createdToken}
          </code>
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">Name *</label>
          <input
            type="text"
            bind:value={name}
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
            placeholder="My Server"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">Hostname *</label>
          <input
            type="text"
            bind:value={hostname}
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
            placeholder="server.example.com"
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
          <label class="block text-sm font-medium text-gray-400 mb-1">Agent URL <span class="text-yellow-400">*</span></label>
          <input
            type="text"
            bind:value={agent_url}
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50"
            placeholder="http://1.2.3.4:9090"
          />
          <p class="text-xs text-slate-500 mt-1">Auto-filled from IP. Must be reachable from this panel server.</p>
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-400 mb-1">Description</label>
          <textarea
            bind:value={description}
            rows="2"
            class="w-full px-3 py-2 bg-gray-800/50 border border-gray-700/50 rounded-lg text-white text-sm focus:outline-none focus:ring-1 focus:ring-sky-500/50 resize-none"
            placeholder="Optional notes about this server..."
          ></textarea>
        </div>
      </div>
    {/if}
  </Modal>
{/if}
