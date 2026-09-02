package apihttp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func newMetricsApp(t *testing.T) (*echo.Echo, *store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "metrics_api_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	migrations.Up(d, migrations.AllMigrations)
	s := store.New(d)
	e := NewRouter(s, d, nil)
	return e, s, t.Context()
}

func createTestServerForAPIMetrics(t *testing.T, s *store.Store, ctx context.Context) int64 {
	t.Helper()
	server := &store.Server{
		Name:       "test-server",
		Hostname:   "test.example.com",
		Status:     "online",
		AgentToken: "test-tok",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}
	return server.ID
}

func createAdminSessionForMetrics(t *testing.T, s *store.Store, ctx context.Context) string {
	t.Helper()
	user := &store.User{
		Username:     "metrics-admin",
		Email:        "metrics-admin@test.local",
		PasswordHash: "x",
		Role:         auth.RoleAdmin,
	}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _ := auth.NewToken()
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

func doMetricsRequest(t *testing.T, e *echo.Echo, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestMetricsIngest_Success(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	body := map[string]any{
		"cpu_percent":   45.5,
		"mem_used":      1024 * 1024 * 512,
		"mem_total":     1024 * 1024 * 1024,
		"disk_used":     5000000000,
		"disk_total":    10000000000,
		"net_in_bytes":  1024,
		"net_out_bytes": 2048,
	}

	rec := doMetricsRequest(t, e, "POST", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics", body, token)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	data := result["data"].(map[string]any)
	if data["id"] == nil {
		t.Error("expected data.id")
	}
}

func TestMetricsIngest_PartialFields(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	// Only CPU
	body := map[string]any{"cpu_percent": 12.3}

	rec := doMetricsRequest(t, e, "POST", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics", body, token)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsIngest_InvalidServerID(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "POST", "/api/v1/servers/abc/metrics", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestMetricsIngest_ZeroServerID(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "POST", "/api/v1/servers/0/metrics", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestMetricsIngest_NoAuth(t *testing.T) {
	e, _, _ := newMetricsApp(t)

	rec := doMetricsRequest(t, e, "POST", "/api/v1/servers/1/metrics", map[string]any{}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMetricsLatest_Success(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	cpu := 50.0
	if err := s.Metrics.Insert(ctx, &store.ServerMetric{
		ServerID:   serverID,
		CPUPercent: sql.NullFloat64{Float64: cpu, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics/latest", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	data := result["data"].(map[string]any)
	if data["id"] == nil {
		t.Error("expected data.id")
	}
	if data["server_id"] != float64(serverID) {
		t.Errorf("expected server_id %d, got %v", serverID, data["server_id"])
	}
}

func TestMetricsLatest_NotFound(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics/latest", nil, token)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsLatest_InvalidServerID(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/abc/metrics/latest", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestMetricsLatest_NoAuth(t *testing.T) {
	e, _, _ := newMetricsApp(t)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/1/metrics/latest", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMetricsRange_Success(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	for i := 0; i < 3; i++ {
		m := &store.ServerMetric{
			ServerID:   serverID,
			CPUPercent: sql.NullFloat64{Float64: float64(i * 10), Valid: true},
		}
		if err := s.Metrics.Insert(ctx, m); err != nil {
			t.Fatalf("insert metric: %v", err)
		}
		if _, err := s.DB().ExecContext(ctx, `UPDATE server_metrics SET collected_at = ? WHERE id = ?`, time.Now(), m.ID); err != nil {
			t.Fatalf("update collected_at: %v", err)
		}
	}

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics?range=1h", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Metrics []map[string]any `json:"metrics"`
			Count   int              `json:"count"`
			Range   string           `json:"range"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)

	if result.Data.Count != 3 {
		t.Errorf("expected 3 metrics, got %d", result.Data.Count)
	}
	if result.Data.Range != "1h" {
		t.Errorf("expected range 1h, got %s", result.Data.Range)
	}
}

func TestMetricsRange_InvalidRange(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics?range=invalid", nil, token)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsRange_Empty(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics?range=1h", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Metrics []any `json:"metrics"`
			Count   int   `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)

	if result.Data.Count != 0 {
		t.Errorf("expected 0, got %d", result.Data.Count)
	}
}

func TestMetricsRange_DaysFormat(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	m := &store.ServerMetric{
		ServerID:   serverID,
		CPUPercent: sql.NullFloat64{Float64: 25.0, Valid: true},
	}
	if err := s.Metrics.Insert(ctx, m); err != nil {
		t.Fatalf("insert metric: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx, `UPDATE server_metrics SET collected_at = ? WHERE id = ?`, time.Now(), m.ID); err != nil {
		t.Fatalf("update collected_at: %v", err)
	}

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics?range=7d", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Range string `json:"range"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Range != "7d" {
		t.Errorf("expected range 7d, got %s", result.Data.Range)
	}
}

func TestMetricsRange_DefaultRange(t *testing.T) {
	e, s, ctx := newMetricsApp(t)
	serverID := createTestServerForAPIMetrics(t, s, ctx)
	token := createAdminSessionForMetrics(t, s, ctx)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(serverID, 10)+"/metrics", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Range string `json:"range"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Range != "1h" {
		t.Errorf("expected default range 1h, got %s", result.Data.Range)
	}
}

func TestMetricsRange_NoAuth(t *testing.T) {
	e, _, _ := newMetricsApp(t)

	rec := doMetricsRequest(t, e, "GET", "/api/v1/servers/1/metrics", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
