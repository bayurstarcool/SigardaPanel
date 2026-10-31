<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  // Shortcut route: jump to the first server's terminal page.
  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    let target = '/servers';
    try {
      const d = await apiFetch(auth.token, 'GET', 'servers');
      const servers = d?.servers ?? [];
      if (servers.length > 0) target = `/server/${servers[0].id}/terminal`;
    } catch {
      // fall through to the server list
    }
    goto(target, { replaceState: true });
  });
</script>

<div class="flex min-h-[50vh] items-center justify-center">
  <p class="text-sm text-subtle">Redirecting to terminal…</p>
</div>
