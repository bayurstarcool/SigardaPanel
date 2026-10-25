package apihttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/store"
)

func cookieLogin(t *testing.T) (*echo.Echo, string) {
	t.Helper()
	e, s, ctx := newTestApp(t)
	user := &store.User{Username: "cookietester", Email: "cookie@test.com", PasswordHash: "password123", Role: "user"}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rec := doRequest(t, e, "POST", "/api/v1/auth/login",
		map[string]string{"username": "cookietester", "password": "password123"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d (%s)", rec.Code, rec.Body.String())
	}
	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Token == "" {
		t.Fatal("token is empty")
	}
	return e, result.Data.Token
}

func TestLoginSetsHttpOnlySessionCookie(t *testing.T) {
	e, _ := cookieLogin(t)

	rec := doRequest(t, e, "POST", "/api/v1/auth/login",
		map[string]string{"username": "cookietester", "password": "password123"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d", rec.Code)
	}
	var found bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name != sessionCookieName {
			continue
		}
		found = true
		if ck.Value == "" {
			t.Fatal("session cookie value is empty")
		}
		if !ck.HttpOnly {
			t.Fatal("session cookie must be HttpOnly")
		}
		if ck.SameSite != http.SameSiteStrictMode {
			t.Fatalf("session cookie SameSite = %v, want Strict", ck.SameSite)
		}
		if ck.Path != "/" {
			t.Fatalf("session cookie Path = %q, want /", ck.Path)
		}
	}
	if !found {
		t.Fatal("login response did not set the session cookie")
	}
}

func TestRequireAuthAcceptsSessionCookie(t *testing.T) {
	e, token := cookieLogin(t)

	rec := doRequestWithHeaders(t, e, "GET", "/api/v1/auth/me", nil, "",
		map[string]string{"Cookie": sessionCookieName + "=" + token})
	if rec.Code != http.StatusOK {
		t.Fatalf("auth/me with session cookie: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	e, token := cookieLogin(t)

	rec := doRequestWithHeaders(t, e, "POST", "/api/v1/auth/logout", nil, "",
		map[string]string{"Cookie": sessionCookieName + "=" + token})
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: got %d", rec.Code)
	}
	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not expire the session cookie")
	}

	// The revoked session must no longer authenticate, cookie or not.
	rec2 := doRequestWithHeaders(t, e, "GET", "/api/v1/auth/me", nil, "",
		map[string]string{"Cookie": sessionCookieName + "=" + token})
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("auth/me after logout: got %d, want 401", rec2.Code)
	}
}

func TestRegisterCanBeDisabled(t *testing.T) {
	e, _, _ := newTestApp(t)

	t.Setenv("SIGARDAPANEL_ALLOW_REGISTRATION", "false")
	rec := doRequest(t, e, "POST", "/api/v1/auth/register", map[string]string{
		"name": "x", "email": "x@test.com", "password": "password123",
	}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("register with registration disabled: got %d, want 403", rec.Code)
	}
}
