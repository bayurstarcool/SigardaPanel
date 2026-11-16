<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import ResultGrid from './ResultGrid.svelte';
  import { Play, Lock, LockOpen, History, Trash2, CircleCheck } from 'lucide-svelte';
  import { looksLikeWrite, type Result } from './types';

  let { dbId, dbType, onchanged }: { dbId: number; dbType: string; onchanged?: () => void } = $props();

  type Outcome = { kind: 'rows' | 'exec'; result?: Result; affected: number; ms: number; message?: string };

  let sql = $state('');
  let readOnly = $state(true);
  let running = $state(false);
  let error = $state('');
  let outcome = $state<Outcome | null>(null);
  let confirmOpen = $state(false);
  let showHistory = $state(false);

  // History stays in this browser only (per database).
  const historyKey = $derived(`sigarda.sql-history.${dbId}`);
  let history = $state<string[]>([]);
  $effect(() => {
    try { history = JSON.parse(localStorage.getItem(historyKey) || '[]'); } catch { history = []; }
  });
  function remember(q: string) {
    history = [q, ...history.filter((h) => h !== q)].slice(0, 30);
    try { localStorage.setItem(historyKey, JSON.stringify(history)); } catch { /* storage unavailable */ }
  }
  function clearHistory() {
    history = [];
    try { localStorage.removeItem(historyKey); } catch { /* storage unavailable */ }
  }

  function run(text = sql) {
    sql = text;
    if (!sql.trim() || running) return;
    if (!readOnly && looksLikeWrite(sql)) { confirmOpen = true; return; }
    execute();
  }

  async function execute() {
    running = true;
    error = '';
    const q = sql;
    try {
      const d = await apiFetch(auth.token, 'POST', `databases/${dbId}/manager/sql`, { sql: q, read_only: readOnly });
      const r = d.result;
      outcome = r.kind === 'rows'
        ? { kind: 'rows', result: r, affected: 0, ms: r.elapsed_ms ?? 0 }
        : { kind: 'exec', affected: r.rows_affected ?? 0, ms: r.elapsed_ms ?? 0, message: r.message };
      remember(q.trim());
      if (r.kind !== 'rows' && !readOnly) onchanged?.();
    } catch (e: any) {
      error = e.message || 'The statement failed';
      outcome = null;
    } finally {
      running = false;
    }
  }

  const examples = $derived(dbType === 'postgres'
    ? ["SELECT table_name FROM information_schema.tables WHERE table_schema = 'public';", 'SELECT now(), version();']
    : ['SHOW TABLES;', 'SELECT NOW(), VERSION();']);
</script>

<div class="flex flex-col">
  <div class="flex flex-wrap items-center justify-between gap-2 border-b border-line px-3 py-2">
    <div class="flex items-center gap-1.5">
      <button onclick={() => run()} disabled={running || !sql.trim()} class="btn btn-primary btn-sm">
        <Play size={13} /> {running ? 'Running…' : 'Run'}
      </button>
      <span class="hidden text-[11px] text-subtle sm:inline">Ctrl/⌘ + Enter</span>
    </div>
    <div class="flex items-center gap-1.5">
      <button onclick={() => (readOnly = !readOnly)} class="btn btn-sm {readOnly ? 'btn-secondary' : 'btn-danger-ghost'}" aria-pressed={!readOnly}
        title={readOnly ? 'Statements run in a read-only transaction: nothing can be changed' : 'Statements can change data and structure'}>
        {#if readOnly}<Lock size={13} /> Read-only{:else}<LockOpen size={13} /> Writes allowed{/if}
      </button>
      <button onclick={() => (showHistory = !showHistory)} class="btn btn-ghost btn-sm" aria-pressed={showHistory} disabled={!history.length}>
        <History size={13} /> History
      </button>
    </div>
  </div>

  {#if showHistory && history.length}
    <div class="max-h-48 overflow-y-auto border-b border-line bg-surface-2/40">
      {#each history as h, i (i)}
        <button onclick={() => { sql = h; showHistory = false; }} class="block w-full truncate px-3 py-1.5 text-left font-mono text-[12px] text-muted hover:bg-surface-2 hover:text-fg" title={h}>{h}</button>
      {/each}
      <button onclick={clearHistory} class="flex w-full items-center gap-1.5 px-3 py-1.5 text-[12px] text-subtle hover:text-danger"><Trash2 size={12} /> Clear history</button>
    </div>
  {/if}

  <div class="h-44 border-b border-line">
    {#await import('$components/CodeEditor.svelte') then m}
      <m.default content={sql} onChange={(t: string) => (sql = t)} language={dbType === 'postgres' ? 'pgsql' : 'mysql'} hideHeader hideFooter height="11rem" onRun={(t: string) => run(t)} />
    {/await}
  </div>

  {#if error}
    <p class="border-b border-line px-4 py-3 font-mono text-[12px] whitespace-pre-wrap text-danger">{error}</p>
  {:else if outcome?.kind === 'rows' && outcome.result}
    <ResultGrid result={outcome.result} maxHeight="max-h-[calc(100dvh-480px)]" />
    <div class="border-t border-line px-3 py-2 text-[12px] text-subtle tabular-nums">
      {outcome.result.rows.length} row{outcome.result.rows.length === 1 ? '' : 's'}{outcome.result.has_more ? ' (first 500 shown)' : ''} · {outcome.ms} ms
      {#if outcome.result.truncated}<span class="text-warning"> · long values are cut</span>{/if}
    </div>
  {:else if outcome?.kind === 'exec'}
    <p class="flex items-center gap-2 px-4 py-4 text-[13px] text-success">
      <CircleCheck size={15} /> Done · {outcome.affected} row{outcome.affected === 1 ? '' : 's'} affected · {outcome.ms} ms
    </p>
  {:else}
    <div class="space-y-2 px-4 py-5 text-[13px] text-subtle">
      <p>Run SQL as the database's own account. Read-only mode is on by default; switch it off to change data or structure.</p>
      <div class="flex flex-wrap gap-1.5">
        {#each examples as ex (ex)}
          <button onclick={() => (sql = ex)} class="rounded-md border border-line px-2 py-1 font-mono text-[11px] text-muted hover:border-accent hover:text-fg">{ex}</button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<ConfirmDialog bind:open={confirmOpen} title="Run a statement that changes data?" destructive confirmLabel="Run it"
  message="This statement may change or delete data, and it runs right away (it cannot be undone from here). Make sure you have a backup."
  onconfirm={execute} />
