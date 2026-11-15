package apihttp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/license"
)

func TestCheckinGetsBoundLicense(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	oldPub := license.PublicKeys
	license.PublicKeys = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	t.Cleanup(func() { license.PublicKeys = oldPub })

	var got checkinPayload
	answer := func(p checkinPayload) map[string]any { return map[string]any{} }
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/checkin" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{"data": answer(got)})
	}))
	defer master.Close()
	t.Setenv("SIGARDAPANEL_MASTER_URL", master.URL)

	f := newAuthzFixture(t)
	_, root := f.user("root", auth.RoleSuperAdmin)
	sign := func(c license.Claims) string {
		k, err := license.Sign(priv, c)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	state := func() (tier, status string) {
		var r struct {
			Data struct{ Tier, Status string } `json:"data"`
		}
		json.Unmarshal(f.do("GET", "/api/v1/license", nil, root).Body.Bytes(), &r)
		return r.Data.Tier, r.Data.Status
	}
	checkin := func(want int) {
		t.Helper()
		if rec := f.do("POST", "/api/v1/license/checkin", nil, root); rec.Code != want {
			t.Fatalf("checkin = %d %s", rec.Code, rec.Body)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)

	// First check-in: the master issues a free license bound to the install.
	answer = func(p checkinPayload) map[string]any {
		return map[string]any{"license_key": sign(license.Claims{ID: "free-1", Tier: "free", IssuedAt: now, InstallID: p.InstallID}), "latest_version": "9.9.9"}
	}
	checkin(http.StatusOK)
	if got.InstallID == "" || got.Version == "" || got.Tier != "free" || got.OS == "" {
		t.Fatalf("payload: %+v", got)
	}
	if strings.Contains(strings.ToLower(got.InstallID), "root") {
		t.Fatal("install id carries user data")
	}
	if tier, status := state(); tier != "free" || status != "active" {
		t.Fatalf("after free: %s %s", tier, status)
	}
	installID := got.InstallID

	// A key bound to another installation is refused.
	answer = func(p checkinPayload) map[string]any {
		return map[string]any{"license_key": sign(license.Claims{ID: "pro-x", Tier: "pro", IssuedAt: now, InstallID: "someone-else"})}
	}
	checkin(http.StatusBadGateway)
	if tier, _ := state(); tier != "free" {
		t.Fatalf("foreign key applied: %s", tier)
	}

	// An upgrade bought on the master arrives with the next check-in.
	answer = func(p checkinPayload) map[string]any {
		return map[string]any{"license_key": sign(license.Claims{ID: "pro-1", Tier: "pro", MaxServers: 5, IssuedAt: now, ExpiresAt: now.AddDate(1, 0, 0), InstallID: p.InstallID})}
	}
	checkin(http.StatusOK)
	if tier, status := state(); tier != "pro" || status != "active" || got.InstallID != installID {
		t.Fatalf("after upgrade: %s %s", tier, status)
	}
	if got.LicenseID != "free-1" { // payload of that check-in still had the free license
		t.Fatalf("license id sent: %q", got.LicenseID)
	}

	// The master revokes it: the panel drops it.
	answer = func(p checkinPayload) map[string]any {
		return map[string]any{"license_status": "revoked", "revoked_license_id": p.LicenseID}
	}
	checkin(http.StatusOK)
	if tier, _ := state(); tier != "free" {
		t.Fatalf("after revoke: %s", tier)
	}

	// A key bound to this install cannot be pasted into another panel.
	bound := sign(license.Claims{ID: "pro-2", Tier: "pro", IssuedAt: now, InstallID: "another-install"})
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": bound}, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("bound key on other install = %d", rec.Code)
	}

	// Turned off: no check-in.
	t.Setenv("SIGARDAPANEL_CHECKIN", "off")
	checkin(http.StatusConflict)
}

func TestMasterURLMustBeHTTPS(t *testing.T) {
	for raw, ok := range map[string]bool{"": true, "https://panel.sigarda.com/": true, "http://127.0.0.1:9": true, "http://panel.sigarda.com": false, "ftp://x": false} {
		t.Setenv("SIGARDAPANEL_MASTER_URL", raw)
		if _, err := masterURL(); (err == nil) != ok {
			t.Errorf("%q: %v", raw, err)
		}
	}
}
