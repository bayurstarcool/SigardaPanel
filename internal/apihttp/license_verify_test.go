package apihttp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/license"
)

func TestLicenseActivationVerifiesSignedKeys(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := license.PublicKeys
	license.PublicKeys = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { license.PublicKeys = old })

	f := newAuthzFixture(t)
	_, root := f.user("root", auth.RoleSuperAdmin)
	state := func() (tier, status string) {
		var resp struct {
			Data struct{ Tier, Status string } `json:"data"`
		}
		json.Unmarshal(f.do("GET", "/api/v1/license", nil, root).Body.Bytes(), &resp)
		return resp.Data.Tier, resp.Data.Status
	}
	now := time.Now().UTC().Truncate(time.Second)
	sign := func(c license.Claims) string {
		k, err := license.Sign(priv, c)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}

	// A made-up hex key (accepted before keys were signed) is refused.
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": "0123456789abcdef0123456789abcdef"}, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("hex key = %d", rec.Code)
	}
	// A key signed by someone else is refused.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	foreign, _ := license.Sign(other, license.Claims{ID: "x", Tier: "max", IssuedAt: now})
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": foreign}, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign key = %d", rec.Code)
	}
	if tier, status := state(); tier != "free" || status != "none" {
		t.Fatalf("after refusals: %s %s", tier, status)
	}

	// A valid key activates its tier.
	key := sign(license.Claims{ID: "lic_1", Licensee: "Acme", Tier: "pro", MaxServers: 7, IssuedAt: now, ExpiresAt: now.AddDate(1, 0, 0)})
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": key}, root); rec.Code != http.StatusOK {
		t.Fatalf("valid key = %d %s", rec.Code, rec.Body)
	}
	if tier, status := state(); tier != "pro" || status != "active" {
		t.Fatalf("after activation: %s %s", tier, status)
	}

	// An expired key is refused.
	expired := sign(license.Claims{ID: "lic_2", Tier: "max", IssuedAt: now.AddDate(-2, 0, 0), ExpiresAt: now.AddDate(-1, 0, 0)})
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": expired}, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired key = %d", rec.Code)
	}

	// A plain key stored by an older panel no longer grants a tier.
	if _, err := f.s.Licenses.Activate(f.ctx, "0123456789abcdef0123456789abcdef", "pro", 10); err != nil {
		t.Fatal(err)
	}
	if tier, status := state(); tier != "free" || status != "invalid" {
		t.Fatalf("legacy key: %s %s", tier, status)
	}
}
