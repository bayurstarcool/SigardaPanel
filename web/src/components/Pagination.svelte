<script lang="ts">
  let { 
    totalItems = 0, 
    pageSize = 20, 
    currentPage = 1, 
    onPageChange = (page: number) => {} 
  } = $props();

  let totalPages = $derived(Math.ceil(totalItems / pageSize));
  let pages = $derived(generatePages(currentPage, totalPages));

  function generatePages(current: number, total: number): (number | string)[] {
    if (total <= 7) {
      return Array.from({ length: total }, (_, i) => i + 1);
    }

    const pages: (number | string)[] = [];
    
    if (current <= 3) {
      pages.push(1, 2, 3, 4, '...', total);
    } else if (current >= total - 2) {
      pages.push(1, '...', total - 3, total - 2, total - 1, total);
    } else {
      pages.push(1, '...', current - 1, current, current + 1, '...', total);
    }

    return pages;
  }

  function goToPage(page: number | string) {
    if (typeof page === 'number' && page >= 1 && page <= totalPages) {
      onPageChange(page);
    }
  }
</script>

{#if totalPages > 1}
  <div class="pagination">
    <button 
      class="page-btn"
      disabled={currentPage === 1}
      onclick={() => goToPage(currentPage - 1)}
    >
      ←
    </button>

    {#each pages as page}
      {#if page === '...'}
        <span class="ellipsis">...</span>
      {:else}
        <button 
          class="page-btn"
          class:active={page === currentPage}
          onclick={() => goToPage(page)}
        >
          {page}
        </button>
      {/if}
    {/each}

    <button 
      class="page-btn"
      disabled={currentPage === totalPages}
      onclick={() => goToPage(currentPage + 1)}
    >
      →
    </button>
  </div>
{/if}

<style>
  .pagination {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 1rem 0;
  }
  .page-btn {
    min-width: 32px;
    height: 32px;
    padding: 0 8px;
    border: 1px solid #d1d5db;
    border-radius: 6px;
    background: white;
    color: #374151;
    font-size: 0.875rem;
    cursor: pointer;
    transition: all 0.2s;
  }
  .page-btn:hover:not(:disabled) {
    background: #f3f4f6;
    border-color: #9ca3af;
  }
  .page-btn.active {
    background: #3b82f6;
    color: white;
    border-color: #3b82f6;
  }
  .page-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .ellipsis {
    padding: 0 4px;
    color: #6b7280;
  }
</style>
