package apihttp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/secrets"
	"sigardapanel/internal/store"
)

type jobHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newJobHandler(s *store.Store, a *audit.Recorder) *jobHandler {
	return &jobHandler{store: s, audit: a}
}

func (h *jobHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	filter := store.JobListFilter{Limit: 100}
	if v := c.QueryParam("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit")
		}
		filter.Limit = n
	}
	if v := c.QueryParam("offset"); v != "" {
		n, _ := strconv.Atoi(v)
		filter.Offset = n
	}
	if v := c.QueryParam("status"); v != "" {
		filter.Status = v
	}
	if v := c.QueryParam("type"); v != "" {
		filter.Type = v
	}
	if v := c.QueryParam("server_id"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		filter.ServerID = n
	}
	if v := c.QueryParam("site_id"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		filter.SiteID = n
	}

	var list []jobmodel.Job
	var err error
	if v := jobVisibilityFor(c, h.store); v.all {
		list, err = h.store.Jobs.List(c.Request().Context(), filter)
	} else {
		// Non super_admins only see jobs of sites they can access, plus
		// server-level jobs when they are admins.
		list, err = h.store.ListJobsScoped(c.Request().Context(), filter, v.scope)
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(list))
	for _, j := range list {
		out = append(out, jobToMap(j))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"jobs": out, "count": len(out)},
		RequestID: rid,
	})
}

func (h *jobHandler) get(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	job, err := h.store.Jobs.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if job == nil || !canSeeJob(c, h.store, job, sitePermRead) {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      jobToMap(*job),
		RequestID: rid,
	})
}

func (h *jobHandler) cancel(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	job, err := h.store.Jobs.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if job == nil || !canSeeJob(c, h.store, job, sitePermRead) {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	// Canceling changes the site's pending work: needs write on the site
	// (server-level jobs: admin).
	if !canSeeJob(c, h.store, job, sitePermWrite) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient site permission")
	}

	if job.Status != jobmodel.StatusQueued {
		return echo.NewHTTPError(http.StatusConflict, "cannot cancel job in status "+job.Status)
	}

	if err := h.store.Jobs.Cancel(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionJobCancel, audit.ResourceJob, id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"canceled": true, "id": id},
		RequestID: rid,
	})
}

func jobToMap(j jobmodel.Job) map[string]any {
	m := map[string]any{
		"id":           j.ID,
		"type":         j.Type,
		"status":       j.Status,
		"attempts":     j.Attempts,
		"max_attempts": j.MaxAttempts,
		"created_at":   j.CreatedAt.Format(time.RFC3339),
		"updated_at":   j.UpdatedAt.Format(time.RFC3339),
	}
	if j.Description != "" {
		m["description"] = j.Description
	}
	if j.ServerID.Valid {
		m["server_id"] = j.ServerID.Int64
	}
	if j.SiteID.Valid {
		m["site_id"] = j.SiteID.Int64
	}
	if j.Payload != "" {
		m["payload"] = redactSensitivePayload(j.Payload)
	}
	if j.Result != "" {
		m["result"] = redactSensitivePayload(j.Result)
	}
	if j.Error.Valid {
		m["error"] = j.Error.String
	}
	if j.StartedAt.Valid {
		m["started_at"] = j.StartedAt.Time.Format(time.RFC3339)
	}
	if j.CompletedAt.Valid {
		m["completed_at"] = j.CompletedAt.Time.Format(time.RFC3339)
	}
	return m
}

// redactSensitivePayload parses a JSON job payload and replaces the values of
// credential-looking keys (recursively) with "[REDACTED]", and strips
// credentials embedded in URLs (https://user:token@host/...). If the payload
// is not valid JSON it is returned as-is (payloads are always JSON-encoded
// internally).
func redactSensitivePayload(payload string) string {
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return payload
	}
	redacted := redactValue(v)
	out, err := json.Marshal(redacted)
	if err != nil {
		return payload
	}
	return string(out)
}

// sensitiveKeyFragments mark a payload key as credential-bearing when they
// appear anywhere in it (git_token, api_key, smtp_password, private_key, ...).
var sensitiveKeyFragments = []string{"password", "passwd", "secret", "token", "private_key", "access_key", "api_key", "credential"}

// isSensitivePayloadKey reports whether a JSON key names a credential. The
// exact keys come from the secrets package (the ones encrypted at rest).
func isSensitivePayloadKey(k string) bool {
	if secrets.SensitiveKeys[k] {
		return true
	}
	lk := strings.ToLower(k)
	for _, f := range sensitiveKeyFragments {
		if strings.Contains(lk, f) {
			return true
		}
	}
	return false
}

// urlUserinfoRe matches the userinfo part of a URL ("scheme://user:pass@").
var urlUserinfoRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://)[^/@\s]+@`)

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if secrets.SensitiveKeys[k] {
				t[k] = "[REDACTED]"
			} else if _, isStr := val.(string); isStr && isSensitivePayloadKey(k) {
				t[k] = "[REDACTED]"
			} else {
				t[k] = redactValue(val)
			}
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = redactValue(val)
		}
		return t
	case string:
		return urlUserinfoRe.ReplaceAllString(t, "${1}[REDACTED]@")
	default:
		return v
	}
}
