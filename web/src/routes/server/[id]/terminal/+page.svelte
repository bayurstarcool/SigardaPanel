<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { page } from '$app/stores';
  import { onMount, onDestroy } from 'svelte';
  import { theme } from '$lib/theme.svelte';
  import { ChevronLeft, SquareTerminal, Eraser, X, ArrowUp, ArrowDown, Delete } from 'lucide-svelte';
  const serverId = $page.params.id;

  // ANSI palettes per app theme; chrome colors come from the design tokens.
  const ansiDark = {
    black: '#0d1117', red: '#f85149', green: '#3fb950', yellow: '#d29922',
    blue: '#58a6ff', magenta: '#bc8cff', cyan: '#39c5cf', white: '#c9d1d9',
  };
  const ansiLight = {
    black: '#24292f', red: '#cf222e', green: '#116329', yellow: '#9a6700',
    blue: '#0969da', magenta: '#8250df', cyan: '#1b7c83', white: '#6e7781',
  };

  function xtermTheme() {
    const css = getComputedStyle(document.documentElement);
    const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback;
    const bg = v('--c-surface', '#111113');
    const accent = v('--c-accent', '#6d7cff');
    return {
      background: bg,
      foreground: v('--c-fg', '#f4f4f5'),
      cursor: accent,
      cursorAccent: bg,
      // #RRGGBB + alpha — xterm accepts 8-digit hex.
      selectionBackground: accent.length === 7 ? accent + '55' : accent,
      ...(theme.isLight ? ansiLight : ansiDark),
    };
  }

  // Follow the app theme without re-creating the terminal.
  $effect(() => {
    theme.current;
    if (term) term.options.theme = xtermTheme();
  });

  let resizeObserver: ResizeObserver | null = null;

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
    if (!auth.user) { window.location.href = '/login'; return; }

    const { Terminal } = await import('@xterm/xterm');
    const { FitAddon } = await import('@xterm/addon-fit');
    await import('@xterm/xterm/css/xterm.css');

    term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 14,
      fontFamily: "'JetBrains Mono','Fira Code','SF Mono',monospace",
      theme: xtermTheme(),
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
    resizeObserver = new ResizeObserver(() => {
      if (disposed) return;
      // Only fit on very first observation (initial layout)
      if (!initialFitDone) {
        initialFitDone = true;
        try { fitAddon?.fit(); } catch {}
        sendResize();
      }
    });
    resizeObserver.observe(termEl);
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
    resizeObserver?.disconnect();
    ws?.close();
    term?.dispose();
  });
</script>

<!-- Standalone (no DashboardLayout): the layout's global Ctrl+K shortcut would
     steal a key the shell needs. -->
<div class="flex min-h-dvh flex-col bg-bg p-3 sm:p-4">
  <div class="mb-3 flex items-center justify-between gap-3">
    <a href="/servers" class="inline-flex items-center gap-1 text-[13px] text-subtle transition-colors hover:text-fg">
      <ChevronLeft size={14} /> Servers
    </a>
    <a href="/server/{serverId}/fail2ban" class="link text-[13px]">Fail2ban</a>
  </div>

  <div class="card flex flex-1 flex-col overflow-hidden">
    <div class="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-line bg-surface-2 px-2 max-md:h-9">
      <div class="inline-flex h-7 min-w-0 items-center gap-1.5 rounded-md bg-surface px-3 font-mono text-xs text-fg ring-1 ring-line ring-inset">
        <SquareTerminal size={13} class="shrink-0 text-accent" />
        <span class="truncate">root@{hostname}</span>
      </div>
      <div class="flex gap-0.5">
        <button class="btn btn-ghost btn-icon btn-xs" onclick={() => { term?.clear(); term?.write('\x1b[H'); }} title="Clear" aria-label="Clear terminal">
          <Eraser size={14} />
        </button>
        <a href="/servers" class="btn btn-ghost btn-icon btn-xs" title="Close" aria-label="Close terminal">
          <X size={14} />
        </a>
      </div>
    </div>

    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div
      class="h-[calc(100dvh-230px)] min-h-[300px] cursor-text bg-surface px-1 pt-1 max-md:min-h-[250px]"
      bind:this={termEl}
      onclick={() => term?.focus()}
    ></div>

    <div class="sticky bottom-0 z-10 flex shrink-0 flex-col gap-1 border-t border-line bg-surface-2 px-2 py-1.5">
      <div class="flex gap-1">
        {#each ['c', 'z', 'd', 'l', 'u'] as ch (ch)}
          <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendCtrl(ch)}>Ctrl+{ch.toUpperCase()}</button>
        {/each}
      </div>
      <div class="flex gap-1">
        <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendKey('\t')}>Tab</button>
        <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendKey('\x1b')}>Esc</button>
        <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendKey('\x1b[A')} aria-label="Arrow up"><ArrowUp size={13} /></button>
        <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendKey('\x1b[B')} aria-label="Arrow down"><ArrowDown size={13} /></button>
        <button class="btn btn-secondary btn-xs flex-1 font-mono font-semibold active:bg-accent/15 active:text-accent" onclick={() => sendKey('\x7f')} aria-label="Backspace"><Delete size={13} /></button>
      </div>
    </div>

    <div class="flex h-7 shrink-0 items-center justify-between border-t border-line bg-surface-2 px-3 font-mono text-[11px] text-subtle">
      <div class="flex min-w-0 items-center gap-2">
        <span class="dot {connected ? 'text-success' : 'text-danger'}"></span>
        <span>{connected ? 'PTY connected' : 'Connecting…'}</span>
        <span class="text-line-strong">·</span>
        <span class="truncate">{hostname}</span>
      </div>
      <div class="max-md:hidden"><span class="font-semibold text-success">TUI</span> supported</div>
    </div>
  </div>
</div>
