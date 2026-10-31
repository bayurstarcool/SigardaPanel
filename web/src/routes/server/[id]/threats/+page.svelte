<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { RefreshCw, Loader2, Save, Activity, FileWarning, Network, Clock, Globe, Ban, CircleCheck, TriangleAlert, Check } from 'lucide-svelte';

  const serverId = $page.params.id;
  let data = $state<any>(null);
  let baseline = $state<any>(null);
  let loading = $state(true);
  let baselineBusy = $state(false);
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null);

  onMount(() => {
    // auth.user sudah terisi: layout hanya me-render halaman setelah auth.init
    // selesai, jadi pemeriksaan role di sini aman dari race init.
    if (auth.user && !hasRole(auth.user.role, 'admin')) { goto('/dashboard', { replaceState: true }); return; }
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

  const diffTone: Record<string, string> = { changed: 'badge-warning', new: 'badge-info' };
</script>

<DashboardLayout active="Threat Monitor">
  <PageHeader
    title="Threat monitor"
    back="/servers"
    backLabel="Servers"
    description="Compromise indicators collected by the agent — SSH brute force, file integrity, suspicious connections, persistence, web attacks."
  >
    {#snippet actions()}
      <button class="btn btn-secondary btn-sm" onclick={load} disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
      <button class="btn btn-primary btn-sm" onclick={saveBaseline} disabled={baselineBusy}>
        {#if baselineBusy}<Loader2 size={14} class="animate-spin" />{:else}<Save size={14} />{/if} Save integrity baseline
      </button>
    {/snippet}
  </PageHeader>

  {#if message}
    <div class="mb-6 flex items-center gap-2 rounded-lg px-4 py-3 text-[13px] {message.type === 'success' ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}">
      {#if message.type === 'success'}<Check size={14} class="shrink-0" />{:else}<TriangleAlert size={14} class="shrink-0" />{/if}
      {message.text}
    </div>
  {/if}

  {#if loading && !data}
    <div class="grid gap-4 md:grid-cols-2">
      {#each Array(4) as _, i (i)}<div class="skeleton h-56 rounded-xl"></div>{/each}
    </div>
  {:else if data}
    <div class="grid gap-4 md:grid-cols-2">
      <!-- SSH -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><Activity size={16} class="text-subtle" /> SSH activity</h2></div>
        <div class="card-body">
          <div class="mb-4 grid grid-cols-3 gap-2">
            <div class="rounded-lg bg-surface-2 px-3 py-2">
              <div class="stat-label">Failed</div>
              <div class="text-lg font-semibold text-danger tabular-nums">{data.ssh?.failed_total ?? 0}</div>
            </div>
            <div class="rounded-lg bg-surface-2 px-3 py-2">
              <div class="stat-label">Accepted</div>
              <div class="text-lg font-semibold text-fg tabular-nums">{data.ssh?.accepted_total ?? 0}</div>
            </div>
            <div class="rounded-lg bg-surface-2 px-3 py-2">
              <div class="stat-label">Attacker IPs</div>
              <div class="text-lg font-semibold text-warning tabular-nums">{data.ssh?.unique_attacker_ips ?? 0}</div>
            </div>
          </div>
          <div class="max-h-44 divide-y divide-line overflow-auto text-xs">
            {#each (data.ssh?.top_attackers ?? []).slice(0, 8) as a, i (i)}
              <div class="flex items-center justify-between gap-2 py-1.5">
                <span class="font-mono text-fg">{a.ip}</span>
                <span class="flex min-w-0 items-center gap-2">
                  <span class="truncate text-muted">{a.count}× {a.usernames?.length ? `(${a.usernames.join(', ')})` : ''}</span>
                  <button title="Ban via fail2ban" aria-label="Ban {a.ip}" class="btn btn-danger-ghost btn-icon btn-xs" onclick={() => banIp(a.ip)}>
                    <Ban size={12} />
                  </button>
                </span>
              </div>
            {:else}
              <p class="text-[13px] text-subtle">No failed logins in the current logs.</p>
            {/each}
          </div>
          {#if (data.ssh?.recent_accepted ?? []).length}
            <div class="mt-4 space-y-0.5 border-t border-line pt-3 text-xs text-muted">
              <div class="stat-label mb-1">Recent successful logins</div>
              {#each data.ssh.recent_accepted.slice(-4).reverse() as l, i (i)}
                <div><span class="font-medium text-fg">{l.user}</span> from <span class="font-mono">{l.ip}</span> ({l.method})</div>
              {/each}
            </div>
          {/if}
        </div>
      </section>

      <!-- Integrity -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><FileWarning size={16} class="text-subtle" /> File integrity</h2></div>
        <div class="card-body">
          {#if baseline?.has_baseline}
            {@const diffs = baseline.compare?.diffs ?? []}
            {#if diffs.length === 0}
              <p class="flex items-center gap-2 text-[13px] text-success"><CircleCheck size={14} /> All {baseline.compare?.baseline_count ?? 0} watched files match the baseline.</p>
            {:else}
              <div class="divide-y divide-line text-xs">
                {#each diffs as d, i (i)}
                  <div class="flex items-center gap-2 py-1.5">
                    <span class="badge {diffTone[d.status] ?? 'badge-danger'}">{d.status}</span>
                    <span class="truncate font-mono text-fg">{d.path}</span>
                  </div>
                {/each}
              </div>
            {/if}
          {:else}
            <p class="text-[13px] text-muted">No baseline saved yet. Snapshot the current known-good state of critical files (passwd, shadow, sudoers, sshd_config, cron, authorized_keys) with the button above.</p>
          {/if}
        </div>
      </section>

      <!-- Connections -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><Network size={16} class="text-subtle" /> Outbound connections</h2></div>
        <div class="card-body">
          {#if (data.connections?.suspicious_only ?? []).length}
            <div class="divide-y divide-line text-xs">
              {#each data.connections.suspicious_only as c, i (i)}
                <div class="py-1.5 text-danger"><span class="font-mono">{c.remote}</span> — {c.reason}</div>
              {/each}
            </div>
          {:else}
            <p class="flex items-center gap-2 text-[13px] text-success"><CircleCheck size={14} /> No connections to known miner/C2 ports.</p>
          {/if}
          <p class="mt-3 text-xs text-subtle tabular-nums">{data.connections?.total ?? 0} established remote endpoints in total.</p>
        </div>
      </section>

      <!-- Persistence -->
      <section class="card">
        <div class="card-header"><h2 class="card-title flex items-center gap-2"><Clock size={16} class="text-subtle" /> Persistence</h2></div>
        <div class="card-body space-y-3 text-[13px]">
          <div>
            <div class="stat-label">UID-0 accounts</div>
            <div class="mt-0.5 flex items-center gap-1.5 {(data.persistence?.uid_zero_users ?? []).length > 1 ? 'text-danger' : 'text-fg'}">
              {(data.persistence?.uid_zero_users ?? ['?']).join(', ')}
              {#if (data.persistence?.uid_zero_users ?? []).length > 1}
                <span class="badge badge-danger"><TriangleAlert size={12} /> more than root</span>
              {/if}
            </div>
          </div>
          <div>
            <div class="stat-label">Cron entries</div>
            <div class="mt-0.5 text-fg tabular-nums">{(data.persistence?.cron_files ?? []).length} files</div>
          </div>
          <div>
            <div class="stat-label">SSH keys</div>
            <div class="mt-0.5 space-y-0.5">
              {#each (data.persistence?.authorized_keys ?? []) as k, i (i)}
                <div class="text-muted"><span class="text-fg">{k.owner}</span>: <span class="font-mono text-xs">{k.note}</span></div>
              {:else}
                <div class="text-subtle">none</div>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <!-- Web attacks -->
      <section class="card md:col-span-2">
        <div class="card-header">
          <h2 class="card-title flex items-center gap-2"><Globe size={16} class="text-subtle" /> Web attack patterns</h2>
          <span class="text-xs text-subtle tabular-nums">{data.web?.lines_scanned ?? 0} nginx log lines scanned</span>
        </div>
        <div class="card-body grid gap-6 text-xs md:grid-cols-2">
          <div class="divide-y divide-line">
            {#each Object.entries(data.web?.pattern_hits ?? {}) as [name, count] (name)}
              <div class="flex justify-between py-1.5">
                <span class="text-warning">{name}</span><span class="text-muted tabular-nums">{count}×</span>
              </div>
            {:else}
              <p class="flex items-center gap-2 text-[13px] text-success"><CircleCheck size={14} /> No attack patterns in the scanned logs.</p>
            {/each}
          </div>
          <div class="divide-y divide-line">
            {#each (data.web?.top_offenders ?? []).slice(0, 8) as o, i (i)}
              <div class="flex justify-between py-1.5">
                <span class="font-mono text-fg">{o.ip}</span><span class="text-muted tabular-nums">{o.count} hits</span>
              </div>
            {:else}
              <p class="text-[13px] text-subtle">No offending IPs.</p>
            {/each}
          </div>
        </div>
      </section>
    </div>
  {/if}
</DashboardLayout>
