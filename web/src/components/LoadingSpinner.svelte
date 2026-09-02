<script lang="ts">
  let {
    size = 'md',
    message = '',
  }: {
    size?: 'sm' | 'md' | 'lg';
    message?: string;
  } = $props();

  const sizes: Record<string, string> = {
    sm: 'w-5 h-5 border-2',
    md: 'w-8 h-8 border-[3px]',
    lg: 'w-12 h-12 border-4',
  };
</script>

<div class="flex flex-col items-center justify-center gap-3 py-4">
  <div
    class="rounded-full border-gray-600 border-t-indigo-500 animate-spin {sizes[size]}"
    role="status"
    aria-label="Loading"
  ></div>
  {#if message}
    <p class="text-sm text-gray-400">{message}</p>
  {/if}
</div>
