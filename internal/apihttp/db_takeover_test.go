package apihttp

import (
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestMySQLAccountsCannotBeTakenOver(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	bob, _ := f.user("bob", auth.RoleUser)
	aSite := f.site(alice, "alicesite", "php")
	bSite := f.site(bob, "bobsite", "php")
	aDB := &store.Database{SiteID: aSite.ID, Name: "alice_db", Type: "mysql", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci"}
	bDB := &store.Database{SiteID: bSite.ID, Name: "bob_db", Type: "mysql", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci"}
	f.s.Databases.Create(f.ctx, aDB)
	f.s.Databases.Create(f.ctx, bDB)
	bobUser := &store.DBUser{DatabaseID: bDB.ID, Username: "bob_app", HostPattern: "localhost", Privileges: "ALL"}
	f.s.DBUsers.Create(f.ctx, bobUser)
	aliceUser := &store.DBUser{DatabaseID: aDB.ID, Username: "alice_app", HostPattern: "localhost", Privileges: "ALL"}
	f.s.DBUsers.Create(f.ctx, aliceUser)
	a := strconv.FormatInt(aDB.ID, 10)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"root", map[string]any{"username": "root", "host_pattern": "localhost"}, http.StatusForbidden},
		{"another tenant's user", map[string]any{"username": "bob_app", "host_pattern": "localhost"}, http.StatusForbidden},
		{"unknown server account", map[string]any{"username": "wordpress", "host_pattern": "localhost"}, http.StatusForbidden},
		{"privilege injection", map[string]any{"username": "alice_app", "host_pattern": "%", "privileges": "ALL ON *.* TO 'alice_app'@'%' --"}, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := doRequest(t, f.e, http.MethodPost, "/api/v1/databases/"+a+"/grant-user", c.body, aliceTok); rec.Code != c.want {
			t.Errorf("grant %s = %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	// "Creating" an existing account (then rotating its password) is refused.
	for _, name := range []string{"root", "bob_app"} {
		rec := doRequest(t, f.e, http.MethodPost, "/api/v1/databases/"+a+"/users", map[string]any{"username": name, "password": "Str0ng-Passw0rd!", "host_pattern": "localhost"}, aliceTok)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
			t.Errorf("create existing %s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// A shared account registered for bob's database too cannot be rotated by alice.
	f.s.DBUsers.Create(f.ctx, &store.DBUser{DatabaseID: aDB.ID, Username: "bob_app", HostPattern: "localhost", Privileges: "ALL"})
	shared, _ := f.s.DBUsers.GetByUsernameAndDatabase(f.ctx, "bob_app", aDB.ID)
	if rec := doRequest(t, f.e, http.MethodPost, "/api/v1/databases/"+a+"/users/"+strconv.FormatInt(shared.ID, 10)+"/rotate-password", nil, aliceTok); rec.Code != http.StatusForbidden {
		t.Errorf("rotate a shared account = %d %s", rec.Code, rec.Body.String())
	}
}
