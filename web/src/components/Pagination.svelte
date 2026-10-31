<script lang="ts">
  import { ChevronLeft, ChevronRight } from 'lucide-svelte';

  let {
    totalItems = 0,
    pageSize = 20,
    currentPage = 1,
    onPageChange = (_page: number) => {},
  }: { totalItems?: number; pageSize?: number; currentPage?: number; onPageChange?: (page: number) => void } = $props();

  const totalPages = $derived(Math.max(1, Math.ceil(totalItems / pageSize)));
  const pages = $derived(generatePages(currentPage, totalPages));
  const from = $derived(totalItems === 0 ? 0 : (currentPage - 1) * pageSize + 1);
  const to = $derived(Math.min(currentPage * pageSize, totalItems));

  function generatePages(current: number, total: number): (number | '…')[] {
    if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
    if (current <= 3) return [1, 2, 3, 4, '…', total];
    if (current >= total - 2) return [1, '…', total - 3, total - 2, total - 1, total];
    return [1, '…', current - 1, current, current + 1, '…', total];
  }

  function go(p: number) {
    if (p >= 1 && p <= totalPages && p !== currentPage) onPageChange(p);
  }
</script>

{#if totalPages > 1}
  <div class="flex flex-wrap items-center justify-between gap-3 pt-4 text-[13px]">
    <span class="text-subtle tabular-nums">{from}–{to} of {totalItems}</span>
    <div class="flex items-center gap-1">
      <button class="btn btn-ghost btn-icon btn-xs" disabled={currentPage === 1} onclick={() => go(currentPage - 1)} aria-label="Previous page">
        <ChevronLeft size={15} />
      </button>
      {#each pages as p, i (i)}
        {#if p === '…'}
          <span class="px-1 text-subtle">…</span>
        {:else}
          <button
            class="btn btn-xs min-w-7 tabular-nums {p === currentPage ? 'bg-surface-2 text-fg' : 'btn-ghost'}"
            aria-current={p === currentPage ? 'page' : undefined}
            onclick={() => go(p)}>{p}</button>
        {/if}
      {/each}
      <button class="btn btn-ghost btn-icon btn-xs" disabled={currentPage === totalPages} onclick={() => go(currentPage + 1)} aria-label="Next page">
        <ChevronRight size={15} />
      </button>
    </div>
  </div>
{/if}
