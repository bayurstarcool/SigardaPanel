package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SiteDNSRecord is a DNS record the panel keeps track of for a site.
// Managed records were created by the panel (and carry its comment marker);
// unmanaged ones already existed and pointed at the server, and are never
// changed or removed by the panel.
type SiteDNSRecord struct {
	ID       int64
	SiteID   int64
	UserID   int64 // whose Cloudflare token created / found it
	ZoneID   string
	ZoneName string
	RecordID string
	Name     string
	Type     string
	Content  string
	Managed  bool
}

// SiteDNSStore persists site_dns_records.
type SiteDNSStore struct{ db *sql.DB }

// Add records one entry.
func (s *SiteDNSStore) Add(ctx context.Context, r *SiteDNSRecord) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO site_dns_records (site_id, user_id, zone_id, zone_name, record_id, name, type, content, managed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.SiteID, r.UserID, r.ZoneID, r.ZoneName, r.RecordID, r.Name, r.Type, r.Content, boolToInt(r.Managed))
	if err != nil {
		return fmt.Errorf("add site dns record: %w", err)
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// ListBySite returns the site's entries, oldest first.
func (s *SiteDNSStore) ListBySite(ctx context.Context, siteID int64) ([]SiteDNSRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, user_id, zone_id, zone_name, record_id, name, type, content, managed FROM site_dns_records WHERE site_id = ? ORDER BY id`, siteID)
	if err != nil {
		return nil, fmt.Errorf("list site dns records: %w", err)
	}
	defer rows.Close()
	var out []SiteDNSRecord
	for rows.Next() {
		var r SiteDNSRecord
		var managed int
		if err := rows.Scan(&r.ID, &r.SiteID, &r.UserID, &r.ZoneID, &r.ZoneName, &r.RecordID, &r.Name, &r.Type, &r.Content, &managed); err != nil {
			return nil, err
		}
		r.Managed = managed == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Delete removes one entry.
func (s *SiteDNSStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM site_dns_records WHERE id = ?`, id)
	return err
}

// DeleteBySite removes the site's entries.
func (s *SiteDNSStore) DeleteBySite(ctx context.Context, siteID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM site_dns_records WHERE site_id = ?`, siteID)
	return err
}
