package apihttp

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type metricHandler struct {
	store *store.Store
}

func newMetricHandler(s *store.Store) *metricHandler {
	return &metricHandler{store: s}
}

func (h *metricHandler) ingest(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	var body struct {
		CPUPercent  *float64 `json:"cpu_percent"`
		MemUsed     *int64   `json:"mem_used"`
		MemTotal    *int64   `json:"mem_total"`
		DiskUsed    *int64   `json:"disk_used"`
		DiskTotal   *int64   `json:"disk_total"`
		SwapUsed    *int64   `json:"swap_used"`
		SwapTotal   *int64   `json:"swap_total"`
		NetInBytes  *int64   `json:"net_in_bytes"`
		NetOutBytes *int64   `json:"net_out_bytes"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	metric := &store.ServerMetric{
		ServerID: serverID,
	}
	if body.CPUPercent != nil {
		metric.CPUPercent = sql.NullFloat64{Float64: *body.CPUPercent, Valid: true}
	}
	if body.MemUsed != nil {
		metric.MemUsed = sql.NullInt64{Int64: *body.MemUsed, Valid: true}
	}
	if body.MemTotal != nil {
		metric.MemTotal = sql.NullInt64{Int64: *body.MemTotal, Valid: true}
	}
	if body.DiskUsed != nil {
		metric.DiskUsed = sql.NullInt64{Int64: *body.DiskUsed, Valid: true}
	}
	if body.DiskTotal != nil {
		metric.DiskTotal = sql.NullInt64{Int64: *body.DiskTotal, Valid: true}
	}
	if body.SwapUsed != nil {
		metric.SwapUsed = sql.NullInt64{Int64: *body.SwapUsed, Valid: true}
	}
	if body.SwapTotal != nil {
		metric.SwapTotal = sql.NullInt64{Int64: *body.SwapTotal, Valid: true}
	}
	if body.NetInBytes != nil {
		metric.NetInBytes = sql.NullInt64{Int64: *body.NetInBytes, Valid: true}
	}
	if body.NetOutBytes != nil {
		metric.NetOutBytes = sql.NullInt64{Int64: *body.NetOutBytes, Valid: true}
	}

	if err := h.store.Metrics.Insert(c.Request().Context(), metric); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      map[string]any{"id": metric.ID, "ok": true},
		RequestID: rid,
	})
}

func (h *metricHandler) latest(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	metric, err := h.store.Metrics.GetLatest(c.Request().Context(), serverID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if metric == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no metrics yet")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      metricToMap(*metric),
		RequestID: rid,
	})
}

func (h *metricHandler) range_(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	rangeStr := c.QueryParam("range")
	if rangeStr == "" {
		rangeStr = "1h"
	}

	// Convert "7d" style to duration
	duration, err := parseRangeDuration(rangeStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid range (e.g. 1h, 24h, 7d)")
	}

	since := time.Now().Add(-duration)
	metrics, err := h.store.Metrics.GetRange(c.Request().Context(), serverID, since, 1000)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, metricToMap(m))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"metrics": out, "count": len(out), "range": rangeStr},
		RequestID: rid,
	})
}

func (h *metricHandler) gpu(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	serverID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || serverID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	metric, err := h.store.Metrics.GetLatest(c.Request().Context(), serverID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if metric == nil {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"gpu_detected": false},
			RequestID: rid,
		})
	}

	if !metric.GPUCount.Valid || metric.GPUCount.Int64 == 0 {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"gpu_detected": false},
			RequestID: rid,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"gpu_detected":     true,
			"gpu_name":         metric.GPUName.String,
			"gpu_count":        metric.GPUCount.Int64,
			"gpu_utilization":  metric.GPUUtilization.Float64,
			"gpu_memory_used":  metric.GPUMemoryUsed.Int64,
			"gpu_memory_total": metric.GPUMemoryTotal.Int64,
			"gpu_temperature":  metric.GPUTemperature.Float64,
			"gpu_power_draw":   metric.GPUPowerDraw.Float64,
			"gpu_fan_speed":    metric.GPUFanSpeed.Float64,
			"collected_at":     metric.CollectedAt,
		},
		RequestID: rid,
	})
}

func metricToMap(m store.ServerMetric) map[string]any {
	out := map[string]any{
		"id":           m.ID,
		"server_id":    m.ServerID,
		"collected_at": m.CollectedAt,
	}
	if m.CPUPercent.Valid {
		out["cpu_percent"] = m.CPUPercent.Float64
	}
	if m.MemUsed.Valid {
		out["mem_used"] = m.MemUsed.Int64
	}
	if m.MemTotal.Valid {
		out["mem_total"] = m.MemTotal.Int64
	}
	if m.DiskUsed.Valid {
		out["disk_used"] = m.DiskUsed.Int64
	}
	if m.DiskTotal.Valid {
		out["disk_total"] = m.DiskTotal.Int64
	}
	if m.SwapUsed.Valid {
		out["swap_used"] = m.SwapUsed.Int64
	}
	if m.SwapTotal.Valid {
		out["swap_total"] = m.SwapTotal.Int64
	}
	if m.NetInBytes.Valid {
		out["net_in_bytes"] = m.NetInBytes.Int64
	}
	if m.NetOutBytes.Valid {
		out["net_out_bytes"] = m.NetOutBytes.Int64
	}
	if m.GPUName.Valid {
		out["gpu_name"] = m.GPUName.String
	}
	if m.GPUCount.Valid {
		out["gpu_count"] = m.GPUCount.Int64
	}
	if m.GPUUtilization.Valid {
		out["gpu_utilization"] = m.GPUUtilization.Float64
	}
	if m.GPUMemoryUsed.Valid {
		out["gpu_memory_used"] = m.GPUMemoryUsed.Int64
	}
	if m.GPUMemoryTotal.Valid {
		out["gpu_memory_total"] = m.GPUMemoryTotal.Int64
	}
	if m.GPUTemperature.Valid {
		out["gpu_temperature"] = m.GPUTemperature.Float64
	}
	if m.GPUPowerDraw.Valid {
		out["gpu_power_draw"] = m.GPUPowerDraw.Float64
	}
	if m.GPUFanSpeed.Valid {
		out["gpu_fan_speed"] = m.GPUFanSpeed.Float64
	}
	return out
}

func parseRangeDuration(s string) (time.Duration, error) {
	// Support "7d" by converting to hours
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err != nil {
			return 0, err
		}
		days, _ = strconv.Atoi(strings.TrimSuffix(s, "d"))
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
