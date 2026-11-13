<script lang="ts">
  import { auth, bearerHeaders } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import Modal from '$components/Modal.svelte';
  import {
    Folder, FolderOpen, File, FileText, FileCode, FileImage, FileVideoCamera,
    FileMusic, FileArchive, FileCog, FileBraces, FileTerminal, FileKey,
    RefreshCw, Search, Plus, X, Check, CornerUpLeft, Upload, Save, LoaderCircle,
    Trash2, Pencil, ChevronRight, Globe, Database, TriangleAlert, Download, House,
    MoreHorizontal, FolderPlus, FilePlus, FolderInput, CheckCheck, Link2, KeyRound, PackageOpen,
  } from 'lucide-svelte';

  let { siteId }: { siteId: number } = $props();

  // CodeMirror is heavy; it is fetched the first time a file is opened.
  let editorModule: Promise<typeof import('$components/CodeEditor.svelte')> | null = null;
  function loadEditor() { return (editorModule ??= import('$components/CodeEditor.svelte')); }

  let fmPath = $state('.');
  let fmEntries = $state<any[]>([]);
  let fmLoading = $state(false);
  let fmLoaded = $state(false);
  let fmBreadcrumbs = $state<{ name: string; path: string }[]>([]);
  let fmEditFile = $state<string | null>(null);
  let fmEditContent = $state('');
  let fmEditSaving = $state(false);
  let fmShowCreate = $state(false);
  let fmNewName = $state('');
  let fmNewIsDir = $state(false);
  let fmCreating = $state(false);
  let fmEditOriginal = $state('');
  let fmSearch = $state('');
  let fmContextMenu = $state<{ show: boolean; x: number; y: number; entry: any }>({ show: false, x: 0, y: 0, entry: null });
  let fmSelectedEntries = $state<Set<string>>(new Set());
  let fmRenameEntry = $state<any>(null);
  let fmRenameNewName = $state('');
  let fmMoveModal = $state(false);
  let fmMoveDest = $state('');
  let fmMoveDirs = $state<string[]>([]);
  let fmBusy = $state(false);
  let fmCompressModal = $state(false);
  let fmCompressName = $state('archive.zip');
  let fmUploading = $state(false);
  let fmOpening = $state<string | null>(null);
  let fmChmodEntry = $state<any>(null);
  let fmChmodMode = $state('644');

  // Uploads pass the web proxy (34 MiB body limit) and the API (32 MiB).
  const MAX_UPLOAD = 32 * 1024 * 1024;
  const dirOf = (p: string) => (p === '/' || p === '.' ? '.' : p.replace(/^\/+/, ''));
  const isZip = (name: string) => name.toLowerCase().endsWith('.zip');
  // "-rwxr-xr-x" -> "755" (the API returns Go's mode string).
  function octalMode(mode: string): string {
    const m = (mode || '').slice(-9);
    if (m.length !== 9) return '';
    let out = '';
    for (let i = 0; i < 9; i += 3) {
      out += String((m[i] !== '-' ? 4 : 0) + (m[i + 1] !== '-' ? 2 : 0) + (m[i + 2] !== '-' && m[i + 2] !== 'S' && m[i + 2] !== 'T' ? 1 : 0));
    }
    return out;
  }
  function fmDate(iso: string): string {
    const d = new Date(iso);
    return isNaN(+d) ? '' : d.toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  const fmFilteredEntries = $derived(fmSearch ? fmEntries.filter((e: any) => e.name.toLowerCase().includes(fmSearch.toLowerCase())) : fmEntries);
  const dirCount = $derived(fmEntries.filter((e: any) => e.is_dir).length);
  const fileCount = $derived(fmEntries.length - dirCount);

  function fmFormatSize(bytes: number) { if (bytes === 0) return '0 B'; const k = 1024; const sizes = ['B', 'KB', 'MB', 'GB']; const i = Math.floor(Math.log(bytes) / Math.log(k)); return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]; }

  function fmDetectLang(name: string): string {
    const ext = name.split('.').pop()?.toLowerCase() || '';
    const base = name.split('/').pop()?.toLowerCase() || '';
    const map: Record<string, string> = { js: 'javascript', mjs: 'javascript', jsx: 'jsx', ts: 'typescript', tsx: 'tsx', py: 'python', go: 'go', rs: 'rust', rb: 'ruby', php: 'php', java: 'java', html: 'html', css: 'css', scss: 'scss', vue: 'vue', svelte: 'svelte', json: 'json', yaml: 'yaml', yml: 'yaml', md: 'markdown', sh: 'bash', sql: 'sql', toml: 'toml', xml: 'xml', env: 'env', txt: 'text', log: 'text' };
    if (base === 'dockerfile') return 'docker'; if (base === 'makefile') return 'makefile';
    return map[ext] || 'text';
  }

  function fmFileIconWithColor(name: string): { icon: any; color: string } {
    const ext = name.split('.').pop()?.toLowerCase() || '';
    const base = name.split('/').pop()?.toLowerCase() || '';
    const codeExts = ['js','mjs','cjs','jsx','ts','tsx','py','go','rs','rb','php','java','kt','swift','vue','svelte','c','h','cpp','cs','lua','r','wasm','tf'];
    if (codeExts.includes(ext)) return { icon: FileCode, color: 'text-info' };
    if (ext === 'html' || ext === 'htm') return { icon: Globe, color: 'text-warning' };
    if (['css','scss','less'].includes(ext)) return { icon: FileCode, color: 'text-accent' };
    if (ext === 'json') return { icon: FileBraces, color: 'text-warning' };
    if (['yml','yaml','toml','conf','ini','xml'].includes(ext)) return { icon: FileCog, color: 'text-muted' };
    if (ext === 'env') return { icon: FileKey, color: 'text-warning' };
    if (['png','jpg','jpeg','gif','webp','ico','svg'].includes(ext)) return { icon: FileImage, color: 'text-accent' };
    if (['mp4','mov','avi','mkv','webm'].includes(ext)) return { icon: FileVideoCamera, color: 'text-danger' };
    if (['mp3','wav','ogg','flac','aac'].includes(ext)) return { icon: FileMusic, color: 'text-accent' };
    if (['zip','tar','gz','bz2','rar','7z','xz'].includes(ext)) return { icon: FileArchive, color: 'text-warning' };
    if (['sh','bash','zsh'].includes(ext)) return { icon: FileTerminal, color: 'text-success' };
    if (['md','txt','log','rtf'].includes(ext)) return { icon: FileText, color: 'text-muted' };
    if (['sql','graphql'].includes(ext)) return { icon: Database, color: 'text-info' };
    if (base === 'dockerfile') return { icon: FileCog, color: 'text-info' };
    if (base === 'makefile') return { icon: FileCog, color: 'text-info' };
    return { icon: File, color: 'text-subtle' };
  }

  $effect(() => { if (fmEntries.length === 0 && !fmLoading && !fmLoaded) { fmLoaded = true; loadFiles(); } });

  function buildBreadcrumbs(path: string) {
    const parts = path.split('/').filter((p) => p && p !== '.'); fmBreadcrumbs = [{ name: 'root', path: '.' }]; let acc = '';
    for (const p of parts) { acc = acc ? acc + '/' + p : p; fmBreadcrumbs.push({ name: p, path: acc }); }
  }

  async function loadFiles(path?: string) {
    fmLoading = true;
    try {
      let p = path || fmPath; if (p.startsWith('/')) p = p.slice(1) || '.'; if (p === '') p = '.';
      const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/files?path=${encodeURIComponent(p)}`);
      const next = d?.current === '.' ? '/' : '/' + (d?.current || p);
      // A selection or filter belongs to the folder it was made in.
      if (next !== fmPath) { fmSelectedEntries = new Set(); fmSearch = ''; }
      fmEntries = d?.entries || []; fmPath = next; buildBreadcrumbs(fmPath);
    }
    catch (e: any) { toasts.error(e.message); } finally { fmLoading = false; }
  }

  async function fmNavigate(entry: any) { if (entry.is_dir) { fmEditFile = null; fmEntries = []; fmSelectedEntries = new Set(); await loadFiles(entry.path); } else { await fmOpenFile(entry.path); } }
  async function fmOpenFile(path: string) {
    loadEditor(); fmOpening = path;
    try {
      const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/files/read?path=${encodeURIComponent(path)}`);
      if (d?.binary) {
        // Editing would corrupt binary / non-UTF-8 files.
        toasts.info('This file is binary or not UTF-8 text, so it is downloaded instead of opened.');
        fmDownload({ path, name: path.split('/').pop() });
        return;
      }
      fmEditFile = path; fmEditContent = d?.content || ''; fmEditOriginal = d?.content || '';
    } catch (e: any) {
      // Too large for the editor: offer the download.
      if (/too large/i.test(e.message || '')) { toasts.info('Too large to edit here (over 1 MB); downloading instead.'); fmDownload({ path, name: path.split('/').pop() }); }
      else toasts.error(e.message);
    } finally { fmOpening = null; }
  }
  const fmDirty = $derived(fmEditFile !== null && fmEditContent !== fmEditOriginal);
  // Leaving the editor must not silently drop unsaved edits.
  function fmCloseEditor(): boolean {
    if (fmDirty && !confirm('Discard unsaved changes?')) return false;
    fmEditFile = null; return true;
  }
  function onBeforeUnload(e: BeforeUnloadEvent) { if (fmDirty) { e.preventDefault(); e.returnValue = ''; } }
  function onEditorKey(e: KeyboardEvent) {
    if (fmEditFile && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); if (!fmEditSaving) fmSaveFile(); }
  }
  async function fmSaveFile() { if (!fmEditFile) return; fmEditSaving = true; try { await apiFetch(auth.token, 'POST', `sites/${siteId}/files/write`, { path: fmEditFile, content: fmEditContent }); fmEditOriginal = fmEditContent; toasts.success('File saved'); } catch (e: any) { toasts.error(e.message); } finally { fmEditSaving = false; } }
  async function fmCreateEntry() {
    if (!fmNewName) return; fmCreating = true;
    try { let basePath = fmPath.startsWith('/') ? fmPath.slice(1) : fmPath; if (basePath === '' || basePath === '.') basePath = ''; const p = basePath ? basePath + '/' + fmNewName : fmNewName; await apiFetch(auth.token, 'POST', `sites/${siteId}/files/create`, { path: p, is_dir: fmNewIsDir }); fmShowCreate = false; fmNewName = ''; toasts.success(fmNewIsDir ? 'Directory created' : 'File created'); fmNewIsDir = false; fmEntries = []; await loadFiles(fmPath); }
    catch (e: any) { toasts.error(e.message); } finally { fmCreating = false; }
  }
  async function fmDeleteEntry(entry: any) { if (!confirm(entry.is_dir ? `Delete the folder "${entry.name}" and everything in it?` : `Delete "${entry.name}"?`)) return; try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/files?path=${encodeURIComponent(entry.path)}`); fmEntries = fmEntries.filter(e => e.path !== entry.path); toasts.success(`Deleted "${entry.name}"`); } catch (e: any) { toasts.error(e.message); } }
  async function fmDeleteSelected() {
    const paths = [...fmSelectedEntries];
    if (paths.length === 0 || !confirm(`Delete ${paths.length} item(s)?`)) return;
    for (const p of paths) { try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/files?path=${encodeURIComponent(p)}`); } catch (e: any) { toasts.error(e.message); } }
    fmSelectedEntries = new Set(); fmEntries = []; loadFiles(fmPath);
  }
  function fmToggleSelect(path: string) { const next = new Set(fmSelectedEntries); if (next.has(path)) next.delete(path); else next.add(path); fmSelectedEntries = next; }
  function fmRename(entry: any) { fmRenameEntry = entry; fmRenameNewName = entry.name; }
  async function fmSaveRename(entry: any) { if (!fmRenameNewName || fmRenameNewName === entry.name) { fmRenameEntry = null; return; } try { await apiFetch(auth.token, 'POST', `sites/${siteId}/files/rename`, { old_path: entry.path, new_name: fmRenameNewName }); fmRenameEntry = null; toasts.success('Renamed'); fmEntries = []; await loadFiles(fmPath); } catch (e: any) { toasts.error(e.message); } }
  // Check the response first: a plain link would save an error message under
  // the file's name. The probe is aborted as soon as the headers arrive.
  async function fmDownload(entry: any) {
    const url = `/api/proxy/sites/${siteId}/files/download?path=${encodeURIComponent(entry.path)}`;
    const ctl = new AbortController();
    try {
      const r = await fetch(url, { headers: bearerHeaders(), signal: ctl.signal, cache: 'no-store' });
      if (!r.ok) { const d = await r.json().catch(() => null); ctl.abort(); toasts.error(d?.error?.message || `Download failed (${r.status})`); return; }
      ctl.abort();
    } catch (e: any) { if (e?.name !== 'AbortError') { toasts.error(e?.message || 'Download failed'); return; } }
    const a = document.createElement('a'); a.href = url; a.download = entry.name || ''; document.body.appendChild(a); a.click(); document.body.removeChild(a);
  }
  async function fmExtract(entry: any) {
    fmBusy = true;
    try {
      const d = await apiFetch(auth.token, 'POST', `sites/${siteId}/files/extract`, { path: entry.path, dest: dirOf(fmPath) });
      toasts.success(`Extracted ${d?.extracted ?? 0} file(s)` + (d?.skipped ? `, ${d.skipped} skipped (already exist or not regular files)` : ''));
      await loadFiles(fmPath);
    } catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; }
  }
  function fmOpenChmod(entry: any) { fmChmodEntry = entry; fmChmodMode = octalMode(entry.mode) || (entry.is_dir ? '755' : '644'); }
  async function fmSaveChmod() {
    if (!fmChmodEntry) return; fmBusy = true;
    try { await apiFetch(auth.token, 'POST', `sites/${siteId}/files/chmod`, { path: fmChmodEntry.path, mode: fmChmodMode }); toasts.success(`Permissions set to ${fmChmodMode}`); fmChmodEntry = null; await loadFiles(fmPath); }
    catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; }
  }
  async function fmMoveSelected() { const paths = [...fmSelectedEntries]; if (paths.length === 0) return; fmBusy = true; try { const dest = fmMoveDest.replace(/^\/+/, '') || '.'; await apiFetch(auth.token, 'POST', `sites/${siteId}/files/move`, { paths, dest }); fmMoveModal = false; fmMoveDest = ''; fmSelectedEntries = new Set(); toasts.success(`Moved ${paths.length} item(s)`); } catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; await loadFiles(fmPath); } }
  async function fmCompressSelected() { const paths = [...fmSelectedEntries]; if (paths.length === 0) return; fmBusy = true; try { const dest = (fmPath === '/' || fmPath === '.' ? '.' : fmPath.replace(/^\/+/, '')); await apiFetch(auth.token, 'POST', `sites/${siteId}/files/compress`, { paths, name: fmCompressName, dest }); fmCompressModal = false; fmCompressName = 'archive.zip'; fmSelectedEntries = new Set(); toasts.success(`Compressed ${paths.length} item(s)`); fmEntries = []; await loadFiles(fmPath); } catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; } }
  async function fmUploadFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    const big = Array.from(files).filter((f) => f.size > MAX_UPLOAD);
    if (big.length) { toasts.error(`${big.map((f) => f.name).join(', ')}: larger than 32 MB. Upload big files over SFTP, or as a .zip in parts.`); return; }
    fmUploading = true;
    const dir = (fmPath === '/' || fmPath === '.') ? '.' : fmPath.replace(/^\/+/, '');
    try { for (const file of Array.from(files)) { const formData = new FormData(); formData.append('file', file); formData.append('path', dir); await fetch(`/api/proxy/sites/${siteId}/files/upload`, { method: 'POST', headers: bearerHeaders(), body: formData }).then(async r => { if (!r.ok) { const d = await r.json().catch(() => null); throw new Error(d?.error?.message || 'Upload failed'); } }); } fmEntries = []; await loadFiles(fmPath); toasts.success(`Uploaded ${files.length} file(s)`); }
    catch (e: any) { toasts.error(e.message); } finally { fmUploading = false; }
  }
  function fmSelectAll() {
    const all = fmFilteredEntries.map((e: any) => e.path);
    fmSelectedEntries = fmSelectedEntries.size === all.length ? new Set() : new Set(all);
  }
  function fmSelectOnly(entry: any) { fmSelectedEntries = new Set([entry.path]); }
  function fmHideContextMenu() { fmContextMenu = { show: false, x: 0, y: 0, entry: null }; }
  function fmShowMenuAt(x: number, y: number, entry: any) {
    fmContextMenu = { show: true, x: Math.min(x, window.innerWidth - 200), y: Math.min(y, window.innerHeight - 220), entry };
  }
  function fmGoUp() { if (!fmCloseEditor()) return; fmEntries = []; const parts = fmPath.split('/').filter(Boolean); parts.pop(); loadFiles('/' + parts.join('/') || '/'); }
  function fmOpenCreate(isDir: boolean) { fmShowCreate = !fmShowCreate || fmNewIsDir !== isDir; fmNewName = ''; fmNewIsDir = isDir; }
</script>

<svelte:window onbeforeunload={onBeforeUnload} onkeydown={onEditorKey} />

{#if fmEditFile}
  <div class="card overflow-hidden">
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-2.5">
      <div class="flex min-w-0 items-center gap-2">
        <button onclick={() => { if (fmCloseEditor()) { fmEntries = []; fmLoaded = false; loadFiles(fmPath); } }} class="btn btn-ghost btn-icon btn-sm" aria-label="Back to files" title="Back to files">
          <CornerUpLeft size={14} />
        </button>
        <div class="min-w-0">
          <div class="truncate text-sm font-medium text-fg">{fmEditFile.split('/').pop()}</div>
          <div class="truncate font-mono text-xs text-subtle">{fmEditFile}</div>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <span class="badge uppercase">{fmDetectLang(fmEditFile)}</span>
        <button onclick={fmSaveFile} disabled={fmEditSaving} class="btn btn-primary btn-sm" title="Save (Ctrl+S)">
          {#if fmEditSaving}<LoaderCircle size={14} class="animate-spin" />{:else}<Save size={14} />{/if}
          {fmEditSaving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </div>
    <div class="w-full overflow-hidden">
      {#await loadEditor()}
        <div class="skeleton m-1 h-[calc(100dvh-280px)]"></div>
      {:then m}
        <m.default content={fmEditContent} onChange={(v: string) => fmEditContent = v} language={fmDetectLang(fmEditFile)} height="calc(100dvh - 280px)" hideHeader={true} hideFooter={true} />
      {:catch}
        <p class="p-5 text-[13px] text-danger">Failed to load the editor. Reload the page and try again.</p>
      {/await}
    </div>
    <div class="flex items-center justify-between gap-3 border-t border-line px-4 py-2 text-xs text-subtle tabular-nums">
      <span>Lines: {fmEditContent.split('\n').length}</span>
      <span>Chars: {fmEditContent.length.toLocaleString()}</span>
      {#if fmEditContent !== fmEditOriginal}
        <span class="flex items-center gap-1 text-warning"><TriangleAlert size={12} /> Unsaved</span>
      {:else}
        <span class="flex items-center gap-1 text-success"><Check size={12} /> Saved</span>
      {/if}
    </div>
  </div>
{:else}
  <div class="card overflow-hidden">
    <!-- Breadcrumbs + search -->
    <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5">
      <nav class="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto" aria-label="Path">
        <button onclick={() => { fmEntries = []; loadFiles('.'); }} class="btn btn-ghost btn-icon btn-xs" aria-label="Go to site root" title="Go to site root"><House size={14} /></button>
        {#each fmBreadcrumbs as bc, i (bc.path)}
          {#if bc.path !== '.'}
            <ChevronRight size={12} class="shrink-0 text-subtle" />
            <button onclick={() => { fmEditFile = null; fmEntries = []; loadFiles(bc.path); }} class="btn btn-xs shrink-0 font-mono {i === fmBreadcrumbs.length - 1 ? 'text-fg' : 'btn-ghost'}" title={bc.path}>{bc.name}</button>
          {/if}
        {/each}
      </nav>
      <div class="relative w-full sm:w-52">
        <Search size={14} class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-subtle" />
        <input bind:value={fmSearch} aria-label="Filter files" placeholder="Filter…" class="input h-8 pr-8 pl-8" />
        {#if fmSearch}
          <button onclick={() => (fmSearch = '')} class="btn btn-ghost btn-icon btn-xs absolute top-1/2 right-0.5 -translate-y-1/2" aria-label="Clear filter"><X size={12} /></button>
        {/if}
      </div>
    </div>

    <!-- Toolbar -->
    <div class="flex items-center gap-1 overflow-x-auto border-b border-line px-3 py-2">
      {#if fmPath !== '/' && fmPath !== '.'}
        <button onclick={fmGoUp} class="btn btn-ghost btn-sm"><CornerUpLeft size={14} /> Up</button>
      {/if}
      <button onclick={() => fmOpenCreate(false)} class="btn btn-ghost btn-sm"><FilePlus size={14} /> File</button>
      <button onclick={() => fmOpenCreate(true)} class="btn btn-ghost btn-sm"><FolderPlus size={14} /> Folder</button>
      <label class="btn btn-ghost btn-sm">
        {#if fmUploading}<LoaderCircle size={14} class="animate-spin" />{:else}<Upload size={14} />{/if} Upload
        <input type="file" multiple class="hidden" onchange={(e) => { fmUploadFiles((e.target as HTMLInputElement).files); (e.target as HTMLInputElement).value = ''; }} />
      </label>
      <span class="mx-1 h-4 w-px shrink-0 bg-line"></span>
      <button onclick={fmSelectAll} class="btn btn-ghost btn-sm" disabled={fmFilteredEntries.length === 0}><CheckCheck size={14} /> {fmSelectedEntries.size > 0 && fmSelectedEntries.size === fmFilteredEntries.length ? 'Select none' : 'Select all'}</button>
      <button onclick={() => loadFiles(fmPath)} class="btn btn-ghost btn-sm" aria-label="Reload"><RefreshCw size={14} class={fmLoading ? 'animate-spin' : ''} /><span class="max-sm:hidden">Reload</span></button>
    </div>

    {#if fmShowCreate}
      <div class="flex flex-wrap items-center gap-2 border-b border-line bg-surface-2/50 px-4 py-2.5">
        <label for="fm-new-name" class="sr-only">New {fmNewIsDir ? 'folder' : 'file'} name</label>
        <input id="fm-new-name" bind:value={fmNewName} placeholder={fmNewIsDir ? 'folder name' : 'file name'} onkeydown={(e) => e.key === 'Enter' && fmCreateEntry()} class="input h-8 max-w-[280px] font-mono text-[13px]" />
        <select bind:value={fmNewIsDir} aria-label="Entry type" class="input h-8 w-auto text-[13px]"><option value={false}>File</option><option value={true}>Folder</option></select>
        <button onclick={fmCreateEntry} disabled={fmCreating || !fmNewName} class="btn btn-primary btn-sm"><Plus size={14} /> {fmCreating ? 'Creating…' : 'Create'}</button>
        <button onclick={() => (fmShowCreate = false)} class="btn btn-ghost btn-sm">Cancel</button>
      </div>
    {/if}

    {#if fmSelectedEntries.size > 0}
      <div class="sticky top-2 z-20 flex flex-wrap items-center gap-2 border-b border-line bg-elevated px-4 py-2">
        <span class="badge badge-accent"><Check size={12} /> {fmSelectedEntries.size} selected</span>
        <div class="ml-auto flex items-center gap-1 overflow-x-auto">
          <button onclick={() => { fmMoveDirs = fmEntries.filter((e: any) => e.is_dir).map((e: any) => e.path); fmMoveModal = true; }} class="btn btn-secondary btn-xs"><FolderInput size={12} /> Move</button>
          <button onclick={() => { fmCompressModal = true; }} class="btn btn-secondary btn-xs"><FileArchive size={12} /> Compress</button>
          <button onclick={fmDeleteSelected} class="btn btn-danger-ghost btn-xs"><Trash2 size={12} /> Delete</button>
          <button onclick={() => { fmSelectedEntries = new Set(); }} class="btn btn-ghost btn-xs"><X size={12} /> Clear</button>
        </div>
      </div>
    {/if}

    {#if fmLoading}
      <div class="space-y-3 p-4">{#each [1, 2, 3, 4, 5, 6] as i (i)}<div class="skeleton h-7" style="width: {40 + i * 8}%"></div>{/each}</div>
    {:else if fmFilteredEntries.length === 0}
      <div class="flex flex-col items-center justify-center px-4 py-14 text-center">
        <div class="mb-3 flex size-11 items-center justify-center rounded-xl border border-line bg-surface-2 text-subtle"><Folder size={20} strokeWidth={1.75} /></div>
        <p class="text-sm font-semibold text-fg">{fmSearch ? `No results for "${fmSearch}"` : 'Empty directory'}</p>
        {#if fmSearch}<button onclick={() => (fmSearch = '')} class="link mt-1 text-[13px]">Clear filter</button>{/if}
      </div>
    {:else}
      <div class="hidden items-center gap-3 border-b border-line bg-surface-2/40 px-4 py-1.5 text-[11px] font-medium tracking-wide text-subtle uppercase md:flex">
        <span class="w-7 shrink-0"></span>
        <span class="flex-1">Name</span>
        <span class="w-20 shrink-0 text-right">Size</span>
        <span class="w-36 shrink-0 max-lg:hidden">Modified</span>
        <span class="w-12 shrink-0 text-center">Perm</span>
        <span class="w-6 shrink-0"></span>
      </div>
      <ul class="max-h-[50vh] divide-y divide-line overflow-y-auto lg:max-h-[60vh]">
        {#each fmFilteredEntries as entry (entry.path)}
          {@const selected = fmSelectedEntries.has(entry.path)}
          <li
            class="group flex min-h-11 items-center gap-3 px-4 py-1.5 text-sm transition-colors duration-100 {selected ? 'bg-accent/10' : 'hover:bg-surface-2/60'}"
            oncontextmenu={(e) => { e.preventDefault(); e.stopPropagation(); fmShowMenuAt(e.clientX, e.clientY, entry); }}>
            <!-- The icon is the selection checkbox; the name opens. -->
            <button
              onclick={() => fmToggleSelect(entry.path)}
              class="flex size-7 shrink-0 items-center justify-center rounded-md transition-colors {selected ? 'bg-accent text-white' : 'bg-surface-2 hover:ring-1 hover:ring-accent/50'}"
              role="checkbox" aria-checked={selected} aria-label="Select {entry.name}" title="Select">
              {#if selected}
                <Check size={14} />
              {:else if entry.is_dir}
                <Folder size={15} class="text-accent" />
              {:else}
                {@const fileIcon = fmFileIconWithColor(entry.name)}
                <fileIcon.icon size={14} class={fileIcon.color} />
              {/if}
            </button>
            <button
              onclick={() => { if (!fmContextMenu.show) fmNavigate(entry); }}
              ondblclick={(e) => e.preventDefault()}
              class="flex min-w-0 flex-1 items-center gap-1.5 text-left text-fg hover:text-accent"
              title={entry.is_symlink ? `${entry.name} (symbolic link)` : entry.name}>
              <span class="truncate">{entry.name}{#if entry.is_dir}<span class="ml-0.5 text-subtle">/</span>{/if}</span>
              {#if entry.is_symlink}<Link2 size={12} class="shrink-0 text-subtle" aria-label="symbolic link" />{/if}
              {#if fmOpening === entry.path}<LoaderCircle size={12} class="shrink-0 animate-spin text-subtle" />{/if}
            </button>
            <span class="w-16 shrink-0 text-right font-mono text-xs text-subtle tabular-nums md:w-20">{entry.is_dir ? '—' : fmFormatSize(entry.size)}</span>
            <span class="w-36 shrink-0 truncate text-xs text-subtle tabular-nums max-lg:hidden" title={entry.mod_time}>{fmDate(entry.mod_time)}</span>
            <span class="hidden w-12 shrink-0 text-center font-mono text-xs text-subtle md:inline" title={entry.mode}>{octalMode(entry.mode)}</span>
            <button
              class="btn btn-ghost btn-icon btn-xs shrink-0 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
              aria-label="Actions for {entry.name}"
              onclick={(e) => { e.stopPropagation(); const r = (e.currentTarget as HTMLElement).getBoundingClientRect(); fmShowMenuAt(r.right - 180, r.bottom + 4, entry); }}>
              <MoreHorizontal size={14} />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    <div class="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-line px-4 py-2 text-xs text-subtle tabular-nums">
      <span class="flex items-center gap-1"><Folder size={12} /> {dirCount} dir</span>
      <span class="flex items-center gap-1"><FileText size={12} /> {fileCount} file{fileCount !== 1 ? 's' : ''}</span>
      <span>{fmEntries.length} total</span>
      <span class="ml-auto max-sm:hidden">Click a name to open · the icon selects · right-click for actions</span>
    </div>
  </div>
{/if}

{#if fmContextMenu.show}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50" onclick={() => fmHideContextMenu()} oncontextmenu={(e) => { e.preventDefault(); fmHideContextMenu(); }}>
    <div class="popover animate-pop-in absolute min-w-[180px] py-1 select-none" style="left: {fmContextMenu.x}px; top: {fmContextMenu.y}px;" onclick={(e) => e.stopPropagation()} role="menu" tabindex="-1">
      {#if fmContextMenu.entry}
        {#if fmContextMenu.entry.is_dir}
          <button role="menuitem" onclick={() => { fmNavigate(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><FolderOpen size={14} class="text-subtle" /> Open</button>
        {:else}
          <button role="menuitem" onclick={() => { fmOpenFile(fmContextMenu.entry.path); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><File size={14} class="text-subtle" /> View / edit</button>
          <button role="menuitem" onclick={() => { fmDownload(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><Download size={14} class="text-subtle" /> Download</button>
        {/if}
        {#if !fmContextMenu.entry.is_dir && isZip(fmContextMenu.entry.name)}
          <button role="menuitem" onclick={() => { fmExtract(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><PackageOpen size={14} class="text-subtle" /> Extract here</button>
        {/if}
        <button role="menuitem" onclick={() => { fmRename(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><Pencil size={14} class="text-subtle" /> Rename</button>
        <button role="menuitem" onclick={() => { const en = fmContextMenu.entry; fmSelectOnly(en); fmMoveDirs = fmEntries.filter((x: any) => x.is_dir && x.path !== en.path).map((x: any) => x.path); fmMoveModal = true; fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><FolderInput size={14} class="text-subtle" /> Move…</button>
        <button role="menuitem" onclick={() => { const en = fmContextMenu.entry; fmSelectOnly(en); fmCompressName = (en.name.replace(/\.[^./]+$/, '') || en.name) + '.zip'; fmCompressModal = true; fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><FileArchive size={14} class="text-subtle" /> Compress…</button>
        {#if !fmContextMenu.entry.is_symlink}
          <button role="menuitem" onclick={() => { fmOpenChmod(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-fg transition-colors hover:bg-surface-2"><KeyRound size={14} class="text-subtle" /> Permissions…</button>
        {/if}
        <div class="my-1 h-px bg-line"></div>
        <button role="menuitem" onclick={() => { fmDeleteEntry(fmContextMenu.entry); fmHideContextMenu(); }} class="flex w-full items-center gap-2.5 px-3 py-2 text-[13px] text-danger transition-colors hover:bg-danger/10"><Trash2 size={14} /> Delete</button>
      {/if}
    </div>
  </div>
{/if}

<Modal open={!!fmRenameEntry} title="Rename" maxWidth="sm" onclose={() => (fmRenameEntry = null)}>
  <label for="fm-rename" class="label">New name</label>
  <input id="fm-rename" bind:value={fmRenameNewName} class="input font-mono text-[13px]" onkeydown={(e) => { if (e.key === 'Enter') fmSaveRename(fmRenameEntry); }} />
  {#snippet footer()}
    <button onclick={() => (fmRenameEntry = null)} class="btn btn-secondary btn-sm">Cancel</button>
    <button onclick={() => fmSaveRename(fmRenameEntry)} class="btn btn-primary btn-sm">Rename</button>
  {/snippet}
</Modal>

<Modal bind:open={fmMoveModal} title="Move to…" maxWidth="sm" onclose={() => (fmMoveDest = '')}>
  <div class="mb-3 max-h-48 space-y-0.5 overflow-y-auto">
    <button onclick={() => (fmMoveDest = '.')} class="flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {fmMoveDest === '.' ? 'bg-accent/10 text-accent' : 'text-muted hover:bg-surface-2 hover:text-fg'}"><FolderOpen size={14} /> Root</button>
    {#each fmMoveDirs as dir (dir)}
      <button onclick={() => (fmMoveDest = dir)} class="flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-[13px] transition-colors {fmMoveDest === dir ? 'bg-accent/10 text-accent' : 'text-muted hover:bg-surface-2 hover:text-fg'}"><Folder size={14} /> {dir.split('/').pop() || dir}</button>
    {/each}
  </div>
  <label for="fm-move-dest" class="label">Destination path</label>
  <input id="fm-move-dest" bind:value={fmMoveDest} placeholder="Enter destination path" class="input font-mono text-[13px]" />
  {#snippet footer()}
    <button onclick={() => { fmMoveModal = false; fmMoveDest = ''; }} disabled={fmBusy} class="btn btn-secondary btn-sm">Cancel</button>
    <button onclick={fmMoveSelected} disabled={!fmMoveDest || fmBusy} class="btn btn-primary btn-sm">{fmBusy ? 'Moving…' : `Move ${fmSelectedEntries.size} item(s)`}</button>
  {/snippet}
</Modal>

<Modal bind:open={fmCompressModal} title="Compress to ZIP" maxWidth="sm" onclose={() => (fmCompressName = 'archive.zip')}>
  <p class="mb-4 text-[13px] text-muted">{fmSelectedEntries.size} item(s) will be compressed.</p>
  <label for="fm-compress-name" class="label">Archive name</label>
  <input id="fm-compress-name" bind:value={fmCompressName} class="input font-mono text-[13px]" />
  {#snippet footer()}
    <button onclick={() => { fmCompressModal = false; fmCompressName = 'archive.zip'; }} disabled={fmBusy} class="btn btn-secondary btn-sm">Cancel</button>
    <button onclick={fmCompressSelected} disabled={!fmCompressName || fmBusy} class="btn btn-primary btn-sm">{fmBusy ? 'Compressing…' : 'Compress'}</button>
  {/snippet}
</Modal>

<Modal open={!!fmChmodEntry} title="Permissions" maxWidth="sm" onclose={() => (fmChmodEntry = null)}>
  {#if fmChmodEntry}
    <p class="mb-3 truncate font-mono text-[13px] text-muted">{fmChmodEntry.path}</p>
    <label for="fm-chmod" class="label">Mode (octal)</label>
    <input id="fm-chmod" bind:value={fmChmodMode} maxlength="4" inputmode="numeric" class="input font-mono text-[13px]" onkeydown={(e) => { if (e.key === 'Enter') fmSaveChmod(); }} />
    <p class="hint">Owner / group / others: 4 = read, 2 = write, 1 = execute. Common: 644 files, 755 folders and scripts, 600 secrets such as .env.</p>
    <div class="mt-3 flex flex-wrap gap-1.5">
      {#each ['644', '755', '600', '640', '750'] as m (m)}
        <button onclick={() => (fmChmodMode = m)} class="btn btn-xs {fmChmodMode === m ? 'btn-primary' : 'btn-secondary'} font-mono">{m}</button>
      {/each}
    </div>
  {/if}
  {#snippet footer()}
    <button onclick={() => (fmChmodEntry = null)} disabled={fmBusy} class="btn btn-secondary btn-sm">Cancel</button>
    <button onclick={fmSaveChmod} disabled={fmBusy || !/^[0-7]{3,4}$/.test(fmChmodMode)} class="btn btn-primary btn-sm">{fmBusy ? 'Saving…' : 'Apply'}</button>
  {/snippet}
</Modal>
