<script lang="ts">
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import CodeEditor from '$components/CodeEditor.svelte';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtBytes, fmtDate } from '$lib/format';
  import {
    Folder, File, Upload, Trash2, Download, ArrowLeft,
    FolderPlus, FilePlus, ChevronRight, Loader2, Search, RefreshCw,
    Save, Edit3
  } from 'lucide-svelte';

  interface FileItem {
    name: string;
    path: string;
    is_dir: boolean;
    size: number;
    mod_time: string;
  }

  let currentPath = $state('/');
  let files = $state<FileItem[]>([]);
  let loading = $state(true);
  let error = $state('');
  let searchQuery = $state('');
  let openFile = $state<{ path: string; content: string; name: string } | null>(null);
  let saving = $state(false);
  let showNewModal = $state<'file' | 'folder' | null>(null);
  let newName = $state('');
  let busyAction = $state<string | null>(null);
  let deleteConfirm = $state<string | null>(null);
  let fileInput: HTMLInputElement;

  const breadcrumb = $derived(
    currentPath.split('/').filter(Boolean).map((seg, i, arr) => ({
      name: seg,
      path: '/' + arr.slice(0, i + 1).join('/')
    }))
  );

  const filtered = $derived(
    searchQuery
      ? files.filter(f => f.name.toLowerCase().includes(searchQuery.toLowerCase()))
      : files
  );

  const dirs = $derived(filtered.filter(f => f.is_dir).sort((a, b) => a.name.localeCompare(b.name)));
  const fileItems = $derived(filtered.filter(f => !f.is_dir).sort((a, b) => a.name.localeCompare(b.name)));

  function getLang(name: string): string {
    const ext = name.split('.').pop()?.toLowerCase() || '';
    const map: Record<string, string> = {
      js: 'javascript', jsx: 'jsx', ts: 'typescript', tsx: 'tsx', mjs: 'javascript', cjs: 'javascript',
      py: 'python', go: 'go', html: 'html', htm: 'html', css: 'css', scss: 'css', less: 'css',
      json: 'json', xml: 'xml', svg: 'xml', sql: 'sql', php: 'php', md: 'markdown',
      yaml: 'yaml', yml: 'yaml', toml: 'toml', sh: 'bash', bash: 'bash', zsh: 'bash',
      conf: 'text', env: 'text', txt: 'text', log: 'text', ini: 'text', dockerfile: 'text',
    };
    return map[ext] || 'text';
  }

  async function loadDir(path: string) {
    if (!auth.user) { window.location.href = '/login'; return; }
    loading = true;
    error = '';
    openFile = null;
    try {
      const data = await apiFetch(auth.token, 'GET', `files/list?path=${encodeURIComponent(path)}`);
      currentPath = path;
      files = Array.isArray(data) ? data : data?.files || [];
    } catch (e: any) {
      if (e.message === 'session expired') return;
      error = e.message || 'Failed to load directory';
      files = [];
    } finally {
      loading = false;
    }
  }

  function navigateTo(path: string) {
    if (openFile) { openFile = null; }
    loadDir(path);
  }

  function goUp() {
    const parts = currentPath.split('/').filter(Boolean);
    parts.pop();
    navigateTo('/' + parts.join('/') || '/');
  }

  async function openFileContent(item: FileItem) {
    if (!auth.user) return;
    try {
      const data = await apiFetch(auth.token, 'GET', `files/read?path=${encodeURIComponent(item.path)}`);
      const content = typeof data === 'string' ? data : data?.content || '';
      openFile = { path: item.path, content, name: item.name };
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(`Failed to open: ${e.message}`);
    }
  }

  async function saveFile() {
    if (!auth.user || !openFile) return;
    saving = true;
    try {
      await apiFetch(auth.token, 'POST', 'files/write', { path: openFile.path, content: openFile.content });
      toasts.success('File saved');
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(`Save failed: ${e.message}`);
    } finally {
      saving = false;
    }
  }

  async function createItem() {
    if (!auth.user || !newName.trim()) return;
    const full = currentPath === '/' ? `/${newName.trim()}` : `${currentPath}/${newName.trim()}`;
    try {
      if (showNewModal === 'file') {
        await apiFetch(auth.token, 'POST', 'files/write', { path: full, content: '' });
      } else {
        await apiFetch(auth.token, 'POST', 'files/mkdir', { path: full });
      }
      toasts.success(`${showNewModal === 'file' ? 'File' : 'Folder'} created`);
      showNewModal = null;
      newName = '';
      await loadDir(currentPath);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    }
  }

  async function deleteItem(item: FileItem) {
    if (!auth.user) return;
    busyAction = item.path;
    try {
      await apiFetch(auth.token, 'DELETE', `files/delete?path=${encodeURIComponent(item.path)}`);
      toasts.success(`Deleted ${item.name}`);
      deleteConfirm = null;
      if (openFile?.path === item.path) openFile = null;
      await loadDir(currentPath);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally {
      busyAction = null;
    }
  }

  function downloadFile(item: FileItem) {
    if (!auth.user) return;
    window.open(`/api/proxy/files/download?path=${encodeURIComponent(item.path)}`, '_blank');
  }

  async function handleUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const fileList = input.files;
    if (!fileList?.length || !auth.user) return;
    busyAction = 'uploading';
    try {
      for (const file of Array.from(fileList)) {
        const path = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
        const text = await file.text();
        await apiFetch(auth.token, 'POST', 'files/write', { path, content: text });
      }
      toasts.success(`${fileList.length} file(s) uploaded`);
      await loadDir(currentPath);
    } catch (e: any) {
      if (e.message === 'session expired') return;
      toasts.error(e.message);
    } finally {
      busyAction = null;
      input.value = '';
    }
  }

  $effect(() => { if (auth.user) loadDir("/"); });
</script>

<DashboardLayout active="Files">
  <div class="mb-6">
    <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">File Manager</h1>
    <p class="text-sm text-slate-500 mt-1">Browse, edit, and manage files</p>
  </div>

  {#if openFile}
    <!-- Editor view -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="flex items-center justify-between gap-2 px-4 py-2.5 border-b border-white/[0.07]">
        <div class="flex items-center gap-2 min-w-0">
          <button onclick={() => { openFile = null; }} class="p-1.5 rounded-lg hover:bg-white/[0.06] text-slate-400 hover:text-slate-200 transition shrink-0">
            <ArrowLeft size={16} />
          </button>
          <Edit3 size={14} class="text-sky-400 shrink-0" />
          <span class="text-sm font-mono text-slate-300 truncate">{openFile.name}</span>
          <span class="text-xs text-slate-500 hidden sm:inline">{openFile.path}</span>
        </div>
        <button onclick={saveFile} disabled={saving}
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-sky-500/15 text-sky-400 hover:bg-sky-500/25 text-xs font-medium transition disabled:opacity-50">
          {#if saving}<Loader2 size={13} class="animate-spin" />{:else}<Save size={13} />{/if}
          Save
        </button>
      </div>
      {#if openFile}
        <CodeEditor content={openFile.content} onChange={(v: string) => { if (openFile) openFile.content = v; }} language={getLang(openFile.name)} />
      {/if}
    </div>
  {:else}
    <!-- Toolbar -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <!-- Breadcrumb -->
      <nav class="flex items-center gap-1 text-sm min-w-0 flex-1">
        <button onclick={() => navigateTo('/')}
          class="shrink-0 px-2 py-1 rounded-md hover:bg-white/[0.06] text-slate-400 hover:text-sky-400 font-medium transition">
          <Folder size={14} />
        </button>
        {#each breadcrumb as seg}
          <ChevronRight size={12} class="text-slate-600 shrink-0" />
          <button onclick={() => navigateTo(seg.path)}
            class="px-2 py-1 rounded-md hover:bg-white/[0.06] text-slate-400 hover:text-sky-400 transition truncate max-w-[120px]">
            {seg.name}
          </button>
        {/each}
      </nav>

      <div class="flex items-center gap-1.5">
        {#if currentPath !== '/'}
          <button onclick={goUp} class="p-2 rounded-lg hover:bg-white/[0.06] text-slate-400 hover:text-sky-400 transition" title="Go up">
            <ArrowLeft size={16} />
          </button>
        {/if}
        <button onclick={() => loadDir(currentPath)} class="p-2 rounded-lg hover:bg-white/[0.06] text-slate-400 hover:text-sky-400 transition" title="Refresh">
          <RefreshCw size={16} class={loading ? 'animate-spin' : ''} />
        </button>
        <button onclick={() => fileInput?.click()} disabled={!!busyAction}
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-sky-500/15 text-sky-400 hover:bg-sky-500/25 text-xs font-medium transition disabled:opacity-50">
          <Upload size={14} /> Upload
        </button>
        <button onclick={() => { showNewModal = 'file'; newName = ''; }}
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-white/[0.04] text-slate-400 hover:text-emerald-400 hover:bg-emerald-500/10 text-xs font-medium transition">
          <FilePlus size={14} /> File
        </button>
        <button onclick={() => { showNewModal = 'folder'; newName = ''; }}
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-white/[0.04] text-slate-400 hover:text-amber-400 hover:bg-amber-500/10 text-xs font-medium transition">
          <FolderPlus size={14} /> Folder
        </button>
      </div>

      <input bind:this={fileInput} type="file" multiple class="hidden" onchange={handleUpload} />

      {#if files.length > 5}
        <div class="relative w-full sm:w-auto">
          <Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input bind:value={searchQuery} placeholder="Filter..."
            class="w-full sm:w-44 pl-8 pr-2 py-1.5 text-xs rounded-lg bg-white/[0.04] border border-white/[0.07] text-slate-300 placeholder-slate-600 focus:border-sky-500/30 focus:outline-none" />
        </div>
      {/if}
    </div>

    <!-- File list -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      {#if loading}
        <div class="flex items-center justify-center py-20">
          <Loader2 size={24} class="animate-spin text-sky-400" />
          <span class="ml-3 text-sm text-slate-500">Loading...</span>
        </div>
      {:else if error}
        <div class="flex flex-col items-center justify-center py-16 px-4">
          <p class="text-sm text-red-400 mb-3">{error}</p>
          <button onclick={() => loadDir(currentPath)} class="px-4 py-2 rounded-lg bg-white/[0.06] text-slate-400 hover:text-white text-xs transition">
            Retry
          </button>
        </div>
      {:else if filtered.length === 0}
        <div class="flex flex-col items-center justify-center py-16">
          <Folder size={40} class="text-white/10 mb-3" />
          <p class="text-sm text-slate-500">{searchQuery ? 'No matching files' : 'Empty directory'}</p>
        </div>
      {:else}
        <div class="hidden sm:grid grid-cols-[1fr_100px_140px_80px] gap-3 px-4 py-2 border-b border-white/[0.05] text-xs text-slate-600 font-medium">
          <span>Name</span>
          <span>Size</span>
          <span>Modified</span>
          <span class="text-right">Actions</span>
        </div>

        {#each dirs as item}
          <div class="relative w-full border-b border-white/[0.03] hover:bg-white/[0.03] transition group">
            <div onclick={() => navigateTo(item.path)} role="button" tabindex="0"
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); navigateTo(item.path); } }}
              class="w-full grid grid-cols-[1fr] sm:grid-cols-[1fr_100px_140px_80px] gap-2 sm:gap-3 px-4 py-2.5 text-left cursor-pointer">
              <div class="flex items-center gap-2.5 min-w-0">
                <Folder size={16} class="text-amber-400/70 shrink-0" />
                <span class="text-sm text-slate-300 group-hover:text-sky-400 transition truncate">{item.name}</span>
              </div>
              <span class="text-xs text-slate-600 hidden sm:block">—</span>
              <span class="text-xs text-slate-600 hidden sm:block">{fmtDate(item.mod_time)}</span>
              <div class="flex items-center justify-end gap-1 hidden sm:flex">
                {#if deleteConfirm === item.path}
                  <button onclick={(e) => { e.stopPropagation(); deleteItem(item); }} disabled={busyAction === item.path}
                    class="px-2 py-1 rounded text-xs bg-red-500/20 text-red-400 hover:bg-red-500/30 transition">Del</button>
                  <button onclick={(e) => { e.stopPropagation(); deleteConfirm = null; }}
                    class="px-2 py-1 rounded text-xs bg-white/[0.06] text-slate-500 hover:text-slate-300 transition">No</button>
                {:else}
                  <button onclick={(e) => { e.stopPropagation(); deleteConfirm = item.path; }}
                    class="p-1.5 rounded-lg text-slate-600 hover:text-red-400 hover:bg-red-500/10 opacity-0 group-hover:opacity-100 transition">
                    <Trash2 size={13} />
                  </button>
                {/if}
              </div>
            </div>
            <button onclick={() => { deleteConfirm = deleteConfirm === item.path ? null : item.path; }}
              class="sm:hidden absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg text-slate-600 hover:text-red-400">
              <Trash2 size={13} />
            </button>
          </div>
        {/each}

        {#each fileItems as item}
          <div class="relative w-full border-b border-white/[0.03] hover:bg-white/[0.03] transition group">
            <div onclick={() => openFileContent(item)} role="button" tabindex="0"
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openFileContent(item); } }}
              class="w-full grid grid-cols-[1fr] sm:grid-cols-[1fr_100px_140px_80px] gap-2 sm:gap-3 px-4 py-2.5 text-left cursor-pointer">
              <div class="flex items-center gap-2.5 min-w-0">
                <File size={16} class="text-sky-400/60 shrink-0" />
                <span class="text-sm text-slate-300 group-hover:text-sky-400 transition truncate">{item.name}</span>
              </div>
              <span class="text-xs text-slate-600 hidden sm:block">{fmtBytes(item.size)}</span>
              <span class="text-xs text-slate-600 hidden sm:block">{fmtDate(item.mod_time)}</span>
              <div class="flex items-center justify-end gap-1 hidden sm:flex">
                <button onclick={(e) => { e.stopPropagation(); downloadFile(item); }}
                  class="p-1.5 rounded-lg text-slate-600 hover:text-emerald-400 hover:bg-emerald-500/10 opacity-0 group-hover:opacity-100 transition">
                  <Download size={13} />
                </button>
                {#if deleteConfirm === item.path}
                  <button onclick={(e) => { e.stopPropagation(); deleteItem(item); }} disabled={busyAction === item.path}
                    class="px-2 py-1 rounded text-xs bg-red-500/20 text-red-400 hover:bg-red-500/30 transition">Del</button>
                  <button onclick={(e) => { e.stopPropagation(); deleteConfirm = null; }}
                    class="px-2 py-1 rounded text-xs bg-white/[0.06] text-slate-500 hover:text-slate-300 transition">No</button>
                {:else}
                  <button onclick={(e) => { e.stopPropagation(); deleteConfirm = item.path; }}
                    class="p-1.5 rounded-lg text-slate-600 hover:text-red-400 hover:bg-red-500/10 opacity-0 group-hover:opacity-100 transition">
                    <Trash2 size={13} />
                  </button>
                {/if}
              </div>
            </div>
            <button onclick={() => { deleteConfirm = deleteConfirm === item.path ? null : item.path; }}
              class="sm:hidden absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg text-slate-600 hover:text-red-400">
              <Trash2 size={13} />
            </button>
          </div>
        {/each}
      {/if}
    </div>
  {/if}

  {#if showNewModal}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div class="w-full max-w-sm bg-[var(--bg-secondary)] border border-white/[0.08] rounded-2xl p-5 shadow-2xl">
        <h3 class="text-sm font-semibold text-slate-200 mb-3">
          New {showNewModal === 'file' ? 'File' : 'Folder'}
        </h3>
        <input bind:value={newName} placeholder={showNewModal === 'file' ? 'filename.txt' : 'folder-name'}
          class="w-full px-3 py-2 text-sm rounded-lg bg-white/[0.04] border border-white/[0.08] text-slate-200 placeholder-slate-600 focus:border-sky-500/30 focus:outline-none mb-4"
          onkeydown={(e) => { if (e.key === 'Enter') createItem(); }} />
        <div class="flex justify-end gap-2">
          <button onclick={() => { showNewModal = null; newName = ''; }}
            class="px-3 py-1.5 rounded-lg text-xs text-slate-400 hover:text-slate-200 bg-white/[0.04] hover:bg-white/[0.08] transition">Cancel</button>
          <button onclick={createItem} disabled={!newName.trim()}
            class="px-3 py-1.5 rounded-lg text-xs font-medium bg-sky-500/15 text-sky-400 hover:bg-sky-500/25 transition disabled:opacity-50">Create</button>
        </div>
      </div>
    </div>
  {/if}
</DashboardLayout>
