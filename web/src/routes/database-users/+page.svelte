<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  // Halaman ini digabung ke /databases (section "Database Users").
  onMount(() => { goto('/databases', { replaceState: true }); });
</script>

<div class="flex min-h-dvh items-center justify-center bg-bg text-sm text-subtle">
  Redirecting to <a href="/databases" class="link ml-1">Databases</a>…
</div>
