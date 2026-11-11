package jobs

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigardapanel/internal/store"
)

func fakeAgent(t *testing.T, version, token string, hasAuthCheck bool) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"status":"ok","service":"sigardapanel-agent","version":"` + version + `"}}`))
		case "/auth/check":
			if !hasAuthCheck {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthCheck_TokenAndVersion(t *testing.T) {
	cases := []struct {
		name         string
		agentToken   string
		hasAuthCheck bool
		wantHealth   string
		wantStatus   string
	}{
		{"matching token", "tok", true, "online", "online"},
		{"wrong token", "other", true, "unauthorized", "pending"},
		{"old agent without auth check", "other", false, "online", "online"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := fullWorkerDB(t)
			ts := fakeAgent(t, "1.5.0", tc.agentToken, tc.hasAuthCheck)
			srv := &store.Server{Name: "r", Hostname: "r.test", Status: "pending", AgentToken: "tok",
				AgentURL: sql.NullString{String: ts.URL, Valid: true}}
			if err := s.Servers.Create(ctx, srv); err != nil {
				t.Fatal(err)
			}
			NewHealthChecker(s).checkServer(ctx, *srv)

			got, err := s.Servers.GetByID(ctx, srv.ID)
			if err != nil || got == nil {
				t.Fatalf("get server: %v", err)
			}
			if got.HealthStatus != tc.wantHealth {
				t.Errorf("health_status = %q, want %q", got.HealthStatus, tc.wantHealth)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.AgentVersion.String != "1.5.0" {
				t.Errorf("agent_version = %q, want 1.5.0", got.AgentVersion.String)
			}
		})
	}
}
