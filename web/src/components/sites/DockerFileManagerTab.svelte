<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import {
    FolderOpen, FileCode, RefreshCw, Loader2, Save, X, Plus,
    Trash2, Pencil, ChevronRight, Home, Search, Download, ArrowUpFromLine
  } from 'lucide-svelte';

  let { siteId, serverId, dockerContainer }: { siteId: number; serverId: number; dockerContainer: any } = $props();

  let fmPath = $state('/');
  let fmEntries = $state<any[]>([]);
  let fmLoading = $state(false);
  let fmSearch = $state('');

  // File editor
  let fmEditFile = $state('');
  let fmEditContent = $state('');
  let fmEditOriginal = $state('');
  let fmEditSaving = $state(false);
  let fmEditDirty = $state(false);

  // Create
  let fmShowCreate = $state(false);
  let fmNewName = $state('');
  let fmNewIsDir = $state(false);

  // Rename
  let fmRenameEntry = $state<any>(null);
  let fmRenameNewName = $state('');

  // Breadcrumbs
  let fmBreadcrumbs = $state<{name: string; path: string}[]>([]);

  $effect(() => { if (dockerContainer && !fmLoading && fmEntries.length === 0) { loadFiles(); } });

  function buildBreadcrumbs(path: string) {
    const parts = path === '/' ? [] : path.split('/').filter(Boolean);
    fmBreadcrumbs = [{ name: '~', path: '/' }];
    let acc = '';
    for (const p of parts) {
      acc += '/' + p;
      fmBreadcrumbs.push({ name: p, path: acc });
    }
  }

  async function execInContainer(cmd: string): Promise<string> {
    const d: any = await apiFetch(auth.token, 'POST', `servers/${serverId}/docker/container/exec`, {
      container_id: dockerContainer.id,
      command: cmd,
    });
    if (!d?.ok && d?.ok !== undefined) throw new Error(d?.message || 'exec failed');
    return d?.output || '';
  }

  async function loadFiles(path?: string) {
    fmLoading = true;
    try {
      const p = path || fmPath;
      const output = await execInContainer(`ls -la "${p}" 2>/dev/null`);
      const lines = output.split('\n').filter((l: string) => l.trim() && !l.startsWith('total'));
      fmEntries = lines.map((line: string) => {
        const parts = line.split(/\s+/);
        const isDir = parts[0]?.startsWith('d');
        const name = parts.slice(8).join(' ');
        return {
          name,
          path: p === '/' ? '/' + name : p + '/' + name,
          is_dir: isDir,
          permissions: parts[0],
          size: parts[4],
          owner: parts[2] + ':' + parts[3],
        };
      }).filter((e: any) => e.name && e.name !== '.' && e.name !== '..');
      fmPath = p;
      buildBreadcrumbs(p);
    } catch (e: any) { toasts.error(e.message); }
    fmLoading = false;
  }

  async function fmNavigate(entry: any) {
    if (entry.is_dir) {
      fmEditFile = '';
      fmEntries = [];
      await loadFiles(entry.path);
    } else {
      await fmOpenFile(entry.path);
    }
  }

  async function fmOpenFile(path: string) {
    try {
      const output = await execInContainer(`cat "${path}" 2>/dev/null`);
      fmEditFile = path;
      fmEditContent = output;
      fmEditOriginal = output;
      fmEditDirty = false;
    } catch (e: any) { toasts.error(e.message); }
  }

  async function fmSaveFile() {
    if (!fmEditFile) return;
    fmEditSaving = true;
    try {
      const b64 = btoa(unescape(encodeURIComponent(fmEditContent)));
      await execInContainer(`echo '${b64}' | base64 -d > "${fmEditFile}"`);
      fmEditOriginal = fmEditContent;
      fmEditDirty = false;
      toasts.success('File saved');
    } catch (e: any) { toasts.error(e.message); }
    fmEditSaving = false;
  }

  async function fmCreateEntry() {
    if (!fmNewName) return;
    try {
      const fullPath = fmPath === '/' ? '/' + fmNewName : fmPath + '/' + fmNewName;
      if (fmNewIsDir) {
        await execInContainer(`mkdir -p "${fullPath}"`);
      } else {
        await execInContainer(`touch "${fullPath}"`);
      }
      fmShowCreate = false;
      fmNewName = '';
      fmNewIsDir = false;
      toasts.success(fmNewIsDir ? 'Directory created' : 'File created');
      fmEntries = [];
      await loadFiles();
    } catch (e: any) { toasts.error(e.message); }
  }

  async function fmDeleteEntry(entry: any) {
    if (!confirm(`Delete "${entry.name}"?`)) return;
    try {
      await execInContainer(`rm -rf "${entry.path}"`);
      fmEntries = fmEntries.filter((e: any) => e.path !== entry.path);
      toasts.success(`Deleted "${entry.name}"`);
    } catch (e: any) { toasts.error(e.message); }
  }

  async function fmRename(entry: any) {
    fmRenameEntry = entry;
    fmRenameNewName = entry.name;
  }

  async function fmSaveRename(entry: any) {
    if (!fmRenameNewName || fmRenameNewName === entry.name) { fmRenameEntry = null; return; }
    try {
      const dir = entry.path.substring(0, entry.path.lastIndexOf('/'));
      const newPath = dir + '/' + fmRenameNewName;
      await execInContainer(`mv "${entry.path}" "${newPath}"`);
      fmRenameEntry = null;
      toasts.success('Renamed');
      fmEntries = [];
      await loadFiles();
    } catch (e: any) { toasts.error(e.message); }
  }

  function fmFormatSize(size?: string) {
    if (!size) return '—';
    const n = parseInt(size);
    if (isNaN(n)) return size;
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    return (n / 1048576).toFixed(1) + ' MB';
  }

</script>

<div class="bg-white/[0.03] border border-white/[0.06] rounded-xl overflow-hidden">
  <!-- Header -->
  <div class="px-4 py-2.5 border-b border-white/[0.05] flex items-center gap-2">
    <FolderOpen size={13} class="text-sky-400" />
    <span class="text-xs font-semibold text-slate-200">Container Files</span>
    <span class="text-[0.6rem] text-slate-600 ml-auto">{dockerContainer?.name}</span>
  </div>

  <!-- Breadcrumbs -->
  <div class="px-4 py-2 border-b border-white/[0.04] flex items-center gap-1 text-xs overflow-x-auto">
    {#each fmBreadcrumbs as bc, i}
      {#if i > 0}<ChevronRight size={10} class="text-slate-600 shrink-0" />{/if}
      <button onclick={() => { fmEntries = []; loadFiles(bc.path); }} class="text-slate-400 hover:text-sky-300 transition shrink-0">{bc.name}</button>
    {/each}
  </div>

  <!-- Toolbar -->
  <div class="px-4 py-2 border-b border-white/[0.04] flex items-center gap-2">
    <div class="relative flex-1">
      <Search size={12} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-600" />
      <input bind:value={fmSearch} placeholder="Filter..." class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg pl-7 pr-3 py-1.5 text-xs text-slate-200 outline-none" />
    </div>
    <button onclick={() => { fmShowCreate = !fmShowCreate; }} class="p-1.5 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] text-slate-400 hover:text-white transition"><Plus size={13} /></button>
    <button onclick={() => loadFiles()} disabled={fmLoading} class="p-1.5 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] text-slate-400 hover:text-white transition"><RefreshCw size={13} class={fmLoading ? 'animate-spin' : ''} /></button>
  </div>

  <!-- Create Form -->
  {#if fmShowCreate}
    <div class="px-4 py-3 border-b border-white/[0.04] bg-white/[0.02] flex gap-2 items-end">
      <div class="flex-1">
        <label class="block text-[0.6rem] text-slate-600 mb-1">Name</label>
        <input bind:value={fmNewName} placeholder="filename or dirname" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-200 outline-none" />
      </div>
      <label class="flex items-center gap-1.5 cursor-pointer mb-1.5">
        <input type="checkbox" bind:checked={fmNewIsDir} class="rounded border-white/20 bg-white/5 text-sky-400" />
        <span class="text-[0.65rem] text-slate-500">Dir</span>
      </label>
      <button onclick={fmCreateEntry} disabled={!fmNewName} class="px-3 py-1.5 bg-sky-500/20 text-sky-300 rounded-lg text-xs font-medium hover:bg-sky-500/30 transition disabled:opacity-40">Create</button>
      <button onclick={() => { fmShowCreate = false; fmNewName = ''; }} class="px-3 py-1.5 text-slate-500 hover:text-white text-xs">Cancel</button>
    </div>
  {/if}

  <!-- File Editor -->
  {#if fmEditFile}
    <div class="border-b border-white/[0.04]">
      <div class="px-4 py-2 flex items-center justify-between bg-white/[0.02]">
        <span class="text-xs text-slate-400 font-mono truncate max-w-[200px]">{fmEditFile}</span>
        <div class="flex items-center gap-2">
          {#if fmEditDirty}<span class="text-[0.6rem] text-amber-400">modified</span>{/if}
          <button onclick={() => { fmEditFile = ''; }} class="text-slate-500 hover:text-white"><X size={14} /></button>
        </div>
      </div>
      <textarea
        bind:value={fmEditContent}
        oninput={() => { fmEditDirty = fmEditContent !== fmEditOriginal; }}
        rows={20}
        class="w-full bg-[#0d1117] border-0 p-4 text-xs text-emerald-300 font-mono resize-none outline-none min-h-[300px]"
      ></textarea>
      <div class="px-4 py-2 border-t border-white/[0.04] flex gap-2">
        <button onclick={fmSaveFile} disabled={fmEditSaving || !fmEditDirty}
          class="px-4 py-1.5 bg-sky-500/20 text-sky-300 rounded-lg text-xs font-medium hover:bg-sky-500/30 transition flex items-center gap-1.5 disabled:opacity-40">
          {#if fmEditSaving}<Loader2 size={12} class="animate-spin" />{/if}<Save size={12} /> Save
        </button>
        <button onclick={() => { fmEditFile = ''; }} class="px-4 py-1.5 text-slate-500 hover:text-white text-xs">Cancel</button>
      </div>
    </div>
  {/if}

  <!-- File List -->
  {#if fmLoading}
    <div class="p-6 text-center"><Loader2 size={16} class="animate-spin text-slate-500 mx-auto" /></div>
  {:else if fmFilteredEntries.length === 0}
    <div class="p-6 text-center text-xs text-slate-500">{fmSearch ? 'No matches' : 'Empty directory'}</div>
  {:else}
    <div class="max-h-[50vh] overflow-y-auto">
      {#each fmFilteredEntries as entry}
        {#if fmRenameEntry?.path === entry.path}
          <div class="flex items-center gap-2 px-4 py-2 bg-white/[0.04] border-b border-white/[0.03]">
            <input bind:value={fmRenameNewName} class="flex-1 bg-white/[0.04] border border-sky-400/30 rounded px-2 py-1 text-xs text-slate-200 outline-none" />
            <button onclick={() => fmSaveRename(entry)} class="text-sky-400 hover:text-sky-300"><Save size={13} /></button>
            <button onclick={() => { fmRenameEntry = null; }} class="text-slate-500 hover:text-white"><X size={13} /></button>
          </div>
        {:else}
          <div role="button" tabindex="0" onclick={() => fmNavigate(entry)} onkeydown={(e) => { if (e.key === "Enter") fmNavigate(entry); }} class="w-full flex items-center gap-3 px-4 py-2 text-left hover:bg-white/[0.04] transition border-b border-white/[0.03] last:border-0 group cursor-pointer">
            {#if entry.is_dir}
              <FolderOpen size={14} class="text-sky-400 shrink-0" />
            {:else}
              <FileCode size={14} class="text-slate-500 shrink-0" />
            {/if}
            <span class="text-xs text-slate-200 truncate flex-1">{entry.name}</span>
            <span class="text-[0.6rem] text-slate-600 font-mono w-16 text-right">{fmFormatSize(entry.size)}</span>
            <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition" onclick={(e) => e.stopPropagation()}>
              {#if !entry.is_dir}
                <button onclick={() => fmOpenFile(entry.path)} class="p-1 rounded hover:bg-white/10 text-slate-500 hover:text-white" title="Edit"><Pencil size={11} /></button>
              {/if}
              <button onclick={() => fmRename(entry)} class="p-1 rounded hover:bg-white/10 text-slate-500 hover:text-white" title="Rename"><Pencil size={11} /></button>
              <button onclick={() => fmDeleteEntry(entry)} class="p-1 rounded hover:bg-red-500/20 text-slate-500 hover:text-red-400" title="Delete"><Trash2 size={11} /></button>
            </div>
          </div>
        {/if}
      {/each}
    </div>
  {/if}
</div>
