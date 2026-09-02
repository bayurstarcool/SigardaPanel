<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  let { items = [], onSelect = (item: any) => {} } = $props();
  let searchQuery = $state('');
  let isOpen = $state(false);
  let selectedIndex = $state(0);
  let searchInput = $state<HTMLInputElement>();

  let filteredItems = $derived(
    searchQuery 
      ? items.filter((item: any) => 
          item.label.toLowerCase().includes(searchQuery.toLowerCase()) ||
          item.href?.toLowerCase().includes(searchQuery.toLowerCase())
        )
      : items
  );

  onMount(() => {
    // Keyboard shortcut: Ctrl+K or Cmd+K
    function handleKeydown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        isOpen = !isOpen;
        if (isOpen) {
          setTimeout(() => searchInput?.focus(), 100);
        }
      }
      
      if (e.key === 'Escape') {
        isOpen = false;
        searchQuery = '';
      }

      if (isOpen) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          selectedIndex = Math.min(selectedIndex + 1, filteredItems.length - 1);
        } else if (e.key === 'ArrowUp') {
          e.preventDefault();
          selectedIndex = Math.max(selectedIndex - 1, 0);
        } else if (e.key === 'Enter' && filteredItems[selectedIndex]) {
          e.preventDefault();
          selectItem(filteredItems[selectedIndex]);
        }
      }
    }

    document.addEventListener('keydown', handleKeydown);
    return () => document.removeEventListener('keydown', handleKeydown);
  });

  function selectItem(item: any) {
    if (item.href) {
      goto(item.href);
    }
    onSelect(item);
    isOpen = false;
    searchQuery = '';
  }
</script>

{#if isOpen}
  <div class="search-overlay" onclick={() => { isOpen = false; searchQuery = ''; }}>
    <div class="search-modal" onclick={(e) => e.stopPropagation()}>
      <div class="search-header">
        <span class="search-icon">🔍</span>
        <input
          bind:this={searchInput}
          bind:value={searchQuery}
          type="search"
          placeholder="Search pages..."
          class="search-input"
        />
        <kbd class="kbd">ESC</kbd>
      </div>
      
      <div class="search-results">
        {#if filteredItems.length === 0}
          <div class="no-results">No results found</div>
        {:else}
          {#each filteredItems as item, i}
            <button 
              class="search-item"
              class:selected={i === selectedIndex}
              onclick={() => selectItem(item)}
              onmouseenter={() => selectedIndex = i}
            >
              <span class="item-icon">{item.icon || '📄'}</span>
              <div class="item-content">
                <span class="item-label">{item.label}</span>
                {#if item.href}
                  <span class="item-href">{item.href}</span>
                {/if}
              </div>
            </button>
          {/each}
        {/if}
      </div>
      
      <div class="search-footer">
        <span class="hint">↑↓ Navigate</span>
        <span class="hint">↵ Select</span>
        <span class="hint">ESC Close</span>
      </div>
    </div>
  </div>
{/if}

<style>
  .search-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 20vh;
    z-index: 1000;
  }
  .search-modal {
    background: white;
    border-radius: 12px;
    width: 90%;
    max-width: 480px;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
    overflow: hidden;
  }
  .search-header {
    display: flex;
    align-items: center;
    padding: 12px 16px;
    border-bottom: 1px solid #e5e7eb;
  }
  .search-icon {
    margin-right: 8px;
  }
  .search-input {
    flex: 1;
    border: none;
    outline: none;
    font-size: 1rem;
    background: transparent;
  }
  .kbd {
    padding: 2px 6px;
    background: #f3f4f6;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #6b7280;
  }
  .search-results {
    max-height: 300px;
    overflow-y: auto;
    padding: 8px;
  }
  .no-results {
    padding: 16px;
    text-align: center;
    color: #6b7280;
  }
  .search-item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 8px 12px;
    border: none;
    background: transparent;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.15s;
  }
  .search-item:hover, .search-item.selected {
    background: #f3f4f6;
  }
  .item-icon {
    margin-right: 10px;
    font-size: 1.125rem;
  }
  .item-content {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }
  .item-label {
    font-size: 0.875rem;
    color: #1f2937;
  }
  .item-href {
    font-size: 0.75rem;
    color: #6b7280;
  }
  .search-footer {
    display: flex;
    gap: 16px;
    padding: 8px 16px;
    border-top: 1px solid #e5e7eb;
    background: #f9fafb;
  }
  .hint {
    font-size: 0.75rem;
    color: #6b7280;
  }
</style>
