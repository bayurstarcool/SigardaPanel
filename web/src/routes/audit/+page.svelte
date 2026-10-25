<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { ClipboardCheck } from 'lucide-svelte';
  import { onMount } from 'svelte';

  let logs = $state<any[]>([]);
  let loading = $state(true);

  function parseDetails(details: any) {
    if (!details) return null;
    if (typeof details === 'object') return details;
    try { return JSON.parse(details); } catch { return null; }
  }

  function auditSummary(log: any) {
    const d = parseDetails(log.details || log.detail);
    if (!d) return log.detail || log.message || '';
    const action = d.action || log.action || 'event';
    const itemType = d.item_type || (d.is_dir === true ? 'folder' : d.is_dir === false ? 'file' : 'item');
    const path = d.path || d.old_path || d.new_name || '';

    if (String(action).startsWith('file_')) {
      if (action === 'file_rename') return `${itemType}: ${d.old_path || ''} → ${d.new_name || ''}`;
      return `${itemType}: ${path}`;
    }
    return Object.entries(d).map(([k, v]) => `${k}: ${v}`).join(' · ');
  }

  onMount(async () => {
    if (!auth.user) { loading = false; return; }
    try {
      const d = await apiFetch(auth.token, 'GET', 'audit');
      logs = d?.logs || d?.entries || [];
    } catch {}
    finally { loading = false; }
  });
</script>

<DashboardLayout active="Audit">
  <div class="mb-8">
    <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Audit Log</h1>
    <p class="text-sm text-slate-500 mt-1">{loading ? 'Loading...' : `${logs.length} entry(ies)`}</p>
  </div>

  {#if loading}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20"><div class="animate-pulse text-[var(--text-muted)]">Loading...</div></div>
  {:else if logs.length === 0}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6 text-center py-20">
      <ClipboardCheck size={48} class="mb-4 opacity-30 mx-auto" />
      <h2 class="text-lg font-semibold mb-2">No Audit Entries</h2>
      <p class="text-sm text-slate-500">Audit log entries will appear here.</p>
    </div>
  {:else}
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl overflow-hidden">
      <div class="divide-y divide-white/5">
        {#each logs as l}
          <div class="px-6 py-4 hover:bg-white/[0.02] transition">
            <div class="flex items-center justify-between">
              <div class="text-sm font-medium">{l.action || l.type || 'event'}</div>
              <span class="text-xs text-slate-500">{fmtRelative(l.created_at)}</span>
            </div>
            {#if auditSummary(l)}
              <div class="text-xs text-slate-500 mt-1">{auditSummary(l)}</div>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</DashboardLayout>
