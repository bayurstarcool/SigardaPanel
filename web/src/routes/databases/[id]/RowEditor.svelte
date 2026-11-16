<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { KeyRound, Info } from 'lucide-svelte';
  import { isPartial, type Column } from './types';

  type Field = { value: string; isNull: boolean; useDefault: boolean; locked: boolean };

  let {
    open = $bindable(false),
    mode,
    table,
    columns,
    original = null,
    busy = false,
    onsave,
  }: {
    open: boolean;
    mode: 'insert' | 'edit';
    table: string;
    columns: Column[];
    /** The row being edited, by column name (edit mode). */
    original?: Record<string, string | null> | null;
    busy?: boolean;
    onsave: (values: Record<string, string | null>) => void;
  } = $props();

  let fields = $state<Record<string, Field>>({});

  // Reset the form whenever the dialog opens.
  $effect(() => {
    if (!open) return;
    const next: Record<string, Field> = {};
    for (const c of columns) {
      if (mode === 'edit' && original) {
        const v = original[c.name] ?? null;
        next[c.name] = { value: v ?? '', isNull: v === null, useDefault: false, locked: isPartial(v) };
      } else {
        // Auto-increment / serial columns and columns with a default start on "default".
        const auto = /auto_increment/i.test(c.extra) || /nextval\(|identity/i.test(c.default ?? '');
        next[c.name] = { value: '', isNull: false, useDefault: auto || c.default !== null, locked: false };
      }
    }
    fields = next;
  });

  const isLong = (c: Column) => /text|json|blob|xml/i.test(c.type);

  function changed(c: Column): boolean {
    const f = fields[c.name];
    if (!f || f.locked) return false;
    if (mode === 'insert') return !f.useDefault;
    const before = original?.[c.name] ?? null;
    const after = f.isNull ? null : f.value;
    return before !== after;
  }

  const changes = $derived(columns.filter((c) => changed(c)).length);

  function submit(e: Event) {
    e.preventDefault();
    const values: Record<string, string | null> = {};
    for (const c of columns) {
      if (!changed(c)) continue;
      const f = fields[c.name];
      values[c.name] = f.isNull ? null : f.value;
    }
    onsave(values);
  }
</script>

<Modal bind:open title={mode === 'insert' ? `Insert row into ${table}` : `Edit row in ${table}`} maxWidth="2xl">
  <form id="row-editor" onsubmit={submit} class="space-y-3">
    {#if mode === 'edit'}
      <p class="flex items-start gap-2 rounded-lg bg-surface-2 px-3 py-2 text-[12px] text-muted">
        <Info size={14} class="mt-0.5 shrink-0" />
        Only the fields you change are saved. The row is found by its primary key.
      </p>
    {/if}
    <div class="max-h-[60dvh] space-y-3 overflow-y-auto pr-1">
      {#each columns as c (c.name)}
        {@const f = fields[c.name]}
        {#if f}
          <div class="grid gap-1.5 sm:grid-cols-[11rem_minmax(0,1fr)] sm:items-start sm:gap-3">
            <label for="f-{c.name}" class="min-w-0 pt-1.5">
              <span class="flex items-center gap-1 truncate font-mono text-[13px] text-fg">
                {#if c.key === 'PRI'}<KeyRound size={12} class="shrink-0 text-warning" />{/if}{c.name}
              </span>
              <span class="block truncate text-[11px] text-subtle">{c.type}{c.nullable ? '' : ' · not null'}</span>
            </label>
            <div class="min-w-0 space-y-1">
              {#if f.locked}
                <p class="rounded-md border border-dashed border-line px-3 py-2 text-[12px] text-subtle">
                  This value is binary or too long to edit here; use the SQL console. It is left unchanged.
                </p>
              {:else}
                {#if isLong(c)}
                  <textarea id="f-{c.name}" bind:value={f.value} rows="3" oninput={() => { f.useDefault = false; f.isNull = false; }}
                    class="input min-h-16 font-mono text-[12px] {f.isNull || f.useDefault ? 'text-subtle' : ''}" placeholder={f.useDefault ? 'DEFAULT' : f.isNull ? 'NULL' : ''}></textarea>
                  {#if (f.isNull || f.useDefault) && f.value}<p class="text-[11px] text-subtle">Saved as {f.isNull ? 'NULL' : 'the default'}; typing uses your value.</p>{/if}
                {:else}
                  <input id="f-{c.name}" bind:value={f.value} oninput={() => { f.useDefault = false; f.isNull = false; }}
                    class="input h-9 font-mono text-[12px] {f.isNull || f.useDefault ? 'text-subtle' : ''}" placeholder={f.useDefault ? `DEFAULT${c.default ? ` (${c.default})` : ''}` : f.isNull ? 'NULL' : ''} />
                {/if}
                <div class="flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-muted">
                  {#if c.nullable}
                    <label class="inline-flex items-center gap-1.5"><input type="checkbox" class="checkbox" bind:checked={f.isNull} disabled={f.useDefault} /> NULL</label>
                  {/if}
                  {#if mode === 'insert'}
                    <label class="inline-flex items-center gap-1.5"><input type="checkbox" class="checkbox" bind:checked={f.useDefault} /> Default</label>
                  {/if}
                </div>
              {/if}
            </div>
          </div>
        {/if}
      {/each}
    </div>
  </form>
  {#snippet footer()}
    <button type="button" class="btn btn-secondary" onclick={() => (open = false)}>Cancel</button>
    <button type="submit" form="row-editor" class="btn btn-primary" disabled={busy || (mode === 'edit' && changes === 0)}>
      {busy ? 'Saving…' : mode === 'insert' ? 'Insert row' : changes ? `Save ${changes} change${changes === 1 ? '' : 's'}` : 'No changes'}
    </button>
  {/snippet}
</Modal>
