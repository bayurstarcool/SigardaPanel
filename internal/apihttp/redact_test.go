package apihttp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactSensitivePayload_S3AndPassword(t *testing.T) {
	payload := `{"backup_id":1,"db_password":"s3cr3t","s3_config":{"bucket":"b","access_key":"AKIA","secret_key":"shh"}}`
	got := redactSensitivePayload(payload)

	for _, leaked := range []string{"s3cr3t", "AKIA", "shh"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("secret %q leaked in redacted payload: %s", leaked, got)
		}
	}
	// Non-sensitive fields preserved.
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("redacted output not valid JSON: %v", err)
	}
	if m["backup_id"].(float64) != 1 {
		t.Fatalf("expected backup_id preserved, got %v", m["backup_id"])
	}
	if m["db_password"] != "[REDACTED]" {
		t.Fatalf("expected db_password redacted, got %v", m["db_password"])
	}
	nested := m["s3_config"].(map[string]any)
	if nested["secret_key"] != "[REDACTED]" || nested["access_key"] != "[REDACTED]" {
		t.Fatalf("expected nested s3 creds redacted, got %v", nested)
	}
	if nested["bucket"] != "b" {
		t.Fatalf("expected bucket preserved, got %v", nested["bucket"])
	}
}

func TestRedactSensitivePayload_InvalidJSON(t *testing.T) {
	in := "not-json"
	if got := redactSensitivePayload(in); got != in {
		t.Fatalf("expected passthrough for non-JSON, got %q", got)
	}
}
