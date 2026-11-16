<script lang="ts">
  import { bearerHeaders } from '$lib/auth.svelte';
  import { toasts } from '$lib/toast.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { Download, Upload, FileUp, FolderOpen, CircleCheck } from 'lucide-svelte';
  import { fmtSize } from '../pma';

  let { dbId, dbName, dbType, domain, onchanged }: { dbId: number; dbName: string; dbType: string; domain?: string; onchanged?: () => void } = $props();

  const MAX_UPLOAD = 30 << 20;

  // ── Export ──
  let exporting = $state(false);
  let exported = $state(0);

  async function exportDb() {
    exporting = true;
    exported = 0;
    try {
      const r = await fetch(`/api/proxy/databases/${dbId}/manager/export`, { headers: bearerHeaders(), cache: 'no-store' });
      if (!r.ok || !r.body) {
        const d = await r.json().catch(() => null);
        throw new Error(d?.error?.message || `Export failed (HTTP ${r.status})`);
      }
      const name = /filename="([^"]+)"/.exec(r.headers.get('content-disposition') || '')?.[1] || `${dbName}.sql`;
      const reader = r.body.getReader();
      const chunks: Uint8Array[] = [];
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        exported += value.length;
      }
      const blob = new Blob(chunks as BlobPart[], { type: 'application/sql' });
      // A failure after the dump started is marked at its end (see the agent).
      const tail = await blob.slice(Math.max(0, blob.size - 200)).text();
      if (tail.includes('SIGARDAPANEL EXPORT FAILED')) throw new Error('The export stopped before the end; the file would be incomplete. Try again.');
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
      toasts.success(`Exported ${fmtSize(blob.size)}`);
    } catch (e: any) {
      toasts.error(e.message || 'Export failed');
    } finally {
      exporting = false;
    }
  }

  // ── Import ──
  let source = $state<'upload' | 'path'>('upload');
  let file = $state<File | null>(null);
  let path = $state('');
  let importing = $state(false);
  let confirmOpen = $state(false);
  let lastImport = $state('');
  let fileInput = $state<HTMLInputElement>();

  const ready = $derived(source === 'upload' ? !!file && file.size <= MAX_UPLOAD : /\.sql$/i.test(path.trim()));

  function pick(e: Event) {
    const f = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    if (f && f.size > MAX_UPLOAD) {
      toasts.error(`${f.name} is larger than 30 MB: upload it to the site's folder with the file manager (or SFTP), then import it from there.`);
      file = null;
      return;
    }
    file = f;
  }

  async function doImport() {
    importing = true;
    lastImport = '';
    try {
      let r: Response;
      if (source === 'upload' && file) {
        r = await fetch(`/api/proxy/databases/${dbId}/manager/import`, {
          method: 'POST', headers: { ...bearerHeaders(), 'Content-Type': 'application/octet-stream' }, body: file,
        });
      } else {
        r = await fetch(`/api/proxy/databases/${dbId}/manager/import?path=${encodeURIComponent(path.trim().replace(/^\/+/, ''))}`, {
          method: 'POST', headers: bearerHeaders(),
        });
      }
      const d = await r.json().catch(() => null);
      if (!r.ok || d?.error) throw new Error(d?.error?.message || `Import failed (HTTP ${r.status})`);
      lastImport = source === 'upload' ? file!.name : path.trim();
      toasts.success('Import finished');
      file = null;
      if (fileInput) fileInput.value = '';
      onchanged?.();
    } catch (e: any) {
      toasts.error(e.message || 'Import failed');
    } finally {
      importing = false;
    }
  }
</script>

<div class="grid gap-4 p-4 md:grid-cols-2">
  <div class="space-y-3 rounded-xl border border-line p-4">
    <div class="flex items-center gap-2 text-[14px] font-medium text-fg"><Download size={16} /> Export</div>
    <p class="text-[13px] text-muted">
      Download the whole database (structure and data) as a plain <code>.sql</code> file, made with
      {dbType === 'postgres' ? 'pg_dump' : 'mysqldump'} using the database's own account.
    </p>
    <button onclick={exportDb} disabled={exporting} class="btn btn-primary btn-sm">
      <Download size={13} /> {exporting ? `Exporting… ${fmtSize(exported)}` : `Download ${dbName}.sql`}
    </button>
  </div>

  <div class="space-y-3 rounded-xl border border-line p-4">
    <div class="flex items-center gap-2 text-[14px] font-medium text-fg"><Upload size={16} /> Import</div>
    <p class="text-[13px] text-muted">Run a <code>.sql</code> file against this database. Existing tables with the same names may be replaced.</p>
    <div class="tabs !mb-0" role="tablist">
      <button role="tab" aria-selected={source === 'upload'} onclick={() => (source = 'upload')} class="tab {source === 'upload' ? 'tab-active' : ''}"><FileUp size={13} /> Upload</button>
      <button role="tab" aria-selected={source === 'path'} onclick={() => (source = 'path')} class="tab {source === 'path' ? 'tab-active' : ''}"><FolderOpen size={13} /> From site folder</button>
    </div>
    {#if source === 'upload'}
      <input bind:this={fileInput} type="file" accept=".sql,application/sql,text/plain" onchange={pick} aria-label="SQL file" class="input h-auto py-1.5 text-[13px]" />
      <p class="text-[11px] text-subtle">Up to 30 MB. Bigger dumps: upload them to the site with the file manager or SFTP, then use “From site folder”.</p>
    {:else}
      <input bind:value={path} placeholder="backups/dump.sql" aria-label="Path inside the site folder" class="input h-9 font-mono text-[12px]" />
      <p class="text-[11px] text-subtle">Path relative to the site's folder{domain ? ` (${domain})` : ''}. Any size.</p>
    {/if}
    <button onclick={() => (confirmOpen = true)} disabled={!ready || importing} class="btn btn-secondary btn-sm">
      <Upload size={13} /> {importing ? 'Importing…' : 'Import'}
    </button>
    {#if lastImport}
      <p class="flex items-center gap-1.5 text-[12px] text-success"><CircleCheck size={13} /> Imported {lastImport}</p>
    {/if}
  </div>
</div>

<ConfirmDialog bind:open={confirmOpen} title="Import into {dbName}?" destructive confirmLabel="Import"
  message="The SQL in the file runs against {dbName} now. It can replace or delete existing tables and data. Export a copy first if you may need it."
  onconfirm={doImport} />
