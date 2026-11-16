package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// fakeDiscoverAgent answers /tasks/databases/discover with two databases and
// /tasks/db/view with a login check: only "appuser" / "right-pass" signs in.
func fakeDiscoverAgent(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var views []map[string]any
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tasks/databases/discover":
			w.Write([]byte(`{"ok":true,"databases":[
				{"engine":"mysql","name":"legacy_app","owner":"appuser","size_bytes":4096},
				{"engine":"mysql","name":"known","owner":"","size_bytes":0},
				{"engine":"postgres","name":"Bad-Name","owner":"pg","size_bytes":1}]}`))
		case "/tasks/db/view":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			views = append(views, body)
			mu.Unlock()
			if body["user"] != "appuser" || body["password"] != "right-pass" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"ok":false,"message":"Access denied for user 'appuser'@'localhost'"}`))
				return
			}
			w.Write([]byte(`{"ok":true,"result":{"columns":["name"],"rows":[]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(agent.Close)
	return agent, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), views...)
	}
}

// importFixture: a server whose agent is fakeDiscoverAgent, a site on it with
// a panel database "known", a super admin and an ordinary user.
func importFixture(t *testing.T) (*authzFixture, *store.Site, string, string, func() []map[string]any) {
	t.Helper()
	agent, views := fakeDiscoverAgent(t)
	f := newAuthzFixture(t)
	_, superTok := f.user("root", auth.RoleSuperAdmin)
	owner, userTok := f.user("owner", auth.RoleUser)
	srv := &store.Server{Name: "remote", Hostname: "h", Status: "active", AgentToken: "t", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	if err := f.s.Servers.Create(f.ctx, srv); err != nil {
		t.Fatal(err)
	}
	st := &store.Site{ServerID: srv.ID, Name: "app", Domain: "app.example.com", Runtime: "php",
		RootPath: "/home/owner/htdocs/app.example.com", Status: "active", OwnerID: sql.NullInt64{Int64: owner.ID, Valid: true}}
	if err := f.s.Sites.Create(f.ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Databases.Create(f.ctx, &store.Database{SiteID: st.ID, Name: "known", Type: "mysql"}); err != nil {
		t.Fatal(err)
	}
	return f, st, superTok, userTok, views
}

// Discovery lists the server's databases, marks those already in the panel
// and those the panel cannot handle; it is for super admins only.
func TestDiscoverDatabasesAnnotatesRegistered(t *testing.T) {
	f, st, superTok, userTok, _ := importFixture(t)
	path := "/api/v1/servers/" + strconv.FormatInt(st.ServerID, 10) + "/databases/discover"

	if rec := f.do("GET", path, nil, userTok); rec.Code != http.StatusForbidden {
		t.Fatalf("user discover = %d %s", rec.Code, rec.Body.String())
	}
	rec := f.do("GET", path, nil, superTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("discover = %d %s", rec.Code, rec.Body.String())
	}
	list, _ := decodeData(t, rec)["databases"].([]any)
	if len(list) != 3 {
		t.Fatalf("databases = %v", list)
	}
	got := map[string]map[string]any{}
	for _, item := range list {
		m := item.(map[string]any)
		got[m["name"].(string)] = m
	}
	if got["legacy_app"]["registered"] != false || got["legacy_app"]["importable"] != true || got["legacy_app"]["owner"] != "appuser" {
		t.Fatalf("legacy_app = %v", got["legacy_app"])
	}
	if got["known"]["registered"] != true {
		t.Fatalf("known = %v", got["known"])
	}
	if got["Bad-Name"]["importable"] != false || got["Bad-Name"]["reason"] == "" {
		t.Fatalf("Bad-Name = %v", got["Bad-Name"])
	}
}

// Import refuses databases that are not on the server or already in the
// panel, and logins the database refuses; it stores an external database
// (and the verified account) otherwise.
func TestImportDatabase(t *testing.T) {
	f, st, superTok, userTok, views := importFixture(t)
	body := func(name, user, pass string) map[string]any {
		return map[string]any{"site_id": st.ID, "type": "mysql", "name": name, "username": user, "password": pass}
	}

	if rec := f.do("POST", "/api/v1/databases/import", body("legacy_app", "", ""), userTok); rec.Code != http.StatusForbidden {
		t.Fatalf("user import = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/databases/import", body("missing_db", "", ""), superTok); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown name = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/databases/import", body("known", "", ""), superTok); rec.Code != http.StatusConflict {
		t.Fatalf("registered name = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/databases/import", body("legacy_app", "root", "x"), superTok); rec.Code != http.StatusBadRequest {
		t.Fatalf("root account = %d %s", rec.Code, rec.Body.String())
	}
	rec := f.do("POST", "/api/v1/databases/import", body("legacy_app", "appuser", "wrong-pass"), superTok)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "refused this login") {
		t.Fatalf("wrong password = %d %s", rec.Code, rec.Body.String())
	}
	if d, _ := f.s.Databases.GetByName(f.ctx, "legacy_app"); d != nil {
		t.Fatal("database stored although the login failed")
	}

	rec = f.do("POST", "/api/v1/databases/import", body("legacy_app", "appuser", "right-pass"), superTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	v := views()
	if last := v[len(v)-1]; last["database"] != "legacy_app" || last["type"] != "mysql" || last["action"] != "tables" {
		t.Fatalf("login check = %v", last)
	}
	d, _ := f.s.Databases.GetByName(f.ctx, "legacy_app")
	if d == nil || !d.IsExternal() || d.SiteID != st.ID {
		t.Fatalf("stored database = %+v", d)
	}
	users, _ := f.s.DBUsers.ListByDatabase(f.ctx, d.ID)
	if len(users) != 1 || users[0].Username != "appuser" || users[0].HostPattern != "localhost" || users[0].PasswordHash == "" {
		t.Fatalf("stored users = %+v", users)
	}
	if pw, err := f.s.DBStore.DecryptPassword(users[0].EncryptedPassword); err != nil || pw != "right-pass" {
		t.Fatalf("stored password = %q %v", pw, err)
	}
	if rec := f.do("POST", "/api/v1/databases/import", body("legacy_app", "", ""), superTok); rec.Code != http.StatusConflict {
		t.Fatalf("second import = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("GET", "/api/v1/databases/"+strconv.FormatInt(d.ID, 10), nil, superTok); decodeData(t, rec)["external"] != true {
		t.Fatalf("detail = %s", rec.Body.String())
	}
}

// externalDB stores an external database with one account for the site.
func externalDB(t *testing.T, f *authzFixture, st *store.Site, name string) (*store.Database, *store.DBUser) {
	t.Helper()
	d := &store.Database{SiteID: st.ID, Name: name, Type: "mysql", ManagedBy: store.DatabaseManagedByExternal}
	if err := f.s.Databases.Create(f.ctx, d); err != nil {
		t.Fatal(err)
	}
	enc, _ := f.s.DBStore.EncryptPassword("app-pass-123")
	u := &store.DBUser{DatabaseID: d.ID, Username: name + "_u", PasswordHash: "x", EncryptedPassword: enc, HostPattern: "localhost", Privileges: "ALL"}
	if err := f.s.DBUsers.Create(f.ctx, u); err != nil {
		t.Fatal(err)
	}
	return d, u
}

func noDatabaseJobs(t *testing.T, f *authzFixture) {
	t.Helper()
	jobs, _ := f.s.Jobs.List(f.ctx, store.JobListFilter{})
	for _, j := range jobs {
		if strings.HasPrefix(j.Type, "database.") || strings.HasPrefix(j.Type, "dbuser.") {
			t.Fatalf("queued %s for an external database", j.Type)
		}
	}
}

// Nothing the panel offers changes an external database on the server:
// charset, accounts and passwords are refused; removing an account or the
// database removes only the panel records, without a job.
func TestExternalDatabaseIsNeverChanged(t *testing.T) {
	f, st, superTok, _, _ := importFixture(t)
	d, u := externalDB(t, f, st, "legacy_app")
	base := "/api/v1/databases/" + strconv.FormatInt(d.ID, 10)
	uid := strconv.FormatInt(u.ID, 10)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", base, map[string]any{"charset": "latin1"}},
		{"POST", base + "/users", map[string]any{"username": "newuser", "password": "Some-pass-12345"}},
		{"POST", base + "/users/" + uid + "/rotate-password", nil},
		{"POST", base + "/grant-user", map[string]any{"username": "otheruser"}},
		{"PATCH", "/api/v1/db-users/" + uid, map[string]any{"privileges": "SELECT"}},
	} {
		rec := f.do(tc.method, tc.path, tc.body, superTok)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "external database") {
			t.Fatalf("%s %s = %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	rec := f.do("DELETE", base+"/users/"+uid, nil, superTok)
	if rec.Code != http.StatusOK || decodeData(t, rec)["external"] != true {
		t.Fatalf("delete user = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := f.s.DBUsers.GetByID(f.ctx, u.ID); got != nil {
		t.Fatal("user record still there")
	}

	u2 := &store.DBUser{DatabaseID: d.ID, Username: "second", PasswordHash: "x", HostPattern: "localhost", Privileges: "ALL"}
	if err := f.s.DBUsers.Create(f.ctx, u2); err != nil {
		t.Fatal(err)
	}
	rec = f.do("DELETE", base, nil, superTok)
	if rec.Code != http.StatusOK || decodeData(t, rec)["external"] != true {
		t.Fatalf("delete database = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := f.s.Databases.GetByID(f.ctx, d.ID); got != nil {
		t.Fatal("database record still there")
	}
	if got, _ := f.s.DBUsers.GetByID(f.ctx, u2.ID); got != nil {
		t.Fatal("account record of the removed database still there")
	}
	noDatabaseJobs(t, f)
}

// The account of an external database is never rotated through another
// (panel-managed) database it was granted to.
func TestRotateRefusedForAccountOfExternalDatabase(t *testing.T) {
	f, st, superTok, _, _ := importFixture(t)
	_, u := externalDB(t, f, st, "legacy_app")
	own := &store.Database{SiteID: st.ID, Name: "own_db", Type: "mysql"}
	f.s.Databases.Create(f.ctx, own)
	shared := &store.DBUser{DatabaseID: own.ID, Username: u.Username, PasswordHash: u.PasswordHash, EncryptedPassword: u.EncryptedPassword, HostPattern: "localhost", Privileges: "ALL"}
	f.s.DBUsers.Create(f.ctx, shared)
	rec := f.do("POST", "/api/v1/databases/"+strconv.FormatInt(own.ID, 10)+"/users/"+strconv.FormatInt(shared.ID, 10)+"/rotate-password", nil, superTok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rotate = %d %s", rec.Code, rec.Body.String())
	}
	noDatabaseJobs(t, f)
}

// Deleting a (panel-managed) site removes its external databases from the
// panel without dropping them; its own databases are still dropped.
func TestSiteDeleteKeepsExternalDatabases(t *testing.T) {
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	st := f.site(owner, "app", "php")
	ext, _ := externalDB(t, f, st, "legacy_app")
	own := &store.Database{SiteID: st.ID, Name: "own_db", Type: "mysql"}
	f.s.Databases.Create(f.ctx, own)

	rec := f.do("DELETE", "/api/v1/sites/"+strconv.FormatInt(st.ID, 10), nil, tok)
	if rec.Code >= 300 {
		t.Fatalf("delete site = %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := f.s.Databases.GetByID(f.ctx, ext.ID); got != nil {
		t.Fatal("external database record still there")
	}
	jobs, _ := f.s.Jobs.List(f.ctx, store.JobListFilter{})
	dropped := map[string]bool{}
	for _, j := range jobs {
		if j.Type == jobmodel.TypeDatabaseDelete {
			var p struct {
				Name string `json:"name"`
			}
			json.Unmarshal([]byte(j.Payload), &p)
			dropped[p.Name] = true
		}
	}
	if dropped["legacy_app"] || !dropped["own_db"] {
		t.Fatalf("database.delete jobs for %v", dropped)
	}
}
