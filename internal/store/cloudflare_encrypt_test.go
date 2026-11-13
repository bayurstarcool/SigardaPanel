package store

import (
	"strings"
	"testing"
)

func TestCloudflareTokenIsEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	s := NewCloudflareStore(db)
	db.Exec(`INSERT INTO users (id, username, email, password_hash, role) VALUES (5, 'cf', 'cf@example.com', 'x', 'user')`)
	if err := s.SaveConfig(5, CloudflareConfig{APIToken: "cf-secret-token-123", Email: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	var raw string
	db.QueryRow(`SELECT api_token FROM cloudflare_config WHERE user_id = 5`).Scan(&raw)
	if strings.Contains(raw, "cf-secret-token-123") || !strings.HasPrefix(raw, "enc:v1:") {
		t.Fatalf("stored token = %q", raw)
	}
	cfg, err := s.GetConfig(5)
	if err != nil || cfg.APIToken != "cf-secret-token-123" {
		t.Fatalf("read back %+v %v", cfg, err)
	}
	// Tokens saved before encryption still read.
	db.Exec(`UPDATE cloudflare_config SET api_token = 'legacy-plain' WHERE user_id = 5`)
	if cfg, _ := s.GetConfig(5); cfg.APIToken != "legacy-plain" {
		t.Fatalf("legacy token = %q", cfg.APIToken)
	}
}
