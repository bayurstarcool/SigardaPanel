<script lang="ts">
  import { onMount } from 'svelte';

  let { shortcuts = [] } = $props();
  let showHelp = $state(false);

  onMount(() => {
    function handleKeydown(e: KeyboardEvent) {
      // Don't trigger shortcuts when typing in inputs
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT') {
        return;
      }

      // ? to show shortcuts help
      if (e.key === '?' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault();
        showHelp = !showHelp;
        return;
      }

      // Escape to close help
      if (e.key === 'Escape' && showHelp) {
        showHelp = false;
        return;
      }

      // Execute shortcut
      for (const shortcut of shortcuts) {
        const ctrlMatch = shortcut.ctrl ? (e.ctrlKey || e.metaKey) : true;
        const shiftMatch = shortcut.shift ? e.shiftKey : true;
        const altMatch = shortcut.alt ? e.altKey : true;
        const keyMatch = e.key.toLowerCase() === shortcut.key.toLowerCase();

        if (ctrlMatch && shiftMatch && altMatch && keyMatch) {
          e.preventDefault();
          shortcut.action();
          return;
        }
      }
    }

    document.addEventListener('keydown', handleKeydown);
    return () => document.removeEventListener('keydown', handleKeydown);
  });

  function formatShortcut(shortcut: any): string {
    const parts = [];
    if (shortcut.ctrl) parts.push('Ctrl');
    if (shortcut.shift) parts.push('Shift');
    if (shortcut.alt) parts.push('Alt');
    parts.push(shortcut.key.toUpperCase());
    return parts.join(' + ');
  }
</script>

{#if showHelp}
  <div class="shortcuts-overlay" onclick={() => showHelp = false}>
    <div class="shortcuts-modal" onclick={(e) => e.stopPropagation()}>
      <h2>Keyboard Shortcuts</h2>
      <div class="shortcuts-list">
        {#each shortcuts as shortcut}
          <div class="shortcut-item">
            <span class="shortcut-desc">{shortcut.description}</span>
            <kbd class="shortcut-key">{formatShortcut(shortcut)}</kbd>
          </div>
        {/each}
      </div>
      <div class="shortcuts-footer">
        Press <kbd>?</kbd> to toggle this help
      </div>
    </div>
  </div>
{/if}

<style>
  .shortcuts-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  .shortcuts-modal {
    background: white;
    border-radius: 12px;
    padding: 24px;
    width: 90%;
    max-width: 400px;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  }
  .shortcuts-modal h2 {
    font-size: 1.25rem;
    margin-bottom: 1rem;
    color: #1f2937;
  }
  .shortcuts-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .shortcut-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 0;
    border-bottom: 1px solid #f3f4f6;
  }
  .shortcut-desc {
    font-size: 0.875rem;
    color: #374151;
  }
  .shortcut-key {
    padding: 4px 8px;
    background: #f3f4f6;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 0.75rem;
    font-family: monospace;
    color: #374151;
  }
  .shortcuts-footer {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid #e5e7eb;
    font-size: 0.75rem;
    color: #6b7280;
    text-align: center;
  }
  .shortcuts-footer kbd {
    padding: 2px 6px;
    background: #f3f4f6;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 0.75rem;
  }
</style>
