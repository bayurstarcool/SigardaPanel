package store

import (
	"database/sql"
	"testing"
	"time"
)

// Reads must work for both the canonical format and the legacy time.String()
// format (with monotonic suffix) written by older builds.
func TestSiteSSLExpiresAt_ReadsBothFormats(t *testing.T) {
	d := testDB(t)
	s := NewSiteStore(d)
	if _, err := d.Exec(`INSERT INTO servers (id, name, hostname, agent_token) VALUES (1, 's', 'h', 't')`); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 4, 11, 41, 55, 0, time.UTC)
	for id, v := range map[int64]string{
		1: "2026-12-04 11:41:55",
		2: "2026-12-04 11:41:55.264167352 +0000 UTC m=+7776485.03",
	} {
		if _, err := d.Exec(`INSERT INTO sites (id, server_id, name, domain, root_path, ssl_expires_at) VALUES (?, 1, 'n', ?, '/x', ?)`,
			id, "d"+string(rune('0'+id))+".test", v); err != nil {
			t.Fatal(err)
		}
		site, err := s.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("read %q: %v", v, err)
		}
		if !site.SSLExpiresAt.Valid || !site.SSLExpiresAt.Time.Truncate(time.Second).Equal(want) {
			t.Fatalf("read %q = %+v, want %v", v, site.SSLExpiresAt, want)
		}
	}
}

func TestSiteSSLExpiresAt_WritesCanonical(t *testing.T) {
	d := testDB(t)
	s := NewSiteStore(d)
	if _, err := d.Exec(`INSERT INTO servers (id, name, hostname, agent_token) VALUES (1, 's', 'h', 't')`); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(90 * 24 * time.Hour) // carries a monotonic reading
	site := &Site{ServerID: 1, Name: "n", Domain: "a.test", RootPath: "/x", Status: "active",
		SSLStatus:    sql.NullString{String: "imported", Valid: true},
		SSLExpiresAt: sql.NullTime{Time: exp, Valid: true}}
	if err := s.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	check := func(label string) {
		var raw string
		if err := d.QueryRow(`SELECT CAST(ssl_expires_at AS TEXT) FROM sites WHERE id = ?`, site.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if raw != exp.UTC().Format("2006-01-02 15:04:05") {
			t.Fatalf("%s stored %q, want canonical UTC", label, raw)
		}
	}
	check("Create")
	if err := s.UpdateSSL(ctx, site.ID, "active", "LE", exp); err != nil {
		t.Fatal(err)
	}
	check("UpdateSSL")
}
