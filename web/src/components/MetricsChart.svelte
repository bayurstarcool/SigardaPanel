<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Chart, registerables } from 'chart.js';
  
  Chart.register(...registerables);
  
  let { labels = [], datasets = [], title = '', yLabel = '%', height = '200px' }: {
    labels?: string[];
    datasets?: { label: string; data: number[]; color: string; fillColor?: string }[];
    title?: string;
    yLabel?: string;
    height?: string;
  } = $props();
  
  let canvas: HTMLCanvasElement;
  let chart: Chart | null = null;
  
  function createChart() {
    if (!canvas) return;
    if (chart) chart.destroy();
    
    chart = new Chart(canvas, {
      type: 'line',
      data: {
        labels,
        datasets: datasets.map(ds => ({
          label: ds.label,
          data: ds.data,
          borderColor: ds.color,
          backgroundColor: ds.fillColor || ds.color + '15',
          fill: true,
          tension: 0.4,
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
          pointHoverBackgroundColor: ds.color,
        })),
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { intersect: false, mode: 'index' as const },
        plugins: {
          legend: { display: datasets.length > 1, position: 'top' as const, labels: { boxWidth: 12, padding: 16, usePointStyle: true, font: { size: 11 } } },
          title: { display: !!title, text: title, font: { size: 13, weight: 600 }, padding: { bottom: 12 } },
          tooltip: {
            backgroundColor: 'rgba(15,23,42,0.9)',
            titleFont: { size: 11 },
            bodyFont: { size: 11 },
            padding: 10,
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
            ticks: { font: { size: 10 }, maxRotation: 0, maxTicksLimit: 8, color: '#94a3b8' },
            border: { display: false },
          },
          y: {
            beginAtZero: true,
            grid: { color: 'rgba(0,0,0,0.04)' },
            ticks: { font: { size: 10 }, color: '#94a3b8', padding: 8 },
            border: { display: false },
            title: { display: !!yLabel, text: yLabel, font: { size: 10 }, color: '#94a3b8' },
          },
        },
      },
    });
  }
  
  $effect(() => {
    // React to data changes
    labels; datasets;
    createChart();
  });
  
  onMount(() => { createChart(); });
  onDestroy(() => { chart?.destroy(); });
</script>

<div style="height: {height}" class="relative">
  <canvas bind:this={canvas}></canvas>
</div>
