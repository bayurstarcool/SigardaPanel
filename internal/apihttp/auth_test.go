package apihttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func newTestApp(t *testing.T) (*echo.Echo, *store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "api_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	migrations.Up(d, migrations.AllMigrations)
	s := store.New(d)
	e := NewRouter(s, d, nil)
	return e, s, t.Context()
}

func doRequest(t *testing.T, e *echo.Echo, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLogin_Success(t *testing.T) {
	e, s, ctx := newTestApp(t)

	user := &store.User{Username: "admin", Email: "admin@test.com", PasswordHash: "password123", Role: "admin"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "password123"}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)

	if result.Data.Token == "" {
		t.Fatal("token is empty")
	}
	if len(result.Data.Token) != 64 {
		t.Fatalf("expected token length 64, got %d", len(result.Data.Token))
	}
	if result.Data.User.Username != "admin" {
		t.Fatalf("expected username admin, got %s", result.Data.User.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	e, s, ctx := newTestApp(t)

	user := &store.User{Username: "alice", Email: "alice@test.com", PasswordHash: "correct-password", Role: "user"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/auth/login", map[string]string{"username": "alice", "password": "wrong-password"}, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp.Error.Code != "invalid_credentials" {
		t.Fatalf("expected invalid_credentials, got %s", errResp.Error.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	e, _, _ := newTestApp(t)

	rec := doRequest(t, e, "POST", "/api/v1/auth/login", map[string]string{"username": "nobody", "password": "any"}, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogout_Success(t *testing.T) {
	e, s, ctx := newTestApp(t)

	user := &store.User{Username: "bob", Email: "bob@test.com", PasswordHash: "secret", Role: "user"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/auth/logout", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLogout_NoToken(t *testing.T) {
	e, _, _ := newTestApp(t)

	rec := doRequest(t, e, "POST", "/api/v1/auth/logout", nil, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMe_Success(t *testing.T) {
	e, s, ctx := newTestApp(t)

	user := &store.User{Username: "carol", Email: "carol@test.com", PasswordHash: "pass123", Role: "admin"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/auth/me", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	data := result["data"].(map[string]any)
	if data["username"] != "carol" {
		t.Fatalf("expected carol, got %v", data["username"])
	}
}

func TestMe_InvalidToken(t *testing.T) {
	e, _, _ := newTestApp(t)

	rec := doRequest(t, e, "GET", "/api/v1/auth/me", nil, makeToken(64))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedEndpoints(t *testing.T) {
	e, s, ctx := newTestApp(t)

	user := &store.User{Username: "dave", Email: "dave@test.com", PasswordHash: "pass", Role: "user"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{UserID: user.ID, TokenHash: auth.HashToken(token)}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	endpoints := []struct {
		method, path string
	}{
		{"GET", "/api/v1/servers"},
		{"GET", "/api/v1/sites"},
		{"GET", "/api/v1/jobs"},
	}
	for _, ep := range endpoints {
		rec := doRequest(t, e, ep.method, ep.path, nil, token)
		// These return 501 Not Implemented, not 401 (token is valid)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s returned 401 instead of 501", ep.method, ep.path)
		}
	}
}

func TestProtectedEndpoints_NoToken(t *testing.T) {
	e, _, _ := newTestApp(t)

	endpoints := []struct {
		method, path string
	}{
		{"GET", "/api/v1/servers"},
		{"POST", "/api/v1/servers"},
		{"GET", "/api/v1/sites"},
		{"POST", "/api/v1/sites"},
		{"GET", "/api/v1/jobs"},
		{"POST", "/api/v1/auth/logout"},
	}
	for _, ep := range endpoints {
		rec := doRequest(t, e, ep.method, ep.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

func makeToken(n int) string {
	b := make([]byte, n/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}
