package apihttp

import (
	"strings"
	"testing"
)

func TestAuditSQLHidesLiterals(t *testing.T) {
	got := auditSQL("UPDATE users\n  SET password = 'hunter2', note='it''s' WHERE id = 3")
	if strings.Contains(got, "hunter2") || strings.Contains(got, "it''s") {
		t.Fatalf("literal leaked: %q", got)
	}
	if got != "UPDATE users SET password = '…', note='…' WHERE id = 3" {
		t.Fatalf("got %q", got)
	}
	long := auditSQL(strings.Repeat("é", 600))
	if !strings.HasSuffix(long, "…") || len(long) > 504 {
		t.Fatalf("not truncated cleanly: %d", len(long))
	}
}
