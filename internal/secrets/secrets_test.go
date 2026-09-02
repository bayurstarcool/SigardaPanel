package secrets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := encryptWith(key, "s3cr3t-value")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, prefix) {
		t.Fatalf("expected prefix, got %q", enc)
	}
	if strings.Contains(enc, "s3cr3t-value") {
		t.Fatalf("plaintext leaked in ciphertext: %q", enc)
	}
	dec, err := decryptWith(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "s3cr3t-value" {
		t.Fatalf("roundtrip mismatch: %q", dec)
	}
}

func TestDecryptPassthroughPlaintext(t *testing.T) {
	key := make([]byte, 32)
	got, err := decryptWith(key, "not-encrypted")
	if err != nil || got != "not-encrypted" {
		t.Fatalf("expected passthrough, got %q err=%v", got, err)
	}
}

func TestEncryptEmpty(t *testing.T) {
	key := make([]byte, 32)
	got, err := encryptWith(key, "")
	if err != nil || got != "" {
		t.Fatalf("expected empty passthrough, got %q err=%v", got, err)
	}
}

func TestWalkEncryptsSensitiveKeysOnly(t *testing.T) {
	payload := `{"backup_id":1,"domain":"x.com","db_password":"pw","s3_config":{"bucket":"b","access_key":"AK","secret_key":"SK"}}`
	var v any
	_ = json.Unmarshal([]byte(payload), &v)
	enc := func(s string) (string, error) { return "ENC(" + s + ")", nil }
	out, err := walk(v, false, enc)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["domain"] != "x.com" {
		t.Fatalf("non-sensitive domain altered: %v", m["domain"])
	}
	if m["backup_id"].(float64) != 1 {
		t.Fatalf("backup_id altered: %v", m["backup_id"])
	}
	if m["db_password"] != "ENC(pw)" {
		t.Fatalf("db_password not encrypted: %v", m["db_password"])
	}
	s3 := m["s3_config"].(map[string]any)
	if s3["access_key"] != "ENC(AK)" || s3["secret_key"] != "ENC(SK)" {
		t.Fatalf("nested s3 creds not encrypted: %v", s3)
	}
	if s3["bucket"] != "b" {
		t.Fatalf("bucket altered: %v", s3["bucket"])
	}
}
