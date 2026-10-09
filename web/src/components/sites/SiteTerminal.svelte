<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { onMount, onDestroy } from 'svelte';
  import { Terminal as TerminalIcon, Maximize2, Minimize2, Power, Loader2 } from 'lucide-svelte';

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

  function connect() {
    if (disposed || connecting) return;
    connecting = true;

    // Dynamic import xterm on first use
    if (!term) {
      initTerminal().then(() => doConnect());
    } else {
      doConnect();
    }
  }

  async function initTerminal() {
    const { Terminal } = await import('@xterm/xterm');
    const { FitAddon } = await import('@xterm/addon-fit');
    await import('@xterm/xterm/css/xterm.css');

    term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 13,
      fontFamily: "'JetBrains Mono','Fira Code','SF Mono',monospace",
      theme: {
        background: '#0a0e14',
        foreground: '#c9d1d9',
        cursor: '#ffffff',
        cursorAccent: '#0a0e14',
        selectionBackground: '#264f78',
        black: '#0d1117', red: '#f85149', green: '#3fb950', yellow: '#d29922',
        blue: '#58a6ff', magenta: '#bc8cff', cyan: '#39c5cf', white: '#c9d1d9',
      },
      scrollback: 3000,
      allowProposedApi: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termEl);

    term.onData((data: string) => {
      if (ws?.readyState === WebSocket.OPEN) ws.send(data);
    });
    term.onResize(() => {
      if (ws?.readyState === WebSocket.OPEN && term) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    });
  }

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

  function toggleExpand() {
    expanded = !expanded;
    if (expanded && term) {
      // Fit after DOM update
      setTimeout(() => { fitAddon?.fit(); }, 50);
    }
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

<div class="terminal-wrapper" class:expanded>
  <div class="terminal-header">
    <div class="header-left">
      <TerminalIcon size={14} class="text-sky-400" />
      <span class="header-title">SSH Terminal</span>
      <span class="header-user">{getSystemUser()}@server</span>
      {#if connected}
        <span class="status-badge connected">Connected</span>
      {:else if connecting}
        <span class="status-badge connecting">Connecting...</span>
      {:else}
        <span class="status-badge disconnected">Disconnected</span>
      {/if}
    </div>
    <div class="header-actions">
      {#if !expanded}
        {#if !connected && !connecting}
          <button onclick={connect} class="action-btn connect-btn">
            <Power size={12} /> Connect
          </button>
        {:else}
          <button onclick={disconnect} class="action-btn disconnect-btn">
            <Power size={12} /> Disconnect
          </button>
        {/if}
      {/if}
      <button onclick={toggleExpand} class="action-btn" title={expanded ? 'Collapse' : 'Expand'}>
        {#if expanded}<Minimize2 size={13} />{:else}<Maximize2 size={13} />{/if}
      </button>
    </div>
  </div>

  {#if expanded}
    <div class="terminal-body" bind:this={termEl} onclick={() => term?.focus()}></div>
    <div class="terminal-keys">
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
  {:else}
    <!-- Collapsed terminal preview -->
    <div class="terminal-collapsed">
      <div class="collapsed-text">
        {#if connected}
          <span class="text-emerald-400">●</span> Terminal active — <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">ssh {getSystemUser()}@server</code>
        {:else}
          Click <strong>Connect</strong> to open a terminal as <code class="text-sky-300 bg-black/30 px-1.5 py-0.5 rounded">{getSystemUser()}</code> on this site's server.
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .terminal-wrapper {
    background: #0a0e14;
    border: 1px solid rgba(255,255,255,0.07);
    border-radius: 16px;
    overflow: hidden;
    transition: all 0.2s;
  }
  .terminal-wrapper.expanded {
    box-shadow: 0 8px 40px rgba(0,0,0,0.5);
  }
  .terminal-header {
    display: flex; align-items: center; justify-content: space-between;
    background: #161b22; border-bottom: 1px solid rgba(255,255,255,0.04);
    padding: 0 12px; height: 38px;
  }
  .header-left {
    display: flex; align-items: center; gap: 8px;
    font-family: 'JetBrains Mono', monospace;
  }
  .header-title {
    font-size: 0.75rem; font-weight: 600; color: #e6edf3;
  }
  .header-user {
    font-size: 0.65rem; color: #8b949e; background: rgba(255,255,255,0.04);
    padding: 2px 8px; border-radius: 6px;
  }
  .status-badge {
    font-size: 0.6rem; padding: 2px 8px; border-radius: 999px; font-weight: 500;
  }
  .status-badge.connected { background: rgba(63,185,80,0.15); color: #3fb950; }
  .status-badge.connecting { background: rgba(210,153,34,0.15); color: #d29922; }
  .status-badge.disconnected { background: rgba(248,81,73,0.15); color: #f85149; }
  .header-actions { display: flex; gap: 4px; align-items: center; }
  .action-btn {
    display: flex; align-items: center; gap: 4px;
    height: 26px; padding: 0 10px; border: 1px solid rgba(255,255,255,0.08);
    background: rgba(255,255,255,0.04); color: #8b949e;
    border-radius: 6px; font-size: 0.65rem; font-weight: 500;
    font-family: 'JetBrains Mono', monospace; cursor: pointer; transition: all 0.15s;
  }
  .action-btn:hover { background: rgba(255,255,255,0.08); color: #e6edf3; }
  .connect-btn { color: #3fb950; border-color: rgba(63,185,80,0.2); }
  .connect-btn:hover { background: rgba(63,185,80,0.12); }
  .disconnect-btn { color: #f85149; border-color: rgba(248,81,73,0.2); }
  .disconnect-btn:hover { background: rgba(248,81,73,0.12); }
  .terminal-body {
    height: 400px; cursor: text;
  }
  .terminal-keys {
    display: flex; flex-direction: column; gap: 3px; padding: 5px 8px;
    background: #161b22; border-top: 1px solid rgba(255,255,255,0.04);
  }
  .keys-row { display: flex; gap: 4px; }
  .key-btn {
    flex: 1; height: 28px; border: 1px solid rgba(255,255,255,0.08);
    background: rgba(255,255,255,0.04); color: #8b949e;
    border-radius: 6px; font-size: 0.6rem; font-weight: 600;
    font-family: 'JetBrains Mono', monospace; cursor: pointer; transition: all 0.1s;
  }
  .key-btn:active { background: rgba(88,166,255,0.2); color: #58a6ff; }
  .terminal-collapsed {
    padding: 12px 16px;
  }
  .collapsed-text {
    font-size: 0.8rem; color: #8b949e; line-height: 1.5;
  }
  @media (max-width: 768px) {
    .terminal-body { height: 300px; }
    .header-user { display: none; }
  }
</style>
