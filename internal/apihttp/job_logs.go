package apihttp

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type jobLogHandler struct {
	store *store.Store
}

func newJobLogHandler(s *store.Store) *jobLogHandler {
	return &jobLogHandler{store: s}
}

func (h *jobLogHandler) append(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || jobID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	// Only the worker side (admin-scoped service tokens) and admins may append
	// log lines (the route also requires RequireAdmin); admins additionally
	// need access to the job's site.
	if err := h.requireJob(c, jobID, sitePermWrite); err != nil {
		return err
	}

	var body struct {
		LineNumber int    `json:"line_number"`
		Message    string `json:"message"`
		Stream     string `json:"stream"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if body.Stream == "" {
		body.Stream = "stdout"
	}

	log := &store.JobLog{
		JobID:      jobID,
		LineNumber: body.LineNumber,
		Message:    body.Message,
		Stream:     body.Stream,
	}
	if err := h.store.JobLogs.Append(c.Request().Context(), log); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      map[string]any{"id": log.ID, "ok": true},
		RequestID: rid,
	})
}

// requireJob verifies the job exists and the caller may access it at the
// given site permission level; inaccessible jobs are reported as not found.
func (h *jobLogHandler) requireJob(c echo.Context, jobID int64, required string) error {
	job, err := h.store.Jobs.GetByID(c.Request().Context(), jobID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if job == nil {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	if isAdminServiceToken(c) {
		return nil
	}
	if !canSeeJob(c, h.store, job, required) {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	return nil
}

func (h *jobLogHandler) getSince(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	jobID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || jobID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	since := 0
	if v := c.QueryParam("since"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid since")
		}
		since = n
	}
	if err := h.requireJob(c, jobID, sitePermRead); err != nil {
		return err
	}

	logs, err := h.store.JobLogs.GetSince(c.Request().Context(), jobID, since, 1000)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		out = append(out, map[string]any{
			"line_number": l.LineNumber,
			"message":     l.Message,
			"stream":      l.Stream,
			"created_at":  l.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"logs":  out,
			"count": len(out),
			"since": since,
		},
		RequestID: rid,
	})
}
