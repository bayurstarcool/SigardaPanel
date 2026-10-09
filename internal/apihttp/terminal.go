package apihttp

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

const terminalTicketTTL = 30 * time.Second

var terminalSessionHint = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)

// terminalTicket is a single-use, short-lived grant to open one web terminal.
// Browsers cannot set an Authorization header on a WebSocket, so the session
// token is exchanged (over normal authenticated HTTP) for a ticket that is
// bound server-side to the account to run as. The browser never chooses the
// unix user and never sees the agent token.
type terminalTicket struct {
	userID   int64
	agentURL string
	token    string
	runAs    string // "" = root shell (admins only)
	session  string
	expires  time.Time
}

type terminalHandler struct {
	store *store.Store
	audit *audit.Recorder
	sites *siteHandler

	mu      sync.Mutex
	tickets map[string]terminalTicket
}

func newTerminalHandler(s *store.Store, a *audit.Recorder, sites *siteHandler) *terminalHandler {
	return &terminalHandler{store: s, audit: a, sites: sites, tickets: make(map[string]terminalTicket)}
}

func (h *terminalHandler) put(t terminalTicket) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for k, v := range h.tickets {
		if now.After(v.expires) {
			delete(h.tickets, k)
		}
	}
	t.expires = now.Add(terminalTicketTTL)
	h.tickets[id] = t
	return id, nil
}

func (h *terminalHandler) take(id string) (terminalTicket, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.tickets[id]
	delete(h.tickets, id) // single use
	if !ok || time.Now().After(t.expires) {
		return terminalTicket{}, false
	}
	return t, true
}

// POST /api/v1/terminal/tickets {server_id | site_id, session?}
//   - site_id: shell as that site's unix user (needs write access to the site)
//   - server_id only: root shell (admins only)
func (h *terminalHandler) createTicket(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()

	var body struct {
		ServerID int64  `json:"server_id"`
		SiteID   int64  `json:"site_id"`
		Session  string `json:"session"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	user, err := h.store.Users.GetByID(ctx, sess.UserID)
	if err != nil || user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
	}

	serverID, runAs := body.ServerID, ""
	if body.SiteID > 0 {
		site, err := h.store.Sites.GetByID(ctx, body.SiteID)
		if err != nil || site == nil || !h.sites.checkSitePermission(ctx, sess, site, "write") {
			return echo.NewHTTPError(http.StatusNotFound, "site not found")
		}
		if !site.SystemUser.Valid || siteinput.SystemUser(site.SystemUser.String) != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "site has no valid system user")
		}
		serverID, runAs = site.ServerID, site.SystemUser.String
	} else if !auth.HasRole(user.Role, auth.RoleAdmin) {
		return echo.NewHTTPError(http.StatusForbidden, "server terminal requires an admin")
	}
	if serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "server_id required")
	}
	server, err := h.store.Servers.GetByID(ctx, serverID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "server has no reachable agent")
	}

	// Agent-side session ids live in the user's own namespace so one account
	// can never reattach to another's shell.
	prefix := "u" + itoa(sess.UserID) + "-"
	sid := body.Session
	if sid != "" && !terminalSessionHint.MatchString(sid) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid session")
	}
	if sid == "" {
		raw := make([]byte, 9)
		rand.Read(raw)
		sid = base64.RawURLEncoding.EncodeToString(raw)
	}
	if !strings.HasPrefix(sid, prefix) {
		sid = prefix + sid
	}

	ticket, err := h.put(terminalTicket{
		userID: sess.UserID, agentURL: server.AgentURL.String, token: server.AgentToken, runAs: runAs, session: sid,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, sess.UserID, "terminal_open", audit.ResourceServer, serverID, map[string]any{
		"site_id": body.SiteID, "run_as": firstNonEmpty(runAs, "root"), "ip": c.RealIP(),
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"ticket": ticket, "session": sid}, RequestID: rid})
}

var terminalClientUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser client; the ticket is still required
		}
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	},
}

// GET /api/v1/ws/terminal?ticket=...  — authenticated by the single-use ticket.
func (h *terminalHandler) serve(c echo.Context) error {
	t, ok := h.take(c.QueryParam("ticket"))
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired ticket")
	}

	target, err := url.Parse(strings.TrimRight(t.agentURL, "/") + "/ws/terminal")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "bad agent url")
	}
	switch target.Scheme {
	case "http":
		target.Scheme = "ws"
	case "https":
		target.Scheme = "wss"
	default:
		return echo.NewHTTPError(http.StatusBadGateway, "bad agent url")
	}
	q := url.Values{"session": {t.session}}
	if t.runAs != "" {
		q.Set("user", t.runAs)
	}
	target.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	agentConn, resp, err := dialer.Dial(target.String(), http.Header{"Authorization": {"Bearer " + t.token}})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		slog.Warn("terminal: agent dial failed", "status", status, "error", err)
		return echo.NewHTTPError(http.StatusBadGateway, "cannot reach agent terminal")
	}
	defer agentConn.Close()

	client, err := terminalClientUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return nil // Upgrade already wrote the error response
	}
	defer client.Close()
	client.SetReadLimit(1 << 20)

	done := make(chan struct{}, 2)
	pump := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := src.ReadMessage()
			if err != nil {
				return
			}
			if err := dst.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}
	go pump(agentConn, client)
	go pump(client, agentConn)
	<-done
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
