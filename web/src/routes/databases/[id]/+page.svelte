<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import LoadError from '$components/LoadError.svelte';
  import {
    Table2, Eye, Search, RefreshCw, ChevronLeft, ChevronRight, ArrowUp, ArrowDown, Filter,
    KeyRound, Columns3, Rows3, Lock, Copy, Check, Plus, X,
  } from 'lucide-svelte';
  import { fmtSize } from '../pma';

  type Result = { columns: string[]; rows: (string | null)[][]; has_more: boolean; truncated?: boolean; elapsed_ms?: number };
  type TableInfo = { schema: string; name: string; type: string; rows: number; size: number };

  const dbId = $derived(Number($page.params.id));

  let info = $state<{ name: string; type: string; account: string; site_id?: number; domain?: string } | null>(null);
  let tables = $state<TableInfo[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  let tableFilter = $state('');

  let current = $state<TableInfo | null>(null);
  let view = $state<'data' | 'structure'>('data');

  // Data tab
  let data = $state<Result | null>(null);
  let dataLoading = $state(false);
  let dataError = $state('');
  let pageSize = $state(50);
  let offset = $state(0);
  let orderBy = $state('');
  let desc = $state(false);

  // Structure tab
  let structure = $state<Result | null>(null);
  let structureLoading = $state(false);

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

  let copied = $state('');

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
      info = { name: d.database, type: d.type, account: d.account, site_id: d.site_id, domain: d.domain };
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
    filters = [];
    applied = [];
    load();
  }

  function load() {
    if (view === 'data') loadData();
    else if (view === 'structure') loadStructure();
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
    } catch (e: any) {
      dataError = e.message || 'Failed to load rows';
    } finally {
      dataLoading = false;
    }
  }

  async function loadStructure() {
    if (!current) return;
    structureLoading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `databases/${dbId}/viewer/columns?${tableParams(current)}`);
      structure = d.result;
    } catch (e: any) {
      dataError = e.message;
    } finally {
      structureLoading = false;
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
    if (v === 'structure' && !structure) loadStructure();
  }

  function copyCell(v: string | null, key: string) {
    navigator.clipboard.writeText(v ?? 'NULL');
    copied = key;
    setTimeout(() => (copied = ''), 1200);
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

{#snippet grid(r: Result, sortable: boolean)}
  {#if r.columns.length === 0}
    <p class="p-5 text-[13px] text-subtle">The statement returned no columns.</p>
  {:else}
    <div class="max-h-[calc(100dvh-340px)] min-h-40 overflow-auto">
      <table class="w-max min-w-full border-collapse text-[13px]">
        <thead class="sticky top-0 z-10 bg-surface-2">
          <tr>
            <th class="w-10 border-b border-line px-2 py-2 text-right text-[11px] font-medium text-subtle">#</th>
            {#each r.columns as col, ci (ci)}
              <th class="border-b border-l border-line px-3 py-2 text-left font-medium whitespace-nowrap text-fg">
                {#if sortable}
                  <button onclick={() => sortBy(col)} class="inline-flex items-center gap-1 hover:text-accent" title="Sort by {col}">
                    {col}
                    {#if orderBy === col}{#if desc}<ArrowDown size={12} />{:else}<ArrowUp size={12} />{/if}{/if}
                  </button>
                {:else}{col}{/if}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each r.rows as row, ri (ri)}
            <tr class="hover:bg-surface-2/50">
              <td class="border-b border-line px-2 py-1.5 text-right text-[11px] text-subtle tabular-nums">{(sortable ? offset : 0) + ri + 1}</td>
              {#each row as cell, ci (ci)}
                {@const key = `${ri}:${ci}`}
                <td
                  class="group max-w-[28rem] border-b border-l border-line px-3 py-1.5 font-mono text-[12px] whitespace-nowrap"
                  title={cell === null ? 'NULL' : cell.length > 80 ? cell.slice(0, 1000) : ''}>
                  <div class="flex items-center gap-1.5">
                    {#if cell === null}
                      <span class="text-subtle italic">NULL</span>
                    {:else}
                      <span class="truncate {cell.startsWith('[binary') ? 'text-subtle italic' : 'text-fg'}">{cell}</span>
                    {/if}
                    <button onclick={() => copyCell(cell, key)} class="opacity-0 group-hover:opacity-100 text-subtle hover:text-fg" aria-label="Copy value">
                      {#if copied === key}<Check size={11} />{:else}<Copy size={11} />{/if}
                    </button>
                  </div>
                </td>
              {/each}
            </tr>
          {:else}
            <tr><td colspan={r.columns.length + 1} class="px-4 py-8 text-center text-[13px] text-subtle">No rows</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/snippet}

<DashboardLayout active="Databases">
  <PageHeader title={info ? info.name : 'Database'} back="/databases" backLabel="Databases" description={info?.domain ? `Site ${info.domain}` : ''}>
    {#snippet meta()}
      {#if info}
        <span class="badge">{info.type === 'postgres' ? 'PostgreSQL' : 'MySQL'}</span>
        <span class="badge badge-success" title="Every query runs in a read-only transaction"><Lock size={11} /> Read-only</span>
        <span class="badge" title="Database account used by the viewer"><KeyRound size={11} /> {info.account}</span>
      {/if}
    {/snippet}
    {#snippet actions()}
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
          </div>
          {#if current}
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
                <button onclick={addFilter} disabled={!columnNames.length} class="btn btn-ghost btn-xs"><Plus size={13} /> Filter</button>
                {#if filters.length}
                  <button onclick={applyFilters} class="btn btn-primary btn-xs"><Filter size={12} /> Apply</button>
                {/if}
                {#if applied.length}
                  <button onclick={clearFilters} class="btn btn-ghost btn-xs">Clear filters</button>
                  <span class="text-[12px] text-subtle">{applied.length} filter{applied.length === 1 ? '' : 's'} active</span>
                {/if}
              </div>
            </div>
            <div class:opacity-60={dataLoading}>{@render grid(data, true)}</div>
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
          {#if structureLoading && !structure}
            <div class="space-y-2 p-4">{#each [1, 2, 3, 4] as i (i)}<div class="skeleton h-6"></div>{/each}</div>
          {:else if structure}
            {@render grid(structure, false)}
          {/if}
        {/if}
      </section>
    </div>
  {/if}
</DashboardLayout>
