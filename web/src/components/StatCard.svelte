<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    label,
    value,
    hint = '',
    icon = null,
    tone = '',
    href = '',
    children,
  }: {
    label: string;
    value: string | number;
    hint?: string;
    icon?: any;
    /** '', 'success', 'warning', 'danger', 'info', 'accent' — colors the icon only */
    tone?: string;
    href?: string;
    children?: Snippet;
  } = $props();

  const toneCls: Record<string, string> = {
    success: 'text-success', warning: 'text-warning', danger: 'text-danger', info: 'text-info', accent: 'text-accent',
  };
</script>

<svelte:element
  this={href ? 'a' : 'div'}
  {href}
  class="card block p-5 {href ? 'transition-colors hover:border-line-strong' : ''}"
>
  <div class="flex items-center justify-between gap-2">
    <span class="stat-label">{label}</span>
    {#if icon}
      {@const Icon = icon}
      <span class="icon-tile size-7"><Icon size={15} strokeWidth={1.75} class={toneCls[tone] ?? 'text-muted'} /></span>
    {/if}
  </div>
  <!-- Words ("Configured", "php") read better smaller than figures. -->
  <div class="stat-value {typeof value === 'string' && !/^[\d.,%\s–—-]*$/.test(value) ? 'text-[20px]' : ''}">{value}</div>
  {#if hint}<div class="mt-2 text-xs text-subtle">{hint}</div>{/if}
  {#if children}{@render children()}{/if}
</svelte:element>
