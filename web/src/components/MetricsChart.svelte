<script lang="ts">
  import { onMount, onDestroy, untrack } from 'svelte';
  import { theme } from '$lib/theme.svelte';
  import type { Chart as ChartT } from 'chart.js';

  // chart.js is loaded on mount (dynamic import) so it stays out of the
  // initial page chunk. Colors are read from the theme tokens at runtime and
  // re-applied when the theme changes. Animations are off: updates are a
  // single repaint, nothing keeps the GPU busy.

  type Tone = 'accent' | 'info' | 'success' | 'warning' | 'danger';

  let {
    labels = [],
    datasets = [],
    title = '',
    yLabel = '%',
    height = '200px',
  }: {
    labels?: string[];
    /** Series color comes from a theme token: `tone` (default accent, then info). */
    datasets?: { label: string; data: number[]; tone?: Tone }[];
    title?: string;
    yLabel?: string;
    height?: string;
  } = $props();

  let canvas: HTMLCanvasElement;
  let chart: ChartT | null = null;
  let ready = $state(false);
  let destroyed = false;
  let ChartCtor: typeof ChartT | null = null;

  const defaultTones: Tone[] = ['accent', 'info', 'warning', 'success'];

  function tokens() {
    const css = getComputedStyle(document.documentElement);
    const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback;
    return {
      accent: v('--c-accent', '#6d7cff'),
      info: v('--c-info', '#38bdf8'),
      success: v('--c-success', '#34d399'),
      warning: v('--c-warning', '#fbbf24'),
      danger: v('--c-danger', '#f87171'),
      line: v('--c-line', '#26262a'),
      subtle: v('--c-subtle', '#71717a'),
      muted: v('--c-muted', '#a1a1aa'),
      fg: v('--c-fg', '#f4f4f5'),
      elevated: v('--c-elevated', '#1a1a1d'),
    };
  }

  // #rrggbb -> #rrggbbaa (flat, low-alpha area fill; no gradients).
  function alpha(hex: string, a: number): string {
    if (!/^#[0-9a-f]{6}$/i.test(hex)) return 'transparent';
    return hex + Math.round(a * 255).toString(16).padStart(2, '0');
  }

  function buildDatasets(t: ReturnType<typeof tokens>) {
    return datasets.map((ds, i) => {
      const color = t[ds.tone ?? defaultTones[i % defaultTones.length]];
      return {
        label: ds.label,
        data: ds.data,
        borderColor: color,
        backgroundColor: alpha(color, 0.08),
        fill: datasets.length === 1,
        tension: 0.3,
        borderWidth: 1.5,
        pointRadius: 0,
        pointHoverRadius: 3,
        pointHoverBackgroundColor: color,
      };
    });
  }

  function buildOptions(t: ReturnType<typeof tokens>): any {
    return {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      interaction: { intersect: false, mode: 'index' },
      plugins: {
        legend: {
          display: datasets.length > 1,
          position: 'top',
          align: 'end',
          labels: { boxWidth: 8, boxHeight: 8, padding: 12, usePointStyle: true, color: t.muted, font: { size: 11 } },
        },
        title: { display: false },
        tooltip: {
          backgroundColor: t.elevated,
          borderColor: t.line,
          borderWidth: 1,
          titleColor: t.fg,
          bodyColor: t.muted,
          titleFont: { size: 11 },
          bodyFont: { size: 11 },
          padding: 8,
          cornerRadius: 8,
          displayColors: true,
          boxWidth: 8,
          boxHeight: 8,
          boxPadding: 4,
        },
      },
      scales: {
        x: {
          grid: { display: false },
          ticks: { font: { size: 10 }, maxRotation: 0, maxTicksLimit: 8, color: t.subtle },
          border: { display: false },
        },
        y: {
          beginAtZero: true,
          grid: { color: t.line },
          ticks: { font: { size: 10 }, color: t.subtle, padding: 8 },
          border: { display: false },
          title: { display: !!yLabel, text: yLabel, font: { size: 10 }, color: t.subtle },
        },
      },
    };
  }

  function render() {
    if (!ChartCtor || !canvas || destroyed) return;
    const t = tokens();
    if (!chart) {
      chart = new ChartCtor(canvas, {
        type: 'line',
        data: { labels: [...labels], datasets: buildDatasets(t) as any },
        options: buildOptions(t),
      });
      return;
    }
    // Update in place: no destroy/re-create per poll.
    chart.data.labels = [...labels];
    chart.data.datasets = buildDatasets(t) as any;
    chart.options = buildOptions(t);
    chart.update('none');
  }

  onMount(async () => {
    const m = await import('chart.js');
    if (destroyed) return;
    m.Chart.register(
      m.LineController, m.LineElement, m.PointElement, m.LinearScale, m.CategoryScale,
      m.Filler, m.Tooltip, m.Legend,
    );
    m.Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
    ChartCtor = m.Chart;
    ready = true;
  });

  // Re-render on data, label or theme change (theme.current flips html.dark/.light
  // synchronously before effects run, so the CSS variables are already updated).
  $effect(() => {
    labels; datasets; yLabel; theme.current;
    if (!ready) return;
    untrack(render);
  });

  onDestroy(() => {
    destroyed = true;
    chart?.destroy();
    chart = null;
  });
</script>

<div>
  {#if title}<h3 class="card-title mb-3">{title}</h3>{/if}
  <div style="height: {height}" class="relative">
    {#if !ready}<div class="skeleton absolute inset-0"></div>{/if}
    <canvas bind:this={canvas} aria-label={title || 'Chart'}></canvas>
    {#if ready && labels.length === 0}
      <div class="absolute inset-0 flex items-center justify-center text-xs text-subtle">No data for this range</div>
    {/if}
  </div>
</div>
