<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtDateTime } from '$lib/format';
  import Modal from '$components/Modal.svelte';
  import { Copy, Check, TriangleAlert, Terminal, ListChecks, LoaderCircle, CircleCheck } from 'lucide-svelte';
  import { onDestroy } from 'svelte';

  let {
    open = $bindable(false),
    oncreated,
  }: {
    open: boolean;
    oncreated?: () => void;
  } = $props();

  const DOCS = 'https://github.com/bayurstarcool/SigardaPanel/blob/main/docs/INSTALL.md#adding-remote-servers';

  let mode = $state<'command' | 'manual'>('command');
  let error = $state('');

  // ── One-line installer ──
  let generating = $state(false);
  let reg = $state<any>(null); // POST agents/tokens response
  let agentIP = $state('');
  let knownIds = new Set<number>();
  let registered = $state<any>(null);
  let poll: ReturnType<typeof setInterval> | null = null;

  // ── Manual ──
  let name = $state('');
  let hostname = $state('');
  let ip_address = $state('');
  let agent_url = $state('');
  let agentUrlEdited = $state(false);
  let ssh_port = $state(22);
  let description = $state('');
  let saving = $state(false);
  let createdToken = $state('');
  let createdServer = $state<any>(null);
  let panelVersion = $state('');

  let canManage = $derived(hasRole(auth.user?.role, 'admin'));
  let copied = $state('');

  $effect(() => {
    if (open) {
      mode = 'command';
      error = '';
      reg = null;
      agentIP = '';
      registered = null;
      name = '';
      hostname = '';
      ip_address = '';
      agent_url = '';
      agentUrlEdited = false;
      ssh_port = 22;
      description = '';
      createdToken = '';
      createdServer = null;
      fetch('/api/v1/version')
        .then((r) => (r.ok ? r.json() : null))
        .then((d) => (panelVersion = d?.data?.version && d.data.version !== 'dev' ? 'v' + d.data.version : ''))
        .catch(() => {});
    } else {
      stopPolling();
    }
  });

  onDestroy(stopPolling);

  // Follow the IP while the agent URL has not been edited by hand. (The old
  // "fill once if empty" logic captured only the first typed character.)
  $effect(() => {
    if (!agentUrlEdited) agent_url = ip_address.trim() ? `http://${ip_address.trim()}:7710` : '';
  });

  function sq(v: string) {
    return `'${v.replace(/'/g, `'\\''`)}'`;
  }

  let installCommand = $derived.by(() => {
    const base: string = reg?.install_command || '';
    if (!base) return '';
    const ip = agentIP.trim();
    return ip ? base.replace(/\| sudo bash$/, `| sudo SIGARDAPANEL_AGENT_IP=${sq(ip)} bash`) : base;
  });

  let agentEnv = $derived(
    `install -d -m 750 /etc/sigardapanel\ncat > /etc/sigardapanel/agent.env <<'EOF'\nSIGARDAPANEL_AGENT_ADDR=0.0.0.0:7710\nSIGARDAPANEL_AGENT_TOKEN=${createdToken}\nEOF\nchmod 600 /etc/sigardapanel/agent.env`,
  );

  async function generate() {
    generating = true;
    error = '';
    try {
      const list: any = await apiFetch(auth.token, 'GET', 'servers');
      knownIds = new Set((list?.servers ?? []).map((s: any) => s.id));
      reg = await apiFetch(auth.token, 'POST', 'agents/tokens');
      registered = null;
      if (reg?.install_command) startPolling();
    } catch (e: any) {
      error = e.message || 'Failed to create a registration token';
    } finally {
      generating = false;
    }
  }

  function startPolling() {
    stopPolling();
    poll = setInterval(async () => {
      try {
        const list: any = await apiFetch(auth.token, 'GET', 'servers');
        const fresh = (list?.servers ?? []).find((s: any) => !knownIds.has(s.id));
        if (fresh) {
          registered = fresh;
          stopPolling();
          oncreated?.();
        }
      } catch {
        // keep polling; transient errors are fine here
      }
    }, 5000);
  }

  function stopPolling() {
    if (poll) clearInterval(poll);
    poll = null;
  }

  async function create() {
    saving = true;
    error = '';
    try {
      const body: any = { name, hostname };
      if (ip_address) body.ip_address = ip_address.trim();
      if (agent_url) body.agent_url = agent_url.trim();
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

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = key;
      setTimeout(() => (copied = ''), 2000);
    } catch {
      // clipboard unavailable (non-HTTPS): the text stays visible for manual copy
    }
  }

  function close() {
    open = false;
  }
</script>

{#snippet copyBlock(text: string, key: string, label: string)}
  <div class="flex items-start gap-2">
    <pre class="flex-1 overflow-x-auto rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all text-fg" aria-label={label}>{text}</pre>
    <button onclick={() => copy(text, key)} class="btn btn-secondary btn-sm" aria-label="Copy {label}">
      {#if copied === key}<Check size={14} /> Copied{:else}<Copy size={14} /> Copy{/if}
    </button>
  </div>
{/snippet}

{#if canManage}
  <Modal bind:open title={createdServer ? 'Server created' : 'Add server'} maxWidth="xl">
    {#snippet footer()}
      {#if createdServer || registered}
        <button onclick={close} class="btn btn-primary btn-sm">Done</button>
      {:else if mode === 'manual'}
        <button onclick={close} class="btn btn-secondary btn-sm">Cancel</button>
        <button onclick={create} disabled={saving || !name || !hostname || !agent_url} class="btn btn-primary btn-sm">
          {saving ? 'Creating…' : 'Create server'}
        </button>
      {:else}
        <button onclick={close} class="btn btn-secondary btn-sm">Close</button>
      {/if}
    {/snippet}

    {#if error}
      <div class="mb-4 rounded-lg bg-danger/10 px-3 py-2 text-[13px] text-danger" role="alert">{error}</div>
    {/if}

    {#if !createdServer}
      <div class="tabs mb-4" role="tablist">
        <button role="tab" aria-selected={mode === 'command'} class="tab {mode === 'command' ? 'tab-active' : ''}" onclick={() => (mode = 'command')}>
          <Terminal size={14} /> Install command
        </button>
        <button role="tab" aria-selected={mode === 'manual'} class="tab {mode === 'manual' ? 'tab-active' : ''}" onclick={() => (mode = 'manual')}>
          <ListChecks size={14} /> Manual setup
        </button>
      </div>
    {/if}

    {#if createdServer}
      <div class="space-y-4">
        <div class="rounded-lg bg-success/10 px-3 py-2 text-[13px] text-success">
          Server <strong class="font-semibold">{createdServer.name}</strong> created. It shows as online once its agent answers.
        </div>

        <div>
          <p class="label">Agent token</p>
          <p class="mb-2 flex items-center gap-1.5 text-xs text-warning">
            <TriangleAlert size={12} /> Copy it now. It is not shown again; <em>Edit → Regenerate</em> issues a new one.
          </p>
          {@render copyBlock(createdToken, 'token', 'agent token')}
        </div>

        <div>
          <p class="label">On the new server, as root</p>
          <ol class="mb-2 list-decimal space-y-1 pl-5 text-xs text-muted">
            <li>Install the <code class="code">sigardapanel</code> binary {panelVersion ? `(${panelVersion}, the panel's version)` : '(same version as the panel)'} and verify it, as in the guide.</li>
            <li>Write the agent configuration:</li>
          </ol>
          {@render copyBlock(agentEnv, 'env', 'agent.env commands')}
          <ol start="3" class="mt-2 list-decimal space-y-1 pl-5 text-xs text-muted">
            <li>Create the <code class="code">sigardapanel-agent</code> systemd unit and start it.</li>
            <li>Allow port 7710 only from this panel: <code class="code">ufw allow from &lt;panel-ip&gt; to any port 7710 proto tcp</code></li>
          </ol>
          <p class="mt-2 text-xs text-subtle">
            Step by step: <a class="link" href={DOCS} target="_blank" rel="noopener noreferrer">Adding remote servers</a>.
          </p>
        </div>
      </div>
    {:else if mode === 'command'}
      <div class="space-y-4 text-[13px]">
        <p class="text-muted">
          Run one command as root on a fresh Ubuntu/Debian server. It installs the agent
          {reg?.agent_version && reg.agent_version !== 'latest' ? `(${reg.agent_version})` : ''}, verifies the signed release
          and registers the server here. The command contains a one-time token that expires after one hour.
        </p>

        {#if !reg}
          <button onclick={generate} disabled={generating} class="btn btn-primary btn-sm">
            {generating ? 'Generating…' : 'Generate install command'}
          </button>
        {:else if !reg.install_command}
          <div class="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-fg" role="alert">
            <p class="font-semibold">The one-line installer is not available on this panel.</p>
            <p class="mt-1 text-muted">
              {reg.installer_error}. The installer only talks to the panel over HTTPS: set
              <code class="code">SIGARDAPANEL_PUBLIC_URL=https://…</code> in <code class="code">/etc/sigardapanel/panel.env</code>
              and restart <code class="code">sigardapanel-api</code>, or use <button class="link" onclick={() => (mode = 'manual')}>Manual setup</button>.
            </p>
          </div>
        {:else}
          <div>
            <label class="label" for="cs-agent-ip">Address the panel uses to reach the server <span class="text-subtle">(optional)</span></label>
            <input id="cs-agent-ip" type="text" bind:value={agentIP} class="input font-mono text-[13px]" placeholder="detected public IP" />
            <p class="hint">
              {#if reg.allow_private_agents}
                Enter the private/VPC address if the panel reaches the server over a private network.
              {:else}
                Public addresses only. For private/VPC addresses set <code class="code">SIGARDAPANEL_ALLOW_PRIVATE_AGENTS=true</code> in the panel's <code class="code">panel.env</code> first, or use Manual setup.
              {/if}
            </p>
          </div>

          <div>
            <p class="label">Install command</p>
            {@render copyBlock(installCommand, 'cmd', 'install command')}
            <p class="hint">Expires {fmtDateTime(reg.expires_at)}. Single use. Treat it like a password.</p>
          </div>

          <ul class="list-disc space-y-1 pl-5 text-xs text-muted">
            <li>The server needs outbound HTTPS to <code class="code">{reg.panel_url}</code> and GitHub, and the panel must reach the server on port 7710.</li>
            <li>The panel's certificate must be trusted by the server. With a self-signed certificate, copy it to the server and add <code class="code">SIGARDAPANEL_PANEL_CA=/path/panel.crt</code> before <code class="code">bash</code> (and <code class="code">--cacert</code> to the first curl).</li>
            <li>Afterwards allow port 7710 only from this panel's address.</li>
          </ul>

          {#if registered}
            <div class="flex items-center gap-2 rounded-lg bg-success/10 px-3 py-2 text-success">
              <CircleCheck size={16} /> <span><strong>{registered.name}</strong> registered ({registered.ip_address}). Open its Stack page to install software.</span>
            </div>
          {:else}
            <div class="flex items-center gap-2 text-xs text-subtle">
              <LoaderCircle size={14} class="animate-spin" /> Waiting for the server to register…
            </div>
          {/if}
        {/if}
      </div>
    {:else}
      <div class="space-y-4">
        <p class="text-[13px] text-muted">
          Register the server here, then install and configure the agent yourself. Use this for private networks,
          panels without a trusted HTTPS certificate, or when you prefer to run each step.
        </p>
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
          <input id="cs-agent" type="text" bind:value={agent_url} oninput={() => (agentUrlEdited = true)} class="input font-mono text-[13px]" placeholder="http://1.2.3.4:7710" />
          <p class="hint">Filled in from the IP address. Must be reachable from this panel (a private/VPN address is fine).</p>
        </div>
        <div>
          <label class="label" for="cs-desc">Description</label>
          <textarea id="cs-desc" bind:value={description} rows="2" class="input resize-none" placeholder="Optional notes about this server…"></textarea>
        </div>
      </div>
    {/if}
  </Modal>
{/if}
