package apihttp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/features"
	"sigardapanel/internal/license"
)

// Free covers one server with every feature; a paid key raises the server count.
func TestServerLimitFollowsTheLicense(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := license.PublicKeys
	license.PublicKeys = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { license.PublicKeys = old })

	f := newAuthzFixture(t)
	_, root := f.user("root", auth.RoleSuperAdmin)
	add := func(name, ip string) int {
		return f.do("POST", "/api/v1/servers", map[string]any{"name": name, "hostname": name, "ip_address": ip}, root).Code
	}
	count := func() int { l, _ := f.s.Servers.List(f.ctx); return len(l) }
	if count() == 0 {
		if code := add("first", "203.0.113.11"); code != http.StatusCreated {
			t.Fatalf("first server on free = %d", code)
		}
	}
	if code := add("second", "203.0.113.12"); code != http.StatusForbidden {
		t.Fatalf("second server on free = %d, want 403", code)
	}

	id, _ := installID(f.ctx, f.s.DB())
	now := time.Now().UTC().Truncate(time.Second)
	key, err := license.Sign(priv, license.Claims{ID: "lic_basic", Tier: "basic", InstallID: id, IssuedAt: now, ExpiresAt: now.AddDate(1, 0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if rec := f.do("POST", "/api/v1/license/activate", map[string]any{"key": key}, root); rec.Code != http.StatusOK {
		t.Fatalf("activate basic = %d %s", rec.Code, rec.Body)
	}
	for i := count(); i < 3; i++ {
		if code := add("extra"+string(rune('a'+i)), "203.0.113."+string(rune('2'))+string(rune('0'+i))); code != http.StatusCreated {
			t.Fatalf("server %d on basic = %d", i+1, code)
		}
	}
	if code := add("fourth", "203.0.113.44"); code != http.StatusForbidden {
		t.Fatalf("fourth server on basic (3) = %d, want 403", code)
	}
}

func TestFreeTierHasEveryFeatureButMultiServer(t *testing.T) {
	for feat, tier := range features.FeatureTierMap {
		switch feat {
		case features.FeatMultiServer, features.FeatAgentManagement, features.FeatWhiteLabel, features.FeatPrioritySupport:
			if features.HasFeature("free", feat) {
				t.Errorf("free has %s (needs %s)", feat, tier)
			}
		default:
			if !features.HasFeature("free", feat) {
				t.Errorf("free lacks %s", feat)
			}
		}
	}
	if features.TierLimits["free"].MaxServers != 1 || features.TierLimits["free"].MaxSites != -1 {
		t.Errorf("free limits: %+v", features.TierLimits["free"])
	}
}
