<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import {
    Key, UserPlus, Terminal, Trash2, Download, Lock, Loader2,
    Copy, Check, X, AlertTriangle,
  } from 'lucide-svelte';
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

<!-- ═══════ Per-Site SSH Terminal ═══════ -->
<div class="mb-3">
  <SiteTerminal {siteId} {site} />
</div>

{#if sshInfoData}
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300 mb-3">
    <div class="flex items-center gap-2.5 px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-emerald-400/10 flex items-center justify-center"><Download size={13} class="text-emerald-400" /></div><span class="text-sm font-semibold text-slate-200">FTP Access</span><span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">ProFTPD</span></div>
    <div class="p-3.5 sm:p-5">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
        <div><div class="text-[11px] text-slate-500 mb-1">Host</div><div class="flex items-center gap-2"><code class="text-sm text-slate-200 font-mono bg-black/30 px-2 py-1 rounded-lg">{sshInfoData.host}</code><button onclick={() => copyText(sshInfoData.host, 'host')} class="text-sky-400/60 hover:text-sky-400 transition"><Copy size={12} /></button></div></div>
        <div><div class="text-[11px] text-slate-500 mb-1">Port</div><code class="text-sm text-slate-200 font-mono bg-black/30 px-2 py-1 rounded-lg">21</code></div>
        <div><div class="text-[11px] text-slate-500 mb-1">Username</div><code class="text-sm text-slate-200 font-mono bg-black/30 px-2 py-1 rounded-lg">{sshInfoData.system_user}</code></div>
        <div><div class="text-[11px] text-slate-500 mb-1">Root Path</div><code class="text-sm text-slate-200 font-mono bg-black/30 px-2 py-1 rounded-lg truncate block max-w-[200px]">{sshInfoData.root_path}</code></div>
      </div>
      <div class="bg-black/20 rounded-xl p-4 space-y-3">
        <div class="flex items-center gap-2 text-xs text-slate-300 font-medium"><Terminal size={12} class="text-emerald-400" /> Quick Guide</div>
        <ol class="text-xs text-slate-400 space-y-2 list-decimal list-inside">
          <li>Open your FTP client (FileZilla, WinSCP, Cyberduck)</li>
          <li>Enter host <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">{sshInfoData.host}</code> port <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">21</code></li>
          <li>Username: <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">{sshInfoData.system_user}</code></li>
          <li>For SSH: <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">{sshInfoData.ssh_command}</code></li>
        </ol>
      </div>
    </div>
  </div>
{/if}

<div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl shadow-lg overflow-hidden hover:border-white/[0.12] transition-all duration-300">
  <div class="flex items-center justify-between px-4 sm:px-5 py-2.5 sm:py-3.5 border-b border-white/[0.05]">
    <div class="flex items-center gap-2.5"><div class="w-6 h-6 sm:w-7 sm:h-7 rounded-lg bg-sky-400/10 flex items-center justify-center"><Key size={12} class="text-sky-400" /></div><span class="text-sm font-semibold text-slate-200">SSH Access</span><span class="text-[0.65rem] text-slate-500 bg-white/[0.04] px-2 py-0.5 rounded-full border border-white/[0.06]">{sshUsers.length}</span></div>
    {#if hasRole(auth.user?.role, 'admin')}<button onclick={() => showCreateSSH = !showCreateSSH} class="inline-flex items-center gap-1 px-3 py-1.5 rounded-xl text-xs font-medium bg-sky-400/[0.08] text-sky-400 border border-sky-400/15 hover:bg-sky-400/[0.12] hover:border-sky-400/25 transition-all duration-200"><UserPlus size={12} /> Add SSH User</button>{/if}
  </div>
  {#if showCreateSSH}
    <div class="px-5 py-4 border-b border-white/[0.05] bg-white/[0.01]">
      <div class="space-y-3">
        <div><label class="block text-xs text-slate-500 mb-1">Username</label><div class="w-full bg-white/[0.02] border border-white/[0.05] rounded-xl px-4 py-2.5 text-sm text-slate-400 font-mono">{sshUsername()}</div></div>
        <div><label class="block text-xs text-slate-500 mb-1">SSH Public Keys <span class="text-slate-600">(optional)</span></label><textarea bind:value={newSSH.ssh_keys} rows={4} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 focus:ring-2 focus:ring-sky-400/10 transition-all duration-200 placeholder:text-slate-600 resize-none" placeholder={"ssh-ed25519 AAAA... user1@host\nssh-ed25519 BBBB... user2@host"}></textarea></div>
        <div class="flex items-center gap-2">
          <button onclick={createSSH} disabled={sshBusy} class="px-4 py-2.5 rounded-xl text-sm font-medium bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all disabled:opacity-40 cursor-pointer border-0">{sshBusy ? '...' : 'Create'}</button>
          <button onclick={() => showCreateSSH = false} class="px-3 py-2.5 rounded-xl text-sm text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition cursor-pointer border-0">Cancel</button>
        </div>
      </div>
    </div>
  {/if}
  {#if sshLoading}
    <div class="divide-y divide-white/5">{#each Array(2) as _}<div class="px-5 py-4 flex items-center gap-3"><div class="w-8 h-8 bg-white/[0.04] rounded-lg animate-pulse"></div><div class="space-y-1"><div class="h-3 w-20 bg-white/[0.04] rounded animate-pulse"></div></div></div>{/each}</div>
  {:else if sshUsers.length === 0}
    <div class="flex flex-col items-center justify-center py-14"><Key size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">No SSH users</p><p class="text-xs text-slate-600">Add an SSH user for this site</p></div>
  {:else}
    <div class="divide-y divide-white/[0.04]">{#each sshUsers as u}<div class="flex items-center justify-between px-5 py-3.5 hover:bg-white/[0.02] transition"><div class="flex items-center gap-3"><Terminal size={14} class="text-slate-500" /><span class="text-sm font-medium text-slate-200">{u.username}</span><span class="text-xs text-slate-500 font-mono">{u.home_dir || '—'}</span></div><button onclick={() => deleteSSH(u)} class="text-xs text-red-400/60 hover:text-red-400 transition p-1 rounded-lg hover:bg-red-400/10"><Trash2 size={14} /></button></div>{/each}</div>
  {/if}
</div>

{#if sshKeyModal}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onclick={() => sshKeyModal = null}>
    <div class="bg-[#161b22] border border-white/[0.1] rounded-2xl p-6 max-w-lg w-full shadow-2xl" onclick={(e) => e.stopPropagation()}>
      <div class="flex items-center justify-between mb-4"><h3 class="text-lg font-semibold text-slate-200">SSH Keys Generated</h3><button onclick={() => sshKeyModal = null} class="text-slate-500 hover:text-slate-300 transition"><X size={18} /></button></div>
      <p class="text-xs text-amber-400 mb-4 flex items-center gap-1.5"><AlertTriangle size={14} /> Save the private key now — it won't be shown again.</p>
      <div class="space-y-3">
        <div><label class="block text-xs text-slate-500 mb-1">Public Key</label><div class="flex"><pre class="flex-1 bg-[#0d1117] text-emerald-300 font-mono text-[11px] p-3 rounded-xl overflow-x-auto">{sshKeyModal.pub}</pre><button onclick={() => copyText(sshKeyModal!.pub, 'sshpub')} class="ml-2 text-slate-500 hover:text-slate-300 transition">{#if copiedField === 'sshpub'}<Check size={14} class="text-emerald-400" />{:else}<Copy size={14} />{/if}</button></div></div>
        <div><label class="block text-xs text-slate-500 mb-1">Private Key</label><div class="flex"><pre class="flex-1 bg-[#0d1117] text-amber-300 font-mono text-[11px] p-3 rounded-xl overflow-x-auto max-h-40">{sshKeyModal.priv}</pre><button onclick={() => copyText(sshKeyModal!.priv, 'sshpriv')} class="ml-2 text-slate-500 hover:text-slate-300 transition">{#if copiedField === 'sshpriv'}<Check size={14} class="text-emerald-400" />{:else}<Copy size={14} />{/if}</button></div></div>
      </div>
    </div>
  </div>
{/if}
