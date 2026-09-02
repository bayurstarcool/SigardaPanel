package apihttp

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/features"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type licenseHandler struct {
	store *store.Store
}

func newLicenseHandler(s *store.Store) *licenseHandler {
	return &licenseHandler{store: s}
}

// GetLicense returns current license info + feature access map.
func (h *licenseHandler) getLicense(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	license, err := h.store.Licenses.GetActive(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get license")
	}

	tier := features.TierFree
	var activatedAt, expiresAt *string
	var licenseKey *string

	if license != nil {
		tier = license.Tier
		if license.ActivatedAt.Valid {
			s := license.ActivatedAt.Time.Format("2006-01-02T15:04:05Z")
			activatedAt = &s
		}
		if license.ExpiresAt.Valid {
			s := license.ExpiresAt.Time.Format("2006-01-02T15:04:05Z")
			expiresAt = &s
		}
		// Mask key: show only first 8 chars
		if len(license.Key) > 8 {
			masked := license.Key[:8] + "..."
			licenseKey = &masked
		}
	}

	limits := features.TierLimits[tier]
	featureMap := features.TierFeatures(tier)

	// Count current servers
	serverCount, err := h.store.Servers.List(c.Request().Context())
	if err != nil {
		// Log error but don't fail the request
		serverCount = []store.Server{}
	}

	data := map[string]interface{}{
		"tier":         tier,
		"tier_name":    features.TierDescriptions[tier],
		"key":          licenseKey,
		"activated_at": activatedAt,
		"expires_at":   expiresAt,
		"features":     featureMap,
		"limits": map[string]interface{}{
			"servers": map[string]interface{}{
				"current": len(serverCount),
				"max":     limits.MaxServers,
			},
			"sites": map[string]interface{}{
				"current": 0,
				"max":     limits.MaxSites,
			},
		},
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      data,
		RequestID: rid,
	})
}

// ActivateLicense activates a license key.
func (h *licenseHandler) activateLicense(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var req struct {
		Key string `json:"key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "license key is required")
	}

	// Validate key format: must be a hex string of at least 32 characters
	if len(req.Key) < 32 {
		return echo.NewHTTPError(http.StatusBadRequest, "license key must be at least 32 characters")
	}
	for _, c := range req.Key {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return echo.NewHTTPError(http.StatusBadRequest, "license key must be a valid hex string")
		}
	}
	tier := features.TierPro

	// Deactivate any existing license first
	if err := h.store.Licenses.Deactivate(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to deactivate old license")
	}

	// Activate new — use tier defaults for max servers
	maxServers := 10
	if tier == "free" {
		maxServers = 1
	}
	_, err := h.store.Licenses.Activate(c.Request().Context(), req.Key, tier, maxServers)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to activate license")
	}

	keyPreview := req.Key
	if len(keyPreview) > 8 {
		keyPreview = keyPreview[:8] + "..."
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]interface{}{
			"status": "activated",
			"tier":   tier,
			"key":    keyPreview,
		},
		RequestID: rid,
	})
}

// DeactivateLicense reverts to free tier.
func (h *licenseHandler) deactivateLicense(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	if err := h.store.Licenses.Deactivate(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to deactivate license")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]interface{}{
			"status": "deactivated",
			"tier":   features.TierFree,
		},
		RequestID: rid,
	})
}

// GetFeatures returns all features with access status for the current tier.
func (h *licenseHandler) getFeatures(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	license, err := h.store.Licenses.GetActive(c.Request().Context())
	tier := features.TierFree
	if err == nil && license != nil {
		tier = license.Tier
	}

	featureMap := features.TierFeatures(tier)

	type featureInfo struct {
		Name         string `json:"name"`
		Available    bool   `json:"available"`
		RequiredTier string `json:"required_tier"`
	}

	allFeatures := make([]featureInfo, 0, len(features.FeatureTierMap))
	for feat, reqTier := range features.FeatureTierMap {
		allFeatures = append(allFeatures, featureInfo{
			Name:         feat,
			Available:    featureMap[feat],
			RequiredTier: reqTier,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]interface{}{
			"current_tier": tier,
			"features":     allFeatures,
		},
		RequestID: rid,
	})
}
