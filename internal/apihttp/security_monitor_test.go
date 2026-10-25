package apihttp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"sigardapanel/internal/store"
)

func TestSecurityBaselineSaveAndCompare(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)

	var hash atomic.Value
	hash.Store("aaaa1111")
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/tasks/security/integrity") {
			fmt.Fprintf(w, `{"ok":true,"data":[{"path":"/etc/passwd","exists":true,"sha256":"%s","mode":"-rw-r--r--","uid":0,"size":1234},{"path":"/etc/shadow","exists":true,"sha256":"bbbb2222","mode":"-rw-r-----","uid":0,"size":567}]}`, hash.Load())
			return
		}
		w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	defer agent.Close()

	srv := &store.Server{Name: "sec", Hostname: "sec", Status: "active", AgentToken: "tok",
		AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/servers/" + itoa(srv.ID)

	// No baseline yet.
	rec := doRequest(t, e, "GET", base+"/security/baseline", nil, adminToken)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_baseline":false`) {
		t.Fatalf("baseline before save: %d %s", rec.Code, rec.Body.String())
	}

	// Save the baseline.
	rec = doRequest(t, e, "POST", base+"/security/baseline", nil, adminToken)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"saved":true`) {
		t.Fatalf("baseline save: %d %s", rec.Code, rec.Body.String())
	}

	// Identical live state -> no diffs.
	rec = doRequest(t, e, "GET", base+"/security/baseline", nil, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("baseline compare: %d", rec.Code)
	}
	var cmp struct {
		Data struct {
			HasBaseline bool `json:"has_baseline"`
			Compare     struct {
				Diffs []map[string]any `json:"diffs"`
			} `json:"compare"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&cmp)
	if !cmp.Data.HasBaseline || len(cmp.Data.Compare.Diffs) != 0 {
		t.Fatalf("expected clean compare, got %s", rec.Body.String())
	}

	// Tamper with /etc/passwd and add an unexpected file.
	hash.Store("cccc3333")
	rec = doRequest(t, e, "GET", base+"/security/baseline", nil, adminToken)
	json.NewDecoder(rec.Body).Decode(&cmp)
	if len(cmp.Data.Compare.Diffs) != 1 || cmp.Data.Compare.Diffs[0]["status"] != "changed" {
		t.Fatalf("expected one changed diff, got %s", rec.Body.String())
	}

	// Non-admin is rejected.
	userToken := newUserToken(t, s, ctx)
	rec = doRequest(t, e, "GET", base+"/security/monitor", nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("monitor as non-admin: got %d, want 403", rec.Code)
	}

	// Admin monitor aggregates sections.
	rec = doRequest(t, e, "GET", base+"/security/monitor", nil, adminToken)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"integrity"`) {
		t.Fatalf("monitor: %d %s", rec.Code, rec.Body.String())
	}
}
