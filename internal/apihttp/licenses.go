package apihttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/features"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/license"
	"sigardapanel/internal/store"
)

type licenseHandler struct {
	store *store.Store
}

func newLicenseHandler(s *store.Store) *licenseHandler {
	return &licenseHandler{store: s}
}

// effectiveLicense is the panel's license as it stands now: the stored key
// is verified (signature, expiry) every time, so an expired, altered or
// made-up key never grants a tier.
type effectiveLicense struct {
	Tier       string
	Status     string // none, active, expired, invalid
	Message    string
	Claims     *license.Claims
	Stored     *store.License
	MaxServers int
}

func currentLicense(ctx context.Context, s *store.Store) effectiveLicense {
	out := effectiveLicense{Tier: features.TierFree, Status: "none", MaxServers: features.TierLimits[features.TierFree].MaxServers}
	lic, err := s.Licenses.GetActive(ctx)
	if err != nil || lic == nil {
		return out
	}
	out.Stored = lic
	c, err := license.Verify(lic.Key, time.Now())
	out.Claims = c
	switch {
	case err == nil:
		out.Tier, out.Status = c.Tier, "active"
		out.MaxServers = c.MaxServers
		if out.MaxServers == 0 {
			out.MaxServers = features.TierLimits[c.Tier].MaxServers
		}
	case errors.Is(err, license.ErrExpired):
		out.Status, out.Message = "expired", "The license expired on "+c.ExpiresAt.Format("2006-01-02")+": renew it to get its features back."
	default:
		// Includes keys activated before keys were signed (plain hex).
		out.Status, out.Message = "invalid", "The stored license key is not valid ("+err.Error()+"): activate a key issued by SigardaPanel."
	}
	return out
}

func maskKey(k string) string {
	if len(k) > 12 {
		return k[:12] + "…"
	}
	return "…"
}

// getLicense returns the current license and the feature access map.
func (h *licenseHandler) getLicense(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	eff := currentLicense(ctx, h.store)

	var activatedAt, expiresAt, key, licensee, licenseID *string
	if eff.Stored != nil {
		if eff.Stored.ActivatedAt.Valid {
			s := eff.Stored.ActivatedAt.Time.UTC().Format(time.RFC3339)
			activatedAt = &s
		}
		k := maskKey(eff.Stored.Key)
		key = &k
	}
	if eff.Claims != nil {
		if !eff.Claims.ExpiresAt.IsZero() {
			s := eff.Claims.ExpiresAt.UTC().Format(time.RFC3339)
			expiresAt = &s
		}
		licensee, licenseID = &eff.Claims.Licensee, &eff.Claims.ID
	}

	servers, err := h.store.Servers.List(ctx)
	if err != nil {
		servers = []store.Server{}
	}
	limits := features.TierLimits[eff.Tier]

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"tier":         eff.Tier,
			"tier_name":    features.TierDescriptions[eff.Tier],
			"status":       eff.Status,
			"message":      eff.Message,
			"key":          key,
			"licensee":     licensee,
			"license_id":   licenseID,
			"activated_at": activatedAt,
			"expires_at":   expiresAt,
			"features":     features.TierFeatures(eff.Tier),
			"limits": map[string]any{
				"servers": map[string]any{"current": len(servers), "max": eff.MaxServers},
				"sites":   map[string]any{"current": 0, "max": limits.MaxSites},
			},
		},
		RequestID: rid,
	})
}

// activateLicense verifies a signed license key and makes it the panel's
// license. Only keys issued (signed) by SigardaPanel are accepted.
func (h *licenseHandler) activateLicense(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()

	var req struct {
		Key string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	key := strings.Join(strings.Fields(req.Key), "")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "license key is required")
	}
	claims, err := license.Verify(key, time.Now())
	if err != nil {
		if errors.Is(err, license.ErrNoKeys) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	maxServers := claims.MaxServers
	if maxServers == 0 {
		maxServers = features.TierLimits[claims.Tier].MaxServers
	}
	if err := h.store.Licenses.Deactivate(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to deactivate old license")
	}
	if _, err := h.store.Licenses.Activate(ctx, key, claims.Tier, maxServers); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to activate license")
	}

	var expires *string
	if !claims.ExpiresAt.IsZero() {
		s := claims.ExpiresAt.UTC().Format(time.RFC3339)
		expires = &s
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"status":     "activated",
			"tier":       claims.Tier,
			"licensee":   claims.Licensee,
			"license_id": claims.ID,
			"expires_at": expires,
			"key":        maskKey(key),
		},
		RequestID: rid,
	})
}

// deactivateLicense reverts to the free tier.
func (h *licenseHandler) deactivateLicense(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	if err := h.store.Licenses.Deactivate(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to deactivate license")
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "deactivated", "tier": features.TierFree},
		RequestID: rid,
	})
}

// getFeatures returns all features with access status for the current tier.
func (h *licenseHandler) getFeatures(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	tier := currentLicense(c.Request().Context(), h.store).Tier
	featureMap := features.TierFeatures(tier)

	type featureInfo struct {
		Name         string `json:"name"`
		Available    bool   `json:"available"`
		RequiredTier string `json:"required_tier"`
	}
	all := make([]featureInfo, 0, len(features.FeatureTierMap))
	for feat, reqTier := range features.FeatureTierMap {
		all = append(all, featureInfo{Name: feat, Available: featureMap[feat], RequiredTier: reqTier})
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"current_tier": tier, "features": all},
		RequestID: rid,
	})
}
