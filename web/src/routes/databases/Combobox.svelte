<script lang="ts">
  // Search-as-you-type picker. The suggestion list renders in normal flow
  // (not absolutely positioned) so it works inside scrolling modals.
  let {
    id,
    label,
    value = $bindable(''),
    options,
    optionLabel,
    optionKey,
    placeholder = '',
    emptyText = 'No results',
    disabled = false,
    mono = false,
    oninput,
    onselect,
  }: {
    id: string;
    label: string;
    value?: string;
    options: any[];
    optionLabel: (o: any) => string;
    optionKey: (o: any) => string | number;
    placeholder?: string;
    emptyText?: string;
    disabled?: boolean;
    mono?: boolean;
    oninput?: () => void;
    onselect: (o: any) => void;
  } = $props();

  let open = $state(false);

  function pick(o: any) {
    onselect(o);
    open = false;
  }
</script>

<div>
  <label for={id} class="label">{label}</label>
  <input
    {id}
    bind:value
    {placeholder}
    {disabled}
    autocomplete="off"
    role="combobox"
    aria-expanded={open}
    aria-controls="{id}-list"
    class="input {mono ? 'font-mono' : ''}"
    oninput={() => { open = true; oninput?.(); }}
    onfocus={() => (open = true)}
    onblur={() => setTimeout(() => (open = false), 150)}
    onkeydown={(e) => { if (e.key === 'Escape' && open) { e.stopPropagation(); open = false; } }} />
  {#if open && !disabled}
    <ul id="{id}-list" role="listbox" class="mt-1 max-h-48 overflow-y-auto rounded-lg border border-line bg-elevated py-1 animate-fade-in">
      {#each options as o (optionKey(o))}
        <li>
          <button
            type="button"
            role="option"
            aria-selected={optionLabel(o) === value}
            onmousedown={(e) => e.preventDefault()}
            onclick={() => pick(o)}
            class="block w-full cursor-pointer truncate px-3 py-1.5 text-left text-[13px] text-fg transition-colors duration-100 hover:bg-surface-2 {mono ? 'font-mono' : ''}">
            {optionLabel(o)}
          </button>
        </li>
      {:else}
        <li class="px-3 py-1.5 text-[13px] text-subtle">{emptyText}</li>
      {/each}
    </ul>
  {/if}
</div>
