package apihttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
)

type apiKeyCreateResp struct {
	Data struct {
		Token string `json:"token"`
		Key   struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Prefix string `json:"prefix"`
		} `json:"key"`
	} `json:"data"`
}

func TestAPIKeys_CreateShowsTokenOnceAndStoresHash(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", "user")

	rec := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "ci"}, aliceTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d (%s)", rec.Code, rec.Body.String())
	}
	var cr apiKeyCreateResp
	if err := json.Unmarshal(rec.Body.Bytes(), &cr); err != nil {
		t.Fatal(err)
	}
	raw := cr.Data.Token
	if len(raw) != 64 || cr.Data.Key.Prefix != raw[:8] || cr.Data.Key.Name != "ci" {
		t.Fatalf("unexpected create response: %+v", cr.Data)
	}

	// Stored hashed, never raw.
	k, err := f.s.APIKeys.GetByKeyHash(f.ctx, auth.HashToken(raw))
	if err != nil || k == nil || k.UserID != alice.ID {
		t.Fatalf("key not stored by hash: %v %+v", err, k)
	}
	var rawCount int
	f.s.DB().QueryRow(`SELECT COUNT(*) FROM user_api_keys WHERE key_hash = ? OR prefix = ?`, raw, raw).Scan(&rawCount)
	if rawCount != 0 {
		t.Fatal("raw key persisted")
	}

	// The list never returns key material.
	rec = f.do("GET", "/api/v1/api-keys", nil, aliceTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, raw) || strings.Contains(body, k.KeyHash) {
		t.Fatal("list leaks key material")
	}

	// The key authenticates as alice.
	rec = f.do("GET", "/api/v1/auth/me", nil, raw)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"alice"`) {
		t.Fatalf("api key auth: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIKeys_ScopedToCaller(t *testing.T) {
	f := newAuthzFixture(t)
	_, aliceTok := f.user("alice", "user")
	_, bobTok := f.user("bob", "admin")

	rec := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "alice-key"}, aliceTok)
	var cr apiKeyCreateResp
	json.Unmarshal(rec.Body.Bytes(), &cr)
	id := itoa(cr.Data.Key.ID)

	// Bob (even as admin) neither sees nor revokes alice's key.
	rec = f.do("GET", "/api/v1/api-keys", nil, bobTok)
	if strings.Contains(rec.Body.String(), "alice-key") {
		t.Fatal("bob sees alice's key")
	}
	f.run([]authzCase{
		{"other user revoke", "DELETE", "/api/v1/api-keys/" + id, nil, bobTok, http.StatusNotFound},
		{"bad id", "DELETE", "/api/v1/api-keys/abc", nil, aliceTok, http.StatusBadRequest},
		{"empty name", "POST", "/api/v1/api-keys", map[string]any{"name": "  "}, aliceTok, http.StatusBadRequest},
		{"bad expiry", "POST", "/api/v1/api-keys", map[string]any{"name": "x", "expires_in_days": -1}, aliceTok, http.StatusBadRequest},
		{"unauthenticated", "GET", "/api/v1/api-keys", nil, "", http.StatusUnauthorized},
	})

	// Revoking makes the key stop working.
	raw := cr.Data.Token
	f.run([]authzCase{
		{"owner revoke", "DELETE", "/api/v1/api-keys/" + id, nil, aliceTok, http.StatusOK},
		{"revoked key", "GET", "/api/v1/auth/me", nil, raw, http.StatusUnauthorized},
	})
}

func TestAPIKeys_CannotManageKeysOrSessionOnlyEndpoints(t *testing.T) {
	f := newAuthzFixture(t)
	_, adminTok := f.user("root", "super_admin")
	rec := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "auto"}, adminTok)
	var cr apiKeyCreateResp
	json.Unmarshal(rec.Body.Bytes(), &cr)
	raw := cr.Data.Token
	svc := f.serviceToken("*")

	f.run([]authzCase{
		{"key mints key", "POST", "/api/v1/api-keys", map[string]any{"name": "more"}, raw, http.StatusForbidden},
		{"key lists keys", "GET", "/api/v1/api-keys", nil, raw, http.StatusForbidden},
		{"key manages users", "GET", "/api/v1/users", nil, raw, http.StatusForbidden},
		{"key logs out", "POST", "/api/v1/auth/logout", nil, raw, http.StatusForbidden},
		{"service token keys", "GET", "/api/v1/api-keys", nil, svc, http.StatusForbidden},
		{"service token logout", "POST", "/api/v1/auth/logout", nil, svc, http.StatusForbidden},
		// Ordinary admin endpoints still work with the owner's role.
		{"key lists servers", "GET", "/api/v1/servers", nil, raw, http.StatusOK},
	})
	// The admin's own session must survive the key's logout attempt.
	if rec := f.do("GET", "/api/v1/auth/me", nil, adminTok); rec.Code != http.StatusOK {
		t.Fatalf("session revoked by key logout: %d", rec.Code)
	}
}

func TestAPIKeys_LimitPerUser(t *testing.T) {
	f := newAuthzFixture(t)
	_, tok := f.user("alice", "user")
	for i := 0; i < 20; i++ {
		if rec := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "k" + itoa(int64(i))}, tok); rec.Code != http.StatusCreated {
			t.Fatalf("create %d: %d", i, rec.Code)
		}
	}
	if rec := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "one-too-many"}, tok); rec.Code != http.StatusConflict {
		t.Fatalf("limit: got %d", rec.Code)
	}
}

func TestAuditPurge_SuperAdminOnlyAndKeepsRecentAndRecordsItself(t *testing.T) {
	f := newAuthzFixture(t)
	_, rootTok := f.user("root", "super_admin")
	_, adminTok := f.user("adm", "admin")
	_, userTok := f.user("usr", "user")
	db := f.s.DB()
	db.Exec(`INSERT INTO audit_logs (action, resource, created_at) VALUES ('old', 'x', datetime('now','-200 days'))`)
	db.Exec(`INSERT INTO audit_logs (action, resource, created_at) VALUES ('new', 'x', datetime('now','-1 days'))`)

	f.run([]authzCase{
		{"user", "DELETE", "/api/v1/audit-logs?older_than_days=90", nil, userTok, http.StatusForbidden},
		{"admin", "DELETE", "/api/v1/audit-logs?older_than_days=90", nil, adminTok, http.StatusForbidden},
		{"service token", "DELETE", "/api/v1/audit-logs?older_than_days=90", nil, f.serviceToken("*"), http.StatusForbidden},
		{"missing days", "DELETE", "/api/v1/audit-logs", nil, rootTok, http.StatusBadRequest},
		{"zero days", "DELETE", "/api/v1/audit-logs?older_than_days=0", nil, rootTok, http.StatusBadRequest},
	})

	rec := f.do("DELETE", "/api/v1/audit-logs?older_than_days=90", nil, rootTok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted":1`) {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body.String())
	}
	var old, recent, marker int
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='old'`).Scan(&old)
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='new'`).Scan(&recent)
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='audit.purge'`).Scan(&marker)
	if old != 0 || recent != 1 || marker != 1 {
		t.Fatalf("after purge old=%d recent=%d marker=%d", old, recent, marker)
	}
}
