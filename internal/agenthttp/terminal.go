package agenthttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow non-browser clients (curl, etc.)
		}
		// Allow same-origin requests
		host := r.Host
		return strings.Contains(origin, host)
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ptySession persists across WebSocket reconnections.
// One goroutine reads from PTY and writes to the current ws.
type ptySession struct {
	ptmx   *os.File
	cmd    *exec.Cmd
	cancel context.CancelFunc

	// Current WebSocket — swapped atomically on reconnect
	ws   *websocket.Conn
	wsMu sync.RWMutex

	mu sync.Mutex // guards ptmx writes from WS→PTY side

	// Last time this session saw any activity (WS input or PTY output)
	lastActivity time.Time

	// Pending output buffer for reconnect: captures recent PTY output
	pendingMu sync.Mutex
	pending   []byte
}

const (
	maxPTYSessions   = 50
	ptyIdleTimeout   = 30 * time.Minute
	ptyCleanupPeriod = 5 * time.Minute
)

var (
	ptySessions   = make(map[string]*ptySession)
	ptySessionsMu sync.RWMutex
)

func init() {
	go ptyCleanupLoop()
}

// ptyCleanupLoop periodically removes idle sessions and enforces the max session cap.
func ptyCleanupLoop() {
	ticker := time.NewTicker(ptyCleanupPeriod)
	defer ticker.Stop()
	for range ticker.C {
		cleanIdleSessions()
	}
}

// cleanIdleSessions removes sessions idle longer than ptyIdleTimeout,
// and if the map still exceeds maxPTYSessions, removes the oldest.
func cleanIdleSessions() {
	now := time.Now()
	ptySessionsMu.Lock()
	defer ptySessionsMu.Unlock()

	// Build a list of sessions with their last-activity timestamps
	type candidate struct {
		id         string
		lastActive time.Time
		sess       *ptySession
	}
	var sessions []candidate
	for id, sess := range ptySessions {
		// Use the pty session creation as a baseline; track last WS read/write
		// via a simple heuristic: if no ws connected and pending buffer is empty,
		// consider idle. For robustness, we track lastActivity on the session.
		sess.mu.Lock()
		last := sess.lastActivity
		sess.mu.Unlock()
		if last.IsZero() {
			last = now // newly created, not idle yet
		}
		sessions = append(sessions, candidate{id: id, lastActive: last, sess: sess})
	}

	// Remove idle sessions
	for _, s := range sessions {
		if now.Sub(s.lastActive) > ptyIdleTimeout {
			slog.Info("terminal: closing idle session", "session", s.id, "idle_since", s.lastActive)
			s.sess.cancel()
			if s.sess.ptmx != nil {
				s.sess.ptmx.Close()
			}
			if s.sess.cmd.Process != nil {
				s.sess.cmd.Process.Kill()
			}
			delete(ptySessions, s.id)
		}
	}

	// Enforce max session count by removing oldest
	if len(ptySessions) > maxPTYSessions {
		// Rebuild session list after removals
		var remaining []candidate
		for id, sess := range ptySessions {
			sess.mu.Lock()
			last := sess.lastActivity
			sess.mu.Unlock()
			if last.IsZero() {
				last = now
			}
			remaining = append(remaining, candidate{id: id, lastActive: last, sess: sess})
		}
		// Sort by lastActive ascending (oldest first)
		for i := 0; i < len(remaining)-1; i++ {
			for j := i + 1; j < len(remaining); j++ {
				if remaining[j].lastActive.Before(remaining[i].lastActive) {
					remaining[i], remaining[j] = remaining[j], remaining[i]
				}
			}
		}
		excess := len(remaining) - maxPTYSessions
		for i := 0; i < excess; i++ {
			s := remaining[i]
			slog.Info("terminal: evicting session (over limit)", "session", s.id)
			s.sess.cancel()
			if s.sess.ptmx != nil {
				s.sess.ptmx.Close()
			}
			if s.sess.cmd.Process != nil {
				s.sess.cmd.Process.Kill()
			}
			delete(ptySessions, s.id)
		}
	}
}

func getOrCreateSession(sessionID string) (*ptySession, bool) {
	ptySessionsMu.RLock()
	sess, exists := ptySessions[sessionID]
	ptySessionsMu.RUnlock()
	return sess, exists
}

func saveSession(sessionID string, sess *ptySession) {
	ptySessionsMu.Lock()
	ptySessions[sessionID] = sess
	ptySessionsMu.Unlock()
}

func removeSession(sessionID string) {
	ptySessionsMu.Lock()
	if sess, ok := ptySessions[sessionID]; ok {
		sess.cancel()
		if sess.ptmx != nil {
			sess.ptmx.Close()
		}
		if sess.cmd.Process != nil {
			sess.cmd.Process.Kill()
		}
		delete(ptySessions, sessionID)
	}
	ptySessionsMu.Unlock()
}

// ptyReader runs for the lifetime of a session.
func ptyReader(ctx context.Context, sess *ptySession, sessionID string) {
	slog.Info("terminal: ptyReader started", "session", sessionID)
	defer slog.Info("terminal: ptyReader exited", "session", sessionID)

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		sess.ptmx.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := sess.ptmx.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			slog.Info("terminal: pty read error", "session", sessionID, "error", err)
			return
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		// Touch last activity timestamp
		sess.mu.Lock()
		sess.lastActivity = time.Now()
		sess.mu.Unlock()

		// Get current WebSocket
		sess.wsMu.RLock()
		ws := sess.ws
		sess.wsMu.RUnlock()

		if ws != nil {
			// Send directly to ws
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				slog.Debug("terminal: ws write failed", "session", sessionID, "error", err)
				// Buffer for reconnect
				sess.pendingMu.Lock()
				sess.pending = append(sess.pending, data...)
				if len(sess.pending) > 65536 {
					sess.pending = sess.pending[len(sess.pending)-65536:]
				}
				sess.pendingMu.Unlock()
			}
		} else {
			// No ws attached, buffer for reconnect
			sess.pendingMu.Lock()
			sess.pending = append(sess.pending, data...)
			if len(sess.pending) > 65536 {
				sess.pending = sess.pending[len(sess.pending)-65536:]
			}
			sess.pendingMu.Unlock()
		}
	}
}

func (r *Router) terminalWebSocket(c echo.Context) error {
	ws, err := terminalUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		slog.Error("terminal: websocket upgrade failed", "error", err)
		return err
	}
	defer ws.Close()

	sessionID := c.QueryParam("session")
	if sessionID == "" {
		sessionID = "pty_" + time.Now().Format("20060102150405.000")
	}

	// Optional: run shell as a specific user (site-level SSH access)
	runAsUser := strings.TrimSpace(c.QueryParam("user"))

	sess, exists := getOrCreateSession(sessionID)
	if !exists {
		// New session
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}

		var cmd *exec.Cmd
		if runAsUser != "" && runAsUser != "root" {
			// Run shell as the specified user via su
			slog.Info("terminal: spawning shell as user", "user", runAsUser)
			cmd = exec.Command("su", "-", runAsUser, "-s", shell)
		} else {
			cmd = exec.Command(shell)
		}
		cmd.Env = append(os.Environ(),
			"TERM=xterm-256color",
			"LANG=en_US.UTF-8",
			"LC_ALL=en_US.UTF-8",
		)
		ptmx, err := pty.Start(cmd)
		if err != nil {
			slog.Error("terminal: pty start failed", "error", err)
			ws.WriteMessage(websocket.TextMessage, []byte("Failed to start shell: "+err.Error()))
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		sess = &ptySession{
			ptmx:   ptmx,
			cmd:    cmd,
			cancel: cancel,
			ws:     ws,
		}
		saveSession(sessionID, sess)
		slog.Info("terminal: new session", "session", sessionID, "pid", cmd.Process.Pid, "shell", shell, "user", runAsUser)
		pty.Setsize(ptmx, &pty.Winsize{Cols: 120, Rows: 40})
		go ptyReader(ctx, sess, sessionID)
	} else {
		// Reattach
		slog.Info("terminal: reattached to session", "session", sessionID, "pid", sess.cmd.Process.Pid)

		// Swap ws
		sess.wsMu.Lock()
		sess.ws = ws
		sess.wsMu.Unlock()

		// Flush any pending buffered output to new ws
		sess.pendingMu.Lock()
		if len(sess.pending) > 0 {
			pending := sess.pending
			sess.pending = nil
			sess.pendingMu.Unlock()
			ws.WriteMessage(websocket.BinaryMessage, pending)
			slog.Info("terminal: flushed pending output", "session", sessionID, "bytes", len(pending))
		} else {
			sess.pendingMu.Unlock()
		}

		// Trigger SIGWINCH to make bash redraw prompt
		sess.mu.Lock()
		pty.Setsize(sess.ptmx, &pty.Winsize{Cols: 120, Rows: 40})
		// Send Enter to force bash to redraw prompt
		sess.ptmx.Write([]byte("\n"))
		sess.mu.Unlock()
	}

	// Send session ID
	sessionMsg, _ := json.Marshal(map[string]string{"type": "session", "id": sessionID})
	ws.WriteMessage(websocket.TextMessage, sessionMsg)

	// WS → PTY loop
	for {
		messageType, message, err := ws.ReadMessage()
		if err != nil {
			slog.Info("terminal: websocket read error", "error", err)
			return nil
		}
		// Touch last activity on any WS input
		sess.mu.Lock()
		sess.lastActivity = time.Now()
		sess.mu.Unlock()

		switch messageType {
		case websocket.TextMessage:
			if len(message) > 0 && message[0] == '{' {
				var msg resizeMsg
				if json.Unmarshal(message, &msg) == nil && msg.Type == "resize" {
					sess.mu.Lock()
					pty.Setsize(sess.ptmx, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
					sess.mu.Unlock()
					continue
				}
			}
			sess.mu.Lock()
			sess.ptmx.Write(message)
			sess.mu.Unlock()
		case websocket.BinaryMessage:
			sess.mu.Lock()
			sess.ptmx.Write(message)
			sess.mu.Unlock()
		}
	}
}
