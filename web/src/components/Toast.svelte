<script lang="ts">
  import { CircleCheck, CircleX, TriangleAlert, Info, X } from 'lucide-svelte';
  import type { ToastItem } from '$lib/toast.svelte';
  import { onMount } from 'svelte';

  let items = $state<ToastItem[]>([]);

  onMount(() => {
    function onToast(e: Event) {
      const t = (e as CustomEvent<ToastItem>).detail;
      items = [...items.slice(-4), t];
      if (t.duration > 0) {
        setTimeout(() => dismiss(t.id), t.duration);
      }
    }
    window.addEventListener('toast', onToast);
    return () => window.removeEventListener('toast', onToast);
  });

  function dismiss(id: number) {
    items = items.filter((t) => t.id !== id);
  }

  const icons: Record<string, any> = { success: CircleCheck, error: CircleX, warning: TriangleAlert, info: Info };
  const tones: Record<string, string> = { success: 'text-success', error: 'text-danger', warning: 'text-warning', info: 'text-info' };
</script>

<div class="pointer-events-none fixed right-4 bottom-4 z-[60] flex w-full max-w-sm flex-col gap-2 max-sm:left-4 max-sm:w-auto" aria-live="polite">
  {#each items as t (t.id)}
    {@const Icon = icons[t.type] ?? Info}
    <div class="popover animate-pop-in pointer-events-auto flex items-start gap-3 px-3.5 py-3">
      <Icon size={16} class="mt-px shrink-0 {tones[t.type] ?? 'text-info'}" />
      <p class="flex-1 text-[13px] leading-snug text-fg">{t.message}</p>
      <button onclick={() => dismiss(t.id)} class="shrink-0 cursor-pointer text-subtle transition-colors hover:text-fg" aria-label="Dismiss">
        <X size={14} />
      </button>
    </div>
  {/each}
</div>
