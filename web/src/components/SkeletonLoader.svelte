<script lang="ts">
  let { lines = 3, width = '100%', height = '12px' } = $props();
</script>

<div class="skeleton" style="width: {width}">
  {#each Array(lines) as _, i}
    <div 
      class="skeleton-line" 
      style="height: {height}; width: {Math.random() * 40 + 60}%"
    ></div>
  {/each}
</div>

<style>
  .skeleton {
    animation: pulse 1.5s ease-in-out infinite;
  }
  .skeleton-line {
    background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
    background-size: 200% 100%;
    border-radius: 4px;
    margin-bottom: 8px;
    animation: shimmer 1.5s infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  @keyframes shimmer {
    0% { background-position: -200% 0; }
    100% { background-position: 200% 0; }
  }
</style>
