<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { theme } from '$lib/theme.svelte';
  import { licenseStore } from '$lib/license';
  import { onMount } from 'svelte';
  import '../app.css';

  let { children } = $props();
  let ready = $state(false);

  onMount(() => {
    theme.init();
    auth.init()
      .catch((error) => {
        console.error('[AUTH] initialization failed:', error);
      })
      .finally(() => {
        if (auth.token) {
          licenseStore.fetch();
        }
        ready = true;
      });
  });

  // Reactive guard: redirect ke /login kalau session expired di tengah jalan
  // Ini penting — api.ts clearSession() via handler, bukan redirect langsung
  $effect(() => {
    if (ready && !auth.token && window.location.pathname !== '/login' && window.location.pathname !== '/register' && window.location.pathname !== '/pricing' && !window.location.pathname.startsWith('/member') && !window.location.pathname.startsWith('/docs') && window.location.pathname !== '/') {
      window.location.href = '/login';
    }
  });
</script>

<svelte:head>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet" />
  <style>
    :root { font-family: 'Inter', system-ui, -apple-system, sans-serif; }
  </style>
</svelte:head>

{#if !ready}
  <div class="min-h-screen bg-[var(--bg-primary)] flex flex-col items-center justify-center gap-4">
    <div class="relative">
      <div class="absolute inset-0 rounded-2xl bg-gradient-to-r from-sky-400/20 to-indigo-400/20 blur-xl animate-pulse"></div>
      <div class="relative w-14 h-14 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_8px_32px_rgba(56,189,248,0.3)]">
        <svg class="animate-spin h-6 w-6 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      </div>
    </div>
    <div class="text-lg font-semibold tracking-tight bg-gradient-to-r from-sky-300 to-indigo-300 bg-clip-text text-transparent">SigardaPanel</div>
  </div>
{:else}
  {@render children()}
{/if}
