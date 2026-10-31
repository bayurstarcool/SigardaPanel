<script lang="ts">
  // Flat SVG ring gauge. Colors come from theme tokens via `currentColor`
  // (text-* utilities), so it follows light/dark without any JS.
  let {
    value = 0,
    max = 100,
    label = '',
    tone = 'accent',
    displayUnit = '',
    thresholds = true,
    detail = '',
    sub = '',
  }: {
    value?: number;
    max?: number;
    label?: string;
    /** Base tone: accent | info | success | warning | danger */
    tone?: string;
    /** 'bytes' shows "used / max" in bytes under the label */
    displayUnit?: string;
    /** Switch to warning ≥70% and danger ≥90% (off for relative gauges like network) */
    thresholds?: boolean;
    /** Extra line under the label (overrides the bytes line) */
    detail?: string;
    /** Second, quieter line */
    sub?: string;
  } = $props();

  const radius = 34;
  const circumference = 2 * Math.PI * radius;
  const pct = $derived(max > 0 ? Math.max(0, Math.min((value / max) * 100, 100)) : 0);
  const offset = $derived(circumference - (pct / 100) * circumference);

  const toneCls: Record<string, string> = {
    accent: 'text-accent', info: 'text-info', success: 'text-success', warning: 'text-warning', danger: 'text-danger',
  };
  const activeCls = $derived(
    thresholds && pct >= 90 ? 'text-danger' : thresholds && pct >= 70 ? 'text-warning' : (toneCls[tone] ?? 'text-accent'),
  );

  function fmtBytes(b: number): string {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }

  const line = $derived(detail || (displayUnit === 'bytes' && max > 0 ? `${fmtBytes(value)} / ${fmtBytes(max)}` : ''));
</script>

<div class="flex flex-col items-center gap-2">
  <div class="relative size-20" role="img" aria-label="{label} {Math.round(pct)}%">
    <svg class="size-full -rotate-90" viewBox="0 0 80 80" aria-hidden="true">
      <circle cx="40" cy="40" r={radius} fill="none" stroke="currentColor" stroke-width="6" class="text-surface-2" />
      <circle
        cx="40"
        cy="40"
        r={radius}
        fill="none"
        stroke="currentColor"
        stroke-width="6"
        stroke-linecap="round"
        stroke-dasharray={circumference}
        stroke-dashoffset={offset}
        class={activeCls}
      />
    </svg>
    <div class="absolute inset-0 flex items-center justify-center">
      <span class="text-base font-semibold text-fg tabular-nums">{Math.round(pct)}%</span>
    </div>
  </div>
  <div class="text-center">
    <div class="text-xs font-medium text-muted">{label}</div>
    {#if line}<div class="mt-0.5 text-[11px] text-subtle tabular-nums">{line}</div>{/if}
    {#if sub}<div class="text-[11px] text-subtle tabular-nums">{sub}</div>{/if}
  </div>
</div>
