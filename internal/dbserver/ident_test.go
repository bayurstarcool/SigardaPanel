package dbserver

import (
	"testing"

	"sigardapanel/internal/siteinput"
)

func TestIdentAllowList(t *testing.T) {
	for _, ok := range []string{"shop_db", "user1", "10.0.0.%", "%", "localhost", "my-app$1"} {
		if err := checkIdents(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a`b", "a'b", "a b", "x;DROP", "a\\b", "ä"} {
		if err := checkIdents(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := safeIdent("ab`c' d;"); got != "abcd" {
		t.Errorf("safeIdent = %q", got)
	}
	for _, ok := range []string{"utf8mb4", "utf8mb4_unicode_ci"} {
		if _, err := charsetName(ok); err != nil {
			t.Errorf("charset %q refused", ok)
		}
	}
	for _, bad := range []string{"", "utf8mb4 COLLATE x", "latin1,", "a-b"} {
		if _, err := charsetName(bad); err == nil {
			t.Errorf("charset %q accepted", bad)
		}
	}
}

func TestCanonicalPrivileges(t *testing.T) {
	got, err := siteinput.CanonicalDBPrivileges(" select ,  insert,create   view")
	if err != nil || got != "SELECT, INSERT, CREATE VIEW" {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"ALL ON *.* TO x", "GRANT OPTION", "SUPER", "SELECT; DROP"} {
		if _, err := siteinput.CanonicalDBPrivileges(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
