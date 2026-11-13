<script lang="ts">
  import { CheckCircle2, XCircle, Loader2, X } from 'lucide-svelte';
  import type { StackOp } from './stackOps';

  let {
    op,
    title = '',
    ondismiss,
  }: { op: StackOp | null; title?: string; ondismiss?: () => void } = $props();

  let showLog = $state(false);
  let logEl = $state<HTMLPreElement | null>(null);

  // A failure opens the log so the apt/systemd error is visible right away.
  $effect(() => {
    if (op?.done && !op.ok && op.output) showLog = true;
  });
  $effect(() => {
    if (showLog && logEl && op?.output) logEl.scrollTop = logEl.scrollHeight;
  });

  function secs(ms?: number) {
    if (!ms && ms !== 0) return '';
    const s = Math.round(ms / 1000);
    return s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s`;
  }
</script>

{#if op}
  {@const running = !op.done}
  <div
    class="mb-4 rounded-lg border px-4 py-3 text-[13px] {running
      ? 'border-accent/30 bg-accent/5 text-fg'
      : op.ok
        ? 'border-success/30 bg-success/10 text-success'
        : 'border-danger/30 bg-danger/10 text-danger'}"
    role="status"
    aria-live="polite"
    data-testid="stack-op"
    data-status={op.status}
  >
    <div class="flex items-start gap-2">
      {#if running}<Loader2 size={16} class="mt-0.5 shrink-0 animate-spin text-accent" />
      {:else if op.ok}<CheckCircle2 size={16} class="mt-0.5 shrink-0" />
      {:else}<XCircle size={16} class="mt-0.5 shrink-0" />{/if}
      <div class="min-w-0 flex-1">
        {#if title}<div class="font-medium">{title}</div>{/if}
        <div class="break-words" data-testid="stack-op-message">{op.message}</div>
        {#if op.duration_ms !== undefined}
          <div class="mt-0.5 text-xs opacity-70">{running ? 'Running for' : 'Took'} {secs(op.duration_ms)}</div>
        {/if}
      </div>
      {#if op.output}
        <button onclick={() => (showLog = !showLog)} class="btn btn-ghost btn-xs shrink-0">{showLog ? 'Hide' : 'Show'} log</button>
      {/if}
      {#if !running && ondismiss}
        <button onclick={ondismiss} class="btn btn-ghost btn-icon btn-xs shrink-0" aria-label="Dismiss"><X size={12} /></button>
      {/if}
    </div>
    {#if showLog && op.output}
      <pre
        bind:this={logEl}
        class="mt-3 max-h-64 overflow-auto rounded-md border border-line bg-surface-2 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-muted">{op.output}</pre>
    {/if}
  </div>
{/if}
