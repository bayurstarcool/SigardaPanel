package apihttp

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigardapanel/internal/store"
)

func TestContainerBelongsToMatchesExactly(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"containers":[{"id":"abcdef1234567890aaaa","name":"/sp-site-7"},{"id":"abc999","name":"/other"}]}`))
	}))
	defer agent.Close()
	h := &stackHandler{}
	srv := &store.Server{AgentURL: sql.NullString{String: agent.URL, Valid: true}, AgentToken: "t"}
	allowed := map[string]bool{"sp-site-7": true}
	for id, want := range map[string]bool{
		"sp-site-7":             true,
		"abcdef1234567890aaaa":  true,
		"abcdef123456":          true,
		"abc":                   false, // also a prefix of another container
		"other":                 false,
		"abcdef1234567890aaaaX": false,
	} {
		if got := h.containerBelongsTo(srv, id, allowed); got != want {
			t.Errorf("%q = %v, want %v", id, got, want)
		}
	}
}
