package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestDatabaseViewerUsesTheDatabaseAccountOnTheSiteServer(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		calls = append(calls, body)
		mu.Unlock()
		if r.URL.Path != "/tasks/db/view" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"ok":true,"result":{"columns":["name"],"rows":[["products"]],"has_more":false}}`))
	}))
	defer agent.Close()

	e, s, ctx := newTestAppWithAllMigrations(t)
	owner := roleToken(t, s, ctx, "owner", auth.RoleUser)
	ownerUser, _ := s.Users.GetByUsername(ctx, "owner")
	outsider := roleToken(t, s, ctx, "outsider", auth.RoleUser)
	srv := &store.Server{Name: "remote", Hostname: "h", Status: "active", AgentToken: "t", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "x", Domain: "x.example.com", Runtime: "php", RootPath: "/home/owner/htdocs/x.example.com",
		Status: "active", OwnerID: sql.NullInt64{Int64: ownerUser.ID, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	d := &store.Database{SiteID: site.ID, Name: "shop", Type: "mysql"}
	if err := s.Databases.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/databases/" + strconv.FormatInt(d.ID, 10) + "/viewer"

	// No account with a stored password: a clear 409, nothing sent.
	if rec := doRequest(t, e, http.MethodGet, base+"/tables", nil, owner); rec.Code != http.StatusConflict {
		t.Fatalf("no account = %d %s", rec.Code, rec.Body.String())
	}
	enc, err := s.DBStore.EncryptPassword("Shop-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DBUsers.Create(ctx, &store.DBUser{DatabaseID: d.ID, Username: "shopu", PasswordHash: "x", EncryptedPassword: enc, HostPattern: "localhost", Privileges: "ALL"}); err != nil {
		t.Fatal(err)
	}

	if rec := doRequest(t, e, http.MethodGet, base+"/tables", nil, outsider); rec.Code != http.StatusNotFound {
		t.Fatalf("outsider = %d %s", rec.Code, rec.Body.String())
	}
	rec := doRequest(t, e, http.MethodGet, base+"/tables", nil, owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("tables = %d %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	last := calls[len(calls)-1]
	mu.Unlock()
	if last["action"] != "tables" || last["database"] != "shop" || last["user"] != "shopu" || last["password"] != "Shop-pass-123" || last["type"] != "mysql" {
		t.Fatalf("agent call = %v", last)
	}

	rec = doRequest(t, e, http.MethodGet, base+"/rows?table=products&limit=50&offset=100&order_by=id&desc=1", nil, owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("rows = %d %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	last = calls[len(calls)-1]
	mu.Unlock()
	if last["table"] != "products" || last["limit"] != float64(50) || last["offset"] != float64(100) || last["order_by"] != "id" || last["desc"] != true {
		t.Fatalf("rows call = %v", last)
	}

	rec = doRequest(t, e, http.MethodPost, base+"/query", map[string]any{"sql": "SELECT 1"}, owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("query = %d %s", rec.Code, rec.Body.String())
	}
}
