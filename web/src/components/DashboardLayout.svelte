<script lang="ts">
  import type { Snippet } from 'svelte';
  import Sidebar from './Sidebar.svelte';
  import DashboardHeader from './DashboardHeader.svelte';
  import Toast from './Toast.svelte';
  import GlobalSearch from './GlobalSearch.svelte';

  let { active = '', wide = false, children }: { active?: string; wide?: boolean; children: Snippet } = $props();
  let mobileOpen = $state(false);
  let searchOpen = $state(false);

  function onKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      searchOpen = !searchOpen;
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="min-h-dvh bg-bg">
  <Sidebar {active} {mobileOpen} onCloseMobile={() => (mobileOpen = false)} />
  <DashboardHeader {mobileOpen} onToggleMobile={() => (mobileOpen = !mobileOpen)} onSearchOpen={() => (searchOpen = true)} />

  <main class="min-w-0 lg:pl-60">
    <div class="mx-auto w-full px-4 py-6 sm:px-6 lg:px-8 lg:py-8 {wide ? 'max-w-[1600px]' : 'max-w-7xl'}">
      {@render children()}
    </div>
  </main>

  <Toast />
  <GlobalSearch open={searchOpen} onClose={() => (searchOpen = false)} />
</div>
