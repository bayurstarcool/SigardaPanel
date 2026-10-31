<script lang="ts">
  let { size = 28, withText = false }: { size?: number; withText?: boolean } = $props();
</script>

<span class="inline-flex items-center gap-2.5">
  <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true" class="shrink-0">
    <rect width="32" height="32" rx="8" class="fill-accent" />
    <path d="M10 11.5h12M10 16h8M10 20.5h12" stroke="white" stroke-width="2.4" stroke-linecap="round" />
  </svg>
  {#if withText}
    <span class="text-[15px] font-semibold tracking-tight text-fg">Sigarda<span class="text-subtle">Panel</span></span>
  {/if}
</span>
