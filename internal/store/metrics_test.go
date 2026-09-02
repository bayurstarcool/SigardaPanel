package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func testMetricsDB(t *testing.T) *sql.DB {
	t.Helper()
	f, _ := os.CreateTemp("", "metrics_test_*.db")
	f.Close()
	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		database.Close()
		os.Remove(f.Name())
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
		os.Remove(f.Name())
	})
	return database
}

func createTestServerForMetrics(t *testing.T, s *Store, ctx context.Context) int64 {
	t.Helper()
	srv := &Server{
		Name:       "metrics-server",
		Hostname:   "metrics.example.com",
		Status:     "online",
		AgentToken: "tok",
	}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}
	return srv.ID
}

func TestMetricInsert_Basic(t *testing.T) {
	db := testMetricsDB(t)
	s := New(db)
	ctx := context.Background()
	serverID := createTestServerForMetrics(t, s, ctx)

	m := &ServerMetric{
		ServerID:    serverID,
		CPUPercent:  sql.NullFloat64{Float64: 45.5, Valid: true},
		MemUsed:     sql.NullInt64{Int64: 512, Valid: true},
		MemTotal:    sql.NullInt64{Int64: 1024, Valid: true},
		DiskUsed:    sql.NullInt64{Int64: 1000, Valid: true},
		DiskTotal:   sql.NullInt64{Int64: 2000, Valid: true},
		NetInBytes:  sql.NullInt64{Int64: 10, Valid: true},
		NetOutBytes: sql.NullInt64{Int64: 20, Valid: true},
	}
	if err := s.Metrics.Insert(ctx, m); err != nil {
		t.Fatalf("insert metric: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("expected metric ID to be set")
	}
}

func TestMetricGetLatest_Found(t *testing.T) {
	db := testMetricsDB(t)
	s := New(db)
	ctx := context.Background()
	serverID := createTestServerForMetrics(t, s, ctx)

	first := &ServerMetric{
		ServerID:   serverID,
		CPUPercent: sql.NullFloat64{Float64: 10.0, Valid: true},
	}
	second := &ServerMetric{
		ServerID:   serverID,
		CPUPercent: sql.NullFloat64{Float64: 20.0, Valid: true},
	}
	if err := s.Metrics.Insert(ctx, first); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if err := s.Metrics.Insert(ctx, second); err != nil {
		t.Fatalf("insert second: %v", err)
	}

	latest, err := s.Metrics.GetLatest(ctx, serverID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest metric, got nil")
	}
	if latest.CPUPercent.Float64 != 20.0 {
		t.Fatalf("expected cpu 20.0, got %v", latest.CPUPercent.Float64)
	}
}

func TestMetricGetLatest_NotFound(t *testing.T) {
	db := testMetricsDB(t)
	s := New(db)
	ctx := context.Background()
	serverID := createTestServerForMetrics(t, s, ctx)

	got, err := s.Metrics.GetLatest(ctx, serverID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for no metrics")
	}
}

func TestMetricGetRange(t *testing.T) {
	db := testMetricsDB(t)
	s := New(db)
	ctx := context.Background()
	serverID := createTestServerForMetrics(t, s, ctx)

	now := time.Now()
	for i := 0; i < 3; i++ {
		_, err := db.ExecContext(ctx,
			`INSERT INTO server_metrics (server_id, collected_at, cpu_percent) VALUES (?, ?, ?)`,
			serverID, now.Add(-time.Duration(2-i)*time.Minute), float64(i*10),
		)
		if err != nil {
			t.Fatalf("insert metric %d: %v", i, err)
		}
	}

	metrics, err := s.Metrics.GetRange(ctx, serverID, now.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("get range: %v", err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}
	// newest first
	if metrics[0].CPUPercent.Float64 != 20.0 {
		t.Fatalf("expected newest cpu 20.0, got %v", metrics[0].CPUPercent.Float64)
	}

	// future since should return empty
	future, err := s.Metrics.GetRange(ctx, serverID, now.Add(time.Hour), 100)
	if err != nil {
		t.Fatalf("get future range: %v", err)
	}
	if len(future) != 0 {
		t.Fatalf("expected 0 metrics, got %d", len(future))
	}
}

func TestMetricCleanupOlderThan(t *testing.T) {
	db := testMetricsDB(t)
	s := New(db)
	ctx := context.Background()
	serverID := createTestServerForMetrics(t, s, ctx)

	old := &ServerMetric{
		ServerID:    serverID,
		CollectedAt: time.Now().Add(-2 * time.Minute),
		CPUPercent:  sql.NullFloat64{Float64: 1.0, Valid: true},
	}
	new := &ServerMetric{
		ServerID:    serverID,
		CollectedAt: time.Now().Add(-2 * time.Minute),
		CPUPercent:  sql.NullFloat64{Float64: 2.0, Valid: true},
	}
	if err := s.Metrics.Insert(ctx, old); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := s.Metrics.Insert(ctx, new); err != nil {
		t.Fatalf("insert new: %v", err)
	}
	// Insert relies on SQLite's CURRENT_TIMESTAMP default; explicitly move both
	// rows into the past so this test exercises deletion rather than insertion
	// timestamp defaults.
	if _, err := db.ExecContext(ctx,
		"UPDATE server_metrics SET collected_at = ? WHERE id IN (?, ?)",
		old.CollectedAt, old.ID, new.ID,
	); err != nil {
		t.Fatalf("age metrics: %v", err)
	}

	cutoff := time.Now().Add(-time.Minute)
	deleted, err := s.Metrics.CleanupOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	latest, err := s.Metrics.GetLatest(ctx, serverID)
	if err != nil {
		t.Fatalf("get latest after cleanup: %v", err)
	}
	if latest != nil {
		t.Fatal("expected nil after cleanup")
	}
}
