<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Key, UserPlus, Terminal, Trash2, Download, Copy, Check, TriangleAlert } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';
  import SiteTerminal from './SiteTerminal.svelte';

  let { siteId, site }: { siteId: number; site: any } = $props();

  let sshUsers = $state<any[]>([]);
  let sshLoading = $state(false);
  let sshLoaded = $state(false);
  let showCreateSSH = $state(false);
  let newSSH = $state({ username: '', ssh_keys: '' });
  let sshBusy = $state(false);
  let sshKeyModal = $state<{ pub: string; priv: string } | null>(null);
  let sshInfoData = $state<any>(null);
  let sshInfoLoaded = $state(false);
  let copiedField = $state('');

  const isAdmin = $derived(hasRole(auth.user?.role, 'admin'));

  function copyText(text: string, field: string) { navigator.clipboard.writeText(text); copiedField = field; setTimeout(() => copiedField = '', 1500); }
  function sshUsername(): string { if (!site?.domain) return 'deploy'; return 'deploy-' + site.domain.replace(/\./g, '-'); }

  $effect(() => { if (sshUsers.length === 0 && !sshLoading && sshLoaded === false) { sshLoaded = true; loadSSH(); } });
  $effect(() => { if (!sshInfoLoaded) { sshInfoLoaded = true; loadSSHInfo(); } });

  async function loadSSH() {
    sshLoading = true;
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssh-ftp`); sshUsers = d?.users || []; }
    catch {} finally { sshLoading = false; }
  }
  async function createSSH() {
    sshBusy = true;
    try { const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/ssh-ftp`, { username: sshUsername(), ssh_keys: newSSH.ssh_keys }); sshKeyModal = { pub: d?.ssh_public_key || '', priv: d?.ssh_private_key || '' }; showCreateSSH = false; newSSH = { username: '', ssh_keys: '' }; await loadSSH(); }
    catch (e: any) { toasts.error(e.message); } finally { sshBusy = false; }
  }
  async function deleteSSH(u: any) {
    if (!confirm(`Delete SSH user "${u.username}"?`)) return;
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/ssh-ftp/${u.id}`); sshUsers = sshUsers.filter(x => x.id !== u.id); }
    catch (e: any) { toasts.error(e.message); }
  }
  async function loadSSHInfo() {
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssh/info`); if (d) sshInfoData = d; } catch {}
    sshInfoLoaded = true;
  }
</script>

<div class="space-y-4">
  <SiteTerminal {siteId} {site} />

  {#if sshInfoData}
    <div class="card">
      <div class="card-header">
        <div class="flex items-center gap-2">
          <Download size={16} class="text-subtle" />
          <h3 class="card-title">FTP access</h3>
          <span class="badge">ProFTPD</span>
        </div>
      </div>
      <div class="card-body space-y-4">
        <dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <dt class="stat-label mb-1">Host</dt>
            <dd class="flex items-center gap-1.5">
              <span class="code">{sshInfoData.host}</span>
              <button onclick={() => copyText(sshInfoData.host, 'host')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy host">
                {#if copiedField === 'host'}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}
              </button>
            </dd>
          </div>
          <div><dt class="stat-label mb-1">Port</dt><dd><span class="code">21</span></dd></div>
          <div><dt class="stat-label mb-1">Username</dt><dd><span class="code">{sshInfoData.system_user}</span></dd></div>
          <div class="min-w-0"><dt class="stat-label mb-1">Root path</dt><dd class="truncate"><span class="code" title={sshInfoData.root_path}>{sshInfoData.root_path}</span></dd></div>
        </dl>
        <div class="rounded-lg border border-line bg-surface-2 p-4">
          <p class="mb-2 flex items-center gap-2 text-[13px] font-medium text-fg"><Terminal size={14} class="text-subtle" /> Quick guide</p>
          <ol class="list-inside list-decimal space-y-1.5 text-[13px] text-muted">
            <li>Open your FTP client (FileZilla, WinSCP, Cyberduck)</li>
            <li>Enter host <span class="code">{sshInfoData.host}</span> port <span class="code">21</span></li>
            <li>Username: <span class="code">{sshInfoData.system_user}</span></li>
            <li>For SSH: <span class="code break-all">{sshInfoData.ssh_command}</span></li>
          </ol>
        </div>
      </div>
    </div>
  {/if}

  <div class="card">
    <div class="card-header">
      <div class="flex items-center gap-2">
        <Key size={16} class="text-subtle" />
        <h3 class="card-title">SSH access</h3>
        <span class="badge tabular-nums">{sshUsers.length}</span>
      </div>
      {#if isAdmin}
        <button onclick={() => (showCreateSSH = !showCreateSSH)} class="btn btn-secondary btn-sm"><UserPlus size={14} /> Add SSH user</button>
      {/if}
    </div>
    {#if showCreateSSH}
      <div class="space-y-3 border-b border-line bg-surface-2/50 px-5 py-4">
        <div>
          <span class="label">Username</span>
          <div class="input flex items-center font-mono text-[13px] text-muted">{sshUsername()}</div>
        </div>
        <div>
          <label for="ssh-keys" class="label">SSH public keys <span class="text-subtle">(optional)</span></label>
          <textarea id="ssh-keys" bind:value={newSSH.ssh_keys} rows={4} class="input resize-none font-mono text-[13px]" placeholder={"ssh-ed25519 AAAA... user1@host\nssh-ed25519 BBBB... user2@host"}></textarea>
        </div>
        <div class="flex items-center gap-2">
          <button onclick={createSSH} disabled={sshBusy} class="btn btn-primary btn-sm">{sshBusy ? 'Creating…' : 'Create'}</button>
          <button onclick={() => (showCreateSSH = false)} class="btn btn-ghost btn-sm">Cancel</button>
        </div>
      </div>
    {/if}
    {#if sshLoading}
      <div class="space-y-3 p-4">{#each Array(2) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
    {:else if sshUsers.length === 0}
      <EmptyState compact icon={Key} title="No SSH users" description="Add an SSH user for this site." />
    {:else}
      <ul class="divide-y divide-line">
        {#each sshUsers as u (u.id)}
          <li class="flex items-center justify-between gap-3 px-5 py-3">
            <div class="flex min-w-0 items-center gap-3">
              <Terminal size={14} class="shrink-0 text-subtle" />
              <span class="text-sm font-medium text-fg">{u.username}</span>
              <span class="truncate font-mono text-xs text-subtle max-sm:hidden">{u.home_dir || '—'}</span>
            </div>
            <button onclick={() => deleteSSH(u)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete SSH user {u.username}"><Trash2 size={14} /></button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<Modal open={!!sshKeyModal} title="SSH keys generated" maxWidth="lg" onclose={() => (sshKeyModal = null)}>
  {#if sshKeyModal}
    <p class="mb-4 flex items-center gap-1.5 text-[13px] text-warning"><TriangleAlert size={14} /> Save the private key now — it won't be shown again.</p>
    <div class="space-y-3">
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <span class="label mb-0">Public key</span>
          <button onclick={() => copyText(sshKeyModal!.pub, 'sshpub')} class="btn btn-ghost btn-xs">
            {#if copiedField === 'sshpub'}<Check size={12} class="text-success" /> Copied{:else}<Copy size={12} /> Copy{/if}
          </button>
        </div>
        <pre class="overflow-x-auto rounded-lg border border-line bg-surface-2 p-3 font-mono text-[11px] text-fg">{sshKeyModal.pub}</pre>
      </div>
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <span class="label mb-0">Private key</span>
          <button onclick={() => copyText(sshKeyModal!.priv, 'sshpriv')} class="btn btn-ghost btn-xs">
            {#if copiedField === 'sshpriv'}<Check size={12} class="text-success" /> Copied{:else}<Copy size={12} /> Copy{/if}
          </button>
        </div>
        <pre class="max-h-40 overflow-auto rounded-lg border border-line bg-surface-2 p-3 font-mono text-[11px] text-fg">{sshKeyModal.priv}</pre>
      </div>
    </div>
  {/if}
</Modal>
