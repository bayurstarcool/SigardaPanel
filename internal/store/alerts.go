package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AlertChannel represents a notification channel configuration.
type AlertChannel struct {
	ID        int64
	Name      string
	Type      string
	Enabled   bool
	Config    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Alert represents a single alert record.
type Alert struct {
	ID        int64
	ChannelID int64
	Severity  string
	Subject   string
	Message   string
	Status    string
	Error     sql.NullString
	CreatedAt time.Time
	SentAt    sql.NullTime
}

// AlertChannelRepository defines operations for alert channels.
type AlertChannelRepository interface {
	Create(ctx context.Context, ch *AlertChannel) error
	GetByID(ctx context.Context, id int64) (*AlertChannel, error)
	List(ctx context.Context) ([]AlertChannel, error)
	Update(ctx context.Context, ch *AlertChannel) error
	Delete(ctx context.Context, id int64) error
}

// AlertRepository defines operations for alerts.
type AlertRepository interface {
	Create(ctx context.Context, a *Alert) error
	List(ctx context.Context, limit int) ([]Alert, error)
	MarkSent(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, errMsg string) error
}

// SQLiteAlertChannelStore implements AlertChannelRepository against SQLite.
type SQLiteAlertChannelStore struct {
	db *sql.DB
}

// NewAlertChannelStore returns a new SQLite-backed alert channel store.
func NewAlertChannelStore(db *sql.DB) *SQLiteAlertChannelStore {
	return &SQLiteAlertChannelStore{db: db}
}

// Create inserts a new alert channel.
func (s *SQLiteAlertChannelStore) Create(ctx context.Context, ch *AlertChannel) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO alert_channels (name, type, enabled, config) VALUES (?, ?, ?, ?)`,
		ch.Name, ch.Type, boolToInt(ch.Enabled), ch.Config,
	)
	if err != nil {
		return fmt.Errorf("insert alert channel: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	ch.ID = id
	return nil
}

// GetByID retrieves an alert channel by ID. Returns (nil, nil) if not found.
func (s *SQLiteAlertChannelStore) GetByID(ctx context.Context, id int64) (*AlertChannel, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, enabled, config, created_at, updated_at FROM alert_channels WHERE id = ?`, id)
	return scanAlertChannel(row)
}

// List returns all alert channels ordered by id ASC.
func (s *SQLiteAlertChannelStore) List(ctx context.Context) ([]AlertChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, enabled, config, created_at, updated_at FROM alert_channels ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query alert channels: %w", err)
	}
	defer rows.Close()

	var out []AlertChannel
	for rows.Next() {
		var ch AlertChannel
		var enabled int
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &enabled, &ch.Config, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan alert channel: %w", err)
		}
		ch.Enabled = enabled != 0
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// Update modifies an alert channel.
func (s *SQLiteAlertChannelStore) Update(ctx context.Context, ch *AlertChannel) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_channels SET name = ?, type = ?, enabled = ?, config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		ch.Name, ch.Type, boolToInt(ch.Enabled), ch.Config, ch.ID,
	)
	if err != nil {
		return fmt.Errorf("update alert channel: %w", err)
	}
	return nil
}

// Delete removes an alert channel.
func (s *SQLiteAlertChannelStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete alert channel: %w", err)
	}
	return nil
}

// SQLiteAlertStore implements AlertRepository against SQLite.
type SQLiteAlertStore struct {
	db *sql.DB
}

// NewAlertStore returns a new SQLite-backed alert store.
func NewAlertStore(db *sql.DB) *SQLiteAlertStore {
	return &SQLiteAlertStore{db: db}
}

// Create inserts a new alert with status "pending".
func (s *SQLiteAlertStore) Create(ctx context.Context, a *Alert) error {
	if a.Status == "" {
		a.Status = "pending"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO alerts (channel_id, severity, subject, message, status) VALUES (?, ?, ?, ?, ?)`,
		a.ChannelID, a.Severity, a.Subject, a.Message, a.Status,
	)
	if err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	a.ID = id
	return nil
}

// List returns alerts ordered by id DESC with a limit.
func (s *SQLiteAlertStore) List(ctx context.Context, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_id, severity, subject, message, status, error, created_at, sent_at FROM alerts ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query alerts: %w", err)
	}
	defer rows.Close()

	var out []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.ChannelID, &a.Severity, &a.Subject, &a.Message, &a.Status, &a.Error, &a.CreatedAt, &a.SentAt); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// MarkSent updates an alert as sent.
func (s *SQLiteAlertStore) MarkSent(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET status = ?, sent_at = CURRENT_TIMESTAMP WHERE id = ?`,
		"sent", id,
	)
	if err != nil {
		return fmt.Errorf("mark alert sent: %w", err)
	}
	return nil
}

// MarkFailed updates an alert as failed with an error message.
func (s *SQLiteAlertStore) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET status = ?, error = ? WHERE id = ?`,
		"failed", sql.NullString{String: errMsg, Valid: errMsg != ""}, id,
	)
	if err != nil {
		return fmt.Errorf("mark alert failed: %w", err)
	}
	return nil
}

func scanAlertChannel(row *sql.Row) (*AlertChannel, error) {
	var ch AlertChannel
	var enabled int
	err := row.Scan(&ch.ID, &ch.Name, &ch.Type, &enabled, &ch.Config, &ch.CreatedAt, &ch.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan alert channel: %w", err)
	}
	ch.Enabled = enabled != 0
	return &ch, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
