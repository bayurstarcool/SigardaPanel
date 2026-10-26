<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { ShieldAlert, RefreshCw, Loader2, Save, Activity, FileWarning, Network, Clock, Globe, Ban } from 'lucide-svelte';

  const serverId = $page.params.id;
  let data = $state<any>(null);
  let baseline = $state<any>(null);
  let loading = $state(true);
  let baselineBusy = $state(false);
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null);

  onMount(() => {
    // auth.user sudah terisi: layout hanya me-render halaman setelah auth.init
    // selesai, jadi pemeriksaan role di sini aman dari race init.
    if (auth.user && !hasRole(auth.user.role, 'admin')) { window.location.href = '/dashboard'; return; }
    load();
  });

  async function banIp(ip: string) {
    try {
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/ban`, { jail: 'sshd', ip });
      message = { type: 'success', text: `${ip} banned (sshd jail)` };
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    }
  }

  async function load() {
    loading = true;
    try {
      const [m, b] = await Promise.all([
        apiFetch(auth.token, 'GET', `servers/${serverId}/security/monitor`),
        apiFetch(auth.token, 'GET', `servers/${serverId}/security/baseline`).catch(() => null),
      ]);
      data = m;
      baseline = b;
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { loading = false; }
  }

  async function saveBaseline() {
    baselineBusy = true;
    try {
      const r = await apiFetch(auth.token, 'POST', `servers/${serverId}/security/baseline`);
      message = { type: 'success', text: `Baseline saved (${r?.files ?? '?'} files)` };
      baseline = await apiFetch(auth.token, 'GET', `servers/${serverId}/security/baseline`);
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { baselineBusy = false; }
  }

  const card = 'rounded-xl border border-slate-800 bg-slate-900/60 p-5';
  const cardTitle = 'flex items-center gap-2 text-sm font-semibold text-slate-200 mb-3';
</script>

<DashboardLayout active="Threat Monitor">
<div class="max-w-6xl mx-auto p-6 space-y-6">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-3">
      <ShieldAlert size={26} class="text-amber-400" />
      <div>
        <h1 class="text-xl font-bold text-slate-100">Threat Monitor</h1>
        <p class="text-xs text-slate-500">Compromise indicators collected by the agent — SSH brute force, file integrity, suspicious connections, persistence, web attacks.</p>
      </div>
    </div>
    <div class="flex gap-2">
      <button class="px-3 py-1.5 text-xs rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 flex items-center gap-1.5" onclick={load} disabled={loading}>
        {#if loading}<Loader2 size={13} class="animate-spin" />{:else}<RefreshCw size={13} />{/if} Refresh
      </button>
      <button class="px-3 py-1.5 text-xs rounded-lg bg-emerald-700 hover:bg-emerald-600 text-white flex items-center gap-1.5" onclick={saveBaseline} disabled={baselineBusy}>
        {#if baselineBusy}<Loader2 size={13} class="animate-spin" />{:else}<Save size={13} />{/if} Save integrity baseline
      </button>
    </div>
  </div>

  {#if message}
    <div class="rounded-lg px-4 py-2 text-sm {message.type === 'success' ? 'bg-emerald-900/50 text-emerald-300' : 'bg-red-900/50 text-red-300'}">{message.text}</div>
  {/if}

  {#if loading && !data}
    <div class="flex justify-center py-20"><Loader2 size={22} class="animate-spin text-slate-500" /></div>
  {:else if data}
    <div class="grid md:grid-cols-2 gap-4">
      <!-- SSH -->
      <div class={card}>
        <div class={cardTitle}><Activity size={15} class="text-rose-400" /> SSH activity</div>
        <div class="grid grid-cols-3 gap-2 text-center mb-3">
          <div class="rounded-lg bg-slate-950/60 py-2">
            <div class="text-lg font-bold text-rose-400">{data.ssh?.failed_total ?? 0}</div>
            <div class="text-[10px] text-slate-500 uppercase">Failed</div>
          </div>
          <div class="rounded-lg bg-slate-950/60 py-2">
            <div class="text-lg font-bold text-emerald-400">{data.ssh?.accepted_total ?? 0}</div>
            <div class="text-[10px] text-slate-500 uppercase">Accepted</div>
          </div>
          <div class="rounded-lg bg-slate-950/60 py-2">
            <div class="text-lg font-bold text-amber-400">{data.ssh?.unique_attacker_ips ?? 0}</div>
            <div class="text-[10px] text-slate-500 uppercase">Attacker IPs</div>
          </div>
        </div>
        <div class="space-y-1 max-h-44 overflow-auto text-xs">
          {#each (data.ssh?.top_attackers ?? []).slice(0, 8) as a}
            <div class="flex justify-between items-center rounded bg-slate-950/40 px-2 py-1">
              <span class="font-mono text-rose-300">{a.ip}</span>
              <span class="flex items-center gap-2">
                <span class="text-slate-400">{a.count}× {a.usernames?.length ? `(${a.usernames.join(', ')})` : ''}</span>
                <button title="Ban via fail2ban" class="text-slate-500 hover:text-rose-400" onclick={() => banIp(a.ip)}>
                  <Ban size={12} />
                </button>
              </span>
            </div>
          {:else}
            <p class="text-slate-500">No failed logins in the current logs.</p>
          {/each}
        </div>
        {#if (data.ssh?.recent_accepted ?? []).length}
          <div class="mt-3 text-[11px] text-slate-500 space-y-0.5">
            <div class="uppercase tracking-wide mb-1">Recent successful logins</div>
            {#each data.ssh.recent_accepted.slice(-4).reverse() as l}
              <div><span class="text-emerald-300">{l.user}</span> from <span class="font-mono">{l.ip}</span> ({l.method})</div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Integrity -->
      <div class={card}>
        <div class={cardTitle}><FileWarning size={15} class="text-amber-400" /> File integrity</div>
        {#if baseline?.has_baseline}
          {@const diffs = baseline.compare?.diffs ?? []}
          {#if diffs.length === 0}
            <p class="text-sm text-emerald-400">✓ All {baseline.compare?.baseline_count ?? 0} watched files match the baseline.</p>
          {:else}
            <div class="space-y-1 text-xs">
              {#each diffs as d}
                <div class="rounded px-2 py-1 {d.status === 'changed' ? 'bg-amber-950/50 text-amber-300' : d.status === 'new' ? 'bg-sky-950/50 text-sky-300' : 'bg-red-950/50 text-red-300'}">
                  <span class="uppercase font-semibold">{d.status}</span> <span class="font-mono">{d.path}</span>
                </div>
              {/each}
            </div>
          {/if}
        {:else}
          <p class="text-sm text-slate-400">No baseline saved yet. Snapshot the current known-good state of critical files (passwd, shadow, sudoers, sshd_config, cron, authorized_keys) with the button above.</p>
        {/if}
      </div>

      <!-- Connections -->
      <div class={card}>
        <div class={cardTitle}><Network size={15} class="text-fuchsia-400" /> Outbound connections</div>
        {#if (data.connections?.suspicious_only ?? []).length}
          <div class="space-y-1 text-xs">
            {#each data.connections.suspicious_only as c}
              <div class="rounded bg-red-950/50 px-2 py-1 text-red-300">
                <span class="font-mono">{c.remote}</span> — {c.reason}
              </div>
            {/each}
          </div>
        {:else}
          <p class="text-sm text-emerald-400">✓ No connections to known miner/C2 ports.</p>
        {/if}
        <p class="mt-2 text-[11px] text-slate-500">{data.connections?.total ?? 0} established remote endpoints in total.</p>
      </div>

      <!-- Persistence -->
      <div class={card}>
        <div class={cardTitle}><Clock size={15} class="text-sky-400" /> Persistence</div>
        <div class="text-xs space-y-2">
          <div>
            <span class="text-slate-500 uppercase text-[10px]">UID-0 accounts</span>
            <div class={(data.persistence?.uid_zero_users ?? []).length > 1 ? 'text-red-300' : 'text-emerald-300'}>
              {(data.persistence?.uid_zero_users ?? ['?']).join(', ')}
              {(data.persistence?.uid_zero_users ?? []).length > 1 ? ' ⚠ more than root!' : ''}
            </div>
          </div>
          <div>
            <span class="text-slate-500 uppercase text-[10px]">Cron entries</span>
            <div class="text-slate-300">{(data.persistence?.cron_files ?? []).length} files</div>
          </div>
          <div>
            <span class="text-slate-500 uppercase text-[10px]">SSH keys</span>
            <div class="space-y-0.5">
              {#each (data.persistence?.authorized_keys ?? []) as k}
                <div class="text-slate-400"><span class="text-slate-300">{k.owner}</span>: <span class="font-mono text-[10px]">{k.note}</span></div>
              {:else}
                <div class="text-slate-500">none</div>
              {/each}
            </div>
          </div>
        </div>
      </div>

      <!-- Web attacks (full width) -->
      <div class="{card} md:col-span-2">
        <div class={cardTitle}><Globe size={15} class="text-orange-400" /> Web attack patterns <span class="text-[10px] font-normal text-slate-500">({data.web?.lines_scanned ?? 0} nginx log lines scanned)</span></div>
        <div class="grid md:grid-cols-2 gap-4 text-xs">
          <div class="space-y-1">
            {#each Object.entries(data.web?.pattern_hits ?? {}) as [name, count]}
              <div class="flex justify-between rounded bg-slate-950/40 px-2 py-1">
                <span class="text-orange-300">{name}</span><span class="text-slate-400">{count}×</span>
              </div>
            {:else}
              <p class="text-emerald-400">✓ No attack patterns in the scanned logs.</p>
            {/each}
          </div>
          <div class="space-y-1">
            {#each (data.web?.top_offenders ?? []).slice(0, 8) as o}
              <div class="flex justify-between rounded bg-slate-950/40 px-2 py-1">
                <span class="font-mono text-orange-300">{o.ip}</span><span class="text-slate-400">{o.count} hits</span>
              </div>
            {:else}
              <p class="text-slate-500">No offending IPs.</p>
            {/each}
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>
</DashboardLayout>
