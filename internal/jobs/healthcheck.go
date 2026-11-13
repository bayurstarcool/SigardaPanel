package jobs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"sigardapanel/internal/notify"
	"sigardapanel/internal/store"
)

const healthCheckInterval = 60 * time.Second
const healthCheckTimeout = 10 * time.Second

// HealthChecker periodically pings all registered agents and updates server health status.
type HealthChecker struct {
	store  *store.Store
	client *http.Client
	wg     sync.WaitGroup
	// authClient verifies TLS (it carries the agent token).
	authClient *http.Client
	stopCh     chan struct{}
	stopped    chan struct{}
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(s *store.Store) *HealthChecker {
	// Use a custom transport that skips TLS verification for self-signed certs
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &HealthChecker{
		store: s,
		client: &http.Client{
			Timeout:   healthCheckTimeout,
			Transport: transport,
		},
		// The token check sends the agent token: never over a TLS
		// connection whose certificate was not verified.
		authClient: &http.Client{Timeout: healthCheckTimeout},
		stopCh:     make(chan struct{}),
		stopped:    make(chan struct{}),
	}
}

// Start begins the health check loop in a goroutine.
func (hc *HealthChecker) Start(ctx context.Context) {
	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		defer close(hc.stopped)
		hc.loop(ctx)
	}()
}

// Stop signals the health checker to stop and waits for it to finish.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
	<-hc.stopped
}

func (hc *HealthChecker) loop(ctx context.Context) {
	// Initial check after a short delay
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stopCh:
			return
		case <-timer.C:
			hc.checkAll(ctx)
			timer.Reset(healthCheckInterval)
		}
	}
}

func (hc *HealthChecker) checkAll(ctx context.Context) {
	servers, err := hc.store.Servers.List(ctx)
	if err != nil {
		slog.Error("healthcheck: list servers", "error", err)
		return
	}

	for _, srv := range servers {
		if !srv.AgentURL.Valid || srv.AgentURL.String == "" {
			continue
		}
		go hc.checkServer(ctx, srv)
	}
}

func (hc *HealthChecker) checkServer(ctx context.Context, srv store.Server) {
	healthURL := strings.TrimRight(srv.AgentURL.String, "/") + "/health"

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		slog.Error("healthcheck: create request", "server_id", srv.ID, "error", err)
		hc.updateAndAlert(ctx, srv, "offline", 0)
		return
	}

	resp, err := hc.client.Do(req)
	elapsed := time.Since(start)
	latencyMs := elapsed.Milliseconds()

	if err != nil {
		slog.Warn("healthcheck: agent unreachable", "server_id", srv.ID, "name", srv.Name, "url", healthURL, "error", err)
		hc.updateAndAlert(ctx, srv, "offline", latencyMs)
		return
	}
	defer resp.Body.Close()

	// Read response body to detect issues
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		slog.Warn("healthcheck: read body failed", "server_id", srv.ID, "error", err)
		hc.updateAndAlert(ctx, srv, "offline", latencyMs)
		return
	}

	// Check for valid JSON response with status "ok"
	var healthResp struct {
		Data struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &healthResp); err != nil {
		slog.Warn("healthcheck: invalid response", "server_id", srv.ID, "status", resp.StatusCode)
		hc.updateAndAlert(ctx, srv, "offline", latencyMs)
		return
	}

	// Determine status based on response code and latency
	var status string
	if resp.StatusCode >= 500 || healthResp.Data.Status != "ok" {
		status = "offline"
	} else if latencyMs > 1000 {
		status = "degraded"
	} else {
		status = "online"
	}

	if healthResp.Data.Version != "" && healthResp.Data.Version != srv.AgentVersion.String {
		if err := hc.store.Servers.UpdateAgentVersion(ctx, srv.ID, healthResp.Data.Version); err != nil {
			slog.Warn("healthcheck: store agent version", "server_id", srv.ID, "error", err)
		}
	}

	// /health is public: an agent with a different token still answers it.
	// Check the token too, so a typo in agent.env shows up as "unauthorized"
	// instead of a server that looks online while every task fails.
	if status != "offline" && hc.tokenRejected(ctx, srv) {
		status = "unauthorized"
	}

	hc.updateAndAlert(ctx, srv, status, latencyMs)
}

// tokenRejected reports whether the agent refuses the panel's token. Agents
// older than v1.6.1 have no /auth/check (404); they are not judged.
func (hc *HealthChecker) tokenRejected(ctx context.Context, srv store.Server) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(srv.AgentURL.String, "/")+"/auth/check", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+srv.AgentToken)
	client := hc.authClient
	if client == nil {
		client = &http.Client{Timeout: healthCheckTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		slog.Warn("healthcheck: agent rejects the panel's token", "server_id", srv.ID, "name", srv.Name)
		return true
	}
	return false
}

func (hc *HealthChecker) updateAndAlert(ctx context.Context, srv store.Server, status string, latencyMs int64) {
	// Check if server was previously online (for offline alerting)
	previousStatus := srv.HealthStatus

	// Update health in database
	if err := hc.store.Servers.UpdateHealth(ctx, srv.ID, status, latencyMs); err != nil {
		slog.Error("healthcheck: update health", "server_id", srv.ID, "error", err)
		return
	}

	// Promote server status from "pending" to "online" when agent responds
	if (status == "online" || status == "degraded") && (srv.Status == "pending" || srv.Status == "offline") {
		if err := hc.store.Servers.UpdateStatus(ctx, srv.ID, "online"); err != nil {
			slog.Warn("healthcheck: promote status", "server_id", srv.ID, "error", err)
		}
	}
	// Mark server offline when unreachable
	if status == "offline" && (srv.Status == "online" || srv.Status == "degraded" || srv.Status == "active") {
		if err := hc.store.Servers.UpdateStatus(ctx, srv.ID, "offline"); err != nil {
			slog.Warn("healthcheck: demote status", "server_id", srv.ID, "error", err)
		}
	}

	// Alert if server just went offline
	if status == "offline" && (previousStatus == "online" || previousStatus == "degraded" || previousStatus == "unknown") {
		sender := notify.NewSender(hc.store)
		sender.Send(ctx, "warning", "Server Offline", "Server '"+srv.Name+"' ("+srv.Hostname+") is unreachable")
		slog.Warn("healthcheck: server went offline", "server_id", srv.ID, "name", srv.Name, "previous", previousStatus)
	}

	if status == "unauthorized" && previousStatus != "unauthorized" {
		sender := notify.NewSender(hc.store)
		sender.Send(ctx, "warning", "Agent token rejected", "Server '"+srv.Name+"' ("+srv.Hostname+") answers but rejects the panel's agent token; update SIGARDAPANEL_AGENT_TOKEN in its /etc/sigardapanel/agent.env")
	}

	// Alert if server recovered from offline
	if (status == "online" || status == "degraded") && previousStatus == "offline" {
		sender := notify.NewSender(hc.store)
		sender.Send(ctx, "info", "Server Recovered", "Server '"+srv.Name+"' ("+srv.Hostname+") is back online")
		slog.Info("healthcheck: server recovered", "server_id", srv.ID, "name", srv.Name, "status", status)
	}
}
