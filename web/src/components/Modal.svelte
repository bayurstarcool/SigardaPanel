<script lang="ts">
  import { X } from 'lucide-svelte';
  import type { Snippet } from 'svelte';

  let {
    open = $bindable(false),
    title = '',
    maxWidth = 'md',
    onclose,
    children,
    footer,
  }: {
    open: boolean;
    title?: string;
    maxWidth?: 'sm' | 'md' | 'lg' | 'xl';
    onclose?: () => void;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  function close() {
    open = false;
    onclose?.();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }

  const maxWidths: Record<string, string> = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
  };
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
    onclick={close}
    role="dialog"
    aria-modal="true"
    aria-label={title}
  >
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="w-full {maxWidths[maxWidth]} bg-gray-900 border border-gray-700/50 rounded-2xl shadow-2xl shadow-black/40"
      onclick={(e) => e.stopPropagation()}
      role="document"
    >
      {#if title}
        <div class="flex items-center justify-between px-6 py-4 border-b border-gray-700/50">
          <h2 class="text-lg font-semibold text-white">{title}</h2>
          <button
            onclick={close}
            class="p-1 rounded-lg text-gray-400 hover:text-white hover:bg-gray-700/50 transition cursor-pointer bg-transparent border-0"
            aria-label="Close"
          >
            <X size={18} />
          </button>
        </div>
      {/if}

      {#if children}
        <div class="px-6 py-4">
          {@render children()}
        </div>
      {/if}

      {#if footer}
        <div class="px-6 py-4 border-t border-gray-700/50 flex items-center justify-end gap-3">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
