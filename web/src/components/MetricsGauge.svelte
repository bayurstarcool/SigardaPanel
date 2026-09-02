<script lang="ts">
  let { value = 0, max = 100, label = '', color = '#38bdf8', unit = '%', displayUnit = '', icon = '' }: {
    value?: number;
    max?: number;
    label?: string;
    color?: string;
    unit?: string;
    displayUnit?: string;
    icon?: string;
  } = $props();
  
  const pct = $derived(max > 0 ? Math.min((value / max) * 100, 100) : 0);
  const radius = 36;
  const circumference = 2 * Math.PI * radius;
  const offset = $derived(circumference - (pct / 100) * circumference);
  
  function getColor(p: number): string {
    if (p >= 90) return '#ef4444';
    if (p >= 70) return '#f59e0b';
    return color;
  }
  
  const activeColor = $derived(getColor(pct));
  
  function fmtBytes(b: number): string {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }
  
  const displayValue = $derived(displayUnit === 'bytes' ? fmtBytes(value) : `${Math.round(pct)}%`);
  const displayMax = $derived(displayUnit === 'bytes' ? fmtBytes(max) : `${max > 0 ? Math.round(pct) : 0}%`);
</script>

<div class="flex flex-col items-center gap-2">
  <div class="relative w-24 h-24">
    <svg class="w-full h-full -rotate-90" viewBox="0 0 80 80">
      <circle cx="40" cy="40" r={radius} fill="none" stroke="currentColor" stroke-width="6"
        class="text-black/[0.04] dark:text-white/[0.05]" />
      <circle cx="40" cy="40" r={radius} fill="none" stroke={activeColor}
        stroke-width="6" stroke-linecap="round"
        stroke-dasharray={circumference} stroke-dashoffset={offset}
        class="transition-all duration-700 ease-out" />
    </svg>
    <div class="absolute inset-0 flex flex-col items-center justify-center">
      <span class="text-lg font-bold tabular-nums" style="color: {activeColor}">
        {Math.round(pct)}%
      </span>
    </div>
  </div>
  <div class="text-center">
    <div class="text-xs font-medium text-[var(--text-secondary)]">{label}</div>
    {#if displayUnit === 'bytes' && max > 0}
      <div class="text-[0.65rem] text-[var(--text-muted)] mt-0.5">
        {fmtBytes(value)} / {fmtBytes(max)}
      </div>
    {/if}
  </div>
</div>
