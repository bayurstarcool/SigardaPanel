<script lang="ts">
  import { page } from '$app/stores';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { AlertTriangle, Home, ArrowLeft } from 'lucide-svelte';

  const statusMessages: Record<number, string> = {
    404: 'Page not found',
    403: 'Access denied',
    401: 'Authentication required',
    500: 'Internal server error',
    502: 'Bad gateway',
    503: 'Service unavailable',
  };
</script>

<DashboardLayout active="">
  <div class="flex flex-col items-center justify-center min-h-[60vh] text-center px-4">
    <div class="w-16 h-16 rounded-2xl bg-red-500/10 border border-red-500/20 flex items-center justify-center mb-6">
      <AlertTriangle size={28} class="text-red-400" />
    </div>

    <h1 class="text-5xl font-bold text-white mb-2">{$page.status}</h1>
    <p class="text-lg text-slate-300 mb-1">
      {statusMessages[$page.status] || 'Something went wrong'}
    </p>
    {#if $page.error?.message}
      <p class="text-sm text-slate-500 mb-8 max-w-md">{$page.error.message}</p>
    {:else}
      <p class="text-sm text-slate-500 mb-8 max-w-md">
        An unexpected error occurred. Please try again.
      </p>
    {/if}

    <div class="flex items-center gap-3">
      <button
        onclick={() => history.back()}
        class="inline-flex items-center gap-2 px-4 py-2.5 rounded-xl text-sm font-medium text-slate-300 bg-white/[0.05] border border-white/[0.08] hover:bg-white/[0.08] transition"
      >
        <ArrowLeft size={16} />
        Go Back
      </button>
      <a
        href="/"
        class="inline-flex items-center gap-2 px-4 py-2.5 rounded-xl text-sm font-medium text-white bg-sky-500/10 border border-sky-500/20 hover:bg-sky-500/20 transition"
      >
        <Home size={16} />
        Dashboard
      </a>
    </div>
  </div>
</DashboardLayout>
