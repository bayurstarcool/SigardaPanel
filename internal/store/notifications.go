package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Notification represents an in-app notification for a user.
type Notification struct {
	ID         int64          `json:"id"`
	UserID     int64          `json:"user_id"`
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Message    string         `json:"message"`
	Severity   string         `json:"severity"`
	Read       bool           `json:"read"`
	EntityType string         `json:"entity_type"`
	EntityID   int64          `json:"entity_id"`
	CreatedAt  time.Time      `json:"created_at"`
	ReadAt     sql.NullTime   `json:"read_at,omitempty"`
}

// NotificationRepository defines notification operations.
type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) error
	GetByID(ctx context.Context, id int64) (*Notification, error)
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]Notification, int, error)
	CountUnread(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, id int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	Delete(ctx context.Context, id int64) error
	DeleteAllRead(ctx context.Context, userID int64) error
}

// SQLiteNotificationStore implements NotificationRepository against SQLite.
type SQLiteNotificationStore struct {
	db *sql.DB
}

// NewNotificationStore returns a new SQLite-backed notification store.
func NewNotificationStore(db *sql.DB) *SQLiteNotificationStore {
	return &SQLiteNotificationStore{db: db}
}

// Create inserts a new notification.
func (s *SQLiteNotificationStore) Create(ctx context.Context, n *Notification) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, type, title, message, severity, entity_type, entity_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Type, n.Title, n.Message, n.Severity, n.EntityType, n.EntityID,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	n.ID = id
	return nil
}

// GetByID retrieves a notification by ID.
func (s *SQLiteNotificationStore) GetByID(ctx context.Context, id int64) (*Notification, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, type, title, message, severity, read, entity_type, entity_id, created_at, read_at
		 FROM notifications WHERE id = ?`, id)
	return scanNotification(row)
}

// ListByUser returns notifications for a user, newest first, with total count.
func (s *SQLiteNotificationStore) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]Notification, int, error) {
	if limit <= 0 {
		limit = 50
	}

	// Count total
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ?`, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, type, title, message, severity, read, entity_type, entity_id, created_at, read_at
		 FROM notifications WHERE user_id = ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Message, &n.Severity,
			&n.Read, &n.EntityType, &n.EntityID, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CountUnread returns the count of unread notifications for a user.
func (s *SQLiteNotificationStore) CountUnread(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read = 0`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return count, nil
}

// MarkRead marks a single notification as read.
func (s *SQLiteNotificationStore) MarkRead(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = 1, read_at = CURRENT_TIMESTAMP WHERE id = ? AND read = 0`, id)
	return err
}

// MarkAllRead marks all notifications for a user as read.
func (s *SQLiteNotificationStore) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = 1, read_at = CURRENT_TIMESTAMP WHERE user_id = ? AND read = 0`, userID)
	return err
}

// Delete removes a single notification.
func (s *SQLiteNotificationStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE id = ?`, id)
	return err
}

// DeleteAllRead removes all read notifications for a user.
func (s *SQLiteNotificationStore) DeleteAllRead(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE user_id = ? AND read = 1`, userID)
	return err
}

func scanNotification(row *sql.Row) (*Notification, error) {
	var n Notification
	err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Message, &n.Severity,
		&n.Read, &n.EntityType, &n.EntityID, &n.CreatedAt, &n.ReadAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan notification: %w", err)
	}
	return &n, nil
}
