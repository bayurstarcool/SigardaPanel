<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  // Halaman ini digabung ke /databases (section "Database Users").
  onMount(() => { goto('/databases', { replaceState: true }); });
</script>

<div class="min-h-screen flex items-center justify-center bg-[#040a14] text-slate-500 text-sm">
  Redirecting to Databases…
</div>
