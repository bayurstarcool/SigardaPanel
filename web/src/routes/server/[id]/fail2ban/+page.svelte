<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import StatCard from '$components/StatCard.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import { Shield, ShieldCheck, Ban, Unlock, RefreshCw, Loader2, TriangleAlert, Check, X, ChevronRight, SquareTerminal } from 'lucide-svelte';

  const serverId = $page.params.id;
  let status = $state<any>(null);
  let jails = $state<any[]>([]);
  let selectedJail = $state<string>('');
  let jailDetail = $state<any>(null);
  let loading = $state(true);
  let jailLoading = $state(false);
  let busy = $state(false);
  let unbanIp = $state('');
  let banIp = $state('');
  let banJail = $state('sshd');
  let message = $state<{ type: 'success' | 'error'; text: string } | null>(null);

  onMount(() => { loadStatus(); });

  async function loadStatus() {
    loading = true;
    try {
      const d = await apiFetch(auth.token, 'GET', `servers/${serverId}/fail2ban/status`);
      status = d;
      jails = (d?.jails ?? []).map((name: string) => ({ name, loading: false }));
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { loading = false; }
  }

  async function loadJail(name: string) {
    selectedJail = name;
    jailLoading = true;
    jailDetail = null;
    try {
      const d = await apiFetch(auth.token, 'GET', `servers/${serverId}/fail2ban/jail?name=${encodeURIComponent(name)}`);
      jailDetail = d;
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { jailLoading = false; }
  }

  async function unbanIP() {
    if (!unbanIp.trim()) return;
    busy = true;
    message = null;
    try {
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/unban`, { jail: selectedJail || 'sshd', ip: unbanIp.trim() });
      message = { type: 'success', text: `Unbanned ${unbanIp}` };
      unbanIp = '';
      if (selectedJail) loadJail(selectedJail);
      loadStatus();
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }

  async function banIP() {
    if (!banIp.trim()) return;
    busy = true;
    message = null;
    try {
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/ban`, { jail: banJail, ip: banIp.trim() });
      message = { type: 'success', text: `Banned ${banIp} in ${banJail}` };
      banIp = '';
      if (selectedJail) loadJail(selectedJail);
      loadStatus();
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }

  async function toggleJail(name: string, disable: boolean) {
    busy = true;
    message = null;
    try {
      const action = disable ? 'disable' : 'enable';
      await apiFetch(auth.token, 'POST', `servers/${serverId}/fail2ban/${action}`, { jail: name });
      message = { type: 'success', text: `${disable ? 'Disabled' : 'Enabled'} jail ${name}` };
      loadStatus();
      if (selectedJail === name) loadJail(name);
    } catch (e: any) {
      message = { type: 'error', text: e.message };
    } finally { busy = false; }
  }
</script>

<DashboardLayout>
  <PageHeader title="Fail2ban" back="/servers" backLabel="Servers" description="Server #{serverId} — intrusion prevention">
    {#snippet actions()}
      <a href="/server/{serverId}/terminal" class="btn btn-secondary btn-sm"><SquareTerminal size={14} /> Terminal</a>
      <button onclick={loadStatus} class="btn btn-secondary btn-sm" disabled={loading}>
        <RefreshCw size={14} class={loading ? 'animate-spin' : ''} /> Refresh
      </button>
    {/snippet}
  </PageHeader>

  {#if message}
    <div class="mb-6 flex items-center gap-2 rounded-lg px-4 py-3 text-[13px] {message.type === 'success' ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}">
      {#if message.type === 'success'}<Check size={14} class="shrink-0" />{:else}<TriangleAlert size={14} class="shrink-0" />{/if}
      {message.text}
    </div>
  {/if}

  {#if loading}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      {#each Array(4) as _, i (i)}<div class="skeleton h-[88px] rounded-xl"></div>{/each}
    </div>
    <div class="skeleton h-40 rounded-xl"></div>
  {:else if !status?.installed}
    <div class="card">
      <EmptyState icon={Ban as any} title="fail2ban is not installed" description="Install fail2ban on this server to enable intrusion prevention." />
    </div>
  {:else}
    <div class="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
      <StatCard label="Status" value={status?.active ? 'Active' : 'Inactive'} icon={Shield as any} tone={status?.active ? 'success' : ''} />
      <StatCard label="Jails" value={jails.length} icon={ShieldCheck as any} />
      <StatCard label="Banned IPs" value={jailDetail?.currently_banned ?? '—'} hint={selectedJail ? `in ${selectedJail}` : 'select a jail'} icon={Ban as any} tone="warning" />
      <StatCard label="Version" value={status?.version?.split('\n')[0] ?? '—'} icon={Shield as any} />
    </div>

    <div class="mb-6 grid gap-4 lg:grid-cols-2">
      <section class="card">
        <div class="card-header"><h2 class="card-title">Jails</h2></div>
        {#if jails.length === 0}
          <EmptyState compact title="No jails active" />
        {:else}
          <ul class="divide-y divide-line">
            {#each jails as jail (jail.name)}
              <li class="flex items-center justify-between gap-2 px-2 py-1 {selectedJail === jail.name ? 'bg-surface-2' : ''}">
                <button
                  onclick={() => loadJail(jail.name)}
                  class="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm font-medium text-fg transition-colors hover:bg-surface-2"
                >
                  <span class="dot text-success"></span>
                  <span class="truncate">{jail.name}</span>
                  <ChevronRight size={14} class="ml-auto text-subtle" />
                </button>
                <button onclick={() => toggleJail(jail.name, true)} disabled={busy} class="btn btn-danger-ghost btn-xs">Disable</button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="card">
        {#if selectedJail}
          <div class="card-header">
            <h2 class="card-title">Jail: <span class="font-mono">{selectedJail}</span></h2>
            <button onclick={() => { selectedJail = ''; jailDetail = null; }} class="btn btn-ghost btn-icon btn-xs" aria-label="Close jail details">
              <X size={14} />
            </button>
          </div>
          {#if jailLoading}
            <div class="space-y-3 p-5">
              <div class="skeleton h-12"></div>
              <div class="skeleton h-8"></div>
            </div>
          {:else if jailDetail}
            <div class="card-body">
              <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
                <div><div class="stat-label">Currently banned</div><div class="text-lg font-semibold text-warning tabular-nums">{jailDetail.currently_banned ?? 0}</div></div>
                <div><div class="stat-label">Total banned</div><div class="text-lg font-semibold text-fg tabular-nums">{jailDetail.total_banned ?? 0}</div></div>
                <div><div class="stat-label">Currently failed</div><div class="text-lg font-semibold text-fg tabular-nums">{jailDetail.currently_failed ?? 0}</div></div>
                <div><div class="stat-label">Total failed</div><div class="text-lg font-semibold text-fg tabular-nums">{jailDetail.failed ?? 0}</div></div>
              </div>

              {#if jailDetail.banned?.length > 0}
                <h3 class="mt-5 mb-2 text-xs font-medium text-subtle">Banned IPs</h3>
                <div class="divide-y divide-line rounded-lg border border-line">
                  {#each jailDetail.banned as ip, i (i)}
                    <div class="flex items-center justify-between px-3 py-1.5">
                      <span class="font-mono text-[13px] text-fg">{ip}</span>
                      <button onclick={() => { unbanIp = ip; unbanIP(); }} disabled={busy} class="btn btn-ghost btn-xs text-success">
                        <Unlock size={12} /> Unban
                      </button>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        {:else}
          <EmptyState compact icon={ShieldCheck as any} title="Select a jail" description="Pick a jail on the left to see its ban statistics and banned IPs." />
        {/if}
      </section>
    </div>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <section class="card card-body">
        <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold text-fg"><Unlock size={16} class="text-subtle" /> Unban IP</h3>
        <label for="f2b-unban" class="sr-only">IP address to unban</label>
        <div class="flex gap-2">
          <input id="f2b-unban" bind:value={unbanIp} placeholder="IP address" class="input flex-1 font-mono text-[13px]" />
          <button onclick={unbanIP} disabled={busy || !unbanIp.trim()} class="btn btn-secondary">
            {#if busy}<Loader2 size={14} class="animate-spin" />{/if} Unban
          </button>
        </div>
        <p class="hint">Removes the IP from {selectedJail || 'sshd'}.</p>
      </section>
      <section class="card card-body">
        <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold text-fg"><Ban size={16} class="text-subtle" /> Ban IP</h3>
        <div class="flex flex-wrap gap-2 sm:flex-nowrap">
          <label for="f2b-ban-jail" class="sr-only">Jail</label>
          <select id="f2b-ban-jail" bind:value={banJail} class="input w-full sm:w-36">
            {#each jails as j (j.name)}<option value={j.name}>{j.name}</option>{/each}
          </select>
          <label for="f2b-ban" class="sr-only">IP address to ban</label>
          <input id="f2b-ban" bind:value={banIp} placeholder="IP address" class="input min-w-0 flex-1 font-mono text-[13px]" />
          <button onclick={banIP} disabled={busy || !banIp.trim()} class="btn btn-danger">
            {#if busy}<Loader2 size={14} class="animate-spin" />{/if} Ban
          </button>
        </div>
      </section>
    </div>
  {/if}
</DashboardLayout>
