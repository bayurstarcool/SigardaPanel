package apihttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// Failed logins from many addresses lock the account, but not for an
// address the owner has signed in from before.
func TestAccountLockSparesKnownAddresses(t *testing.T) {
	e, s, ctx := newTestApp(t)
	// Users.Create hashes PasswordHash itself.
	u := &store.User{Username: "owner", Email: "owner@example.com", PasswordHash: "correct-horse-battery", Role: auth.RoleUser}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	audit.NewRecorder(s.DB()).RecordAction(ctx, u.ID, audit.ActionLogin, audit.ResourceSession, 1, map[string]any{"ip": "198.51.100.7"})

	login := func(ip, password string) int {
		b, _ := json.Marshal(map[string]string{"username": "owner", "password": password})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 21; i++ {
		login([]string{"203.0.113.1", "203.0.113.2", "203.0.113.3"}[i%3], "wrong")
	}
	if code := login("203.0.113.9", "correct-horse-battery"); code != http.StatusTooManyRequests {
		t.Fatalf("unknown address during the lock = %d, want 429", code)
	}
	if code := login("198.51.100.7", "correct-horse-battery"); code != http.StatusOK {
		t.Fatalf("known address during the lock = %d, want 200", code)
	}
}
