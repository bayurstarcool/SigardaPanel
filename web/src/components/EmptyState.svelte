<script lang="ts">
  import type { Component, Snippet } from 'svelte';

  let {
    icon = null,
    title = '',
    description = '',
    actionLabel = '',
    onaction,
    compact = false,
    children,
  }: {
    icon?: Component<any> | null;
    title?: string;
    description?: string;
    actionLabel?: string;
    onaction?: () => void;
    compact?: boolean;
    children?: Snippet;
  } = $props();
</script>

<div class="flex flex-col items-center justify-center px-4 text-center {compact ? 'py-10' : 'py-16'}">
  {#if icon}
    {@const Icon = icon}
    <div class="mb-4 flex size-11 items-center justify-center rounded-xl border border-line bg-surface-2 text-subtle">
      <Icon size={20} strokeWidth={1.75} />
    </div>
  {/if}
  {#if title}
    <h3 class="text-sm font-semibold text-fg">{title}</h3>
  {/if}
  {#if description}
    <p class="mt-1 max-w-sm text-[13px] text-muted">{description}</p>
  {/if}
  {#if children}
    <div class="mt-5">{@render children()}</div>
  {/if}
  {#if actionLabel && onaction}
    <button onclick={onaction} class="btn btn-primary btn-sm mt-5">{actionLabel}</button>
  {/if}
</div>
