<script lang="ts">
  let { status, label = '' }: { status: string; label?: string } = $props();

  const tone: Record<string, string> = {
    active: 'badge-success', online: 'badge-success', succeeded: 'badge-success', success: 'badge-success',
    imported: 'badge-success', valid: 'badge-success', enabled: 'badge-success', healthy: 'badge-success',
    completed: 'badge-success',
    pending: 'badge-warning', warning: 'badge-warning', expiring_soon: 'badge-warning', degraded: 'badge-warning',
    deploying: 'badge-info', running: 'badge-info', in_progress: 'badge-info',
    queued: 'badge-accent',
    disabled: 'badge-danger', failed: 'badge-danger', offline: 'badge-danger', expired: 'badge-danger',
    error: 'badge-danger',
  };

  const key = $derived((status ?? '').toLowerCase());
  const cls = $derived(tone[key] ?? '');
  const text = $derived(label || key.replace(/_/g, ' '));
</script>

<span class="badge capitalize {cls}">
  <span class="dot {key === 'running' || key === 'deploying' ? 'animate-pulse' : ''}"></span>
  {text}
</span>
