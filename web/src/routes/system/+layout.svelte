<script lang="ts">
  // A single-server page: switching servers remounts it, so nothing of the
  // previous server (data, pending requests) survives the switch.
  import type { Snippet } from 'svelte';
  import { serverCtx } from '$lib/serverContext.svelte';

  let { children }: { children: Snippet } = $props();
</script>

{#key serverCtx.pageServerId}{@render children()}{/key}
