package commands

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/db"
	"sigardapanel/internal/store"
)

// init: a password the operator supplies is final; --must-change-password
// marks it temporary (install.sh does the equivalent for a generated one).
func TestInitPasswordChangeRequirement(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	run := func(args ...string) {
		t.Helper()
		var out, errb bytes.Buffer
		if code := New(&out, &errb, strings.NewReader("")).Init(args); code != 0 {
			t.Fatalf("init %v: exit %d: %s", args, code, errb.String())
		}
	}
	run("--user", "chosen", "--password", "supersecret123")
	run("--user", "temp", "--password", "supersecret123", "--must-change-password")

	database, err := db.Open(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := store.New(database)
	for name, want := range map[string]bool{"chosen": false, "temp": true} {
		u, err := s.Users.GetByUsername(t.Context(), name)
		if err != nil || u == nil {
			t.Fatalf("%s: %v", name, err)
		}
		if u.PasswordChangeRequired() != want {
			t.Fatalf("%s: password_change_required=%v, want %v", name, u.PasswordChangeRequired(), want)
		}
	}
}
