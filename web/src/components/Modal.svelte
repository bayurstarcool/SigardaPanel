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
    maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '4xl';
    onclose?: () => void;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  function close() {
    open = false;
    onclose?.();
  }

  function onKeydown(e: KeyboardEvent) {
    // Only the open modal reacts; otherwise every mounted (closed) modal would
    // fire its onclose on any Escape press.
    if (open && e.key === 'Escape') close();
  }

  const maxWidths: Record<string, string> = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-2xl',
    '4xl': 'max-w-4xl',
  };
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
  <!-- Backdrop click closes; Escape is handled on window above. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="overlay animate-fade-in flex items-end justify-center p-0 sm:items-center sm:p-4"
    onclick={(e) => { if (e.target === e.currentTarget) close(); }}
    role="dialog"
    aria-modal="true"
    aria-label={title}
    tabindex="-1"
  >
    <div
      class="popover animate-pop-in flex max-h-[90dvh] w-full flex-col {maxWidths[maxWidth]} max-sm:rounded-b-none"
      role="document"
    >
      {#if title}
        <div class="flex shrink-0 items-center justify-between gap-3 border-b border-line px-5 py-3.5">
          <h2 class="text-[15px] font-semibold text-fg">{title}</h2>
          <button onclick={close} class="btn btn-ghost btn-icon btn-xs" aria-label="Close">
            <X size={16} />
          </button>
        </div>
      {/if}

      {#if children}
        <div class="overflow-y-auto px-5 py-4">
          {@render children()}
        </div>
      {/if}

      {#if footer}
        <div class="flex shrink-0 items-center justify-end gap-2 border-t border-line px-5 py-3">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
