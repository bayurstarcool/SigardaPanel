<script lang="ts">
  let { tier = 'pro', small = false }: { tier?: string; small?: boolean } = $props();
</script>

<span
  class="inline-flex items-center rounded font-semibold tracking-wide uppercase
    {small ? 'px-1 py-px text-[9px]' : 'px-1.5 py-0.5 text-[10px]'}
    {tier === 'enterprise' ? 'bg-accent/15 text-accent' : 'bg-warning/15 text-warning'}"
>
  {tier === 'enterprise' ? 'Enterprise' : 'Pro'}
</span>
