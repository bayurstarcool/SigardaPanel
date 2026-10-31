package store

import (
	"context"
	"fmt"

	"sigardapanel/internal/jobmodel"
)

// JobScope restricts which jobs ListJobsScoped returns for a caller that is
// not a super_admin.
type JobScope struct {
	// UserID, when > 0, includes jobs whose site the user owns or is
	// assigned to.
	UserID int64
	// IncludeServerJobs includes jobs not tied to any site (server-level
	// jobs such as stack installs), which only admins may see.
	IncludeServerJobs bool
}

// ListJobsScoped is Jobs.List restricted to the jobs visible under scope.
// Rows are loaded through Jobs.GetByID so they are decoded exactly like the
// unscoped listing.
func (s *Store) ListJobsScoped(ctx context.Context, filter JobListFilter, scope JobScope) ([]jobmodel.Job, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	var visible string
	var args []any
	switch {
	case scope.UserID > 0 && scope.IncludeServerJobs:
		visible = `(site_id IS NULL OR site_id IN (` + accessibleSitesSubquery + `))`
		args = append(args, scope.UserID, scope.UserID)
	case scope.UserID > 0:
		visible = `site_id IN (` + accessibleSitesSubquery + `)`
		args = append(args, scope.UserID, scope.UserID)
	case scope.IncludeServerJobs:
		visible = `site_id IS NULL`
	default:
		return nil, nil
	}

	query := `SELECT id FROM jobs WHERE ` + visible
	if filter.ServerID > 0 {
		query += ` AND server_id = ?`
		args = append(args, filter.ServerID)
	}
	if filter.SiteID > 0 {
		query += ` AND site_id = ?`
		args = append(args, filter.SiteID)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.Type != "" {
		query += ` AND type = ?`
		args = append(args, filter.Type)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query scoped jobs: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan job id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("rows: %w", err)
	}
	rows.Close()

	out := make([]jobmodel.Job, 0, len(ids))
	for _, id := range ids {
		j, err := s.Jobs.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if j != nil {
			out = append(out, *j)
		}
	}
	return out, nil
}
