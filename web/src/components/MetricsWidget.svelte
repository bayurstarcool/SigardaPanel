<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';

  let m = $state<any>(null);
  let interval: ReturnType<typeof setInterval>;

  let stopped = false;

  async function fetchMetrics() {
    if (stopped || !auth.user) return;
    try {
      const serversData = await apiFetch(auth.token, 'GET', 'servers').catch((e) => {
        if (e.message === 'session expired') { stopped = true; clearInterval(interval); }
        return { servers: [] };
      });
      if (stopped) return;
      const servers = serversData?.servers || [];
      if (servers.length === 0) return;

      const data = await apiFetch(auth.token, 'GET', `servers/${servers[0].id}/metrics/latest`).catch((e) => {
        if (e.message === 'session expired') { stopped = true; clearInterval(interval); }
        return null;
      });
      if (stopped || !data) return;
      m = {
        cpu: { usagePercent: data.cpu_percent || 0 },
        memory: {
          usagePercent: data.mem_total ? (data.mem_used / data.mem_total * 100) : 0,
          usedBytes: data.mem_used || 0,
          totalBytes: data.mem_total || 0
        },
        disk: {
          usagePercent: data.disk_total ? (data.disk_used / data.disk_total * 100) : 0,
          usedBytes: data.disk_used || 0,
          totalBytes: data.disk_total || 0
        },
        swap: {
          usagePercent: data.swap_total ? (data.swap_used / data.swap_total * 100) : 0,
          usedBytes: data.swap_used || 0,
          totalBytes: data.swap_total || 0
        }
      };
    } catch {}
  }

  onMount(() => {
    fetchMetrics();
    interval = setInterval(fetchMetrics, 10000);
    return () => { stopped = true; clearInterval(interval); };
  });

  function colorClasses(color: string) {
    if (color === 'sky') return { bar: 'bg-sky-400', textDark: 'text-sky-300', textLight: 'text-sky-600', bg: 'bg-sky-500/10' };
    if (color === 'emerald') return { bar: 'bg-emerald-400', textDark: 'text-emerald-300', textLight: 'text-emerald-600', bg: 'bg-emerald-500/10' };
    if (color === 'violet') return { bar: 'bg-violet-400', textDark: 'text-violet-300', textLight: 'text-violet-600', bg: 'bg-violet-500/10' };
    return { bar: 'bg-amber-400', textDark: 'text-amber-300', textLight: 'text-amber-600', bg: 'bg-amber-500/10' };
  }

  function fmtBytes(b: number): string {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), units.length - 1);
    return (b / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
  }
</script>

{#if m}
  <div class="grid gap-3 grid-cols-2 sm:grid-cols-4">
    {#each [
      { label: 'CPU', value: m.cpu?.usagePercent ?? 0, color: 'sky', sub: '' },
      { label: 'RAM', value: m.memory?.usagePercent ?? 0, color: 'emerald', sub: m.memory ? `${fmtBytes(m.memory.usedBytes)} / ${fmtBytes(m.memory.totalBytes)}` : '' },
      { label: 'Disk', value: m.disk?.usagePercent ?? 0, color: 'violet', sub: m.disk ? `${fmtBytes(m.disk.usedBytes)} / ${fmtBytes(m.disk.totalBytes)}` : '' },
      { label: 'Swap', value: m.swap?.usagePercent ?? 0, color: 'amber', sub: m.swap ? `${fmtBytes(m.swap.usedBytes)} / ${fmtBytes(m.swap.totalBytes)}` : '' },
    ] as g}
      {@const cc = colorClasses(g.color)}
      <div class="text-center p-3 rounded-xl bg-white/[0.02] dark:bg-white/[0.02] light:bg-black/[0.02] border border-white/[0.05] dark:border-white/[0.05] light:border-black/[0.06]">
        <div class="text-xs text-[var(--text-muted)] mb-1">{g.label}</div>
        <div class="text-lg font-bold {cc.textDark} dark:{cc.textDark} light:{cc.textLight}">{Math.round(g.value)}%</div>
        {#if g.sub}
          <div class="text-[0.6rem] text-slate-500 mt-0.5">{g.sub}</div>
        {/if}
        <div class="w-full h-1.5 bg-white/5 dark:bg-white/5 light:bg-black/[0.04] rounded-full mt-1 overflow-hidden">
          <div class="h-full rounded-full {cc.bar}" style="width: {Math.min(g.value, 100)}%"></div>
        </div>
      </div>
    {/each}
  </div>
{/if}
