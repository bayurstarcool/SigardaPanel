<script lang="ts">
  // Heavy (CodeMirror): import this component lazily, e.g.
  // {#await import('$components/CodeEditor.svelte') then m}<m.default … />{/await}
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view';
  import { EditorState, Compartment } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { bracketMatching, foldGutter, syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';
  import { php } from '@codemirror/lang-php';
  import { javascript } from '@codemirror/lang-javascript';
  import { html } from '@codemirror/lang-html';
  import { css } from '@codemirror/lang-css';
  import { json } from '@codemirror/lang-json';
  import { python } from '@codemirror/lang-python';
  import { oneDarkHighlightStyle } from '@codemirror/theme-one-dark';
  import { Save } from 'lucide-svelte';
  import { theme as appTheme } from '$lib/theme.svelte';

  let {
    content = '',
    value = '',
    language = 'text',
    filename = '',
    onSave = (_content: string) => {},
    onChange = (_content: string) => {},
    readOnly = false,
    /** 'auto' follows the app theme; 'dark' / 'light' force one. */
    theme = 'auto',
    height = '100%',
    hideHeader = false,
    hideFooter = false,
  } = $props();

  let editorContainer: HTMLDivElement;
  let editorView: EditorView | undefined;
  let initialized = false;
  const highlightCompartment = new Compartment();

  const isDark = $derived(theme === 'auto' ? appTheme.isDark : theme === 'dark');

  function getLanguageExtension(lang: string) {
    switch (lang) {
      case 'php': return php();
      case 'javascript': case 'js': case 'mjs': return javascript();
      case 'html': case 'blade': return html();
      case 'css': case 'scss': case 'less': return css();
      case 'json': return json();
      case 'python': case 'py': return python();
      default: return [];
    }
  }

  function highlightFor(dark: boolean) {
    return [EditorView.darkTheme.of(dark), syntaxHighlighting(dark ? oneDarkHighlightStyle : defaultHighlightStyle)];
  }

  onMount(() => {
    if (!editorContainer) return;

    // Chrome uses the app's CSS variables, so it follows the theme without reconfiguring.
    const chromeTheme = EditorView.theme({
      '&': { height: height, width: '100%', backgroundColor: 'var(--c-surface)', color: 'var(--c-fg)' },
      '&.cm-focused': { outline: 'none' },
      '.cm-scroller': { overflow: 'auto', fontFamily: 'ui-monospace, "SF Mono", "JetBrains Mono", Menlo, Consolas, monospace', fontSize: '12.5px', lineHeight: '1.6' },
      '.cm-content': { minWidth: '100%', caretColor: 'var(--c-fg)' },
      '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--c-fg)' },
      '.cm-gutters': { backgroundColor: 'var(--c-surface)', color: 'var(--c-subtle)', borderRight: '1px solid var(--c-line)' },
      '.cm-activeLine': { backgroundColor: 'color-mix(in oklab, var(--c-surface-2) 70%, transparent)' },
      '.cm-activeLineGutter': { backgroundColor: 'var(--c-surface-2)', color: 'var(--c-muted)' },
      '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
        { backgroundColor: 'color-mix(in oklab, var(--c-accent) 28%, transparent)' },
      '.cm-matchingBracket, &.cm-focused .cm-matchingBracket': { backgroundColor: 'color-mix(in oklab, var(--c-accent) 20%, transparent)', outline: 'none' },
      '.cm-foldPlaceholder': { backgroundColor: 'var(--c-surface-2)', border: '1px solid var(--c-line)', color: 'var(--c-muted)' },
    });

    const extensions = [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      history(),
      bracketMatching(),
      foldGutter(),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      highlightCompartment.of(highlightFor(isDark)),
      getLanguageExtension(language),
      chromeTheme,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          const newContent = update.state.doc.toString();
          content = newContent;
          value = newContent;
          onChange(newContent);
        }
      }),
    ];

    if (readOnly) extensions.push(EditorState.readOnly.of(true));

    editorView = new EditorView({
      state: EditorState.create({ doc: content || '', extensions }),
      parent: editorContainer,
    });

    initialized = true;
  });

  onDestroy(() => {
    editorView?.destroy();
  });

  // Follow the app theme (only the syntax colors change; chrome is CSS-variable based).
  $effect(() => {
    const dark = isDark;
    editorView?.dispatch({ effects: highlightCompartment.reconfigure(highlightFor(dark)) });
  });

  // Sync external content changes
  $effect(() => {
    if (editorView && initialized) {
      const currentContent = editorView.state.doc.toString();
      if (content !== currentContent && content !== undefined) {
        editorView.dispatch({
          changes: { from: 0, to: currentContent.length, insert: content },
        });
      }
    }
  });

  // Sync value prop
  $effect(() => {
    if (value && value !== content) {
      content = value;
    }
  });
</script>

<div class="flex h-full w-full flex-col overflow-hidden rounded-lg bg-surface">
  {#if !hideHeader}
    <div class="flex shrink-0 items-center justify-between gap-2 border-b border-line px-3 py-2">
      <span class="truncate font-mono text-xs text-fg">{filename || 'Untitled'}</span>
      <div class="flex items-center gap-2">
        <span class="badge">{language}</span>
        <button class="btn btn-primary btn-xs" onclick={() => onSave(content)} disabled={readOnly}>
          <Save size={12} /> Save
        </button>
      </div>
    </div>
  {/if}

  <div class="min-h-0 flex-1 overflow-hidden" bind:this={editorContainer}></div>

  {#if !hideFooter}
    <div class="flex shrink-0 gap-4 border-t border-line px-3 py-1.5 text-xs text-subtle tabular-nums">
      <span>Lines: {content.split('\n').length}</span>
      <span>Chars: {content.length.toLocaleString()}</span>
    </div>
  {/if}
</div>
