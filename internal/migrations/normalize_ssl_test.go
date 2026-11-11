package migrations

import (
	"os"
	"testing"
	"time"

	"sigardapanel/internal/db"
)

func TestParseLegacyTimestamp(t *testing.T) {
	want := time.Date(2026, 12, 4, 11, 41, 55, 0, time.UTC)
	cases := []string{
		"2026-12-04 11:41:55.264167352 +0000 UTC m=+7776485.03",
		"2026-12-04 11:41:55 +0000 UTC",
		"2026-12-04 18:41:55.5 +0700 WIB m=-1.5",
		"2026-12-04T11:41:55Z",
		"2026-12-04T18:41:55+07:00",
		"2026-12-04 11:41:55",
	}
	for _, c := range cases {
		got, ok := ParseLegacyTimestamp(c)
		if !ok {
			t.Errorf("ParseLegacyTimestamp(%q) failed", c)
			continue
		}
		if !got.Truncate(time.Second).Equal(want) {
			t.Errorf("ParseLegacyTimestamp(%q) = %v, want %v", c, got, want)
		}
	}
	if _, ok := ParseLegacyTimestamp("garbage"); ok {
		t.Error("garbage parsed")
	}
}

func TestNormalizeSSLExpiresAt(t *testing.T) {
	f, err := os.CreateTemp("", "sigardapanel-ssl-norm-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var before []Migration
	for _, m := range AllMigrations {
		if m.Version < NormalizeSSLExpiresAtSchema.Version {
			before = append(before, m)
		}
	}
	if err := Up(d, before); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO servers (id, name, hostname, agent_token) VALUES (1, 's', 'h', 't')`); err != nil {
		t.Fatal(err)
	}
	rows := map[int64]any{
		1: "2026-12-04 11:41:55.264167352 +0000 UTC m=+7776485.03",
		2: "2026-12-04 18:41:55 +0700 WIB",
		3: "2026-12-04 11:41:55",
		4: "not a date",
		5: nil,
	}
	for id, v := range rows {
		if _, err := d.Exec(`INSERT INTO sites (id, server_id, name, domain, root_path, ssl_expires_at) VALUES (?, 1, 'n', ?, '/x', ?)`,
			id, "d"+string(rune('0'+id))+".test", v); err != nil {
			t.Fatal(err)
		}
	}

	if err := Up(d, AllMigrations); err != nil {
		t.Fatalf("migrate 53: %v", err)
	}

	want := map[int64]any{
		1: "2026-12-04 11:41:55",
		2: "2026-12-04 11:41:55",
		3: "2026-12-04 11:41:55",
		4: "not a date",
		5: nil,
	}
	for id, w := range want {
		var got *string
		if err := d.QueryRow(`SELECT CAST(ssl_expires_at AS TEXT) FROM sites WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		switch {
		case w == nil && got != nil:
			t.Errorf("site %d = %q, want NULL", id, *got)
		case w != nil && (got == nil || *got != w.(string)):
			t.Errorf("site %d = %v, want %q", id, got, w)
		}
	}
}
