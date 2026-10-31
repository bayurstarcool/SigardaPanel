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
  <div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
    <div class="mb-5 flex size-11 items-center justify-center rounded-xl bg-danger/10 text-danger">
      <AlertTriangle size={20} />
    </div>
    <p class="font-mono text-sm text-subtle tabular-nums">{$page.status}</p>
    <h1 class="mt-1 text-xl font-semibold tracking-tight text-fg">
      {statusMessages[$page.status] || 'Something went wrong'}
    </h1>
    <p class="mt-2 max-w-md text-sm text-muted">
      {$page.error?.message || 'An unexpected error occurred. Please try again.'}
    </p>
    <div class="mt-6 flex flex-wrap items-center justify-center gap-2">
      <button onclick={() => history.back()} class="btn btn-secondary btn-sm">
        <ArrowLeft size={14} /> Go back
      </button>
      <a href="/dashboard" class="btn btn-primary btn-sm">
        <Home size={14} /> Dashboard
      </a>
    </div>
  </div>
</DashboardLayout>
