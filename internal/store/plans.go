package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Plan represents a subscription plan with pricing and feature config.
type Plan struct {
	ID           int64
	Name         string
	Slug         string
	PriceIDR     int64
	BillingCycle string
	Features     string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PlansRepository defines plan persistence operations.
type PlansRepository interface {
	Create(ctx context.Context, plan *Plan) error
	GetByID(ctx context.Context, id int64) (*Plan, error)
	GetBySlug(ctx context.Context, slug string) (*Plan, error)
	List(ctx context.Context, activeOnly bool) ([]Plan, error)
	Update(ctx context.Context, plan *Plan) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}

// SQLitePlanStore implements PlansRepository against a SQLite database.
type SQLitePlanStore struct {
	db *sql.DB
}

// NewPlanStore returns a SQLite-backed plan store.
func NewPlanStore(db *sql.DB) *SQLitePlanStore {
	return &SQLitePlanStore{db: db}
}

// Create inserts a new plan.
func (s *SQLitePlanStore) Create(ctx context.Context, plan *Plan) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO plans (name, slug, price_idr, billing_cycle, features, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		plan.Name, plan.Slug, plan.PriceIDR, plan.BillingCycle, plan.Features, plan.IsActive,
	)
	if err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	plan.ID = id
	return nil
}

// GetByID retrieves a plan by ID.
func (s *SQLitePlanStore) GetByID(ctx context.Context, id int64) (*Plan, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, price_idr, billing_cycle, features, is_active, created_at, updated_at
		 FROM plans WHERE id = ?`, id)
	return scanPlan(row)
}

// GetBySlug retrieves a plan by slug.
func (s *SQLitePlanStore) GetBySlug(ctx context.Context, slug string) (*Plan, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, price_idr, billing_cycle, features, is_active, created_at, updated_at
		 FROM plans WHERE slug = ?`, slug)
	return scanPlan(row)
}

// List returns all plans, optionally filtered to active only.
func (s *SQLitePlanStore) List(ctx context.Context, activeOnly bool) ([]Plan, error) {
	query := `SELECT id, name, slug, price_idr, billing_cycle, features, is_active, created_at, updated_at FROM plans`
	var args []any
	if activeOnly {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.PriceIDR, &p.BillingCycle, &p.Features, &p.IsActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		p.CreatedAt, _ = parseTimestamp(createdAt)
		p.UpdatedAt, _ = parseTimestamp(updatedAt)
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// Update modifies an existing plan.
func (s *SQLitePlanStore) Update(ctx context.Context, plan *Plan) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE plans SET name = ?, slug = ?, price_idr = ?, billing_cycle = ?, features = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		plan.Name, plan.Slug, plan.PriceIDR, plan.BillingCycle, plan.Features, plan.IsActive, plan.ID,
	)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	return nil
}

// Delete removes a plan by ID.
func (s *SQLitePlanStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	return nil
}

// Count returns total number of plans.
func (s *SQLitePlanStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plans`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count plans: %w", err)
	}
	return count, nil
}

func scanPlan(row *sql.Row) (*Plan, error) {
	var p Plan
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.PriceIDR, &p.BillingCycle, &p.Features, &p.IsActive, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan plan: %w", err)
	}
	p.CreatedAt, _ = parseTimestamp(createdAt)
	p.UpdatedAt, _ = parseTimestamp(updatedAt)
	return &p, nil
}
