<script lang="ts">
  import type { Component, Snippet } from 'svelte';

  let {
    icon = null,
    title = '',
    description = '',
    actionLabel = '',
    onaction,
    children,
  }: {
    icon?: Component | null;
    title?: string;
    description?: string;
    actionLabel?: string;
    onaction?: () => void;
    children?: Snippet;
  } = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 px-4 text-center">
  {#if icon}
    <div class="mb-4 p-4 rounded-2xl bg-gray-800/50 border border-gray-700/30">
      <svelte:component this={icon} size={40} class="text-gray-500" />
    </div>
  {/if}

  {#if title}
    <h3 class="text-lg font-semibold text-white mb-2">{title}</h3>
  {/if}

  {#if description}
    <p class="text-sm text-gray-400 max-w-sm mb-6">{description}</p>
  {/if}

  {#if children}
    <div class="mb-6">
      {@render children()}
    </div>
  {/if}

  {#if actionLabel && onaction}
    <button
      onclick={onaction}
      class="px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium transition cursor-pointer border-0"
    >
      {actionLabel}
    </button>
  {/if}
</div>
