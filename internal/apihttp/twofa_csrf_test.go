package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func TestTwoFactorManagementNeedsOwnInteractiveSession(t *testing.T) {
	f := newAuthzFixture(t)
	alice, aliceTok := f.user("alice", auth.RoleUser)
	key, _ := auth.NewToken()
	keyResp := f.do("POST", "/api/v1/api-keys", map[string]any{"name": "ci"}, aliceTok)
	var created apiKeyCreateResp
	json.Unmarshal(keyResp.Body.Bytes(), &created)
	_ = key

	// Enable 2FA for alice directly.
	secret := "JBSWY3DPEHPK3PXP"
	f.s.TwoFactor.SetTOTPSecret(f.ctx, alice.ID, secret)
	f.s.TwoFactor.EnableTOTP(f.ctx, alice.ID)
	f.s.TwoFactor.StoreRecoveryCodes(f.ctx, alice.ID, []string{"h1", "h2"})

	if rec := f.do("POST", "/api/v1/auth/2fa/recovery-codes", nil, created.Data.Token); rec.Code != http.StatusForbidden {
		t.Fatalf("API key minting recovery codes = %d %s", rec.Code, rec.Body.String())
	}
	root, _ := f.user("root2fa", auth.RoleSuperAdmin)
	imp, _ := auth.NewToken()
	f.s.Sessions.Create(f.ctx, &store.Session{UserID: alice.ID, TokenHash: auth.HashToken(imp), ExpiresAt: time.Now().Add(time.Hour), ImpersonatedBy: nullInt(root.ID)})
	if rec := f.do("POST", "/api/v1/auth/2fa/disable", map[string]any{"code": "000000"}, imp); rec.Code != http.StatusForbidden {
		t.Fatalf("impersonator changing 2FA = %d", rec.Code)
	}
	// Replacing existing codes needs a current TOTP code.
	if rec := f.do("POST", "/api/v1/auth/2fa/recovery-codes", nil, aliceTok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("replace codes without TOTP = %d %s", rec.Code, rec.Body.String())
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if rec := f.do("POST", "/api/v1/auth/2fa/recovery-codes", map[string]any{"code": code}, aliceTok); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "recovery_codes") {
		t.Fatalf("replace codes with TOTP = %d %s", rec.Code, rec.Body.String())
	}
	// The same code cannot be used twice, and wrong guesses are limited.
	if rec := f.do("POST", "/api/v1/auth/2fa/recovery-codes", map[string]any{"code": code}, aliceTok); rec.Code == http.StatusOK {
		t.Fatal("a TOTP code was accepted twice")
	}
	for i := 0; i < 6; i++ {
		if r := f.do("POST", "/api/v1/auth/2fa/disable", map[string]any{"code": "000001"}, aliceTok); r.Code == http.StatusOK {
			t.Fatal("a wrong code disabled 2FA")
		}
	}
	next, _ := totp.GenerateCode(secret, time.Now().Add(30*time.Second))
	if rec := f.do("POST", "/api/v1/auth/2fa/disable", map[string]any{"code": next}, aliceTok); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after repeated wrong codes = %d %s", rec.Code, rec.Body.String())
	}
	if on, _ := f.s.TwoFactor.IsTOTPEnabled(f.ctx, alice.ID); !on {
		t.Fatal("2FA was disabled by failed attempts")
	}
	// Disabling 2FA drops the recovery codes.
	f.s.TwoFactor.DisableTOTP(f.ctx, alice.ID)
	if n, _ := f.s.TwoFactor.CountRecoveryCodes(f.ctx, alice.ID); n != 0 {
		t.Fatalf("%d recovery codes survived disabling 2FA", n)
	}
}

func TestCookieCSRFGuard(t *testing.T) {
	f := newAuthzFixture(t)
	_, tok := f.user("carol", auth.RoleUser)
	send := func(headers map[string]string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"x"}`))
		req.Host = "panel.example.com"
		req.Header.Set("Content-Type", "text/plain")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tok})
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		f.e.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send(map[string]string{"Origin": "https://evil.example.net"}); code != http.StatusForbidden {
		t.Errorf("cross-site Origin = %d", code)
	}
	if code := send(map[string]string{"Sec-Fetch-Site": "same-site"}); code != http.StatusForbidden {
		t.Errorf("sibling subdomain (same-site) = %d", code)
	}
	if code := send(map[string]string{"Origin": "https://panel.example.com", "Sec-Fetch-Site": "same-origin"}); code == http.StatusForbidden {
		t.Errorf("same-origin request refused")
	}
}
