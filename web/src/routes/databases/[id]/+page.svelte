<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import LoadError from '$components/LoadError.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { toasts } from '$lib/toast.svelte';
  import {
    Table2, Eye, Search, RefreshCw, ChevronLeft, ChevronRight, Filter,
    KeyRound, Columns3, Rows3, Plus, X, Terminal, ArrowDownUp, Trash2, ExternalLink, Link2, ListTree,
  } from 'lucide-svelte';
  import { fmtSize, openPma } from '../pma';
  import ResultGrid from './ResultGrid.svelte';
  import RowEditor from './RowEditor.svelte';
  import SqlConsole from './SqlConsole.svelte';
  import ImportExport from './ImportExport.svelte';
  import { isPartial, toColumns, type Column, type Result, type TableInfo } from './types';

  const dbId = $derived(Number($page.params.id));

  let info = $state<{ name: string; type: string; account: string; site_id?: number; domain?: string; pma?: boolean } | null>(null);
  let tables = $state<TableInfo[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  let tableFilter = $state('');

  let current = $state<TableInfo | null>(null);
  let view = $state<'data' | 'structure' | 'sql' | 'transfer'>('data');

  // Data tab
  let data = $state<Result | null>(null);
  let dataLoading = $state(false);
  let dataError = $state('');
  let pageSize = $state(50);
  let offset = $state(0);
  let orderBy = $state('');
  let desc = $state(false);

  // Columns of the current table (structure tab, row editor, primary key)
  let columns = $state<Column[]>([]);
  let structure = $state<Result | null>(null);
  let indexes = $state<Result | null>(null);
  let foreignKeys = $state<Result | null>(null);
  let structureLoading = $state(false);
  const pk = $derived(columns.filter((c) => c.key === 'PRI').map((c) => c.name));
  const isView = $derived(!!current && current.type.toUpperCase().includes('VIEW'));
  const editable = $derived(!isView && pk.length > 0);

  // Row editing
  let selected = $state(new Set<number>());
  let editorOpen = $state(false);
  let editorMode = $state<'insert' | 'edit'>('insert');
  let editorRow = $state<Record<string, string | null> | null>(null);
  let editorKey = $state<Record<string, string | null> | null>(null);
  let saving = $state(false);
  let deleteOpen = $state(false);

  // Filters (column OP value; the value is sent as a bound parameter)
  type Cond = { column: string; op: string; value: string };
  const ops: { key: string; label: string; needsValue: boolean }[] = [
    { key: 'eq', label: '=', needsValue: true },
    { key: 'ne', label: '≠', needsValue: true },
    { key: 'contains', label: 'contains', needsValue: true },
    { key: 'starts', label: 'starts with', needsValue: true },
    { key: 'gt', label: '>', needsValue: true },
    { key: 'ge', label: '≥', needsValue: true },
    { key: 'lt', label: '<', needsValue: true },
    { key: 'le', label: '≤', needsValue: true },
    { key: 'null', label: 'is NULL', needsValue: false },
    { key: 'notnull', label: 'is not NULL', needsValue: false },
  ];
  let filters = $state<Cond[]>([]);
  let applied = $state<Cond[]>([]);
  const columnNames = $derived(data?.columns ?? []);
  const needsValue = (op: string) => ops.find((o) => o.key === op)?.needsValue ?? true;
  function addFilter() { filters = [...filters, { column: columnNames[0] ?? '', op: 'contains', value: '' }]; }
  function applyFilters() { applied = filters.filter((f) => f.column && (f.value !== '' || !needsValue(f.op))).map((f) => ({ ...f })); offset = 0; loadData(); }
  function clearFilters() { filters = []; applied = []; offset = 0; loadData(); }

  const filteredTables = $derived(
    tableFilter ? tables.filter((t) => label(t).toLowerCase().includes(tableFilter.toLowerCase())) : tables,
  );
  const label = (t: TableInfo) => (t.schema ? `${t.schema}.${t.name}` : t.name);
  const tableParams = (t: TableInfo) => `table=${encodeURIComponent(t.name)}${t.schema ? `&schema=${encodeURIComponent(t.schema)}` : ''}`;

  function toTables(r: Result): TableInfo[] {
    const idx = (n: string) => r.columns.indexOf(n);
    return r.rows.map((row) => ({
      schema: (row[idx('schema')] as string) || '',
      name: row[idx('name')] as string,
      type: (row[idx('type')] as string) || '',
      rows: Number(row[idx('rows')] ?? 0),
      size: Number(row[idx('size')] ?? 0),
    }));
  }

  async function loadTables() {
    loading = true;
    loadError = '';
    try {
      const d = await apiFetch(auth.token, 'GET', `databases/${dbId}/viewer/tables`);
      info = { name: d.database, type: d.type, account: d.account, site_id: d.site_id, domain: d.domain, pma: !!d.pma_available };
      tables = toTables(d.result);
      if (current) current = tables.find((t) => label(t) === label(current!)) ?? null;
      if (!current && tables.length) selectTable(tables[0]);
    } catch (e: any) {
      if (e.message !== 'session expired') loadError = e.message || 'Failed to open the database';
    } finally {
      loading = false;
    }
  }

  function selectTable(t: TableInfo) {
    current = t;
    offset = 0;
    orderBy = '';
    desc = false;
    data = null;
    structure = null;
    indexes = null;
    foreignKeys = null;
    columns = [];
    selected = new Set();
    filters = [];
    applied = [];
    if (view !== 'data' && view !== 'structure') view = 'data';
    loadColumns();
    if (view === 'data') loadData();
    else loadStructure();
  }

  async function loadColumns() {
    if (!current) return;
    const t = current;
    try {
      const d = await apiFetch(auth.token, 'GET', `databases/${dbId}/viewer/columns?${tableParams(t)}`);
      if (current !== t) return;
      structure = d.result;
      columns = toColumns(d.result);
    } catch (e: any) {
      if (current === t) dataError = e.message;
    }
  }

  async function loadData() {
    if (!current) return;
    dataLoading = true;
    dataError = '';
    try {
      const q = `${tableParams(current)}&limit=${pageSize}&offset=${offset}` +
        (orderBy ? `&order_by=${encodeURIComponent(orderBy)}&desc=${desc ? 1 : 0}` : '') +
        (applied.length ? `&filters=${encodeURIComponent(JSON.stringify(applied))}` : '');
      const d = await apiFetch(auth.token, 'GET', `databases/${dbId}/viewer/rows?${q}`);
      data = d.result;
      selected = new Set();
    } catch (e: any) {
      dataError = e.message || 'Failed to load rows';
    } finally {
      dataLoading = false;
    }
  }

  async function loadStructure() {
    if (!current) return;
    const t = current;
    structureLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `databases/${dbId}/manager/structure?${tableParams(t)}`);
      if (current !== t) return;
      indexes = d.result.indexes;
      foreignKeys = d.result.foreign_keys;
    } catch (e: any) {
      if (current === t) dataError = e.message;
    } finally {
      structureLoading = false;
    }
  }

  // ── Row edits ──
  function rowRecord(ri: number): Record<string, string | null> {
    const rec: Record<string, string | null> = {};
    data!.columns.forEach((c, ci) => (rec[c] = data!.rows[ri][ci]));
    return rec;
  }
  /** A row can be changed when every primary key value is shown in full. */
  function rowEditable(ri: number): boolean {
    if (!editable || !data) return false;
    return pk.every((k) => {
      const ci = data!.columns.indexOf(k);
      return ci >= 0 && data!.rows[ri][ci] !== null && !isPartial(data!.rows[ri][ci]);
    });
  }
  function keyOf(ri: number) {
    const rec = rowRecord(ri);
    return Object.fromEntries(pk.map((k) => [k, rec[k]]));
  }
  function openInsert() {
    editorMode = 'insert';
    editorRow = null;
    editorKey = null;
    editorOpen = true;
  }
  function openEdit(ri: number) {
    editorMode = 'edit';
    editorRow = rowRecord(ri);
    editorKey = keyOf(ri);
    editorOpen = true;
  }
  async function rowsCall(body: Record<string, unknown>) {
    return apiFetch(auth.token, 'POST', `databases/${dbId}/manager/rows`, { table: current!.name, schema: current!.schema, ...body });
  }
  async function saveRow(values: Record<string, string | null>) {
    saving = true;
    try {
      if (editorMode === 'insert') await rowsCall({ op: 'insert', values });
      else await rowsCall({ op: 'update', keys: [editorKey], values });
      editorOpen = false;
      toasts.success(editorMode === 'insert' ? 'Row inserted' : 'Row saved');
      loadData();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      saving = false;
    }
  }
  async function deleteSelected() {
    const keys = [...selected].filter(rowEditable).map(keyOf);
    if (!keys.length) return;
    saving = true;
    try {
      const d = await rowsCall({ op: 'delete', keys });
      toasts.success(`Deleted ${d.result?.rows_affected ?? keys.length} row(s)`);
      loadData();
      loadTables();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      saving = false;
    }
  }

  function sortBy(col: string) {
    if (orderBy === col) desc = !desc;
    else {
      orderBy = col;
      desc = false;
    }
    offset = 0;
    loadData();
  }

  function switchView(v: typeof view) {
    view = v;
    if (v === 'data' && !data) loadData();
    if (v === 'structure' && !indexes) loadStructure();
  }

  let loadedFor = 0;
  $effect(() => {
    if (dbId && dbId !== loadedFor) {
      loadedFor = dbId;
      current = null;
      loadTables();
    }
  });
</script>


<DashboardLayout active="Databases">
  <PageHeader title={info ? info.name : 'Database'} back="/databases" backLabel="Databases" description={info?.domain ? `Site ${info.domain}` : ''}>
    {#snippet meta()}
      {#if info}
        <span class="badge">{info.type === 'postgres' ? 'PostgreSQL' : 'MySQL'}</span>
        <span class="badge" title="Everything here runs as this database account, with its privileges"><KeyRound size={11} /> {info.account}</span>
      {/if}
    {/snippet}
    {#snippet actions()}
      {#if info?.pma}
        <button onclick={() => openPma(dbId)} class="btn btn-secondary btn-sm" title="Open this database in phpMyAdmin"><ExternalLink size={14} /> phpMyAdmin</button>
      {/if}
      <button onclick={loadTables} disabled={loading} class="btn btn-secondary btn-sm"><RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh</button>
    {/snippet}
  </PageHeader>

  {#if loadError}
    <LoadError message={loadError} onretry={loadTables} />
  {:else}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-[16rem_minmax(0,1fr)]">
      <!-- Tables -->
      <aside class="card flex max-h-[calc(100dvh-220px)] flex-col overflow-hidden max-lg:max-h-72">
        <div class="border-b border-line p-2.5">
          <div class="relative">
            <Search size={14} class="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-subtle" />
            <input bind:value={tableFilter} placeholder="Filter tables…" aria-label="Filter tables" class="input h-8 pl-8 text-[13px]" />
          </div>
        </div>
        <div class="flex-1 overflow-y-auto p-1.5">
          {#if loading && tables.length === 0}
            {#each [1, 2, 3, 4, 5] as i (i)}<div class="skeleton m-1 h-7"></div>{/each}
          {:else if tables.length === 0}
            <p class="p-4 text-center text-[13px] text-subtle">No tables yet.</p>
          {:else}
            {#each filteredTables as t (label(t))}
              <button
                onclick={() => selectTable(t)}
                class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-[13px] transition-colors {current && label(current) === label(t) ? 'bg-accent/10 text-accent' : 'text-muted hover:bg-surface-2 hover:text-fg'}"
                title="{label(t)} · ~{t.rows.toLocaleString()} rows · {fmtSize(t.size)}">
                {#if t.type.includes('VIEW')}<Eye size={14} class="shrink-0" />{:else}<Table2 size={14} class="shrink-0" />{/if}
                <span class="min-w-0 flex-1 truncate font-mono">{label(t)}</span>
                <span class="shrink-0 text-[11px] text-subtle tabular-nums">{t.rows > 0 ? `~${t.rows.toLocaleString()}` : ''}</span>
              </button>
            {/each}
          {/if}
        </div>
        <div class="border-t border-line px-3 py-2 text-[11px] text-subtle">{tables.length} table{tables.length === 1 ? '' : 's'} · counts are estimates</div>
      </aside>

      <!-- Content -->
      <section class="card min-w-0 overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-line px-3 py-2">
          <div class="tabs !mb-0 border-0" role="tablist">
            <button role="tab" aria-selected={view === 'data'} onclick={() => switchView('data')} class="tab {view === 'data' ? 'tab-active' : ''}" disabled={!current}><Rows3 size={14} /> Data</button>
            <button role="tab" aria-selected={view === 'structure'} onclick={() => switchView('structure')} class="tab {view === 'structure' ? 'tab-active' : ''}" disabled={!current}><Columns3 size={14} /> Structure</button>
            <button role="tab" aria-selected={view === 'sql'} onclick={() => switchView('sql')} class="tab {view === 'sql' ? 'tab-active' : ''}"><Terminal size={14} /> SQL</button>
            <button role="tab" aria-selected={view === 'transfer'} onclick={() => switchView('transfer')} class="tab {view === 'transfer' ? 'tab-active' : ''}"><ArrowDownUp size={14} /> Import &amp; export</button>
          </div>
          {#if current && (view === 'data' || view === 'structure')}
            <span class="truncate font-mono text-[13px] text-muted">{label(current)}</span>
          {/if}
        </div>

        {#if view === 'data'}
          {#if !current}
            <p class="p-6 text-center text-[13px] text-subtle">Pick a table.</p>
          {:else if dataError}
            <p class="p-5 text-[13px] text-danger">{dataError}</p>
          {:else if dataLoading && !data}
            <div class="space-y-2 p-4">{#each [1, 2, 3, 4, 5, 6] as i (i)}<div class="skeleton h-6"></div>{/each}</div>
          {:else if data}
            <div class="space-y-2 border-b border-line px-3 py-2">
              {#each filters as f, i (i)}
                <div class="flex flex-wrap items-center gap-1.5">
                  <select bind:value={f.column} aria-label="Column" class="input h-8 w-auto max-w-48 py-0 font-mono text-[12px]">
                    {#each columnNames as c (c)}<option value={c}>{c}</option>{/each}
                  </select>
                  <select bind:value={f.op} aria-label="Operator" class="input h-8 w-auto py-0 text-[12px]">
                    {#each ops as o (o.key)}<option value={o.key}>{o.label}</option>{/each}
                  </select>
                  {#if needsValue(f.op)}
                    <input bind:value={f.value} placeholder="value" aria-label="Value" onkeydown={(e) => e.key === 'Enter' && applyFilters()} class="input h-8 min-w-0 flex-1 font-mono text-[12px] sm:max-w-64" />
                  {/if}
                  <button onclick={() => (filters = filters.filter((_, j) => j !== i))} class="btn btn-ghost btn-icon btn-xs" aria-label="Remove filter"><X size={13} /></button>
                </div>
              {/each}
              <div class="flex flex-wrap items-center gap-1.5">
                {#if editable}
                  <button onclick={openInsert} class="btn btn-secondary btn-xs"><Plus size={13} /> Insert row</button>
                  {#if selected.size}
                    <button onclick={() => (deleteOpen = true)} disabled={saving} class="btn btn-danger-ghost btn-xs"><Trash2 size={13} /> Delete {selected.size}</button>
                  {/if}
                  <span class="mx-1 h-4 w-px bg-line"></span>
                {/if}
                <button onclick={addFilter} disabled={!columnNames.length} class="btn btn-ghost btn-xs"><Filter size={13} /> Filter</button>
                {#if filters.length}
                  <button onclick={applyFilters} class="btn btn-primary btn-xs">Apply</button>
                {/if}
                {#if applied.length}
                  <button onclick={clearFilters} class="btn btn-ghost btn-xs">Clear filters</button>
                  <span class="text-[12px] text-subtle">{applied.length} filter{applied.length === 1 ? '' : 's'} active</span>
                {/if}
                {#if columns.length && !editable}
                  <span class="ml-auto text-[11px] text-subtle" title="Rows are changed by primary key">
                    {isView ? 'Views cannot be edited here' : 'No primary key: edit this table with the SQL console'}
                  </span>
                {/if}
              </div>
            </div>
            <div class:opacity-60={dataLoading}>
              <ResultGrid result={data} rowOffset={offset} sortable {orderBy} {desc} onsort={sortBy}
                selectable={editable} bind:selected canEdit={rowEditable} onedit={editable ? openEdit : undefined} />
            </div>
            <div class="flex flex-wrap items-center justify-between gap-2 border-t border-line px-3 py-2 text-[12px] text-subtle">
              <span class="tabular-nums">
                {#if data.rows.length}Rows {offset + 1}–{offset + data.rows.length}{:else}No rows{/if}
                {#if data.truncated}<span class="text-warning"> · long values are cut</span>{/if}
              </span>
              <div class="flex items-center gap-1.5">
                <select bind:value={pageSize} onchange={() => { offset = 0; loadData(); }} aria-label="Rows per page" class="input h-7 w-auto py-0 text-[12px]">
                  {#each [25, 50, 100, 250, 500] as n (n)}<option value={n}>{n} / page</option>{/each}
                </select>
                <button onclick={() => { offset = Math.max(0, offset - pageSize); loadData(); }} disabled={offset === 0 || dataLoading} class="btn btn-ghost btn-icon btn-xs" aria-label="Previous page"><ChevronLeft size={14} /></button>
                <button onclick={() => { offset += pageSize; loadData(); }} disabled={!data.has_more || dataLoading} class="btn btn-ghost btn-icon btn-xs" aria-label="Next page"><ChevronRight size={14} /></button>
              </div>
            </div>
          {/if}
        {:else if view === 'structure'}
          {#if !structure && (structureLoading || !dataError)}
            <div class="space-y-2 p-4">{#each [1, 2, 3, 4] as i (i)}<div class="skeleton h-6"></div>{/each}</div>
          {:else if dataError}
            <p class="p-5 text-[13px] text-danger">{dataError}</p>
          {:else if structure}
            <div class="max-h-[calc(100dvh-260px)] overflow-y-auto">
              <h3 class="flex items-center gap-1.5 px-4 pt-3 pb-2 text-[12px] font-medium tracking-wide text-subtle uppercase"><Columns3 size={13} /> Columns</h3>
              <ResultGrid result={structure} maxHeight="" />
              <h3 class="flex items-center gap-1.5 border-t border-line px-4 pt-4 pb-2 text-[12px] font-medium tracking-wide text-subtle uppercase"><ListTree size={13} /> Indexes</h3>
              {#if indexes}<ResultGrid result={indexes} maxHeight="" />{:else}<div class="skeleton m-4 h-6"></div>{/if}
              <h3 class="flex items-center gap-1.5 border-t border-line px-4 pt-4 pb-2 text-[12px] font-medium tracking-wide text-subtle uppercase"><Link2 size={13} /> Foreign keys</h3>
              {#if foreignKeys}<ResultGrid result={foreignKeys} maxHeight="" />{:else}<div class="skeleton m-4 h-6"></div>{/if}
              <p class="border-t border-line px-4 py-3 text-[12px] text-subtle">Change the structure (ALTER TABLE, CREATE INDEX…) with the SQL console.</p>
            </div>
          {/if}
        {:else if view === 'sql' && info}
          <SqlConsole {dbId} dbType={info.type} onchanged={loadTables} />
        {:else if view === 'transfer' && info}
          <ImportExport {dbId} dbName={info.name} dbType={info.type} domain={info.domain} onchanged={loadTables} />
        {/if}
      </section>
    </div>
  {/if}
</DashboardLayout>

{#if current && columns.length}
  <RowEditor bind:open={editorOpen} mode={editorMode} table={label(current)} {columns} original={editorRow} busy={saving} onsave={saveRow} />
{/if}
<ConfirmDialog bind:open={deleteOpen} title="Delete {selected.size} row{selected.size === 1 ? '' : 's'}?" destructive confirmLabel="Delete"
  message="The selected rows are deleted from {current ? label(current) : 'the table'} right away. This cannot be undone." onconfirm={deleteSelected} />
