package apihttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/cloudflare"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type cloudflareHandler struct {
	s     *store.Store
	audit *audit.Recorder
}

func newCloudflareHandler(s *store.Store, auditRec *audit.Recorder) *cloudflareHandler {
	return &cloudflareHandler{s: s, audit: auditRec}
}

// clientForUser creates a Cloudflare client for the given user's stored config.
func (h *cloudflareHandler) clientForUser(c echo.Context, userID int64) (*cloudflare.Client, error) {
	cfg, err := h.s.Cloudflare.GetConfig(userID)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	if cfg.APIToken == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "Cloudflare API token not configured. Go to Settings → Cloudflare to set it up.")
	}
	return cloudflare.NewClient(cloudflare.Config{
		APIToken: cfg.APIToken,
		Email:    cfg.Email,
	}), nil
}

// clientFromSession creates a Cloudflare client using the session user's config.
func (h *cloudflareHandler) clientFromSession(c echo.Context) (*cloudflare.Client, int64, error) {
	sess := sessionFromEcho(c)
	if sess == nil {
		return nil, 0, echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	client, err := h.clientForUser(c, sess.UserID)
	return client, sess.UserID, err
}

// GET /api/v1/cloudflare/config
func (h *cloudflareHandler) getConfig(c echo.Context) error {
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	cfg, err := h.s.Cloudflare.GetConfig(sess.UserID)
	if err != nil {
		return err
	}
	masked := cfg.APIToken
	if len(masked) > 8 {
		masked = masked[:4] + "••••" + masked[len(masked)-4:]
	} else if masked != "" {
		masked = "••••"
	}
	hasConfig, _ := h.s.Cloudflare.HasConfig(sess.UserID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"api_token":  masked,
			"raw_token":  cfg.APIToken != "",
			"email":      cfg.Email,
			"configured": hasConfig,
		},
	})
}

// PUT /api/v1/cloudflare/config
func (h *cloudflareHandler) saveConfig(c echo.Context) error {
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		APIToken string `json:"api_token"`
		Email    string `json:"email,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.APIToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "api_token is required")
	}

	client := cloudflare.NewClient(cloudflare.Config{APIToken: req.APIToken, Email: req.Email})

	// Verify token (skip in dev mode)
	verified := false
	verifyUser := ""
	if os.Getenv("SIGARDAPANEL_DEV") == "true" {
		slog.Warn("cloudflare: dev mode, skipping token verification")
		verified = true
		verifyUser = "dev-user"
	} else if v, err := client.Verify(); err == nil {
		verified = true
		verifyUser = v.User
	} else if zones, err := client.ListZones(); err == nil {
		verified = true
		_ = zones
	} else {
		return echo.NewHTTPError(http.StatusBadRequest,
			"Cloudflare API token verification failed. Token may be invalid or missing required permissions (DNS:Edit, Zone:Read). Error: "+err.Error())
	}

	if err := h.s.Cloudflare.SaveConfig(sess.UserID, store.CloudflareConfig{
		APIToken: req.APIToken,
		Email:    req.Email,
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config: "+err.Error())
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, "cloudflare.config.update", "cloudflare", 0, map[string]any{
		"verified": verified,
		"user":     verifyUser,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"status":   "ok",
			"message":  "Cloudflare API token saved and verified",
			"verified": verified,
			"user":     verifyUser,
		},
	})
}

// DELETE /api/v1/cloudflare/config
func (h *cloudflareHandler) deleteConfig(c echo.Context) error {
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	_, err := h.s.Cloudflare.GetConfig(sess.UserID)
	if err != nil {
		return err
	}
	// Delete the config row
	_, err = h.s.Cloudflare.DeleteConfig(sess.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete config: "+err.Error())
	}
	h.audit.RecordAction(c.Request().Context(), sess.UserID, "cloudflare.config.delete", "cloudflare", 0, nil)
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]string{"status": "deleted"}})
}

// GET /api/v1/cloudflare/verify
func (h *cloudflareHandler) verify(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	verify, err := client.Verify()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Cloudflare verification failed: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: verify})
}

// GET /api/v1/cloudflare/zones
func (h *cloudflareHandler) listZones(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zones, err := client.ListZones()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to fetch zones: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"zones": zones,
		"count": len(zones),
	}})
}

// GET /api/v1/cloudflare/zones/search?q=
func (h *cloudflareHandler) searchZones(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	if q == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query parameter 'q' is required")
	}
	zones, err := client.ListZones()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to search zones: "+err.Error())
	}
	var matched []cloudflare.Zone
	q = strings.ToLower(q)
	for _, z := range zones {
		if strings.Contains(strings.ToLower(z.Name), q) {
			matched = append(matched, z)
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"zones": matched,
		"count": len(matched),
	}})
}

// GET /api/v1/cloudflare/zones/:id
func (h *cloudflareHandler) getZone(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	zone, err := client.GetZone(zoneID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to fetch zone: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: zone})
}

// GET /api/v1/cloudflare/zones/:id/dns
func (h *cloudflareHandler) listDNSRecords(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	recordType := c.QueryParam("type")
	records, err := client.ListDNSRecords(zoneID, recordType)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to fetch DNS records: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"records": records,
		"count":   len(records),
	}})
}

// POST /api/v1/cloudflare/zones/:id/dns
func (h *cloudflareHandler) createDNSRecord(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	var req cloudflare.CreateDNSRecordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" || req.Type == "" || req.Content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, type, and content are required")
	}
	if req.TTL == 0 {
		req.TTL = 1
	}
	record, err := client.CreateDNSRecord(zoneID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to create DNS record: "+err.Error())
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.dns.create", "cloudflare", 0, map[string]any{
		"zone_id": zoneID,
		"name":    req.Name,
		"type":    req.Type,
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: record})
}

// PUT /api/v1/cloudflare/zones/:id/dns/:recordId
func (h *cloudflareHandler) updateDNSRecord(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	recordID := c.Param("recordId")
	var req cloudflare.CreateDNSRecordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Type == "" || req.Content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "type and content are required")
	}
	if req.TTL == 0 {
		req.TTL = 1
	}
	record, err := client.UpdateDNSRecord(zoneID, recordID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to update DNS record: "+err.Error())
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.dns.update", "cloudflare", 0, map[string]any{
		"zone_id": zoneID,
		"name":    req.Name,
		"type":    req.Type,
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: record})
}

// PATCH /api/v1/cloudflare/zones/:id/dns/:recordId — partial update (e.g. toggle proxy)
func (h *cloudflareHandler) patchDNSRecord(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	recordID := c.Param("recordId")
	var req map[string]any
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	record, err := client.UpdateDNSRecordPartial(zoneID, recordID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to update DNS record: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: record})
}

// DELETE /api/v1/cloudflare/zones/:id/dns/:recordId
func (h *cloudflareHandler) deleteDNSRecord(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	recordID := c.Param("recordId")
	if err := client.DeleteDNSRecord(zoneID, recordID); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to delete DNS record: "+err.Error())
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.dns.delete", "cloudflare", 0, map[string]any{
		"zone_id":   zoneID,
		"record_id": recordID,
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]string{"status": "deleted"}})
}

// POST /api/v1/cloudflare/zones/:id/purge-cache
func (h *cloudflareHandler) purgeCache(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	var req cloudflare.PurgeCacheRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if !req.PurgeEverything && len(req.Files) == 0 && len(req.Tags) == 0 && len(req.Hosts) == 0 {
		req.PurgeEverything = true
	}
	result, err := client.PurgeCache(zoneID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to purge cache: "+err.Error())
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.cache.purge", "cloudflare", 0, map[string]any{
		"zone_id":          zoneID,
		"purge_everything": req.PurgeEverything,
		"files_count":      len(req.Files),
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: result})
}

// GET /api/v1/cloudflare/zones/:id/analytics?since=&until=
func (h *cloudflareHandler) getAnalytics(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	now := time.Now().UTC()
	until := now
	since := now.Add(-24 * time.Hour)
	if s := c.QueryParam("since"); s != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			since = parsed
		}
	}
	if u := c.QueryParam("until"); u != "" {
		if parsed, err := time.Parse(time.RFC3339, u); err == nil {
			until = parsed
		}
	}
	analytics, err := client.GetAnalytics(zoneID, since, until)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to fetch analytics: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: analytics})
}

// GET /api/v1/cloudflare/zones/:id/analytics/range?days=7
func (h *cloudflareHandler) getAnalyticsRange(c echo.Context) error {
	client, _, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	daysStr := c.QueryParam("days")
	days := 7
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 90 {
		days = d
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	analytics, err := client.GetAnalytics(zoneID, since, now)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "Failed to fetch analytics: "+err.Error())
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: analytics})
}

// POST /api/v1/cloudflare/zones/:id/dns/bulk-delete
func (h *cloudflareHandler) bulkDeleteDNS(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	var req struct {
		RecordIDs []string `json:"record_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	var failed []string
	for _, rid := range req.RecordIDs {
		if err := client.DeleteDNSRecord(zoneID, rid); err != nil {
			failed = append(failed, rid)
		}
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.dns.bulk_delete", "cloudflare", 0, map[string]any{
		"zone_id": zoneID,
		"total":   len(req.RecordIDs),
		"failed":  len(failed),
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"deleted": len(req.RecordIDs) - len(failed),
		"failed":  failed,
	}})
}

// POST /api/v1/cloudflare/zones/:id/dns/import
func (h *cloudflareHandler) importDNSRecords(c echo.Context) error {
	client, userID, err := h.clientFromSession(c)
	if err != nil {
		return err
	}
	zoneID := c.Param("id")
	var req struct {
		Records []cloudflare.CreateDNSRecordRequest `json:"records"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	var created []*cloudflare.DNSRecord
	var errors []string
	for i, r := range req.Records {
		if r.TTL == 0 {
			r.TTL = 1
		}
		record, err := client.CreateDNSRecord(zoneID, r)
		if err != nil {
			errors = append(errors, fmt.Sprintf("[%d] %s %s: %s", i, r.Type, r.Name, err.Error()))
			continue
		}
		created = append(created, record)
	}
	h.audit.RecordAction(c.Request().Context(), userID, "cloudflare.dns.import", "cloudflare", 0, map[string]any{
		"zone_id": zoneID,
		"total":   len(req.Records),
		"created": len(created),
		"errors":  len(errors),
	})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"created": created,
		"errors":  errors,
		"count":   len(created),
	}})
}
