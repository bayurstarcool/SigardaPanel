package apihttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/store"
)

func loginFrom(t *testing.T, e *echo.Echo, remote string, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func newUserWith2FA(t *testing.T) (*echo.Echo, *store.Store, string) {
	t.Helper()
	e, s, ctx := newTestApp(t)
	u := &store.User{Username: "bob", Email: "bob@test.com", PasswordHash: "password123", Role: "admin"}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TwoFactor.SetTOTPSecret(ctx, u.ID, key.Secret()); err != nil {
		t.Fatal(err)
	}
	if err := s.TwoFactor.EnableTOTP(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	codes := []string{"AAAA-BBBB-CCCC-DDDD"}
	if err := s.TwoFactor.StoreRecoveryCodes(ctx, u.ID, []string{hashRecoveryCode(codes[0])}); err != nil {
		t.Fatal(err)
	}
	return e, s, key.Secret()
}

func TestLogin_RecoveryCodeIsOneTime(t *testing.T) {
	e, _, _ := newUserWith2FA(t)
	login := func(code string) int {
		return loginFrom(t, e, "198.51.100.1:1", map[string]string{"username": "bob", "password": "password123", "recovery_code": code}).Code
	}
	if got := login("aaaa bbbb cccc dddd"); got != http.StatusOK { // normalisation: case + spaces
		t.Fatalf("valid recovery code: got %d", got)
	}
	if got := login("AAAA-BBBB-CCCC-DDDD"); got != http.StatusUnauthorized {
		t.Fatalf("reused recovery code must fail, got %d", got)
	}
	if got := login("ZZZZ-ZZZZ-ZZZZ-ZZZZ"); got != http.StatusUnauthorized {
		t.Fatalf("wrong recovery code must fail, got %d", got)
	}
}

func TestLogin_TOTPReplayRejected(t *testing.T) {
	e, _, secret := newUserWith2FA(t)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"username": "bob", "password": "password123", "totp_code": code}
	if got := loginFrom(t, e, "198.51.100.2:1", body).Code; got != http.StatusOK {
		t.Fatalf("first use: got %d", got)
	}
	if got := loginFrom(t, e, "198.51.100.2:1", body).Code; got != http.StatusUnauthorized {
		t.Fatalf("replayed TOTP must fail, got %d", got)
	}
}

func TestLogin_AccountLockoutAcrossIPs(t *testing.T) {
	e, s, ctx := newTestApp(t)
	u := &store.User{Username: "carol", Email: "c@test.com", PasswordHash: "password123", Role: "user"}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ { // every attempt from a different IP
		rec := loginFrom(t, e, fmt.Sprintf("203.0.113.%d:1", i+1), map[string]string{"username": "carol", "password": "wrong"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i, rec.Code)
		}
	}
	// Even the correct password is refused while the account is locked.
	rec := loginFrom(t, e, "203.0.113.200:1", map[string]string{"username": "carol", "password": "password123"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked account: got %d, want 429", rec.Code)
	}
}
