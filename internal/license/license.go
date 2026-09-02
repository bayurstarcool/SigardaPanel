package license

import (
	"encoding/json"
	"fmt"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// Info holds license information.
type Info struct {
	Valid    bool     `json:"valid"`
	Plan     string   `json:"plan"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Company  string   `json:"company"`
	Features []string `json:"features"`
}

// Validator handles license validation with the member portal.
type Validator struct {
	memberPortalURL string
	cache           *Info
	cacheExpiry     time.Time
	mu              sync.RWMutex
}

// NewValidator creates a new license validator.
func NewValidator() *Validator {
	url := os.Getenv("MEMBER_PORTAL_URL")
	if url == "" {
		url = "http://localhost:7800"
	}
	return &Validator{memberPortalURL: url}
}

// Validate validates a license key with the member portal.
func (v *Validator) Validate(licenseKey string) (*Info, error) {
	v.mu.RLock()
	if v.cache != nil && time.Now().Before(v.cacheExpiry) && v.cache.Valid {
		cached := v.cache
		v.mu.RUnlock()
		if cached.Plan != "" {
			return cached, nil
		}
	}
	v.mu.RUnlock()

	url := fmt.Sprintf("%s/api/license/validate", v.memberPortalURL)
	body := fmt.Sprintf(`{"license_key":"%s"}`, licenseKey)

	resp, err := http.Post(url, "application/json", 
		io.NopCloser(
			bytes.NewReader([]byte(body)),
		),
	)
	if err != nil {
		slog.Warn("license validation failed", "error", err)
		return &Info{Valid: false}, nil
	}
	defer resp.Body.Close()

	var result Info
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &Info{Valid: false}, nil
	}

	v.mu.Lock()
	v.cache = &result
	v.cacheExpiry = time.Now().Add(1 * time.Hour)
	v.mu.Unlock()

	return &result, nil
}

// HasFeature checks if the license has a specific feature.
func (v *Validator) HasFeature(info *Info, feature string) bool {
	if info == nil || !info.Valid {
		return false
	}
	for _, f := range info.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsPremium checks if the license is premium or higher.
func (v *Validator) IsPremium(info *Info) bool {
	return info != nil && info.Valid && (info.Plan == "premium" || info.Plan == "enterprise")
}

// IsEnterprise checks if the license is enterprise.
func (v *Validator) IsEnterprise(info *Info) bool {
	return info != nil && info.Valid && info.Plan == "enterprise"
}
