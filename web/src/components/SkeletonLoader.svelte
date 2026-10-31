<script lang="ts">
  // Fixed widths (not random) so re-renders don't reflow the placeholder.
  let { lines = 3, height = '12px' }: { lines?: number; height?: string } = $props();
  const widths = ['100%', '86%', '72%', '92%', '64%'];
</script>

<div class="space-y-2.5" aria-hidden="true">
  {#each Array(lines) as _, i (i)}
    <div class="skeleton" style="height: {height}; width: {widths[i % widths.length]}"></div>
  {/each}
</div>
