<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount, onDestroy } from 'svelte';
  const serverId = $page.params.id;

  let termEl: HTMLDivElement;
  let term: any;
  let fitAddon: any;
  let ws: WebSocket | null = null;
  let connected = $state(false);
  let disposed = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let hostname = $state('server-' + serverId);
  let sessionId = $state('');

  onMount(async () => {
    if (!auth.token) { window.location.href = '/login'; return; }

    const { Terminal } = await import('@xterm/xterm');
    const { FitAddon } = await import('@xterm/addon-fit');
    await import('@xterm/xterm/css/xterm.css');

    term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 14,
      fontFamily: "'JetBrains Mono','Fira Code','SF Mono',monospace",
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#ffffff',
        cursorAccent: '#0d1117',
        selectionBackground: '#264f78',
        black: '#0d1117', red: '#f85149', green: '#3fb950', yellow: '#d29922',
        blue: '#58a6ff', magenta: '#bc8cff', cyan: '#39c5cf', white: '#c9d1d9',
      },
      scrollback: 5000,
      allowProposedApi: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termEl);
    fitAddon.fit();
    term.focus();

    connectWS();

    function sendResize() {
      if (ws?.readyState === WebSocket.OPEN && term) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    }

    term.onData((data: string) => {
      if (ws?.readyState === WebSocket.OPEN) ws.send(data);
    });
    term.onResize(() => sendResize());

    // ResizeObserver: only resize on initial load and orientation changes
    // Never resize while connected — it causes character loss on mobile
    let initialFitDone = false;
    new ResizeObserver(() => {
      if (disposed) return;
      // Only fit on very first observation (initial layout)
      if (!initialFitDone) {
        initialFitDone = true;
        try { fitAddon?.fit(); } catch {}
        sendResize();
      }
    }).observe(termEl);
  });

  async function connectWS() {
    if (disposed) return;
    if (ws) { ws.onclose = null; ws.close(); ws = null; }
    // The browser cannot send an Authorization header on a WebSocket, so trade
    // the session token for a single-use ticket first.
    let ticket: { ticket: string; session: string };
    try {
      ticket = await apiFetch(auth.token, 'POST', 'terminal/tickets', {
        server_id: Number(serverId),
        session: sessionId || undefined,
      });
    } catch (e: any) {
      term?.write(`\r\n\x1b[31m[Terminal unavailable: ${e?.message || 'denied'}]\x1b[0m\r\n`);
      if (!disposed) reconnectTimer = setTimeout(connectWS, 5000);
      return;
    }
    sessionId = ticket.session;
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${window.location.host}/api/v1/ws/terminal?ticket=${encodeURIComponent(ticket.ticket)}`);
    ws.binaryType = 'arraybuffer';
    ws.onopen = () => {
      connected = true;
      term?.focus();
      ws?.send(JSON.stringify({ type: 'resize', cols: term?.cols || 120, rows: term?.rows || 40 }));
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
      if (!disposed) {
        term?.write('\r\n\x1b[38;5;245m[Disconnected. Reconnecting...]\x1b[0m\r\n');
        reconnectTimer = setTimeout(connectWS, 3000);
      }
    };
    ws.onerror = () => { connected = false; };
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

<div class="terminal-page">
  <div class="term-top">
    <div class="term-tabs">
      <button class="tab active">
        <svg class="tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
        <span>root@{hostname}</span>
      </button>
    </div>
    <div class="term-actions">
      <button class="act-btn" onclick={() => { term?.clear(); term?.write('\x1b[H'); }} title="Clear">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
      </button>
      <a href="/servers" class="act-btn" title="Close">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </a>
    </div>
  </div>
  <div class="term-body" bind:this={termEl} onclick={() => term?.focus()}></div>
  <div class="term-keys">
    <div class="keys-row">
      <button class="key-btn" onclick={() => sendCtrl('c')}>Ctrl+C</button>
      <button class="key-btn" onclick={() => sendCtrl('z')}>Ctrl+Z</button>
      <button class="key-btn" onclick={() => sendCtrl('d')}>Ctrl+D</button>
      <button class="key-btn" onclick={() => sendCtrl('l')}>Ctrl+L</button>
      <button class="key-btn" onclick={() => sendCtrl('u')}>Ctrl+U</button>
    </div>
    <div class="keys-row">
      <button class="key-btn" onclick={() => sendKey('\t')}>Tab</button>
      <button class="key-btn" onclick={() => sendKey('\x1b')}>Esc</button>
      <button class="key-btn" onclick={() => sendKey('\x1b[A')}>↑</button>
      <button class="key-btn" onclick={() => sendKey('\x1b[B')}>↓</button>
      <button class="key-btn" onclick={() => sendKey('\x7f')}>⌫</button>
    </div>
  </div>
  <div class="term-status">
    <div class="status-left">
      <span class="status-dot" class:connected></span>
      <span>{connected ? 'PTY Connected' : 'Connecting...'}</span>
      <span class="sep">•</span>
      <span>{hostname}</span>
    </div>
    <div class="status-right">
      <span class="key">TUI</span> Supported
    </div>
  </div>
</div>

<style>
  .terminal-page {
    background: #0d1117; border: 1px solid rgba(255,255,255,.06);
    border-radius: 12px; overflow: hidden;
    box-shadow: 0 8px 40px rgba(0,0,0,.5);
  }
  .term-top {
    display: flex; align-items: center; justify-content: space-between;
    background: #161b22; border-bottom: 1px solid rgba(255,255,255,.04);
    padding: 0 8px; height: 38px;
  }
  .term-tabs { display: flex; align-items: center; height: 100%; }
  .tab {
    display: flex; align-items: center; gap: 6px;
    height: 28px; padding: 0 12px;
    background: #0d1117; border: none; border-radius: 6px 6px 0 0;
    color: #e6edf3; font-size: .7rem;
    font-family: 'JetBrains Mono',monospace;
    box-shadow: 0 -1px 0 #30363d inset;
  }
  .tab-icon { width: 13px; height: 13px; color: #58a6ff; }
  .term-actions { display: flex; gap: 2px; }
  .act-btn {
    display: flex; align-items: center; justify-content: center;
    width: 26px; height: 26px; border: none; background: transparent;
    color: #8b949e; cursor: pointer; border-radius: 6px;
    transition: all .15s; text-decoration: none;
  }
  .act-btn:hover { background: rgba(255,255,255,.06); color: #e6edf3; }
  .term-body { height: calc(100vh - 260px); min-height: 300px; cursor: text; }
  .term-keys {
    display: flex; flex-direction: column; gap: 3px; padding: 5px 8px;
    background: #161b22; border-top: 1px solid rgba(255,255,255,.04);
    position: sticky; bottom: 0; z-index: 50;
  }
  .keys-row { display: flex; gap: 4px; }
  .key-btn {
    flex: 1; height: 30px; border: 1px solid rgba(255,255,255,.08);
    background: rgba(255,255,255,.04); color: #8b949e;
    border-radius: 6px; font-size: .6rem; font-weight: 600;
    font-family: 'JetBrains Mono',monospace; cursor: pointer; transition: all .1s;
  }
  .key-btn:active { background: rgba(88,166,255,.2); color: #58a6ff; }
  .term-status {
    display: flex; align-items: center; justify-content: space-between;
    height: 26px; padding: 0 12px;
    background: #161b22; border-top: 1px solid rgba(255,255,255,.04);
    font-size: .6rem; color: #484f58; font-family: 'JetBrains Mono',monospace;
  }
  .status-left { display: flex; align-items: center; gap: 6px; }
  .status-dot { width: 5px; height: 5px; border-radius: 50%; background: #f85149; transition: background .3s; }
  .status-dot.connected { background: #3fb950; }
  .key { color: #3fb950; font-weight: 600; }
  .sep { color: #30363d; }
  .status-right { display: flex; align-items: center; gap: 6px; }
  @media (max-width: 768px) {
    .term-top { height: 32px; }
    .tab { font-size: .6rem; padding: 0 8px; height: 24px; }
    .act-btn { width: 22px; height: 22px; }
    .term-body { min-height: 250px; }
    .status-right { display: none; }
  }
</style>
