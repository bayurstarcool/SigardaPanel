<script lang="ts">
  import { Check, X, AlertTriangle, Info } from 'lucide-svelte';
  import type { ToastItem } from '$lib/toast.svelte';
  import { onMount } from 'svelte';

  let items = $state<ToastItem[]>([]);

  onMount(() => {
    function onToast(e: Event) {
      const t = (e as CustomEvent<ToastItem>).detail;
      items = [...items, t];
      if (t.duration > 0) {
        setTimeout(() => { items = items.filter(x => x.id !== t.id); }, t.duration);
      }
    }
    window.addEventListener('toast', onToast);
    return () => window.removeEventListener('toast', onToast);
  });

  function dismiss(id: number) {
    items = items.filter(t => t.id !== id);
  }

  function iconFor(type: string) {
    switch (type) {
      case 'success': return Check;
      case 'error': return X;
      case 'warning': return AlertTriangle;
      default: return Info;
    }
  }

  function colorFor(type: string) {
    switch (type) {
      case 'success': return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400';
      case 'error': return 'border-red-500/30 bg-red-500/10 text-red-400';
      case 'warning': return 'border-amber-500/30 bg-amber-500/10 text-amber-400';
      default: return 'border-sky-500/30 bg-sky-500/10 text-sky-400';
    }
  }
</script>

<div class="fixed bottom-6 right-6 z-[9999] flex flex-col gap-2 max-w-sm w-full pointer-events-none">
  {#each items as t (t.id)}
    {@const Icon = iconFor(t.type)}
    {@const colors = colorFor(t.type)}
    <div
      class="pointer-events-auto flex items-start gap-3 px-4 py-3 rounded-xl border backdrop-blur-xl shadow-2xl shadow-black/30 animate-slide-up transition-all duration-300 {colors}"
    >
      <Icon size={16} class="mt-0.5 shrink-0" />
      <p class="text-sm font-medium flex-1 leading-snug">{t.message}</p>
      <button
        onclick={() => dismiss(t.id)}
        class="shrink-0 mt-0.5 opacity-50 hover:opacity-100 transition cursor-pointer bg-transparent border-0 text-inherit"
      >
        <X size={14} />
      </button>
    </div>
  {/each}
</div>

<style>
  @keyframes slide-up {
    from { opacity: 0; transform: translateY(12px) scale(0.96); }
    to { opacity: 1; transform: translateY(0) scale(1); }
  }
  :global(.animate-slide-up) {
    animation: slide-up 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }
</style>
