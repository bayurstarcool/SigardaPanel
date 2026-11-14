<script lang="ts">
  // Shortcut routes (/fail2ban, /threats, /terminal): open the page of the
  // server chosen in the header; with "All servers" the user picks one.
  import { auth } from '$lib/auth.svelte';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { serverCtx } from '$lib/serverContext.svelte';
  import DashboardLayout from './DashboardLayout.svelte';
  import PageHeader from './PageHeader.svelte';
  import ServerRequired from './ServerRequired.svelte';

  let { path, title, active }: { path: string; title: string; active: string } = $props();
  let ready = $state(false);

  onMount(async () => {
    if (!auth.user) { window.location.href = '/login'; return; }
    await serverCtx.ensure();
    if (serverCtx.servers.length === 0) { goto('/servers', { replaceState: true }); return; }
    ready = true;
  });

  $effect(() => {
    const id = serverCtx.pageServerId;
    if (ready && id) goto(`/server/${id}/${path}`, { replaceState: true });
  });
</script>

{#if ready && !serverCtx.pageServerId}
  <DashboardLayout {active}>
    <PageHeader {title} />
    <ServerRequired feature={title} />
  </DashboardLayout>
{:else}
  <div class="flex min-h-[50vh] items-center justify-center">
    <p class="text-sm text-subtle">Opening {title.toLowerCase()}…</p>
  </div>
{/if}
