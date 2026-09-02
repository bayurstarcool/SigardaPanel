<script lang="ts">
  let { tier = 'pro', small = false }: { tier?: string; small?: boolean } = $props();
</script>

<span class="badge {tier}" class:small>
  {#if tier === 'enterprise'}
    ⚡ Enterprise
  {:else}
    ⭐ Pro
  {/if}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.02em;
  }
  .badge.small {
    padding: 1px 6px;
    font-size: 10px;
  }
  .pro {
    background: linear-gradient(135deg, #f59e0b, #f97316);
    color: #000;
  }
  .enterprise {
    background: linear-gradient(135deg, #8b5cf6, #6366f1);
    color: #fff;
  }
</style>
