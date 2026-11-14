<script lang="ts">
  // Pages addressed by server id: that server becomes the header's choice,
  // and switching servers remounts the page for the new id.
  import { page } from '$app/state';
  import { untrack, type Snippet } from 'svelte';
  import { serverCtx } from '$lib/serverContext.svelte';

  let { children }: { children: Snippet } = $props();
  $effect(() => {
    const id = Number(page.params.id);
    untrack(() => serverCtx.adopt(id));
  });
</script>

{#key page.params.id}{@render children()}{/key}
