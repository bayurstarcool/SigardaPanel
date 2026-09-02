# SSH Terminal — WebSocket + PTY + xterm.js

> **For Hermes:** Implement this plan task-by-task.

**Goal:** Full interactive SSH terminal in the browser using WebSocket, PTY, and xterm.js.

**Architecture:**
```
Frontend (xterm.js)
  → WebSocket → SvelteKit route (/api/ws/terminal/:id)
  → WebSocket proxy → Agent (/ws/terminal)
  → PTY (creack/pty) → /bin/bash
```

**Tech Stack:** gorilla/websocket, creack/pty, xterm.js, xterm-addon-fit

---

## Task 1: Install Dependencies

**Files:**
- Modify: `go.mod`
- Modify: `web/package.json`

**Steps:**

```bash
# Go WebSocket + PTY
cd /root/SigardaPanel-Enterprise
export PATH=/usr/local/go/bin:$PATH
go get github.com/gorilla/websocket@latest
go get github.com/creack/pty@latest

# Frontend xterm.js
cd web
npm install xterm @xterm/xterm @xterm/addon-fit @xterm/addon-web-links
```

**Verify:**
```bash
go build ./...
cd web && npm run build
```

---

## Task 2: Agent — WebSocket Terminal Handler

**Files:**
- Create: `internal/agenthttp/terminal.go`
- Modify: `internal/agenthttp/router.go` — register route

**Route:** `GET /ws/terminal` (WebSocket upgrade)

**Implementation:**

```go
// internal/agenthttp/terminal.go
package agenthttp

import (
    "context"
    "io"
    "log/slog"
    "net/http"
    "os"
    "os/exec"
    "sync"

    "github.com/creack/pty"
    "github.com/gorilla/websocket"
    "github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func (r *Router) terminalWebSocket(c echo.Context) error {
    // Auth check
    if err := r.requireAgentAuth(c); err != nil {
        return err
    }

    ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
    if err != nil {
        return err
    }
    defer ws.Close()

    // Create PTY
    shell := os.Getenv("SHELL")
    if shell == "" {
        shell = "/bin/bash"
    }
    cmd := exec.Command(shell)
    cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS=120", "LINES=40")

    ptmx, err := pty.Start(cmd)
    if err != nil {
        ws.WriteMessage(websocket.TextMessage, []byte("Failed to start shell: "+err.Error()))
        return err
    }
    defer ptmx.Close()

    // Handle resize messages
    // Handle terminal I/O
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    var mu sync.Mutex

    // PTY → WebSocket
    go func() {
        buf := make([]byte, 4096)
        for {
            select {
            case <-ctx.Done():
                return
            default:
                n, err := ptmx.Read(buf)
                if err != nil {
                    ws.WriteMessage(websocket.CloseMessage,
                        websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
                    cancel()
                    return
                }
                mu.Lock()
                ws.WriteMessage(websocket.BinaryMessage, buf[:n])
                mu.Unlock()
            }
        }
    }()

    // WebSocket → PTY
    for {
        messageType, message, err := ws.ReadMessage()
        if err != nil {
            cancel()
            break
        }
        switch messageType {
        case websocket.TextMessage:
            // Check for resize command
            // Format: {"type":"resize","cols":120,"rows":40}
            // Or regular input text
            if len(message) > 0 && message[0] == '{' {
                // Try parse as JSON resize command
                // If not resize, treat as input
                ptmx.Write(message)
            } else {
                ptmx.Write(message)
            }
        case websocket.BinaryMessage:
            ptmx.Write(message)
        }
    }

    cmd.Process.Wait()
    return nil
}
```

**Register route in router.go:**
```go
// In newRouter or similar setup function
r.router.GET("/ws/terminal", r.terminalWebSocket)
```

---

## Task 3: SvelteKit WebSocket Proxy

**Files:**
- Create: `web/src/routes/api/ws/terminal/[id]/+server.ts`

**Implementation:**

SvelteKit doesn't natively proxy WebSockets well. Two options:
1. Use nginx to proxy WebSocket directly to agent
2. Use a custom SvelteKit endpoint that upgrades

**Best approach: nginx WebSocket proxy** (simpler, more reliable)

Add to nginx config:
```nginx
# WebSocket terminal proxy
location /api/ws/ {
    proxy_pass http://127.0.0.1:7710;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

But we need auth. Better approach: SvelteKit endpoint that validates token, then proxies WebSocket.

**Alternative: Direct agent WebSocket with token in query**

Agent endpoint: `GET /ws/terminal?token=AGENT_TOKEN`
Frontend connects: `wss://panel.sigarda.dev/api/ws/terminal/1?token=USER_TOKEN`

SvelteKit validates user token, then proxies to agent with agent token.

**Create SvelteKit WebSocket proxy:**

```typescript
// web/src/routes/api/ws/terminal/[id]/+server.ts
import type { RequestHandler } from './$types';
import { json } from '@sveltejs/kit';

const AGENT_BASE = 'http://localhost:7710';

export const GET: RequestHandler = async ({ params, url, request }) => {
  // Validate user auth from query or header
  const token = url.searchParams.get('token');
  if (!token) {
    return json({ error: 'token required' }, { status: 401 });
  }

  // TODO: Validate user token against session DB
  // For now, trust the token exists

  // Proxy WebSocket to agent
  const agentUrl = `${AGENT_BASE}/ws/terminal?token=${token}`;

  // SvelteKit can't proxy WebSocket natively
  // Return redirect to agent URL (client connects directly)
  return json({ agent_url: agentUrl });
};
```

**Actually, simplest approach:** Connect frontend directly to agent WebSocket.

Frontend: `wss://panel.sigarda.dev/ws/terminal?token=AGENT_TOKEN`

But this exposes agent token to frontend. Better: use nginx to proxy with auth header.

**Final approach: nginx WebSocket proxy + agent auth**

1. Agent WebSocket endpoint requires `Authorization: Bearer <agent_token>`
2. Frontend sends user token to SvelteKit endpoint
3. SvelteKit validates user, returns agent URL + token
4. Frontend connects WebSocket to agent URL

OR simpler: Just proxy through nginx with auth injection.

**Simplest working approach:**

Agent endpoint at `/ws/terminal` with agent token auth.
Frontend connects via nginx proxy with agent token in header.

But WebSocket can't set custom headers in browser. So use query param:

Agent: `GET /ws/terminal?token=AGENT_TOKEN`
Nginx proxies `/api/ws/` → agent
Frontend: `wss://panel.sigarda.dev/api/ws/terminal?token=AGENT_TOKEN`

**Problem:** Agent token exposed in URL (logged in nginx access log).

**Better:** SvelteKit server route that upgrades WebSocket and proxies.

This requires a custom Node.js WebSocket server alongside SvelteKit.

**Simplest working solution:**

1. Agent listens on port 7710 with WebSocket at `/ws/terminal`
2. Nginx proxies `/api/ws/` → port 7710 with auth header injection
3. Frontend connects to `/api/ws/terminal`

Nginx config:
```nginx
location /api/ws/ {
    proxy_pass http://127.0.0.1:7710/ws/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header Authorization "Bearer AGENT_TOKEN";
    proxy_read_timeout 3600s;
}
```

This works! Agent token is server-side only, never exposed to frontend.

---

## Task 4: Frontend — xterm.js Terminal

**Files:**
- Modify: `web/src/routes/server/[id]/terminal/+page.svelte`

**Replace** the current command-by-command terminal with xterm.js WebSocket terminal.

**Install xterm.js:**
```bash
cd web
npm install @xterm/xterm @xterm/addon-fit @xterm/addon-web-links
```

**Implementation:**

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import '@xterm/xterm/css/xterm.css';

  const serverId = $page.params.id;
  let terminalEl: HTMLDivElement;
  let term: Terminal;
  let ws: WebSocket;
  let fitAddon: FitAddon;

  onMount(() => {
    term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
      theme: {
        background: '#0a0e14',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        cursorAccent: '#0a0e14',
        selectionBackground: '#264f78',
      },
      allowProposedApi: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());

    term.open(terminalEl);
    fitAddon.fit();

    // Connect WebSocket
    connectWebSocket();

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          type: 'resize',
          cols: term.cols,
          rows: term.rows
        }));
      }
    });
    resizeObserver.observe(terminalEl);

    // Handle terminal input
    term.onData((data) => {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Handle terminal resize
    term.onResize(({ cols, rows }) => {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });
  });

  function connectWebSocket() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${window.location.host}/api/ws/terminal`);

    ws.onopen = () => {
      term.writeln('\x1b[38;5;82m✓\x1b[0m Connected to server');
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        term.write(event.data);
      } else {
        // Binary data
        const reader = new FileReader();
        reader.onload = () => term.write(new Uint8Array(reader.result as ArrayBuffer));
        reader.readAsArrayBuffer(event.data);
      }
    };

    ws.onclose = () => {
      term.writeln('\r\n\x1b[38;5;196m✗\x1b[0m Disconnected. Reconnecting in 3s...');
      setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = () => {
      term.writeln('\r\n\x1b[38;5;196m✗\x1b[0m Connection error');
    };
  }

  onDestroy(() => {
    ws?.close();
    term?.dispose();
  });
</script>

<div class="terminal-container">
  <div class="terminal-header">
    <div class="terminal-dots">
      <span class="dot red"></span>
      <span class="dot yellow"></span>
      <span class="dot green"></span>
    </div>
    <span class="terminal-title">root@server-{serverId}</span>
    <div class="terminal-actions">
      <button onclick={() => { fitAddon?.fit(); }} class="action-btn">Fit</button>
    </div>
  </div>
  <div class="terminal-body" bind:this={terminalEl}></div>
</div>

<style>
  .terminal-container {
    background: #0a0e14;
    border: 1px solid rgba(255,255,255,.06);
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 8px 40px rgba(0,0,0,.5);
  }
  .terminal-header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 16px;
    background: linear-gradient(180deg, #161b22, #0d1117);
    border-bottom: 1px solid rgba(255,255,255,.04);
  }
  .terminal-dots { display: flex; gap: 6px; }
  .dot { width: 10px; height: 10px; border-radius: 50%; }
  .dot.red { background: #ff5f56; }
  .dot.yellow { background: #ffbd2e; }
  .dot.green { background: #27c93f; }
  .terminal-title {
    font-size: .75rem;
    color: #8b949e;
    font-family: 'SF Mono', monospace;
  }
  .terminal-actions { margin-left: auto; }
  .action-btn {
    background: rgba(255,255,255,.06);
    border: none;
    color: #8b949e;
    padding: 4px 8px;
    border-radius: 4px;
    font-size: .7rem;
    cursor: pointer;
  }
  .action-btn:hover { background: rgba(255,255,255,.1); color: #e6edf3; }
  .terminal-body {
    height: calc(100vh - 200px);
    min-height: 400px;
    padding: 8px;
  }
</style>
```

---

## Task 5: Agent — Handle Resize Messages

**Modify:** `internal/agenthttp/terminal.go`

Update WebSocket reader to parse resize JSON:
```json
{"type":"resize","cols":120,"rows":40}
```

Use `pty.Setsize()` to resize the PTY.

---

## Task 6: Nginx WebSocket Proxy

**Modify:** `/etc/nginx/sites-available/panel.sigarda.dev`

Add WebSocket proxy block BEFORE the API proxy:
```nginx
# WebSocket terminal proxy (no auth header — agent validates)
location /api/ws/ {
    proxy_pass http://127.0.0.1:7710/ws/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

---

## Task 7: Build, Test, Deploy

**Steps:**
1. `go build -o sigardapanel ./cmd/sigardapanel`
2. `cd web && npm run build`
3. Fix nginx config
4. Restart services

---

## Files Changed Summary

| File | Action |
|------|--------|
| `go.mod` | Add gorilla/websocket, creack/pty |
| `web/package.json` | Add @xterm/xterm, @xterm/addon-fit, @xterm/addon-web-links |
| `internal/agenthttp/terminal.go` | NEW — WebSocket + PTY handler |
| `internal/agenthttp/router.go` | Register `/ws/terminal` route |
| `web/src/routes/server/[id]/terminal/+page.svelte` | Replace with xterm.js |
| `/etc/nginx/sites-available/panel.sigarda.dev` | Add WebSocket proxy |

---

## Verification

1. Open panel → Server → Terminal
2. See xterm.js terminal with blinking cursor
3. Type `ls -la` → see directory listing
4. Type `htop` → see interactive process viewer
5. Resize browser → terminal auto-fits
6. Close/reopen → reconnects automatically
