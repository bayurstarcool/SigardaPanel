<script lang="ts">
  import { LoaderCircle } from 'lucide-svelte';

  let { size = 'md', message = '' }: { size?: 'sm' | 'md' | 'lg'; message?: string } = $props();
  const px: Record<string, number> = { sm: 16, md: 22, lg: 30 };
</script>

<div class="flex flex-col items-center justify-center gap-3 py-10 text-subtle" role="status" aria-label="Loading">
  <LoaderCircle size={px[size]} class="animate-spin" />
  {#if message}<p class="text-[13px]">{message}</p>{/if}
</div>
