package agenthttp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// redisStatsTask returns Redis statistics.
func (r *Router) redisStatsTask(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if redis-cli is available
	if _, err := r.executor.RunShell(ctx, "which", "redis-cli"); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": "redis-cli not found",
		})
	}

	// Get Redis info
	output, err := r.executor.RunShell(ctx, "redis-cli", "info", "stats")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("redis-cli failed: %s", strings.TrimSpace(output)),
		})
	}

	// Parse stats
	stats := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			stats[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Get memory info
	memOutput, _ := r.executor.RunShell(ctx, "redis-cli", "info", "memory")
	for _, line := range strings.Split(memOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			stats[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Get keyspace
	ksOutput, _ := r.executor.RunShell(ctx, "redis-cli", "info", "keyspace")
	for _, line := range strings.Split(ksOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			stats[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":    true,
		"stats": stats,
	})
}

// redisFlushTask flushes all Redis data.
func (r *Router) redisFlushTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "redis-cli", "flushall")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("flush failed: %s", strings.TrimSpace(output)),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "All Redis data flushed",
		"output":  strings.TrimSpace(output),
	})
}

// redisInfoTask returns detailed Redis information.
func (r *Router) redisInfoTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "redis-cli", "info")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("redis-cli failed: %s", strings.TrimSpace(output)),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":   true,
		"info": strings.TrimSpace(output),
	})
}

// redisFlushDBTask flushes a specific Redis database.
func (r *Router) redisFlushDBTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		DB int `json:"db"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.DB < 0 || payload.DB > 15 {
		return echo.NewHTTPError(http.StatusBadRequest, "db must be 0-15")
	}

	// Select DB and flush
	output, err := r.executor.RunShell(ctx, "redis-cli", "-n", fmt.Sprintf("%d", payload.DB), "flushdb")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("flushdb failed: %s", strings.TrimSpace(output)),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Redis database %d flushed", payload.DB),
		"output":  strings.TrimSpace(output),
	})
}
