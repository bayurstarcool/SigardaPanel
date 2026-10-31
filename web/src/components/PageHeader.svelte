<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronLeft } from 'lucide-svelte';

  let {
    title,
    description = '',
    back = '',
    backLabel = 'Back',
    meta,
    actions,
  }: {
    title: string;
    description?: string;
    back?: string;
    backLabel?: string;
    meta?: Snippet;
    actions?: Snippet;
  } = $props();
</script>

<div class="mb-6">
  {#if back}
    <a href={back} class="mb-3 inline-flex items-center gap-1 text-[13px] text-subtle transition-colors hover:text-fg">
      <ChevronLeft size={14} /> {backLabel}
    </a>
  {/if}
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div class="min-w-0">
      <div class="flex flex-wrap items-center gap-2.5">
        <h1 class="truncate text-xl font-semibold tracking-tight text-fg sm:text-2xl">{title}</h1>
        {#if meta}{@render meta()}{/if}
      </div>
      {#if description}
        <p class="mt-1 text-sm text-muted">{description}</p>
      {/if}
    </div>
    {#if actions}
      <div class="flex shrink-0 flex-wrap items-center gap-2">{@render actions()}</div>
    {/if}
  </div>
</div>
