<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import Modal from '$components/Modal.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Key, UserPlus, Terminal, Trash2, Download, Copy, Check, TriangleAlert, FolderSync, Info } from 'lucide-svelte';
  import { toasts } from '$lib/toast.svelte';
  import SiteTerminal from './SiteTerminal.svelte';

  let { siteId, site }: { siteId: number; site: any } = $props();

  let sshUsers = $state<any[]>([]);
  let sshLoading = $state(false);
  let sshLoaded = $state(false);
  let listError = $state('');
  let showCreateSSH = $state(false);
  let newSSH = $state({ username: '', ssh_keys: '' });
  let sshBusy = $state(false);
  let sshKeyModal = $state<{ username: string; pub: string; priv: string } | null>(null);
  let info = $state<any>(null);
  let copiedField = $state('');

  const usernameRe = /^[a-z][a-z0-9_-]{0,30}$/;
  const usernameValid = $derived(usernameRe.test(newSSH.username));

  function copyText(text: string, field: string) { navigator.clipboard.writeText(text); copiedField = field; setTimeout(() => (copiedField = ''), 1500); }

  // A default name: Linux usernames are limited to 31 characters
  // (^[a-z][a-z0-9_-]{0,30}$), long domains get a short hash suffix.
  function suggestUsername(): string {
    if (!site?.domain) return 'deploy';
    const base = 'deploy-' + site.domain.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/-+$/, '');
    let name = base;
    if (name.length > 31) {
      let h = 5381;
      for (const ch of site.domain) h = ((h << 5) + h + ch.charCodeAt(0)) >>> 0;
      name = base.slice(0, 25).replace(/-+$/, '') + '-' + h.toString(36).slice(0, 5);
    }
    // A second account needs another name.
    if (sshUsers.some((u) => u.username === name)) name = name.slice(0, 28).replace(/-+$/, '') + '-' + (sshUsers.length + 1);
    return name;
  }

  $effect(() => { if (!sshLoaded) { sshLoaded = true; loadSSH(); loadInfo(); } });

  async function loadSSH() {
    sshLoading = true;
    listError = '';
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssh-ftp`); sshUsers = d?.users || []; }
    catch (e: any) { if (e?.message !== 'session expired') listError = e?.message || 'Failed to load accounts'; }
    finally { sshLoading = false; }
  }
  async function loadInfo() {
    try { info = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssh/info`); } catch {}
  }
  function openCreate() {
    showCreateSSH = !showCreateSSH;
    if (showCreateSSH && !newSSH.username) newSSH.username = suggestUsername();
  }
  async function createSSH() {
    if (!usernameValid) return;
    sshBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/ssh-ftp`, { username: newSSH.username, ssh_keys: newSSH.ssh_keys });
      sshKeyModal = { username: newSSH.username, pub: d?.ssh_public_key || '', priv: d?.ssh_private_key || '' };
      showCreateSSH = false;
      newSSH = { username: '', ssh_keys: '' };
      await Promise.all([loadSSH(), loadInfo()]);
    } catch (e: any) { toasts.error(e.message); } finally { sshBusy = false; }
  }
  async function deleteSSH(u: any) {
    if (!confirm(`Delete the SSH/SFTP account "${u.username}"? Its key stops working at once.`)) return;
    try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/ssh-ftp/${u.id}`); sshUsers = sshUsers.filter((x) => x.id !== u.id); loadInfo(); }
    catch (e: any) { toasts.error(e.message); }
  }
  function saveKeyFile(username: string, key: string) {
    const blob = new Blob([key.endsWith('\n') ? key : key + '\n'], { type: 'application/x-pem-file' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = username; // ~/.ssh/<username>, as the commands below expect
    document.body.appendChild(a); a.click(); document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }
  async function downloadKey(u: any) {
    try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/ssh-ftp/${u.id}/key`); saveKeyFile(u.username, d.private_key); }
    catch (e: any) { toasts.error(e.message); }
  }
  const accountInfo = (username: string) => info?.accounts?.find((a: any) => a.username === username);
</script>

<div class="space-y-4">
  <SiteTerminal {siteId} {site} />

  <div class="card">
    <div class="card-header">
      <div class="flex items-center gap-2">
        <Key size={16} class="text-subtle" />
        <h3 class="card-title">SSH &amp; SFTP accounts</h3>
        <span class="badge tabular-nums">{sshUsers.length}</span>
      </div>
      <button onclick={openCreate} class="btn btn-secondary btn-sm"><UserPlus size={14} /> Add account</button>
    </div>

    {#if showCreateSSH}
      <div class="space-y-3 border-b border-line bg-surface-2/50 px-5 py-4">
        <div>
          <label for="ssh-user" class="label">Username</label>
          <input id="ssh-user" bind:value={newSSH.username} maxlength="31" autocomplete="off" class="input font-mono text-[13px] {newSSH.username && !usernameValid ? 'border-danger' : ''}" />
          <p class="hint">Lower case letters, digits, - and _; starts with a letter; at most 31 characters. It must not exist on the server yet.</p>
        </div>
        <div>
          <label for="ssh-keys" class="label">Your SSH public keys <span class="text-subtle">(optional)</span></label>
          <textarea id="ssh-keys" bind:value={newSSH.ssh_keys} rows={3} class="input resize-none font-mono text-[13px]" placeholder={'ssh-ed25519 AAAA... you@laptop'}></textarea>
          <p class="hint">A key pair is also generated for you; its private key is shown once after creating the account.</p>
        </div>
        <div class="flex items-center gap-2">
          <button onclick={createSSH} disabled={sshBusy || !usernameValid} class="btn btn-primary btn-sm">{sshBusy ? 'Creating…' : 'Create account'}</button>
          <button onclick={() => (showCreateSSH = false)} class="btn btn-ghost btn-sm">Cancel</button>
        </div>
      </div>
    {/if}

    {#if sshLoading}
      <div class="space-y-3 p-4">{#each Array(2) as _, i (i)}<div class="skeleton h-9"></div>{/each}</div>
    {:else if listError}
      <p class="px-5 py-4 text-[13px] text-danger">{listError}</p>
    {:else if sshUsers.length === 0}
      <EmptyState compact icon={Key} title="No accounts yet" description="Add an account to connect with SSH or SFTP (FileZilla, WinSCP, Cyberduck)." />
    {:else}
      <ul class="divide-y divide-line">
        {#each sshUsers as u (u.id)}
          {@const a = accountInfo(u.username)}
          <li class="space-y-2 px-5 py-3">
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-2">
                <Terminal size={14} class="shrink-0 text-subtle" />
                <span class="font-mono text-sm font-medium text-fg">{u.username}</span>
                <span class="badge max-sm:hidden">site at /htdocs</span>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button onclick={() => downloadKey(u)} class="btn btn-ghost btn-xs" title="Download the private key"><Download size={13} /><span class="max-sm:hidden">Key</span></button>
                <button onclick={() => deleteSSH(u)} class="btn btn-danger-ghost btn-icon btn-xs" aria-label="Delete account {u.username}"><Trash2 size={14} /></button>
              </div>
            </div>
            {#if a}
              <div class="flex flex-col gap-1.5">
                {#each [['ssh', a.ssh_command], ['sftp', a.sftp_command]] as [k, cmd] (k)}
                  <div class="flex items-center gap-1.5">
                    <code class="code min-w-0 flex-1 truncate text-[12px]" title={cmd}>{cmd}</code>
                    <button onclick={() => copyText(cmd, `${u.id}-${k}`)} class="btn btn-ghost btn-icon btn-xs shrink-0" aria-label="Copy {k} command">
                      {#if copiedField === `${u.id}-${k}`}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  {#if info}
    <div class="card">
      <div class="card-header">
        <div class="flex items-center gap-2">
          <FolderSync size={16} class="text-subtle" />
          <h3 class="card-title">Connect with an SFTP client</h3>
        </div>
      </div>
      <div class="card-body space-y-4">
        <dl class="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div class="col-span-2 sm:col-span-1">
            <dt class="stat-label mb-1">Host</dt>
            <dd class="flex items-center gap-1.5">
              <span class="code truncate">{info.host}</span>
              <button onclick={() => copyText(info.host, 'host')} class="btn btn-ghost btn-icon btn-xs" aria-label="Copy host">
                {#if copiedField === 'host'}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}
              </button>
            </dd>
          </div>
          <div><dt class="stat-label mb-1">Protocol</dt><dd><span class="code">SFTP</span></dd></div>
          <div><dt class="stat-label mb-1">Port</dt><dd><span class="code">{info.port || 22}</span></dd></div>
          <div><dt class="stat-label mb-1">Folder</dt><dd><span class="code">/htdocs</span></dd></div>
        </dl>
        <ol class="list-inside list-decimal space-y-1.5 rounded-lg border border-line bg-surface-2 p-4 text-[13px] text-muted">
          <li>In FileZilla, WinSCP or Cyberduck choose <strong class="font-medium text-fg">SFTP</strong>, host <span class="code">{info.host}</span>, port <span class="code">{info.port || 22}</span>.</li>
          <li>User: one of the accounts above. Authentication: <strong class="font-medium text-fg">key file</strong> (the downloaded private key). Passwords are not accepted.</li>
          <li>The site's files are in <span class="code">/htdocs</span>; uploads belong to the site's user.</li>
        </ol>
        <p class="flex items-start gap-1.5 text-[12px] text-subtle"><Info size={13} class="mt-0.5 shrink-0" /> FTP (port 21) is not offered: it sends passwords in clear text. Use SFTP.</p>
      </div>
    </div>
  {/if}
</div>

<Modal open={!!sshKeyModal} title="Account created" maxWidth="lg" onclose={() => (sshKeyModal = null)}>
  {#if sshKeyModal}
    <p class="mb-4 flex items-center gap-1.5 text-[13px] text-warning"><TriangleAlert size={14} /> Save the private key now. Keep it secret: it gives access to this site.</p>
    <div class="space-y-3">
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <span class="label mb-0">Private key</span>
          <div class="flex gap-1">
            <button onclick={() => saveKeyFile(sshKeyModal!.username, sshKeyModal!.priv)} class="btn btn-primary btn-xs"><Download size={12} /> Download</button>
            <button onclick={() => copyText(sshKeyModal!.priv, 'sshpriv')} class="btn btn-ghost btn-xs">
              {#if copiedField === 'sshpriv'}<Check size={12} class="text-success" /> Copied{:else}<Copy size={12} /> Copy{/if}
            </button>
          </div>
        </div>
        <pre class="max-h-40 overflow-auto rounded-lg border border-line bg-surface-2 p-3 font-mono text-[11px] text-fg">{sshKeyModal.priv}</pre>
        <p class="hint">Save it as <span class="code">~/.ssh/{sshKeyModal.username}</span> and run <span class="code">chmod 600 ~/.ssh/{sshKeyModal.username}</span>.</p>
      </div>
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <span class="label mb-0">Public key</span>
          <button onclick={() => copyText(sshKeyModal!.pub, 'sshpub')} class="btn btn-ghost btn-xs">
            {#if copiedField === 'sshpub'}<Check size={12} class="text-success" /> Copied{:else}<Copy size={12} /> Copy{/if}
          </button>
        </div>
        <pre class="overflow-x-auto rounded-lg border border-line bg-surface-2 p-3 font-mono text-[11px] text-fg">{sshKeyModal.pub}</pre>
      </div>
    </div>
  {/if}
</Modal>
