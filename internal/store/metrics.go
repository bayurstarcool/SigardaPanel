package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ServerMetric is a time-series row for server resource usage.
type ServerMetric struct {
	ID          int64
	ServerID    int64
	CollectedAt time.Time
	CPUPercent  sql.NullFloat64
	MemUsed     sql.NullInt64
	MemTotal    sql.NullInt64
	DiskUsed    sql.NullInt64
	DiskTotal   sql.NullInt64
	SwapUsed    sql.NullInt64
	SwapTotal   sql.NullInt64
	NetInBytes  sql.NullInt64
	NetOutBytes sql.NullInt64
	// GPU fields
	GPUName         sql.NullString
	GPUCount        sql.NullInt64
	GPUUtilization  sql.NullFloat64
	GPUMemoryUsed   sql.NullInt64
	GPUMemoryTotal  sql.NullInt64
	GPUTemperature  sql.NullFloat64
	GPUPowerDraw    sql.NullFloat64
	GPUFanSpeed     sql.NullFloat64
}

const metricsColumns = `id, server_id, collected_at, cpu_percent, mem_used, mem_total, disk_used, disk_total, swap_used, swap_total, net_in_bytes, net_out_bytes, gpu_name, gpu_count, gpu_utilization, gpu_memory_used, gpu_memory_total, gpu_temperature, gpu_power_draw, gpu_fan_speed`

// MetricRepository defines metric storage operations.
type MetricRepository interface {
	Insert(ctx context.Context, m *ServerMetric) error
	GetLatest(ctx context.Context, serverID int64) (*ServerMetric, error)
	GetRange(ctx context.Context, serverID int64, since time.Time, limit int) ([]ServerMetric, error)
	CleanupOlderThan(ctx context.Context, before time.Time) (int64, error)
}

// SQLiteMetricStore is the SQLite implementation of MetricRepository.
type SQLiteMetricStore struct {
	db *sql.DB
}

// NewMetricStore creates a new SQLiteMetricStore.
func NewMetricStore(db *sql.DB) *SQLiteMetricStore {
	return &SQLiteMetricStore{db: db}
}

// Insert stores a new metric row.
func (s *SQLiteMetricStore) Insert(ctx context.Context, m *ServerMetric) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO server_metrics (server_id, cpu_percent, mem_used, mem_total, disk_used, disk_total, swap_used, swap_total, net_in_bytes, net_out_bytes, gpu_name, gpu_count, gpu_utilization, gpu_memory_used, gpu_memory_total, gpu_temperature, gpu_power_draw, gpu_fan_speed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ServerID, m.CPUPercent, m.MemUsed, m.MemTotal, m.DiskUsed, m.DiskTotal, m.SwapUsed, m.SwapTotal, m.NetInBytes, m.NetOutBytes,
		m.GPUName, m.GPUCount, m.GPUUtilization, m.GPUMemoryUsed, m.GPUMemoryTotal, m.GPUTemperature, m.GPUPowerDraw, m.GPUFanSpeed,
	)
	if err != nil {
		return fmt.Errorf("insert metric: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	return nil
}

// GetLatest returns the most recent metric for a server.
func (s *SQLiteMetricStore) GetLatest(ctx context.Context, serverID int64) (*ServerMetric, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+metricsColumns+` FROM server_metrics WHERE server_id = ? ORDER BY collected_at DESC, id DESC LIMIT 1`,
		serverID,
	)
	return scanMetric(row)
}

// GetRange returns metrics for a server since a given time, newest first.
func (s *SQLiteMetricStore) GetRange(ctx context.Context, serverID int64, since time.Time, limit int) ([]ServerMetric, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+metricsColumns+` FROM server_metrics WHERE server_id = ? AND collected_at >= ? ORDER BY collected_at DESC LIMIT ?`,
		serverID, since, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []ServerMetric
	for rows.Next() {
		var m ServerMetric
		if err := scanMetricRow(rows, &m); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

// CleanupOlderThan deletes metrics older than the given time.
func (s *SQLiteMetricStore) CleanupOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM server_metrics WHERE collected_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanMetric(row *sql.Row) (*ServerMetric, error) {
	var m ServerMetric
	err := row.Scan(&m.ID, &m.ServerID, &m.CollectedAt, &m.CPUPercent, &m.MemUsed, &m.MemTotal, &m.DiskUsed, &m.DiskTotal, &m.SwapUsed, &m.SwapTotal, &m.NetInBytes, &m.NetOutBytes, &m.GPUName, &m.GPUCount, &m.GPUUtilization, &m.GPUMemoryUsed, &m.GPUMemoryTotal, &m.GPUTemperature, &m.GPUPowerDraw, &m.GPUFanSpeed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan metric: %w", err)
	}
	return &m, nil
}

func scanMetricRow(rows *sql.Rows, m *ServerMetric) error {
	return rows.Scan(&m.ID, &m.ServerID, &m.CollectedAt, &m.CPUPercent, &m.MemUsed, &m.MemTotal, &m.DiskUsed, &m.DiskTotal, &m.SwapUsed, &m.SwapTotal, &m.NetInBytes, &m.NetOutBytes, &m.GPUName, &m.GPUCount, &m.GPUUtilization, &m.GPUMemoryUsed, &m.GPUMemoryTotal, &m.GPUTemperature, &m.GPUPowerDraw, &m.GPUFanSpeed)
}
