package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LicenseOrder represents a user's order to purchase a plan.
type LicenseOrder struct {
	ID            int64
	UserID        int64
	PlanID        int64
	Status        string
	PaymentMethod sql.NullString
	PaymentProof  sql.NullString
	AmountIDR     int64
	Notes         sql.NullString
	AdminNotes    sql.NullString
	ApprovedBy    sql.NullInt64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// OrdersRepository defines license order persistence operations.
type OrdersRepository interface {
	Create(ctx context.Context, order *LicenseOrder) error
	GetByID(ctx context.Context, id int64) (*LicenseOrder, error)
	List(ctx context.Context, filter OrderListFilter) ([]LicenseOrder, error)
	UpdateStatus(ctx context.Context, id int64, status string, adminNotes string, approvedBy int64) error
	Count(ctx context.Context) (int64, error)
}

// OrderListFilter controls which orders are returned by List.
type OrderListFilter struct {
	Status string // empty = all statuses
	UserID int64  // 0 = all users
}

// SQLiteOrderStore implements OrdersRepository against a SQLite database.
type SQLiteOrderStore struct {
	db *sql.DB
}

// NewOrderStore returns a SQLite-backed order store.
func NewOrderStore(db *sql.DB) *SQLiteOrderStore {
	return &SQLiteOrderStore{db: db}
}

// Create inserts a new license order.
func (s *SQLiteOrderStore) Create(ctx context.Context, order *LicenseOrder) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO license_orders (user_id, plan_id, status, payment_method, payment_proof, amount_idr, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		order.UserID, order.PlanID, order.Status, order.PaymentMethod, order.PaymentProof, order.AmountIDR, order.Notes,
	)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	order.ID = id
	return nil
}

// GetByID retrieves an order by ID.
func (s *SQLiteOrderStore) GetByID(ctx context.Context, id int64) (*LicenseOrder, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, plan_id, status, payment_method, payment_proof, amount_idr, notes, admin_notes, approved_by, created_at, updated_at
		 FROM license_orders WHERE id = ?`, id)
	return scanOrder(row)
}

// List returns orders, optionally filtered by status and/or user.
func (s *SQLiteOrderStore) List(ctx context.Context, filter OrderListFilter) ([]LicenseOrder, error) {
	query := `SELECT id, user_id, plan_id, status, payment_method, payment_proof, amount_idr, notes, admin_notes, approved_by, created_at, updated_at FROM license_orders`
	var args []any
	var conditions []string

	if filter.Status != "" {
		conditions = append(conditions, `status = ?`)
		args = append(args, filter.Status)
	}
	if filter.UserID > 0 {
		conditions = append(conditions, `user_id = ?`)
		args = append(args, filter.UserID)
	}

	if len(conditions) > 0 {
		query += ` WHERE `
		for i, c := range conditions {
			if i > 0 {
				query += ` AND `
			}
			query += c
		}
	}

	query += ` ORDER BY id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var orders []LicenseOrder
	for rows.Next() {
		o, err := scanOrderRow(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *o)
	}
	return orders, rows.Err()
}

// UpdateStatus changes an order's status, admin notes, and approver.
func (s *SQLiteOrderStore) UpdateStatus(ctx context.Context, id int64, status string, adminNotes string, approvedBy int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE license_orders SET status = ?, admin_notes = ?, approved_by = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		status, adminNotes, approvedBy, id,
	)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	return nil
}

// Count returns total number of orders.
func (s *SQLiteOrderStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM license_orders`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count orders: %w", err)
	}
	return count, nil
}

func scanOrder(row *sql.Row) (*LicenseOrder, error) {
	var o LicenseOrder
	var createdAt, updatedAt string
	err := row.Scan(&o.ID, &o.UserID, &o.PlanID, &o.Status, &o.PaymentMethod, &o.PaymentProof, &o.AmountIDR, &o.Notes, &o.AdminNotes, &o.ApprovedBy, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan order: %w", err)
	}
	o.CreatedAt, _ = parseTimestamp(createdAt)
	o.UpdatedAt, _ = parseTimestamp(updatedAt)
	return &o, nil
}

func scanOrderRow(rows *sql.Rows) (*LicenseOrder, error) {
	var o LicenseOrder
	var createdAt, updatedAt string
	if err := rows.Scan(&o.ID, &o.UserID, &o.PlanID, &o.Status, &o.PaymentMethod, &o.PaymentProof, &o.AmountIDR, &o.Notes, &o.AdminNotes, &o.ApprovedBy, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan order: %w", err)
	}
	o.CreatedAt, _ = parseTimestamp(createdAt)
	o.UpdatedAt, _ = parseTimestamp(updatedAt)
	return &o, nil
}
