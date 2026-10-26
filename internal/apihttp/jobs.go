package apihttp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
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

	list, err := h.store.Jobs.List(c.Request().Context(), filter)
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
	if job == nil {
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
	if job == nil {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
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

// sensitivePayloadKeys are JSON payload fields that must never be exposed via
// the API. Job payloads can carry S3 credentials and database passwords for
// agent dispatch; those are redacted before being returned to clients.
var sensitivePayloadKeys = map[string]bool{
	"access_key":  true,
	"secret_key":  true,
	"db_password": true,
	"password":    true,
	"secret":      true,
	"token":       true,
}

// redactSensitivePayload parses a JSON job payload and replaces the values of
// sensitive keys (recursively) with "[REDACTED]". If the payload is not valid
// JSON it is returned as-is (payloads are always JSON-encoded internally).
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

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if sensitivePayloadKeys[k] {
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
	default:
		return v
	}
}
