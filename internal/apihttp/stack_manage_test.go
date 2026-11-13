package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// stackAgent is a fake agent: it records requests and answers like the real
// /tasks/stack endpoints.
type stackAgent struct {
	mu     sync.Mutex
	calls  []string
	bodies []map[string]any
	status int
	answer map[string]any
}

func (a *stackAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	a.mu.Lock()
	a.calls = append(a.calls, r.Method+" "+r.URL.Path)
	a.bodies = append(a.bodies, body)
	status, answer := a.status, a.answer
	a.mu.Unlock()
	if status == 0 {
		status = http.StatusAccepted
	}
	if answer == nil {
		answer = map[string]any{"op_id": "op_0123456789abcdef", "status": "running", "done": false, "ok": false, "action": body["action"], "package": body["package"]}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(answer)
}

func (a *stackAgent) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func roleToken(t *testing.T, s *store.Store, ctx context.Context, name, role string) string {
	t.Helper()
	u := &store.User{Username: name, Email: name + "@example.com", PasswordHash: "x", Role: role}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tok, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{UserID: u.ID, TokenHash: auth.HashToken(tok), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return tok
}

func addServer(t *testing.T, s *store.Store, ctx context.Context, name, agentURL string) int64 {
	t.Helper()
	srv := &store.Server{Name: name, Hostname: name + ".example.com", Status: "active", AgentToken: "agent-token",
		AgentURL: sql.NullString{String: agentURL, Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}
	return srv.ID
}

func addSite(t *testing.T, s *store.Store, ctx context.Context, serverID int64, domain, runtime, php string) {
	t.Helper()
	site := &store.Site{ServerID: serverID, Name: strings.ReplaceAll(domain, ".", "-"), Domain: domain, Runtime: runtime,
		PHPVersion: sql.NullString{String: php, Valid: php != ""}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
}

func stackPath(id int64, rest string) string {
	return "/api/v1/servers/" + strconv.FormatInt(id, 10) + rest
}

func errMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Error.Message
}

// A remote server whose agent must never be contacted: the guards answer first.
const unreachableAgent = "http://192.0.2.10:7710"

func TestStackGuardPHPVersionUsedBySites(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	id := addServer(t, s, ctx, "remote", unreachableAgent)
	addSite(t, s, ctx, id, "shop.example.com", "php", "8.2")
	addSite(t, s, ctx, id, "blog.example.com", "php", "8.3")

	rec := doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "php", "action": "uninstall", "version": "8.2"}, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body)
	}
	msg := errMessage(t, rec)
	if !strings.Contains(msg, "shop.example.com") || strings.Contains(msg, "blog.example.com") {
		t.Fatalf("message should name exactly the 8.2 site: %s", msg)
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'stack_uninstall_refused'`).Scan(&n)
	if n != 1 {
		t.Errorf("refusal should be audited, got %d rows", n)
	}

	rec = doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "php", "action": "uninstall"}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("uninstall without version: expected 400, got %d", rec.Code)
	}
}

func TestStackGuardNginxDockerNodeCertbotMariaDB(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	id := addServer(t, s, ctx, "remote", unreachableAgent)
	addSite(t, s, ctx, id, "app.example.com", "node", "")
	addSite(t, s, ctx, id, "box.example.com", "docker", "")

	for _, pkg := range []string{"nginx", "node", "docker"} {
		rec := doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": pkg, "action": "uninstall"}, tok)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s uninstall: expected 409, got %d: %s", pkg, rec.Code, rec.Body)
		}
	}

	// certbot: refused while a site has a certificate
	site, _ := s.Sites.GetByDomain(ctx, "app.example.com")
	s.DB().Exec(`UPDATE sites SET ssl_status = 'active' WHERE id = ?`, site.ID)
	rec := doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "certbot", "action": "uninstall"}, tok)
	if rec.Code != http.StatusConflict || !strings.Contains(errMessage(t, rec), "app.example.com") {
		t.Errorf("certbot: %d %s", rec.Code, rec.Body)
	}

	// mariadb: refused while the panel manages databases there
	if err := s.Databases.Create(ctx, &store.Database{SiteID: site.ID, Name: "appdb", Type: "mysql"}); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "mysql", "action": "uninstall"}, tok)
	if rec.Code != http.StatusConflict || !strings.Contains(errMessage(t, rec), "appdb") {
		t.Errorf("mariadb: %d %s", rec.Code, rec.Body)
	}
}

func TestStackLocalServerProtections(t *testing.T) {
	agent := &stackAgent{}
	srv := httptest.NewServer(agent) // 127.0.0.1 → the panel host
	defer srv.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	super := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	admin := roleToken(t, s, ctx, "ops", auth.RoleAdmin)
	user := roleToken(t, s, ctx, "joe", auth.RoleUser)
	id := addServer(t, s, ctx, "local", srv.URL)

	post := func(tok string, body map[string]any) *httptest.ResponseRecorder {
		return doRequest(t, e, "POST", stackPath(id, "/stack/action"), body, tok)
	}
	if rec := post(user, map[string]any{"package": "redis", "action": "restart"}); rec.Code != http.StatusForbidden {
		t.Errorf("user: expected 403, got %d", rec.Code)
	}
	if rec := post(admin, map[string]any{"package": "redis", "action": "uninstall"}); rec.Code != http.StatusForbidden {
		t.Errorf("admin uninstall on panel host: expected 403, got %d", rec.Code)
	}
	if rec := post(admin, map[string]any{"package": "php", "action": "stop", "version": "8.3"}); rec.Code != http.StatusForbidden {
		t.Errorf("admin stop on panel host: expected 403, got %d", rec.Code)
	}
	for _, a := range []string{"stop", "uninstall"} {
		if rec := post(super, map[string]any{"package": "nginx", "action": a}); rec.Code != http.StatusConflict {
			t.Errorf("nginx %s on panel host: expected 409, got %d", a, rec.Code)
		}
	}
	if rec := post(super, map[string]any{"package": "certbot", "action": "uninstall"}); rec.Code != http.StatusConflict {
		t.Errorf("certbot uninstall on panel host: expected 409, got %d", rec.Code)
	}
	if n := agent.callCount(); n != 0 {
		t.Fatalf("refused actions must not reach the agent, got %d calls", n)
	}

	// Allowed: admin restarts, super_admin uninstalls redis → forwarded, 202 + op.
	agent.status, agent.answer = http.StatusOK, map[string]any{"ok": true, "message": "redis-server restarted"}
	if rec := post(admin, map[string]any{"package": "redis", "action": "restart"}); rec.Code != http.StatusOK {
		t.Errorf("admin restart: %d %s", rec.Code, rec.Body)
	}
	agent.status, agent.answer = 0, nil
	rec := post(super, map[string]any{"package": "redis", "action": "uninstall"})
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "op_0123456789abcdef") {
		t.Fatalf("super uninstall: %d %s", rec.Code, rec.Body)
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'stack_uninstall' AND details LIKE '%op_0123456789abcdef%'`).Scan(&n)
	if n != 1 {
		t.Errorf("uninstall should be audited with its op id, got %d", n)
	}

	// Command exec on the panel host is super_admin only.
	if rec := doRequest(t, e, "POST", stackPath(id, "/command"), map[string]any{"command": "id"}, admin); rec.Code != http.StatusForbidden {
		t.Errorf("admin command on panel host: expected 403, got %d", rec.Code)
	}
}

func TestStackAgentErrorsAreMapped(t *testing.T) {
	agent := &stackAgent{status: http.StatusUnauthorized, answer: map[string]any{"message": "invalid agent token"}}
	srv := httptest.NewServer(agent)
	defer srv.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	super := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	id := addServer(t, s, ctx, "local", srv.URL)

	rec := doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "redis", "action": "install"}, super)
	if rec.Code != http.StatusBadGateway || !strings.Contains(errMessage(t, rec), "rejected the panel's token") {
		t.Fatalf("agent 401 must not look like a session problem: %d %s", rec.Code, rec.Body)
	}

	agent.status, agent.answer = http.StatusConflict, map[string]any{"ok": false, "message": "another stack operation is running"}
	rec = doRequest(t, e, "POST", stackPath(id, "/stack/action"), map[string]any{"package": "redis", "action": "install"}, super)
	if rec.Code != http.StatusConflict || errMessage(t, rec) != "another stack operation is running" {
		t.Fatalf("agent 409 should pass through with its message: %d %s", rec.Code, rec.Body)
	}

	for _, body := range []map[string]any{
		{"package": "bash", "action": "install"},
		{"package": "php", "action": "switch-version", "version": "8.3"},
		{"package": "php", "action": "install", "version": "8.3; rm -rf /"},
		{"package": "php", "action": "install-extension", "version": "8.3", "extension": "../x"},
	} {
		before := agent.callCount()
		rec := doRequest(t, e, "POST", stackPath(id, "/stack/action"), body, super)
		if rec.Code != http.StatusBadRequest || agent.callCount() != before {
			t.Errorf("%v: expected 400 without agent call, got %d", body, rec.Code)
		}
	}
}

func TestStackOpResultAuditedOnce(t *testing.T) {
	agent := &stackAgent{status: http.StatusOK, answer: map[string]any{
		"op_id": "op_00000000000000aa", "status": "succeeded", "done": true, "ok": true,
		"action": "install", "package": "php", "version": "8.3", "message": "PHP 8.3 installed",
	}}
	srv := httptest.NewServer(agent)
	defer srv.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	admin := roleToken(t, s, ctx, "ops", auth.RoleAdmin)
	id := addServer(t, s, ctx, "local", srv.URL)

	if rec := doRequest(t, e, "GET", stackPath(id, "/stack/ops/../../x"), nil, admin); rec.Code == http.StatusOK {
		t.Error("malformed op id accepted")
	}
	for i := 0; i < 3; i++ {
		rec := doRequest(t, e, "GET", stackPath(id, "/stack/ops/op_00000000000000aa"), nil, admin)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PHP 8.3 installed") {
			t.Fatalf("op: %d %s", rec.Code, rec.Body)
		}
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'stack_install_result'`).Scan(&n)
	if n != 1 {
		t.Errorf("result should be audited exactly once, got %d", n)
	}
}

func TestServerPHPVersionsOnlyFPM(t *testing.T) {
	agent := &stackAgent{status: http.StatusOK, answer: map[string]any{"data": map[string]any{"versions": []any{
		map[string]any{"version": "8.3", "fpm": true},
		map[string]any{"version": "8.1", "fpm": false},
	}}}}
	srv := httptest.NewServer(agent)
	defer srv.Close()
	e, s, ctx := newTestAppWithAllMigrations(t)
	user := roleToken(t, s, ctx, "joe", auth.RoleUser)
	id := addServer(t, s, ctx, "local", srv.URL)

	rec := doRequest(t, e, "GET", stackPath(id, "/php-versions"), nil, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out struct {
		Data struct {
			Versions []string `json:"versions"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if strings.Join(out.Data.Versions, ",") != "8.3" {
		t.Fatalf("versions = %v", out.Data.Versions)
	}
	if b := agent.bodies[0]; b["action"] != "list-versions" || b["package"] != "php" {
		t.Errorf("agent asked %v", b)
	}
}
