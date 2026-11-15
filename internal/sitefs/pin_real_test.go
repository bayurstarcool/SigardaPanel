package sitefs

import (
	"path/filepath"
	"strings"
	"testing"
)

// Processes started as the site user cannot enter the agent's
// /proc/<pid>/fd/<n>; they get the verified real path instead.
func TestPinDirRealPath(t *testing.T) {
	dir := t.TempDir()
	p, err := PinDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	want, _ := filepath.EvalSymlinks(dir)
	if p.Real != want || strings.HasPrefix(p.Real, "/proc/") || !strings.HasPrefix(p.Path, "/proc/") {
		t.Fatalf("Path=%s Real=%s want %s", p.Path, p.Real, want)
	}
}
