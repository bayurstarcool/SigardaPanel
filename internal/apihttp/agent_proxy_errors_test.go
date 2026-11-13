package apihttp

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
)

func TestAgentProxyErrorsAreErrors(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/docker/container/start":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"no such container"}`))
		case "/tasks/redis/flush":
			w.Write([]byte(`{"ok":false,"message":"redis-cli not found"}`))
		case "/tasks/redis/stats":
			w.Write([]byte(`{"ok":false,"message":"Redis is not installed"}`))
		default:
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer agent.Close()
	e, s, ctx := newTestApp(t)
	tok := roleToken(t, s, ctx, "boss", auth.RoleSuperAdmin)
	id := strconv.FormatInt(addServer(t, s, ctx, "srv", agent.URL), 10)

	rec := doRequest(t, e, http.MethodPost, "/api/v1/servers/"+id+"/docker/container/start", map[string]any{"container_id": "abc"}, tok)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error"`) || !strings.Contains(rec.Body.String(), "no such container") {
		t.Fatalf("agent 400 = %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodPost, "/api/v1/servers/"+id+"/redis/flush", map[string]any{}, tok)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "redis-cli not found") {
		t.Fatalf("agent ok:false on POST = %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, e, http.MethodGet, "/api/v1/servers/"+id+"/redis/stats", nil, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("GET ok:false must pass through = %d %s", rec.Code, rec.Body.String())
	}
}
