package apihttp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafePathBlocksSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "site")
	outside := filepath.Join(base, "secret")
	for _, d := range []string{root, outside, filepath.Join(root, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "pw.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	ok := []string{".", "sub", "sub/new/file.txt", "newfile"}
	for _, rel := range ok {
		if _, err := safePath(root, rel); err != nil {
			t.Errorf("safePath(%q) unexpectedly rejected: %v", rel, err)
		}
	}
	bad := []string{"../secret", "link", "link/pw.txt", "link/new.txt", "/etc/passwd", "sub/../../secret"}
	for _, rel := range bad {
		if _, err := safePath(root, rel); err == nil {
			t.Errorf("safePath(%q) should be rejected", rel)
		}
	}
}
