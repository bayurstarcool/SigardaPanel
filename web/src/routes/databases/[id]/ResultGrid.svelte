<script lang="ts">
  import { ArrowUp, ArrowDown, Copy, Check, Pencil } from 'lucide-svelte';
  import type { Result } from './types';

  let {
    result,
    rowOffset = 0,
    sortable = false,
    orderBy = '',
    desc = false,
    onsort,
    selectable = false,
    selected = $bindable(new Set<number>()),
    canEdit,
    onedit,
    maxHeight = 'max-h-[calc(100dvh-360px)]',
  }: {
    result: Result;
    rowOffset?: number;
    sortable?: boolean;
    orderBy?: string;
    desc?: boolean;
    onsort?: (col: string) => void;
    selectable?: boolean;
    selected?: Set<number>;
    canEdit?: (ri: number) => boolean;
    onedit?: (ri: number) => void;
    maxHeight?: string;
  } = $props();

  let copied = $state('');
  function copyCell(v: string | null, key: string) {
    navigator.clipboard.writeText(v ?? 'NULL');
    copied = key;
    setTimeout(() => (copied = ''), 1200);
  }
  function toggle(ri: number) {
    const next = new Set(selected);
    if (next.has(ri)) next.delete(ri);
    else next.add(ri);
    selected = next;
  }
  const selectableRows = $derived(result.rows.map((_, i) => i).filter((i) => !canEdit || canEdit(i)));
  const allSelected = $derived(selectableRows.length > 0 && selectableRows.every((i) => selected.has(i)));
  function toggleAll() {
    selected = allSelected ? new Set() : new Set(selectableRows);
  }
</script>

{#if result.columns.length === 0}
  <p class="p-5 text-[13px] text-subtle">The statement returned no columns.</p>
{:else}
  <div class="{maxHeight} {maxHeight ? 'min-h-40' : ''} overflow-auto">
    <table class="w-max min-w-full border-collapse text-[13px]">
      <thead class="sticky top-0 z-10 bg-surface-2">
        <tr>
          {#if selectable}
            <th class="w-8 border-b border-line px-2 py-2">
              <input type="checkbox" checked={allSelected} onchange={toggleAll} aria-label="Select all rows" class="checkbox" />
            </th>
          {/if}
          <th class="w-10 border-b border-line px-2 py-2 text-right text-[11px] font-medium text-subtle">#</th>
          {#each result.columns as col, ci (ci)}
            <th class="border-b border-l border-line px-3 py-2 text-left font-medium whitespace-nowrap text-fg">
              {#if sortable}
                <button onclick={() => onsort?.(col)} class="inline-flex items-center gap-1 hover:text-accent" title="Sort by {col}">
                  {col}
                  {#if orderBy === col}{#if desc}<ArrowDown size={12} />{:else}<ArrowUp size={12} />{/if}{/if}
                </button>
              {:else}{col}{/if}
            </th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each result.rows as row, ri (ri)}
          {@const editable = !!onedit && (!canEdit || canEdit(ri))}
          <tr class="group/row hover:bg-surface-2/50 {selected.has(ri) ? 'bg-accent/5' : ''}" ondblclick={() => editable && onedit?.(ri)}>
            {#if selectable}
              <td class="border-b border-line px-2 py-1.5 text-center">
                <input type="checkbox" checked={selected.has(ri)} disabled={canEdit && !canEdit(ri)} onchange={() => toggle(ri)} aria-label="Select row {rowOffset + ri + 1}" class="checkbox" />
              </td>
            {/if}
            <td class="border-b border-line px-2 py-1.5 text-right text-[11px] text-subtle tabular-nums">
              {#if editable}
                <button onclick={() => onedit?.(ri)} class="hidden text-subtle group-hover/row:inline-flex hover:text-accent" aria-label="Edit row {rowOffset + ri + 1}" title="Edit row"><Pencil size={12} /></button>
                <span class="group-hover/row:hidden">{rowOffset + ri + 1}</span>
              {:else}{rowOffset + ri + 1}{/if}
            </td>
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
          <tr><td colspan={result.columns.length + (selectable ? 2 : 1)} class="px-4 py-8 text-center text-[13px] text-subtle">No rows</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
