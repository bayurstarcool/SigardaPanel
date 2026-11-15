package apihttp

// Check-in: once a day the panel tells the master panel (panel.sigarda.com)
// that it is running, so every installation has a license and Sigarda can
// see how many panels run which version. The master answers with the
// panel's license key (a free license bound to this installation on the
// first check-in, an upgraded or renewed one later), whether the current
// license was revoked, and the latest version.
//
// Sent: a random install id (no name, domain or email), the version, tier
// and license id, OS and architecture, and the number of servers and sites
// as ranges. Never blocking: without the master the panel keeps running
// and tries again later. Turn off with SIGARDAPANEL_CHECKIN=off.

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/features"
	"sigardapanel/internal/license"
	"sigardapanel/internal/store"
)

const (
	settingInstallID         = "install_id"
	settingCheckinAt         = "checkin_last_at"
	settingCheckinError      = "checkin_last_error"
	settingCheckinLatest     = "checkin_latest_version"
	settingCheckinUpgradeURL = "checkin_upgrade_url"
	defaultMasterURL         = "https://panel.sigarda.com"
)

var checkinHTTP = &http.Client{Timeout: 20 * time.Second}

// checkinMu keeps the scheduled and the manual check-in from overlapping.
var checkinMu sync.Mutex

func checkinEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SIGARDAPANEL_CHECKIN"))) {
	case "off", "false", "0", "no", "disabled":
		return false
	}
	return true
}

// masterURL is the master panel (SIGARDAPANEL_MASTER_URL): https, or http
// to a loopback address (tests, development).
func masterURL() (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("SIGARDAPANEL_MASTER_URL")), "/")
	if raw == "" {
		raw = defaultMasterURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("SIGARDAPANEL_MASTER_URL is not a URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return "", fmt.Errorf("SIGARDAPANEL_MASTER_URL must be https")
	}
	return raw, nil
}

func isLoopbackHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// installID returns this installation's id, creating it once.
func installID(ctx context.Context, db *sql.DB) (string, error) {
	if v, err := store.GetSetting(ctx, db, settingInstallID); err == nil && v != "" {
		return v, nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // UUID v4
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	id := h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
	// INSERT OR IGNORE semantics: a concurrent first call keeps one id.
	if _, err := db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO NOTHING`, settingInstallID, id); err != nil {
		return "", err
	}
	return store.GetSetting(ctx, db, settingInstallID)
}

// bucket turns a count into the range sent to the master.
func bucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n == 1:
		return "1"
	case n <= 5:
		return "2-5"
	case n <= 20:
		return "6-20"
	case n <= 100:
		return "21-100"
	default:
		return "100+"
	}
}

// checkinPayload is exactly what is sent (also shown in Settings).
type checkinPayload struct {
	InstallID string `json:"install_id"`
	Version   string `json:"version"`
	Tier      string `json:"tier"`
	LicenseID string `json:"license_id,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Servers   string `json:"servers"`
	Sites     string `json:"sites"`
}

func buildCheckin(ctx context.Context, s *store.Store) (checkinPayload, error) {
	id, err := installID(ctx, s.DB())
	if err != nil {
		return checkinPayload{}, err
	}
	eff := currentLicense(ctx, s)
	p := checkinPayload{InstallID: id, Version: buildinfo.Version, Tier: eff.Tier, OS: runtime.GOOS, Arch: runtime.GOARCH}
	if eff.Claims != nil {
		p.LicenseID = eff.Claims.ID
	}
	if servers, err := s.Servers.List(ctx); err == nil {
		p.Servers = bucket(len(servers))
	}
	if n, err := s.Sites.Count(ctx, store.SiteListFilter{}); err == nil {
		p.Sites = bucket(n)
	}
	return p, nil
}

type checkinAnswer struct {
	LicenseKey    string `json:"license_key"`
	LicenseStatus string `json:"license_status"` // active, revoked
	RevokedID     string `json:"revoked_license_id"`
	LatestVersion string `json:"latest_version"`
	UpgradeURL    string `json:"upgrade_url"`
	Message       string `json:"message"`
}

// runCheckin checks in once and applies the answer.
func runCheckin(ctx context.Context, s *store.Store) (checkinAnswer, error) {
	checkinMu.Lock()
	defer checkinMu.Unlock()
	db := s.DB()
	var ans checkinAnswer
	err := func() error {
		base, err := masterURL()
		if err != nil {
			return err
		}
		p, err := buildCheckin(ctx, s)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(p)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/checkin", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "SigardaPanel/"+buildinfo.Version)
		resp, err := checkinHTTP.Do(req)
		if err != nil {
			return fmt.Errorf("master panel unreachable: %w", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("master panel answered %d", resp.StatusCode)
		}
		var env struct {
			Data checkinAnswer `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("master panel answer is not valid JSON")
		}
		ans = env.Data
		return applyCheckin(ctx, s, p.InstallID, ans)
	}()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = store.SetSetting(ctx, db, settingCheckinAt, now)
	if err != nil {
		_ = store.SetSetting(ctx, db, settingCheckinError, err.Error())
		return ans, err
	}
	_ = store.SetSetting(ctx, db, settingCheckinError, "")
	_ = store.SetSetting(ctx, db, settingCheckinLatest, ans.LatestVersion)
	if u, err := url.Parse(ans.UpgradeURL); err == nil && u.Scheme == "https" {
		_ = store.SetSetting(ctx, db, settingCheckinUpgradeURL, ans.UpgradeURL)
	}
	return ans, nil
}

// applyCheckin installs a key the master delivered (it must be valid and
// bound to this installation) and drops a revoked license.
func applyCheckin(ctx context.Context, s *store.Store, id string, ans checkinAnswer) error {
	eff := currentLicense(ctx, s)
	if ans.LicenseStatus == "revoked" && eff.Claims != nil && ans.RevokedID == eff.Claims.ID {
		if err := s.Licenses.Deactivate(ctx); err != nil {
			return err
		}
		slog.Warn("license revoked by the master panel", "license_id", ans.RevokedID)
		eff = currentLicense(ctx, s)
	}
	key := strings.TrimSpace(ans.LicenseKey)
	if key == "" {
		return nil
	}
	c, err := license.VerifyFor(key, id, time.Now())
	if err != nil {
		return fmt.Errorf("delivered license key refused: %w", err)
	}
	if c.InstallID != id {
		return errors.New("delivered license key is not bound to this installation")
	}
	if eff.Stored != nil && eff.Stored.Key == key {
		return nil // already active
	}
	// A key the admin activated by hand is kept unless the delivered one is
	// at least as high a tier (an upgrade or renewal of the same license).
	if eff.Status == "active" && eff.Claims != nil && eff.Claims.ID != c.ID && features.TierLevel[c.Tier] < features.TierLevel[eff.Tier] {
		return nil
	}
	maxServers := c.MaxServers
	if err := s.Licenses.Deactivate(ctx); err != nil {
		return err
	}
	if _, err := s.Licenses.Activate(ctx, key, c.Tier, maxServers); err != nil {
		return err
	}
	slog.Info("license delivered by the master panel", "license_id", c.ID, "tier", c.Tier)
	return nil
}

// StartCheckin checks in a few minutes after start and then about daily.
func StartCheckin(ctx context.Context, s *store.Store) {
	if !checkinEnabled() {
		slog.Info("check-in with the master panel is off (SIGARDAPANEL_CHECKIN)")
		return
	}
	go func() {
		wait := 2*time.Minute + jitter(3*time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if _, err := runCheckin(ctx, s); err != nil {
				slog.Info("check-in with the master panel failed; retrying later", "error", err)
				wait = time.Hour + jitter(time.Hour)
			} else {
				wait = 24*time.Hour + jitter(2*time.Hour)
			}
		}
	}()
}

func jitter(max time.Duration) time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64())
}
