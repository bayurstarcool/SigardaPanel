package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"sigardapanel/internal/store"
)

// fakeAgent records the dial-in request and echoes websocket messages.
func fakeAgent(t *testing.T) (*httptest.Server, chan *http.Request) {
	t.Helper()
	seen := make(chan *http.Request, 8)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(r.Context())
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			conn.WriteMessage(mt, append([]byte("echo:"), msg...))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func requestTicket(t *testing.T, e *echo.Echo, token string, body map[string]any) (int, string, string) {
	t.Helper()
	rec := doRequest(t, e, "POST", "/api/v1/terminal/tickets", body, token)
	var out struct {
		Data struct {
			Ticket  string `json:"ticket"`
			Session string `json:"session"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&out)
	return rec.Code, out.Data.Ticket, out.Data.Session
}

func TestTerminal_TicketFlow(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)
	userToken := newUserToken(t, s, ctx)
	agent, seen := fakeAgent(t)

	srv := &store.Server{Name: "local", Hostname: "h", Status: "active", AgentToken: "agent-secret",
		AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}
	regular, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srv.ID, Name: "s", Domain: "s.example.com", Runtime: "static", RootPath: "/home/regular/htdocs/s.example.com",
		OwnerID: sql.NullInt64{Int64: regular.ID, Valid: true}, SystemUser: sql.NullString{String: "regular", Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	other := &store.Site{ServerID: srv.ID, Name: "o", Domain: "o.example.com", Runtime: "static", RootPath: "/home/admin/htdocs/o.example.com",
		SystemUser: sql.NullString{String: "admin", Valid: true}}
	s.Sites.Create(ctx, other)

	// A regular user can never get a root shell, nor a shell on someone else's site.
	if code, _, _ := requestTicket(t, e, userToken, map[string]any{"server_id": srv.ID}); code != http.StatusForbidden {
		t.Errorf("regular user root terminal: got %d, want 403", code)
	}
	if code, _, _ := requestTicket(t, e, userToken, map[string]any{"site_id": other.ID}); code != http.StatusNotFound {
		t.Errorf("regular user on foreign site: got %d, want 404", code)
	}
	if code, _, _ := requestTicket(t, e, "", map[string]any{"server_id": srv.ID}); code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", code)
	}

	web := httptest.NewServer(e)
	defer web.Close()
	wsURL := "ws" + strings.TrimPrefix(web.URL, "http") + "/api/v1/ws/terminal?ticket="

	// Site terminal: runs as the site's unix user, in the caller's session namespace.
	code, ticket, sid := requestTicket(t, e, userToken, map[string]any{"site_id": site.ID, "session": "../../x"})
	if code != http.StatusBadRequest {
		t.Errorf("bad session hint: got %d, want 400", code)
	}
	code, ticket, sid = requestTicket(t, e, userToken, map[string]any{"site_id": site.ID, "session": "abc"})
	if code != http.StatusOK || ticket == "" {
		t.Fatalf("site ticket: %d", code)
	}
	if !strings.HasPrefix(sid, "u"+itoa(regular.ID)+"-") {
		t.Errorf("session %q not namespaced to the user", sid)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+ticket, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	conn.WriteMessage(websocket.TextMessage, []byte("ls"))
	if _, msg, err := conn.ReadMessage(); err != nil || string(msg) != "echo:ls" {
		t.Errorf("proxy round trip: %q %v", msg, err)
	}
	conn.Close()
	req := <-seen
	if got := req.Header.Get("Authorization"); got != "Bearer agent-secret" {
		t.Errorf("agent auth header = %q", got)
	}
	if q := req.URL.Query(); q.Get("user") != "regular" || q.Get("session") != sid {
		t.Errorf("agent query = %v (want user=regular session=%s)", q, sid)
	}

	// Tickets are single use.
	if _, _, err := websocket.DefaultDialer.Dial(wsURL+ticket, nil); err == nil {
		t.Error("ticket reuse must fail")
	}
	// Garbage ticket.
	if _, _, err := websocket.DefaultDialer.Dial(wsURL+"nope", nil); err == nil {
		t.Error("unknown ticket must fail")
	}

	// Admin gets a root shell (no user= param) and cannot be spoofed via the client.
	code, ticket, _ = requestTicket(t, e, adminToken, map[string]any{"server_id": srv.ID})
	if code != http.StatusOK {
		t.Fatalf("admin ticket: %d", code)
	}
	conn, _, err = websocket.DefaultDialer.Dial(wsURL+ticket+"&user=someoneelse", nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	req = <-seen
	if u := req.URL.Query().Get("user"); u != "" {
		t.Errorf("client-supplied user leaked through: %q", u)
	}
}
