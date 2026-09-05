<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import CodeEditor from '$components/CodeEditor.svelte';
  import {
    Folder, FolderOpen, File, FileText, FileCode, FileImage, FileVideoCamera,
    FileMusic, FileArchive, FileCog, FileBraces, FileTerminal, FileKey,
    RefreshCw, Search, Plus, X, Check, CornerUpLeft, Upload, Save, Loader2,
    Trash2, Pencil, ChevronRight, Globe, Database, Lock, AlertTriangle, Download,
  } from 'lucide-svelte';

  let { siteId }: { siteId: number } = $props();

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

  const fmFilteredEntries = $derived(fmSearch ? fmEntries.filter((e: any) => e.name.toLowerCase().includes(fmSearch.toLowerCase())) : fmEntries);

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
    if (codeExts.includes(ext)) return { icon: FileCode, color: 'text-sky-400' };
    if (ext === 'html' || ext === 'htm') return { icon: Globe, color: 'text-orange-400' };
    if (['css','scss','less'].includes(ext)) return { icon: FileCode, color: 'text-pink-400' };
    if (ext === 'json') return { icon: FileBraces, color: 'text-amber-400' };
    if (['yml','yaml','toml','conf','ini','xml'].includes(ext)) return { icon: FileCog, color: 'text-slate-400' };
    if (ext === 'env') return { icon: FileKey, color: 'text-amber-400' };
    if (['png','jpg','jpeg','gif','webp','ico','svg'].includes(ext)) return { icon: FileImage, color: 'text-purple-400' };
    if (['mp4','mov','avi','mkv','webm'].includes(ext)) return { icon: FileVideoCamera, color: 'text-red-400' };
    if (['mp3','wav','ogg','flac','aac'].includes(ext)) return { icon: FileMusic, color: 'text-pink-400' };
    if (['zip','tar','gz','bz2','rar','7z','xz'].includes(ext)) return { icon: FileArchive, color: 'text-amber-400' };
    if (['sh','bash','zsh'].includes(ext)) return { icon: FileTerminal, color: 'text-lime-400' };
    if (['md','txt','log','rtf'].includes(ext)) return { icon: FileText, color: 'text-slate-400' };
    if (['sql','graphql'].includes(ext)) return { icon: Database, color: 'text-sky-400' };
    if (base === 'dockerfile') return { icon: FileCog, color: 'text-cyan-400' };
    if (base === 'makefile') return { icon: FileCog, color: 'text-sky-400' };
    return { icon: File, color: 'text-slate-500' };
  }

  $effect(() => { if (fmEntries.length === 0 && !fmLoading && !fmLoaded) { fmLoaded = true; loadFiles(); } });

  function buildBreadcrumbs(path: string) {
    const parts = path === '.' ? [] : path.split('/'); fmBreadcrumbs = [{ name: 'root', path: '.' }]; let acc = '';
    for (const p of parts) { acc = acc ? acc + '/' + p : p; fmBreadcrumbs.push({ name: p, path: acc }); }
  }

  async function loadFiles(path?: string) {
    fmLoading = true;
    try { let p = path || fmPath; if (p.startsWith('/')) p = p.slice(1) || '.'; if (p === '') p = '.'; const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/files?path=${encodeURIComponent(p)}`); fmEntries = d?.entries || []; fmPath = d?.current === '.' ? '/' : '/' + (d?.current || p); buildBreadcrumbs(fmPath); }
    catch (e: any) { toasts.error(e.message); } finally { fmLoading = false; }
  }

  async function fmNavigate(entry: any) { if (entry.is_dir) { fmEditFile = null; fmEntries = []; fmSelectedEntries.clear(); await loadFiles(entry.path); } else { await fmOpenFile(entry.path); } }
  async function fmOpenFile(path: string) { try { const d = await apiFetch(auth.token, 'GET', `sites/${siteId}/files/read?path=${encodeURIComponent(path)}`); fmEditFile = path; fmEditContent = d?.content || ''; fmEditOriginal = d?.content || ''; } catch (e: any) { toasts.error(e.message); } }
  async function fmSaveFile() { if (!fmEditFile) return; fmEditSaving = true; try { await apiFetch(auth.token, 'POST', `sites/${siteId}/files/write`, { path: fmEditFile, content: fmEditContent }); toasts.success('File saved'); } catch (e: any) { toasts.error(e.message); } finally { fmEditSaving = false; } }
  async function fmCreateEntry() {
    if (!fmNewName) return; fmCreating = true;
    try { let basePath = fmPath.startsWith('/') ? fmPath.slice(1) : fmPath; if (basePath === '' || basePath === '.') basePath = ''; const p = basePath ? basePath + '/' + fmNewName : fmNewName; await apiFetch(auth.token, 'POST', `sites/${siteId}/files/create`, { path: p, is_dir: fmNewIsDir }); fmShowCreate = false; fmNewName = ''; fmNewIsDir = false; toasts.success(fmNewIsDir ? 'Directory created' : 'File created'); fmEntries = []; await loadFiles(fmPath); }
    catch (e: any) { toasts.error(e.message); } finally { fmCreating = false; }
  }
  async function fmDeleteEntry(entry: any) { if (!confirm(`Delete "${entry.name}"?`)) return; try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/files?path=${encodeURIComponent(entry.path)}`); fmEntries = fmEntries.filter(e => e.path !== entry.path); toasts.success(`Deleted "${entry.name}"`); } catch (e: any) { toasts.error(e.message); } }
  function fmToggleSelect(path: string) { const next = new Set(fmSelectedEntries); if (next.has(path)) next.delete(path); else next.add(path); fmSelectedEntries = next; }
  async function fmRename(entry: any) { fmRenameEntry = entry; fmRenameNewName = entry.name; }
  async function fmSaveRename(entry: any) { if (!fmRenameNewName || fmRenameNewName === entry.name) { fmRenameEntry = null; return; } try { await apiFetch(auth.token, 'POST', `sites/${siteId}/files/rename`, { old_path: entry.path, new_name: fmRenameNewName }); fmRenameEntry = null; toasts.success('Renamed'); fmEntries = []; await loadFiles(fmPath); } catch (e: any) { toasts.error(e.message); } }
  function fmDownload(entry: any) { const url = `/api/proxy/sites/${siteId}/files/download?path=${encodeURIComponent(entry.path)}`; const a = document.createElement('a'); a.href = url; a.download = entry.name; document.body.appendChild(a); a.click(); document.body.removeChild(a); }
  async function fmMoveSelected() { const paths = [...fmSelectedEntries]; if (paths.length === 0) return; fmBusy = true; try { const dest = fmMoveDest.replace(/^\/+/, '') || '.'; await apiFetch(auth.token, 'POST', `sites/${siteId}/files/move`, { paths, dest }); fmMoveModal = false; fmMoveDest = ''; fmSelectedEntries = new Set(); toasts.success(`Moved ${paths.length} item(s)`); fmEntries = []; await loadFiles(fmPath); } catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; } }
  async function fmCompressSelected() { const paths = [...fmSelectedEntries]; if (paths.length === 0) return; fmBusy = true; try { const dest = (fmPath === '/' || fmPath === '.' ? '.' : fmPath.replace(/^\/+/, '')); await apiFetch(auth.token, 'POST', `sites/${siteId}/files/compress`, { paths, name: fmCompressName, dest }); fmCompressModal = false; fmCompressName = 'archive.zip'; fmSelectedEntries = new Set(); toasts.success(`Compressed ${paths.length} item(s)`); fmEntries = []; await loadFiles(fmPath); } catch (e: any) { toasts.error(e.message); } finally { fmBusy = false; } }
  async function fmUploadFiles(files: FileList | null) {
    if (!files || files.length === 0) return; fmUploading = true;
    const dir = (fmPath === '/' || fmPath === '.') ? '.' : fmPath.replace(/^\/+/, '');
    try { for (const file of Array.from(files)) { const formData = new FormData(); formData.append('file', file); formData.append('path', dir); await fetch(`/api/proxy/sites/${siteId}/files/upload`, { method: 'POST', headers: { 'Authorization': `Bearer ${auth.token}` }, body: formData }).then(async r => { if (!r.ok) { const d = await r.json().catch(() => null); throw new Error(d?.error?.message || 'Upload failed'); } }); } fmEntries = []; await loadFiles(fmPath); toasts.success(`Uploaded ${files.length} file(s)`); }
    catch (e: any) { toasts.error(e.message); } finally { fmUploading = false; }
  }
  function fmSelectAll() { fmSelectedEntries = new Set(fmFilteredEntries.filter((e: any) => !e.is_dir).map((e: any) => e.path)); }
  function fmHideContextMenu() { fmContextMenu = { show: false, x: 0, y: 0, entry: null }; }
</script>

{#if fmEditFile}
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl overflow-hidden">
    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 px-3 lg:px-5 py-2.5 border-b border-white/[0.05] bg-white/[0.02]">
      <div class="flex items-center gap-2 min-w-0"><button onclick={() => { fmEditFile = null; fmEntries = []; fmLoaded = false; loadFiles(fmPath); }} class="flex-shrink-0 p-1 rounded-lg hover:bg-white/10 transition text-slate-500 hover:text-slate-200"><CornerUpLeft size={13} /></button><div class="min-w-0 truncate"><span class="text-sm font-medium text-slate-200 truncate block">{fmEditFile.split('/').pop()}</span><span class="text-[0.6rem] text-slate-600 font-mono truncate block max-w-[200px] sm:max-w-none">{fmEditFile}</span></div></div>
      <div class="flex items-center gap-1.5 flex-shrink-0 self-end sm:self-auto">
        <span class="text-[0.6rem] text-sky-400 bg-sky-400/10 px-1.5 py-0.5 rounded">{fmDetectLang(fmEditFile).toUpperCase()}</span>
        <button onclick={fmSaveFile} disabled={fmEditSaving} class="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all duration-150 disabled:opacity-50 whitespace-nowrap">{#if fmEditSaving}<Loader2 size={11} class="animate-spin" />{:else}<Save size={11} />{/if} <span>{fmEditSaving ? 'Saving...' : 'Save'}</span></button>
      </div>
    </div>
    <div class="w-full overflow-hidden"><CodeEditor content={fmEditContent} onChange={(v: string) => fmEditContent = v} language={fmDetectLang(fmEditFile)} height="calc(100dvh - 280px)" hideHeader={true} hideFooter={true} /></div>
    <div class="flex items-center justify-between px-5 py-2 border-t border-white/[0.05] bg-white/[0.02] text-xs text-slate-500">
      <span>Lines: {fmEditContent.split('\n').length}</span><span>Chars: {fmEditContent.length.toLocaleString()}</span>
      {#if fmEditContent !== fmEditOriginal}<span class="text-amber-400 flex items-center gap-1"><AlertTriangle size={12} /> Unsaved</span>{:else}<span class="text-emerald-400 flex items-center gap-1"><Check size={12} /> Saved</span>{/if}
    </div>
  </div>
{:else}
  <div class="bg-white/[0.03] border border-white/[0.07] rounded-2xl overflow-hidden group/card hover:border-white/[0.12] transition-all duration-300">
    <div class="px-3 lg:px-5 py-2.5 border-b border-white/[0.05] bg-white/[0.02]">
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center gap-0.5 min-w-0 overflow-x-auto scrollbar-thin scroll-smooth">
          <button onclick={() => { fmEditFile = null; fmEntries = []; fmSelectedEntries = new Set(); loadFiles('.'); }} class="min-w-6 h-6 rounded-lg bg-sky-400/10 hover:bg-sky-400/18 active:bg-sky-400/25 text-sky-300 flex items-center justify-center flex-shrink-0 mr-1 transition border border-sky-400/15" title="Go to site root"><Folder size={14} /></button>
          {#each fmBreadcrumbs as bc, i}{#if bc.path !== '.'}<span class="flex items-center gap-0 flex-shrink-0 whitespace-nowrap"><button onclick={() => { fmEditFile = null; fmEntries = []; loadFiles(bc.path); }} class="text-xs px-1.5 py-0.5 rounded-md transition {i === fmBreadcrumbs.length - 1 ? 'text-sky-300 font-medium bg-sky-400/10' : 'text-slate-500 hover:text-slate-300 hover:bg-white/[0.04]'}" title={bc.path}>{bc.name}</button>{#if i < fmBreadcrumbs.length - 1}<ChevronRight size={9} class="text-slate-600 mx-0.5 flex-shrink-0" />{/if}</span>{/if}{/each}
        </div>
        <div class="flex items-center gap-1.5">
          {#if fmPath !== '/' && fmPath !== '.'}<button onclick={() => { fmEditFile = null; fmEntries = []; const parts = fmPath.split('/').filter(Boolean); parts.pop(); loadFiles('/' + parts.join('/') || '/'); }} class="inline-flex items-center gap-1 px-2 py-1 rounded-lg text-xs text-slate-500 hover:bg-white/[0.06] hover:text-slate-200 transition border border-transparent hover:border-white/[0.08]"><CornerUpLeft size={11} /><span class="hidden sm:inline text-[0.7rem]">Up</span></button>{/if}
          <button onclick={() => loadFiles(fmPath)} class="p-1.5 rounded-lg text-slate-600 hover:bg-white/[0.06] hover:text-sky-400 transition" title="Refresh"><RefreshCw size={12} class={fmLoading ? 'animate-spin' : ''} /></button>
          <div class="flex-1"></div>
          <div class="relative max-w-[160px] sm:max-w-[180px]"><Search size={10} class="absolute left-2 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" /><input bind:value={fmSearch} placeholder="Search..." class="w-full bg-white/[0.04] border border-white/[0.07] rounded-lg pl-6 pr-6 py-1.5 text-xs text-slate-300 outline-none focus:border-sky-400 focus:ring-1 focus:ring-sky-400/20 placeholder:text-slate-600 transition" />{#if fmSearch}<button onclick={() => fmSearch = ''} class="absolute right-1.5 top-1/2 -translate-y-1/2 text-slate-600 hover:text-slate-400"><X size={10} /></button>{/if}</div>
          <button onclick={() => { fmShowCreate = !fmShowCreate; fmNewName = ''; }} class="inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer whitespace-nowrap"><Plus size={12} /><span>New</span></button>
        </div>
      </div>
    </div>
    {#if fmSelectedEntries.size > 0}
      <div class="mx-4 lg:mx-5 mt-3 px-4 py-3 bg-gradient-to-r from-sky-400/[0.12] via-indigo-400/[0.08] to-sky-400/[0.06] border border-sky-400/25 rounded-xl shadow-[0_10px_30px_rgba(56,189,248,0.08)] flex items-center gap-3 flex-wrap sticky top-2 z-20 backdrop-blur-xl">
        <div class="flex items-center gap-2 min-w-0"><span class="w-6 h-6 rounded-lg bg-sky-400 text-[#07111f] flex items-center justify-center shadow-lg shadow-sky-400/20"><Check size={14} /></span><div class="leading-tight"><p class="text-xs font-semibold text-sky-200">{fmSelectedEntries.size} selected</p></div></div>
        <div class="flex items-center gap-2 ml-auto overflow-x-auto pb-0.5">
          <button onclick={() => { fmMoveDirs = fmEntries.filter((e: any) => e.is_dir).map((e: any) => e.path); fmMoveModal = true; }} class="min-h-[34px] inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-medium text-slate-200 bg-white/[0.05] hover:bg-sky-400/12 hover:text-sky-200 transition border border-white/[0.08] cursor-pointer whitespace-nowrap"><CornerUpLeft size={13} /> Move</button>
          <button onclick={() => { fmCompressModal = true; }} class="min-h-[34px] inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-medium text-slate-200 bg-white/[0.05] hover:bg-sky-400/12 hover:text-sky-200 transition border border-white/[0.08] cursor-pointer whitespace-nowrap"><FileText size={13} /> Compress</button>
          <button onclick={() => { const paths = [...fmSelectedEntries]; if (paths.length === 0 || !confirm(`Delete ${paths.length} item(s)?`)) return; (async () => { for (const p of paths) { try { await apiFetch(auth.token, 'DELETE', `sites/${siteId}/files?path=${encodeURIComponent(p)}`); } catch (e: any) { toasts.error(e.message); } } fmSelectedEntries = new Set(); fmEntries = []; loadFiles(fmPath); })(); }} class="min-h-[34px] inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-semibold text-red-300 bg-red-400/10 hover:bg-red-400/15 transition border border-red-400/25 cursor-pointer whitespace-nowrap"><Trash2 size={13} /> Delete</button>
          <button onclick={() => { fmSelectedEntries = new Set(); }} class="min-h-[34px] inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-white/[0.06] transition cursor-pointer whitespace-nowrap"><X size={13} /> Clear</button>
        </div>
      </div>
    {/if}
    {#if fmShowCreate}
      <div class="mx-4 lg:mx-5 mt-3 px-4 py-2.5 bg-white/[0.04] border border-white/[0.07] rounded-lg flex items-center gap-3 flex-wrap">
        <input bind:value={fmNewName} placeholder="file or folder name" onkeydown={(e) => e.key === 'Enter' && fmCreateEntry()} class="max-w-[280px] bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-200 outline-none focus:border-sky-400 focus:ring-1 focus:ring-sky-400/20 font-mono" />
        <select bind:value={fmNewIsDir} class="bg-white/[0.04] border border-white/[0.07] rounded-lg px-3 py-1.5 text-xs text-slate-200 outline-none cursor-pointer"><option value={false}>File</option><option value={true}>Folder</option></select>
        <button onclick={fmCreateEntry} disabled={fmCreating || !fmNewName} class="inline-flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 transition-all duration-150 cursor-pointer whitespace-nowrap disabled:opacity-50">{fmCreating ? '...' : 'Create'}</button>
        <button onclick={() => fmShowCreate = false} class="inline-flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-slate-400 hover:text-slate-200 hover:bg-white/[0.05] transition-all duration-150 cursor-pointer whitespace-nowrap">Cancel</button>
      </div>
    {/if}
    <div class="flex items-center gap-1.5 px-4 py-2.5 border-b border-white/[0.05] overflow-x-auto bg-white/[0.015]">
      <button onclick={() => { fmShowCreate = !fmShowCreate; fmNewName = ''; fmNewIsDir = false; }} class="min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium text-sky-300 hover:bg-sky-400/10 transition border-0 cursor-pointer whitespace-nowrap"><File size={12} /> File</button>
      <button onclick={() => { fmShowCreate = !fmShowCreate; fmNewName = ''; fmNewIsDir = true; }} class="min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium text-sky-300 hover:bg-sky-400/10 transition border-0 cursor-pointer whitespace-nowrap"><Folder size={12} /> Folder</button>
      <label class="min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium text-sky-300 hover:bg-sky-400/10 transition border-0 cursor-pointer whitespace-nowrap">{#if fmUploading}<Loader2 size={12} class="animate-spin" />{:else}<Upload size={12} />{/if} Upload<input type="file" multiple class="hidden" onchange={(e) => { fmUploadFiles((e.target as HTMLInputElement).files); (e.target as HTMLInputElement).value = ''; }} /></label>
      <div class="w-px h-4 bg-white/[0.08] mx-1"></div>
      <button onclick={fmSelectAll} class="min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition border-0 cursor-pointer whitespace-nowrap"><Check size={12} /> Select all</button>
      <button onclick={() => loadFiles(fmPath)} class="min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium text-slate-400 hover:bg-white/[0.05] hover:text-slate-200 transition border-0 cursor-pointer whitespace-nowrap"><RefreshCw size={12} /> Reload</button>
    </div>
    <div class="px-4 lg:px-5 py-2 border-b border-white/[0.03] bg-white/[0.01] text-[0.65rem] text-slate-600 flex flex-wrap items-center gap-x-3 gap-y-1"><span>Folder tap = open</span><span>File tap = select</span><span>Long press/right-click = menu</span></div>
    {#if fmLoading}
      <div class="p-4 lg:p-5 space-y-2">{#each [1,2,3,4,5,6] as i}<div class="flex items-center gap-3"><div class="h-3 w-4 bg-white/[0.04] rounded animate-pulse"></div><div class="h-3 bg-gradient-to-r from-white/[0.03] via-white/[0.06] to-white/[0.03] rounded animate-pulse" style="width: {40 + i * 8}%;"></div></div>{/each}</div>
    {:else if fmFilteredEntries.length === 0}
      <div class="flex flex-col items-center justify-center py-14 lg:py-20"><Folder size={22} class="text-white/[0.08] mb-3" /><p class="text-sm font-medium text-slate-400 mb-0.5">{fmSearch ? `No results for "${fmSearch}"` : 'Empty directory'}</p>{#if fmSearch}<button onclick={() => fmSearch = ''} class="text-xs text-sky-400 hover:text-sky-300 transition mt-1">Clear filter</button>{/if}</div>
    {:else}
      <div class="divide-y divide-white/[0.02] max-h-[50vh] lg:max-h-[60vh] overflow-y-auto select-none">
        {#each fmFilteredEntries as entry (entry.path)}
          <div class="relative flex items-center gap-3 px-4 lg:px-5 py-3 min-h-[44px] hover:bg-white/[0.025] active:bg-white/[0.04] transition-all duration-150 cursor-pointer text-sm group/file {fmSelectedEntries.has(entry.path) ? 'bg-sky-400/[0.08] border-l-[3px] border-l-sky-400 pl-[13px]' : 'border-l-[3px] border-l-transparent'}"
            onclick={() => { if (fmContextMenu.show) return; if (entry.is_dir) fmNavigate(entry); else fmToggleSelect(entry.path); }}
            oncontextmenu={(e) => { e.preventDefault(); e.stopPropagation(); const x = Math.min(e.clientX, window.innerWidth - 200); const y = Math.min(e.clientY, window.innerHeight - 220); fmContextMenu = { show: true, x, y, entry }; }}>
            {#if fmSelectedEntries.has(entry.path)}<span class="absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 rounded-full bg-sky-400 flex items-center justify-center"><Check size={10} class="text-[#0a1628]" /></span>{/if}
            <span class="flex-shrink-0 w-8 h-8 rounded-lg flex items-center justify-center {entry.is_dir ? 'bg-sky-400/10' : 'bg-white/[0.03]'}">
              {#if entry.is_dir}<Folder size={16} class="text-sky-400" />{:else}{@const fileIcon = fmFileIconWithColor(entry.name)}<svelte:component this={fileIcon.icon} size={14} class={fileIcon.color} />{/if}
            </span>
            <span class="text-slate-200 truncate flex-1 min-w-0 group-hover/file:text-sky-200 transition">{entry.name}{#if entry.is_dir}<span class="text-[0.6rem] text-slate-600 ml-2">/</span>{/if}</span>
            <span class="text-[0.65rem] text-slate-600 flex-shrink-0 w-16 lg:w-20 text-right font-mono">{entry.is_dir ? '—' : fmFormatSize(entry.size)}</span>
          </div>
        {/each}
      </div>
    {/if}
    <div class="px-4 lg:px-5 py-2 border-t border-white/[0.05] bg-white/[0.01] flex flex-wrap items-center gap-x-4 gap-y-1">
      <span class="text-[0.65rem] text-slate-600 flex items-center gap-1"><Folder size={10} class="text-sky-400" /> {fmEntries.filter((e: any) => e.is_dir).length} dir</span>
      <span class="text-[0.65rem] text-slate-600 flex items-center gap-1"><FileText size={10} /> {fmEntries.filter((e: any) => !e.is_dir).length} file{fmEntries.filter((e: any) => !e.is_dir).length !== 1 ? 's' : ''}</span>
      <span class="text-[0.65rem] text-slate-700">·</span>
      <span class="text-[0.65rem] text-slate-600">{fmEntries.length} total</span>
    </div>
  </div>
{/if}

{#if fmContextMenu.show}
  <div class="fixed inset-0 z-50" onclick={() => fmHideContextMenu()} oncontextmenu={(e) => { e.preventDefault(); fmHideContextMenu(); }}>
    <div class="absolute bg-[#0d1520] border border-white/[0.1] border rounded-xl shadow-2xl py-1.5 min-w-[180px] backdrop-blur-xl select-none" style="left: {fmContextMenu.x}px; top: {fmContextMenu.y}px;" onclick={(e) => e.stopPropagation()}>
      {#if fmContextMenu.entry}
        {#if fmContextMenu.entry.is_dir}
          <button onclick={() => { fmNavigate(fmContextMenu.entry); fmHideContextMenu(); }} class="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] transition select-none"><FolderOpen size={12} class="text-sky-400" /> Open</button>
        {:else}
          <button onclick={() => { fmOpenFile(fmContextMenu.entry.path); fmHideContextMenu(); }} class="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] transition select-none"><File size={12} class="text-slate-500" /> View/Edit</button>
          <button onclick={() => { fmDownload(fmContextMenu.entry); fmHideContextMenu(); }} class="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] transition select-none"><Download size={12} class="text-slate-500" /> Download</button>
        {/if}
        <button onclick={() => { fmRename(fmContextMenu.entry); fmHideContextMenu(); }} class="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-slate-300 hover:bg-white/[0.06] transition select-none"><Pencil size={12} class="text-slate-500" /> Rename</button>
        <div class="h-px bg-white/[0.06] my-1"></div>
        <button onclick={() => { fmDeleteEntry(fmContextMenu.entry); fmHideContextMenu(); }} class="w-full flex items-center gap-2.5 px-3 py-2 text-xs text-red-400 hover:bg-red-400/10 transition select-none"><Trash2 size={12} /> Delete</button>
      {/if}
    </div>
  </div>
{/if}

{#if fmRenameEntry}
  <div class="fixed inset-0 z-50 bg-black/50 flex items-center justify-center" onclick={() => fmRenameEntry = null}>
    <div class="bg-[#0d1520] border border-white/[0.1] rounded-2xl p-5 w-[90%] max-w-sm" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-sm font-semibold text-slate-200 mb-3">Rename</h3>
      <input bind:value={fmRenameNewName} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50" onkeydown={(e) => { if (e.key === 'Enter') fmSaveRename(fmRenameEntry); }} />
      <div class="flex gap-2 mt-4"><button onclick={() => fmSaveRename(fmRenameEntry)} class="px-4 py-2 rounded-xl text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] border-0 cursor-pointer">Rename</button><button onclick={() => fmRenameEntry = null} class="px-4 py-2 rounded-xl text-xs text-slate-400 hover:text-slate-200 border-0 cursor-pointer">Cancel</button></div>
    </div>
  </div>
{/if}

{#if fmMoveModal}
  <div class="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4" onclick={() => { if (!fmBusy) fmMoveModal = false; }}>
    <div class="bg-[#0d1520] border border-white/[0.1] rounded-2xl p-5 w-[90%] max-w-sm" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-sm font-semibold text-slate-200 mb-3">Move to...</h3>
      <div class="space-y-1 max-h-48 overflow-y-auto mb-3">
        <button onclick={() => fmMoveDest = '.'} class="w-full flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs transition {fmMoveDest === '.' ? 'bg-sky-400/20 text-sky-300' : 'text-slate-400 hover:bg-white/[0.04]'} border-0 cursor-pointer"><FolderOpen size={12} /> Root</button>
        {#each fmMoveDirs as dir}<button onclick={() => fmMoveDest = dir} class="w-full flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs transition {fmMoveDest === dir ? 'bg-sky-400/20 text-sky-300' : 'text-slate-400 hover:bg-white/[0.04]'} border-0 cursor-pointer text-left"><Folder size={12} /> {dir.split('/').pop() || dir}</button>{/each}
      </div>
      <input bind:value={fmMoveDest} placeholder="Enter destination path" class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 mb-3" />
      <div class="flex gap-2"><button onclick={fmMoveSelected} disabled={!fmMoveDest || fmBusy} class="flex-1 px-4 py-2.5 rounded-xl text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] border-0 cursor-pointer disabled:opacity-50 transition-all">{fmBusy ? 'Moving...' : `Move ${fmSelectedEntries.size} item(s)`}</button><button onclick={() => { fmMoveModal = false; fmMoveDest = ''; }} disabled={fmBusy} class="px-4 py-2.5 rounded-xl text-xs text-slate-400 hover:text-slate-200 border-0 cursor-pointer transition">Cancel</button></div>
    </div>
  </div>
{/if}

{#if fmCompressModal}
  <div class="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4" onclick={() => { if (!fmBusy) fmCompressModal = false; }}>
    <div class="bg-[#0d1520] border border-white/[0.1] rounded-2xl p-5 w-[90%] max-w-sm" onclick={(e) => e.stopPropagation()}>
      <h3 class="text-sm font-semibold text-slate-200 mb-1">Compress to ZIP</h3>
      <p class="text-xs text-slate-500 mb-4">{fmSelectedEntries.size} item(s) will be compressed</p>
      <label class="block text-xs text-slate-400 mb-1.5">Archive name</label>
      <input bind:value={fmCompressName} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 font-mono outline-none focus:border-sky-400/50 mb-4" />
      <div class="flex gap-2"><button onclick={fmCompressSelected} disabled={!fmCompressName || fmBusy} class="flex-1 px-4 py-2.5 rounded-xl text-xs font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] border-0 cursor-pointer disabled:opacity-50 transition-all">{fmBusy ? 'Compressing...' : 'Compress'}</button><button onclick={() => { fmCompressModal = false; fmCompressName = 'archive.zip'; }} disabled={fmBusy} class="px-4 py-2.5 rounded-xl text-xs text-slate-400 hover:text-slate-200 border-0 cursor-pointer transition">Cancel</button></div>
    </div>
  </div>
{/if}
