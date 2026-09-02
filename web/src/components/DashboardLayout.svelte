<script lang="ts">
  import Sidebar from './Sidebar.svelte';
  import DashboardHeader from './DashboardHeader.svelte';
  import DashboardFooter from './DashboardFooter.svelte';
  import Toast from './Toast.svelte';
  import GlobalSearch from './GlobalSearch.svelte';

  let { active, children }: { active: string; children: any } = $props();
  let mobileOpen = $state(false);
  let searchOpen = $state(false);
</script>

<svelte:head>
  <style>
    @keyframes mesh-float { 0%, 100% { transform: translate(0, 0) scale(1); } 33% { transform: translate(30px, -30px) scale(1.05); } 66% { transform: translate(-20px, 20px) scale(0.95); } }
    .mesh-orb { animation: mesh-float 20s ease-in-out infinite; will-change: transform; }
  </style>
</svelte:head>

<!-- svelte-ignore event_directive_deprecated -->
<svelte:window on:keydown={(e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    searchOpen = !searchOpen;
  }
}} />

<div class="relative min-h-screen bg-[var(--bg-primary)] transition-colors duration-300">
  <!-- Ambient background -->
  <div class="fixed inset-0 pointer-events-none overflow-hidden">
    <div class="mesh-orb absolute top-0 right-0 w-[800px] h-[800px] rounded-full bg-gradient-to-br from-sky-500/[0.03] to-transparent dark:from-sky-500/[0.04] light:from-sky-200/[0.08]"></div>
    <div class="mesh-orb absolute bottom-0 left-0 w-[600px] h-[600px] rounded-full bg-gradient-to-br from-indigo-500/[0.03] to-transparent dark:from-indigo-500/[0.04] light:from-indigo-200/[0.08]" style="animation-delay: -7s;"></div>
    <div class="absolute inset-0 opacity-[0.02] dark:opacity-[0.02] light:opacity-[0.04]" style="background-image: radial-gradient(rgba(128,128,128,0.2) 1px, transparent 1px); background-size: 32px 32px;"></div>
  </div>

  <Sidebar {active} {mobileOpen} onCloseMobile={() => mobileOpen = false} />
  <DashboardHeader {mobileOpen} onToggleMobile={() => mobileOpen = !mobileOpen} onSearchOpen={() => searchOpen = true} />

  <main class="relative min-h-screen min-w-0 lg:ml-72 p-3 sm:p-4 lg:p-8 xl:p-10 pt-16 lg:pt-8 pb-24">
    <div class="mx-auto max-w-7xl w-full">
      {@render children()}
    </div>
  </main>

  <DashboardFooter />
  <Toast />
  <GlobalSearch open={searchOpen} onClose={() => searchOpen = false} />
</div>
