<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { onMount } from 'svelte';

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    try {
      const d = await apiFetch(auth.token, 'GET', 'servers');
      const servers = d?.servers ?? [];
      if (servers.length > 0) {
        window.location.href = `/server/${servers[0].id}/terminal`;
      } else {
        window.location.href = '/servers';
      }
    } catch {
      window.location.href = '/servers';
    }
  });
</script>

<div class="flex items-center justify-center min-h-[50vh]">
  <p class="text-sm text-slate-500">Redirecting to terminal...</p>
</div>
