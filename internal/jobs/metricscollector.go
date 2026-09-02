package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sigardapanel/internal/notify"
	"sigardapanel/internal/store"
)

// MetricsCollector periodically queries each server's agent for metrics
// and stores them in the database.
type MetricsCollector struct {
	store  *store.Store
	client *http.Client
	stopCh chan struct{}
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(s *store.Store) *MetricsCollector {
	return &MetricsCollector{
		store: s,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				// Allow self-signed certs for agent TLS
			},
		},
		stopCh: make(chan struct{}),
	}
}

// Start begins periodic collection.
func (mc *MetricsCollector) Start(ctx context.Context) {
	slog.Info("metrics collector started", "interval", "60s")
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		time.Sleep(10 * time.Second)
		mc.collectAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-mc.stopCh:
				return
			case <-ticker.C:
				mc.collectAll(ctx)
			}
		}
	}()
}

// Stop halts collection.
func (mc *MetricsCollector) Stop() {
	close(mc.stopCh)
}

func (mc *MetricsCollector) collectAll(ctx context.Context) {
	servers, err := mc.store.Servers.List(ctx)
	if err != nil {
		slog.Warn("metrics: list servers", "error", err)
		return
	}
	if len(servers) == 0 {
		return
	}
	for _, srv := range servers {
		if !srv.AgentURL.Valid || srv.AgentURL.String == "" {
			continue
		}
		mc.collectFromAgent(ctx, srv)
	}
}

func (mc *MetricsCollector) collectFromAgent(ctx context.Context, srv store.Server) {
	// Build agent metrics URL
	metricsURL := strings.TrimRight(srv.AgentURL.String, "/") + "/metrics"

	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		slog.Warn("metrics: create request", "server_id", srv.ID, "error", err)
		return
	}

	resp, err := mc.client.Do(req)
	if err != nil {
		slog.Warn("metrics: agent unreachable", "server_id", srv.ID, "url", metricsURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("metrics: bad status", "server_id", srv.ID, "status", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		slog.Warn("metrics: read body", "server_id", srv.ID, "error", err)
		return
	}

	var apiResp struct {
		Data struct {
			CPUPercent   float64 `json:"cpu_percent"`
			MemUsed      int64   `json:"mem_used"`
			MemTotal     int64   `json:"mem_total"`
			DiskUsed     int64   `json:"disk_used"`
			DiskTotal    int64   `json:"disk_total"`
			SwapUsed     int64   `json:"swap_used"`
			SwapTotal    int64   `json:"swap_total"`
			NetInBytes   int64   `json:"net_in_bytes"`
			NetOutBytes  int64   `json:"net_out_bytes"`
			GPUName      string  `json:"gpu_name"`
			GPUCount     int     `json:"gpu_count"`
			GPUUtil      float64 `json:"gpu_utilization"`
			GPUMemUsed   int64   `json:"gpu_memory_used"`
			GPUMemTotal  int64   `json:"gpu_memory_total"`
			GPUTemp      float64 `json:"gpu_temperature"`
			GPUPower     float64 `json:"gpu_power_draw"`
			GPUFanSpeed  float64 `json:"gpu_fan_speed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		slog.Warn("metrics: parse response", "server_id", srv.ID, "error", err)
		return
	}

	d := apiResp.Data
	metric := &store.ServerMetric{ServerID: srv.ID}
	metric.CPUPercent = sql.NullFloat64{Float64: d.CPUPercent, Valid: true}
	metric.MemUsed = sql.NullInt64{Int64: d.MemUsed, Valid: d.MemUsed >= 0}
	metric.MemTotal = sql.NullInt64{Int64: d.MemTotal, Valid: d.MemTotal >= 0}
	metric.DiskUsed = sql.NullInt64{Int64: d.DiskUsed, Valid: d.DiskUsed >= 0}
	metric.DiskTotal = sql.NullInt64{Int64: d.DiskTotal, Valid: d.DiskTotal >= 0}
	metric.SwapUsed = sql.NullInt64{Int64: d.SwapUsed, Valid: d.SwapUsed >= 0}
	metric.SwapTotal = sql.NullInt64{Int64: d.SwapTotal, Valid: d.SwapTotal >= 0}
	metric.NetInBytes = sql.NullInt64{Int64: d.NetInBytes, Valid: d.NetInBytes >= 0}
	metric.NetOutBytes = sql.NullInt64{Int64: d.NetOutBytes, Valid: d.NetOutBytes >= 0}
	metric.GPUName = sql.NullString{String: d.GPUName, Valid: d.GPUName != ""}
	metric.GPUCount = sql.NullInt64{Int64: int64(d.GPUCount), Valid: d.GPUCount > 0}
	metric.GPUUtilization = sql.NullFloat64{Float64: d.GPUUtil, Valid: d.GPUCount > 0}
	metric.GPUMemoryUsed = sql.NullInt64{Int64: d.GPUMemUsed, Valid: d.GPUCount > 0}
	metric.GPUMemoryTotal = sql.NullInt64{Int64: d.GPUMemTotal, Valid: d.GPUCount > 0}
	metric.GPUTemperature = sql.NullFloat64{Float64: d.GPUTemp, Valid: d.GPUCount > 0}
	metric.GPUPowerDraw = sql.NullFloat64{Float64: d.GPUPower, Valid: d.GPUCount > 0}
	metric.GPUFanSpeed = sql.NullFloat64{Float64: d.GPUFanSpeed, Valid: d.GPUCount > 0}

	if err := mc.store.Metrics.Insert(ctx, metric); err != nil {
		slog.Warn("metrics: insert failed", "server_id", srv.ID, "error", err)
		return
	}
	slog.Debug("metrics: collected from agent", "server_id", srv.ID, "url", metricsURL)

	// Disk usage threshold alerts
	if d.DiskTotal > 0 {
		diskPercent := float64(d.DiskUsed) / float64(d.DiskTotal) * 100
		diskUsedGB := float64(d.DiskUsed) / (1024 * 1024 * 1024)
		diskTotalGB := float64(d.DiskTotal) / (1024 * 1024 * 1024)

		if diskPercent >= 90 {
			sender := notify.NewSender(mc.store)
			sender.Send(ctx, "critical", "Disk Usage Critical",
				fmt.Sprintf("Server %s disk usage at %.1f%% (%.1fGB / %.1fGB). Immediate action required!", srv.Name, diskPercent, diskUsedGB, diskTotalGB))
		} else if diskPercent >= 80 {
			sender := notify.NewSender(mc.store)
			sender.Send(ctx, "warning", "Disk Usage Warning",
				fmt.Sprintf("Server %s disk usage at %.1f%% (%.1fGB / %.1fGB). Consider cleaning up.", srv.Name, diskPercent, diskUsedGB, diskTotalGB))
		}
	}

	// GPU temperature alerts
	if d.GPUCount > 0 && d.GPUTemp >= 85 {
		sender := notify.NewSender(mc.store)
		sender.Send(ctx, "critical", "GPU Temperature Critical",
			fmt.Sprintf("Server %s GPU temperature at %.0f°C. Immediate action required!", srv.Name, d.GPUTemp))
	} else if d.GPUCount > 0 && d.GPUTemp >= 75 {
		sender := notify.NewSender(mc.store)
		sender.Send(ctx, "warning", "GPU Temperature Warning",
			fmt.Sprintf("Server %s GPU temperature at %.0f°C. Monitor closely.", srv.Name, d.GPUTemp))
	}
}
