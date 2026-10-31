<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { theme } from '$lib/theme.svelte';
  import { onDestroy, tick } from 'svelte';
  import { Terminal as TerminalIcon, Maximize2, Minimize2, Power, ArrowUp, ArrowDown, Delete } from 'lucide-svelte';

  let { siteId, site }: { siteId: number; site: any } = $props();

  let termEl: HTMLDivElement;
  let term: any;
  let fitAddon: any;
  let ws: WebSocket | null = null;
  let connected = $state(false);
  let connecting = $state(false);
  let disposed = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let sessionId = $state('');
  let expanded = $state(false);

  function getSystemUser(): string {
    return site?.system_user || 'root';
  }

  async function connect() {
    if (disposed || connecting) return;
    connecting = true;
    // The terminal needs a visible container to measure itself.
    expanded = true;
    await tick();
    try {
      // xterm is imported on first use only.
      if (!term) await initTerminal();
    } catch {
      connecting = false;
      return;
    }
    doConnect();
  }

  function cssVar(name: string, fallback: string): string {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  // Terminal colors follow the app theme: chrome from the design tokens, ANSI palette per mode.
  function xtermTheme(dark: boolean) {
    const bg = cssVar('--c-surface', dark ? '#111113' : '#ffffff');
    const fg = cssVar('--c-fg', dark ? '#f4f4f5' : '#18181b');
    const ansi = dark
      ? { black: '#18181b', red: '#f87171', green: '#34d399', yellow: '#fbbf24', blue: '#60a5fa', magenta: '#c084fc', cyan: '#22d3ee', white: '#e4e4e7',
          brightBlack: '#71717a', brightRed: '#fca5a5', brightGreen: '#6ee7b7', brightYellow: '#fcd34d', brightBlue: '#93c5fd', brightMagenta: '#d8b4fe', brightCyan: '#67e8f9', brightWhite: '#fafafa' }
      : { black: '#18181b', red: '#dc2626', green: '#059669', yellow: '#b45309', blue: '#2563eb', magenta: '#9333ea', cyan: '#0891b2', white: '#a1a1aa',
          brightBlack: '#52525b', brightRed: '#ef4444', brightGreen: '#10b981', brightYellow: '#d97706', brightBlue: '#3b82f6', brightMagenta: '#a855f7', brightCyan: '#06b6d4', brightWhite: '#71717a' };
    return {
      background: bg,
      foreground: fg,
      cursor: fg,
      cursorAccent: bg,
      selectionBackground: dark ? 'rgba(109,124,255,0.35)' : 'rgba(79,93,240,0.25)',
      ...ansi,
    };
  }

  async function initTerminal() {
    const { Terminal } = await import('@xterm/xterm');
    const { FitAddon } = await import('@xterm/addon-fit');
    await import('@xterm/xterm/css/xterm.css');

    term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 13,
      fontFamily: "ui-monospace,'SF Mono','JetBrains Mono',Menlo,Consolas,monospace",
      theme: xtermTheme(theme.isDark),
      scrollback: 3000,
      allowProposedApi: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termEl);
    fitAddon.fit();

    term.onData((data: string) => {
      if (ws?.readyState === WebSocket.OPEN) ws.send(data);
    });
    term.onResize(() => {
      if (ws?.readyState === WebSocket.OPEN && term) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    });
  }

  // Re-theme a live terminal when the app theme changes.
  $effect(() => {
    const dark = theme.isDark;
    if (term) term.options.theme = xtermTheme(dark);
  });

  async function doConnect() {
    if (disposed) return;
    if (ws) { ws.onclose = null; ws.close(); ws = null; }

    // Single-use ticket; the server decides which unix user the shell runs as.
    let ticket: { ticket: string; session: string };
    try {
      ticket = await apiFetch(auth.token, 'POST', 'terminal/tickets', {
        site_id: siteId,
        session: sessionId || undefined,
      });
    } catch (e: any) {
      connecting = false;
      term?.write(`\r\n\x1b[31m[Terminal unavailable: ${e?.message || 'denied'}]\x1b[0m\r\n`);
      return;
    }
    sessionId = ticket.session;
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${window.location.host}/api/v1/ws/terminal?ticket=${encodeURIComponent(ticket.ticket)}`);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      connected = true;
      connecting = false;
      term?.focus();
      ws?.send(JSON.stringify({ type: 'resize', cols: term?.cols || 120, rows: term?.rows || 30 }));
    };

    ws.onmessage = (event: MessageEvent) => {
      const data = event.data instanceof ArrayBuffer ? new Uint8Array(event.data) : event.data;
      const text = typeof data === 'string' ? data : new TextDecoder().decode(data);
      const clean = text.replace(/\0/g, '');
      if (clean.startsWith('{')) {
        try {
          const msg = JSON.parse(clean);
          if (msg.type === 'session' && msg.id) {
            sessionId = msg.id;
            return;
          }
        } catch {}
      }
      term?.write(data);
    };

    ws.onclose = () => {
      connected = false;
      connecting = false;
      if (!disposed && expanded) {
        term?.write('\r\n\x1b[38;5;245m[Disconnected. Reconnecting in 3s...]\x1b[0m\r\n');
        reconnectTimer = setTimeout(doConnect, 3000);
      }
    };

    ws.onerror = () => { connected = false; };
  }

  function disconnect() {
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    connected = false;
    connecting = false;
    sessionId = '';
  }

  async function toggleExpand() {
    expanded = !expanded;
    if (expanded && term) {
      // Fit after the container is visible again.
      await tick();
      fitAddon?.fit();
    }
  }

  function onWindowResize() {
    if (expanded && term) fitAddon?.fit();
  }

  function sendCtrl(ch: string) {
    if (ws?.readyState === WebSocket.OPEN) ws.send(String.fromCharCode(ch.charCodeAt(0) - 96));
    term?.focus();
  }
  function sendKey(seq: string) {
    if (ws?.readyState === WebSocket.OPEN) ws.send(seq);
    term?.focus();
  }

  onDestroy(() => {
    disposed = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    ws?.close();
    term?.dispose();
  });
</script>

<svelte:window onresize={onWindowResize} />

<div class="card overflow-hidden">
  <div class="flex h-11 items-center justify-between gap-2 border-b border-line px-4">
    <div class="flex min-w-0 items-center gap-2">
      <TerminalIcon size={16} class="shrink-0 text-subtle" />
      <span class="card-title">SSH terminal</span>
      <span class="code max-md:hidden">{getSystemUser()}@server</span>
      {#if connected}
        <span class="badge badge-success"><span class="dot"></span>Connected</span>
      {:else if connecting}
        <span class="badge badge-warning"><span class="dot animate-pulse"></span>Connecting…</span>
      {:else}
        <span class="badge max-sm:hidden"><span class="dot"></span>Disconnected</span>
      {/if}
    </div>
    <div class="flex shrink-0 items-center gap-1">
      {#if connected || connecting}
        <button onclick={disconnect} class="btn btn-danger-ghost btn-xs"><Power size={12} /> Disconnect</button>
      {:else}
        <button onclick={connect} class="btn btn-secondary btn-xs"><Power size={12} /> Connect</button>
      {/if}
      <button onclick={toggleExpand} class="btn btn-ghost btn-icon btn-xs" aria-label={expanded ? 'Collapse terminal' : 'Expand terminal'} title={expanded ? 'Collapse' : 'Expand'}>
        {#if expanded}<Minimize2 size={14} />{:else}<Maximize2 size={14} />{/if}
      </button>
    </div>
  </div>

  <!-- Kept mounted so the xterm instance survives collapse/expand. -->
  <div class={expanded ? '' : 'hidden'}>
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="h-[300px] cursor-text bg-surface p-2 md:h-[400px]" bind:this={termEl} onclick={() => term?.focus()}></div>
    <div class="grid grid-cols-5 gap-1 border-t border-line bg-surface-2 p-1.5 font-mono">
      <button class="btn btn-secondary btn-xs" onclick={() => sendCtrl('c')}>Ctrl+C</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendCtrl('z')}>Ctrl+Z</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendCtrl('d')}>Ctrl+D</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendCtrl('l')}>Ctrl+L</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendCtrl('u')}>Ctrl+U</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendKey('\t')}>Tab</button>
      <button class="btn btn-secondary btn-xs" onclick={() => sendKey('\x1b')}>Esc</button>
      <button class="btn btn-secondary btn-xs" aria-label="Arrow up" onclick={() => sendKey('\x1b[A')}><ArrowUp size={12} /></button>
      <button class="btn btn-secondary btn-xs" aria-label="Arrow down" onclick={() => sendKey('\x1b[B')}><ArrowDown size={12} /></button>
      <button class="btn btn-secondary btn-xs" aria-label="Backspace" onclick={() => sendKey('\x7f')}><Delete size={12} /></button>
    </div>
  </div>

  {#if !expanded}
    <p class="px-4 py-3 text-[13px] text-muted">
      {#if connected}
        <span class="inline-flex items-center gap-1.5 text-success"><span class="dot"></span>Terminal active</span> — <span class="code">ssh {getSystemUser()}@server</span>
      {:else}
        Click <strong class="font-medium text-fg">Connect</strong> to open a terminal as <span class="code">{getSystemUser()}</span> on this site's server.
      {/if}
    </p>
  {/if}
</div>
