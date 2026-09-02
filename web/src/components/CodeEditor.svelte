<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view';
  import { EditorState } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { bracketMatching, foldGutter, syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';
  import { php } from '@codemirror/lang-php';
  import { javascript } from '@codemirror/lang-javascript';
  import { html } from '@codemirror/lang-html';
  import { css } from '@codemirror/lang-css';
  import { json } from '@codemirror/lang-json';
  import { python } from '@codemirror/lang-python';
  import { oneDark } from '@codemirror/theme-one-dark';

  let {
    content = '',
    value = '',
    language = 'text',
    filename = '',
    onSave = (content: string) => {},
    onChange = (content: string) => {},
    readOnly = false,
    theme = 'dark',
    height = '100%',
    hideHeader = false,
    hideFooter = false,
  } = $props();

  let editorContainer: HTMLDivElement;
  let onChangeRef = $state(onChange);
  let editorView: EditorView | undefined;
  let initialized = false;

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

  onMount(() => {
    if (!editorContainer) return;

    const extensions = [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      history(),
      bracketMatching(),
      foldGutter(),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      syntaxHighlighting(defaultHighlightStyle),
      getLanguageExtension(language),
      oneDark,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          const newContent = update.state.doc.toString();
          content = newContent;
          value = newContent;
          onChangeRef(newContent);
        }
      }),
      EditorView.theme({
        '&': { height: height, width: '100%' },
        '.cm-scroller': { overflow: 'auto', fontFamily: "'Fira Code', 'Cascadia Code', monospace", fontSize: '12px' },
        '.cm-content': { minWidth: '100%' },
        '.cm-gutters': { borderRight: '1px solid rgba(255,255,255,0.06)' },
      }),
    ];

    if (readOnly) extensions.push(EditorState.readOnly.of(true));

    const state = EditorState.create({
      doc: content || '',
      extensions,
    });

    editorView = new EditorView({
      state,
      parent: editorContainer,
    });

    initialized = true;
  });

  onDestroy(() => {
    editorView?.destroy();
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

<div class="editor-root">
  {#if !hideHeader}
    <div class="editor-header">
      <div class="file-info">
        <span class="filename">{filename || 'Untitled'}</span>
      </div>
      <div class="editor-actions">
        <span class="language-badge">{language}</span>
        <button class="save-btn" onclick={() => onSave(content)} disabled={readOnly}>
          Save
        </button>
      </div>
    </div>
  {/if}

  <div class="editor-wrapper" bind:this={editorContainer}></div>

  {#if !hideFooter}<div class="editor-footer">
    <span>Lines: {content.split('\n').length}</span>
    <span>Chars: {content.length.toLocaleString()}</span>
  </div>{/if}
</div>

<style>
  .editor-root {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
    background: #1e1e2e;
    border-radius: 0.5rem;
    overflow: hidden;
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid rgba(255,255,255,0.06);
    background: rgba(255,255,255,0.02);
    flex-shrink: 0;
  }

  .file-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .filename {
    font-size: 0.75rem;
    font-weight: 500;
    color: #e2e8f0;
  }

  .editor-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .language-badge {
    font-size: 0.6rem;
    padding: 0.15rem 0.5rem;
    border-radius: 0.25rem;
    background: rgba(99, 102, 241, 0.15);
    color: #818cf8;
    font-weight: 500;
  }

  .save-btn {
    font-size: 0.7rem;
    padding: 0.25rem 0.75rem;
    border-radius: 0.375rem;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    color: white;
    border: none;
    cursor: pointer;
    font-weight: 500;
  }

  .save-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .editor-wrapper {
    flex: 1;
    overflow: hidden;
    min-height: 0;
  }

  .editor-footer {
    display: flex;
    gap: 1rem;
    padding: 0.35rem 0.75rem;
    border-top: 1px solid rgba(255,255,255,0.06);
    background: rgba(255,255,255,0.02);
    font-size: 0.65rem;
    color: #64748b;
    flex-shrink: 0;
  }
</style>
