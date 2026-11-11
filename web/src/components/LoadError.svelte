<script lang="ts">
  import EmptyState from './EmptyState.svelte';
  import { AlertTriangle } from 'lucide-svelte';

  // Shown in place of a list/detail when its initial load failed, so an API
  // error never masquerades as "nothing here yet".
  let {
    message,
    title = 'Could not load data',
    onretry,
    compact = true,
  }: { message: string; title?: string; onretry?: () => void; compact?: boolean } = $props();
</script>

<div role="alert">
  <EmptyState {compact} icon={AlertTriangle} {title} description={message} actionLabel={onretry ? 'Try again' : ''} onaction={onretry} />
</div>
