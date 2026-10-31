<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { theme } from '$lib/theme.svelte';
  import { licenseStore } from '$lib/license';
  import { onMount } from 'svelte';
  import Logo from '$components/Logo.svelte';
  import '@fontsource-variable/inter/wght.css';
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
        // auth.token is only non-null during impersonation; the session
        // signal for normal cookie sessions is auth.user.
        if (auth.user) {
          licenseStore.fetch();
        }
        ready = true;
      });
  });

  // Reactive guard: redirect ke /login kalau session expired di tengah jalan
  // Ini penting — api.ts clearSession() via handler, bukan redirect langsung
  $effect(() => {
    if (ready && !auth.user && window.location.pathname !== '/login' && window.location.pathname !== '/register' && window.location.pathname !== '/pricing' && !window.location.pathname.startsWith('/member') && !window.location.pathname.startsWith('/docs') && window.location.pathname !== '/') {
      window.location.href = '/login';
    }
  });
</script>

{#if !ready}
  <div class="flex min-h-dvh flex-col items-center justify-center gap-4 bg-bg">
    <Logo size={40} />
    <div class="h-0.5 w-24 overflow-hidden rounded-full bg-surface-2">
      <div class="h-full w-1/3 animate-[splash_1s_ease-in-out_infinite] rounded-full bg-accent"></div>
    </div>
  </div>
  <style>
    @keyframes splash {
      from { transform: translateX(-100%); }
      to { transform: translateX(300%); }
    }
  </style>
{:else}
  {@render children()}
{/if}
