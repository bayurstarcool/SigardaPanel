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
	GPUName        sql.NullString
	GPUCount       sql.NullInt64
	GPUUtilization sql.NullFloat64
	GPUMemoryUsed  sql.NullInt64
	GPUMemoryTotal sql.NullInt64
	GPUTemperature sql.NullFloat64
	GPUPowerDraw   sql.NullFloat64
	GPUFanSpeed    sql.NullFloat64
}

const metricsColumns = `id, server_id, collected_at, cpu_percent, mem_used, mem_total, disk_used, disk_total, swap_used, swap_total, net_in_bytes, net_out_bytes, gpu_name, gpu_count, gpu_utilization, gpu_memory_used, gpu_memory_total, gpu_temperature, gpu_power_draw, gpu_fan_speed`

// MetricRepository defines metric storage operations.
type MetricRepository interface {
	Insert(ctx context.Context, m *ServerMetric) error
	GetLatest(ctx context.Context, serverID int64) (*ServerMetric, error)
	GetRange(ctx context.Context, serverID int64, since time.Time, limit int) ([]ServerMetric, error)
	GetRangeBucketed(ctx context.Context, serverID int64, since, until time.Time, maxPoints int) ([]ServerMetric, error)
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

// sqliteUTC formats t like SQLite's CURRENT_TIMESTAMP (UTC, second
// precision), the format collected_at is written in, so text comparisons
// against the column are correct regardless of the process time zone.
func sqliteUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// maxRangeRows caps how many raw rows GetRangeBucketed reads for one chart
// (~70 days at the 60s collection interval).
const maxRangeRows = 100000

// GetRangeBucketed returns metrics for a server collected in [since, until],
// oldest first, spread over the whole range: when there are more than
// maxPoints samples they are grouped into maxPoints equal time windows and
// each window is reduced to one averaged point (timestamp and id of the last
// sample in the window). This keeps 24h/7d charts covering the full range
// instead of only the newest N samples.
func (s *SQLiteMetricStore) GetRangeBucketed(ctx context.Context, serverID int64, since, until time.Time, maxPoints int) ([]ServerMetric, error) {
	if maxPoints <= 0 {
		maxPoints = 720
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+metricsColumns+` FROM server_metrics WHERE server_id = ? AND collected_at >= ? AND collected_at < ? ORDER BY collected_at ASC, id ASC LIMIT ?`,
		// Upper bound is exclusive and one second past until: rows may carry
		// sub-second precision that a second-precision bound would cut off.
		serverID, sqliteUTC(since), sqliteUTC(until.Add(time.Second)), maxRangeRows,
	)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var raw []ServerMetric
	for rows.Next() {
		var m ServerMetric
		if err := scanMetricRow(rows, &m); err != nil {
			return nil, err
		}
		raw = append(raw, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(raw) <= maxPoints {
		return raw, nil
	}
	return bucketMetrics(raw, since, until, maxPoints), nil
}

// bucketMetrics averages time-ordered samples into at most n windows of equal
// duration covering [since, until]. Empty windows produce no point.
func bucketMetrics(raw []ServerMetric, since, until time.Time, n int) []ServerMetric {
	span := until.Sub(since)
	if span <= 0 {
		span = raw[len(raw)-1].CollectedAt.Sub(raw[0].CollectedAt) + time.Second
		since = raw[0].CollectedAt
	}
	width := span / time.Duration(n)
	if width <= 0 {
		width = time.Second
	}
	out := make([]ServerMetric, 0, n)
	var acc metricAcc
	cur := -1
	for _, m := range raw {
		idx := int(m.CollectedAt.Sub(since) / width)
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		if idx != cur && acc.count > 0 {
			out = append(out, acc.result())
			acc = metricAcc{}
		}
		cur = idx
		acc.add(m)
	}
	if acc.count > 0 {
		out = append(out, acc.result())
	}
	return out
}

// metricAcc accumulates samples of one bucket.
type metricAcc struct {
	count int
	last  ServerMetric
	f     [5]avgF // cpu, gpu util, gpu temp, gpu power, gpu fan
	i     [10]avgI
}

type avgF struct {
	sum float64
	n   int
}

type avgI struct {
	sum float64
	n   int
}

func (a *avgF) add(v sql.NullFloat64) {
	if v.Valid {
		a.sum += v.Float64
		a.n++
	}
}

func (a avgF) get() sql.NullFloat64 {
	if a.n == 0 {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: a.sum / float64(a.n), Valid: true}
}

func (a *avgI) add(v sql.NullInt64) {
	if v.Valid {
		a.sum += float64(v.Int64)
		a.n++
	}
}

func (a avgI) get() sql.NullInt64 {
	if a.n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(a.sum/float64(a.n) + 0.5), Valid: true}
}

func (a *metricAcc) add(m ServerMetric) {
	a.count++
	a.last = m
	a.f[0].add(m.CPUPercent)
	a.f[1].add(m.GPUUtilization)
	a.f[2].add(m.GPUTemperature)
	a.f[3].add(m.GPUPowerDraw)
	a.f[4].add(m.GPUFanSpeed)
	for k, v := range []sql.NullInt64{m.MemUsed, m.MemTotal, m.DiskUsed, m.DiskTotal, m.SwapUsed, m.SwapTotal, m.NetInBytes, m.NetOutBytes, m.GPUMemoryUsed, m.GPUMemoryTotal} {
		a.i[k].add(v)
	}
}

func (a *metricAcc) result() ServerMetric {
	m := a.last // id, server_id, collected_at, gpu name/count of the last sample
	m.CPUPercent = a.f[0].get()
	m.GPUUtilization = a.f[1].get()
	m.GPUTemperature = a.f[2].get()
	m.GPUPowerDraw = a.f[3].get()
	m.GPUFanSpeed = a.f[4].get()
	m.MemUsed, m.MemTotal = a.i[0].get(), a.i[1].get()
	m.DiskUsed, m.DiskTotal = a.i[2].get(), a.i[3].get()
	m.SwapUsed, m.SwapTotal = a.i[4].get(), a.i[5].get()
	m.NetInBytes, m.NetOutBytes = a.i[6].get(), a.i[7].get()
	m.GPUMemoryUsed, m.GPUMemoryTotal = a.i[8].get(), a.i[9].get()
	return m
}

// PruneMetrics deletes server metrics collected more than olderThan ago and
// returns how many rows were removed. It is meant to be run periodically
// (e.g. hourly) by the scheduler; olderThan <= 0 is a no-op.
func (s *Store) PruneMetrics(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, nil
	}
	return s.Metrics.CleanupOlderThan(ctx, time.Now().Add(-olderThan))
}

// CleanupOlderThan deletes metrics older than the given time.
func (s *SQLiteMetricStore) CleanupOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM server_metrics WHERE collected_at < ?`, sqliteUTC(before))
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
