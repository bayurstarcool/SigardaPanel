package sitefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBlocksSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "site")
	outside := filepath.Join(base, "secret")
	for _, d := range []string{root, outside, filepath.Join(root, "sub")} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(outside, "pw.txt"), []byte("x"), 0o644)
	os.Symlink(outside, filepath.Join(root, "link"))
	f := &FS{Root: root}
	for _, rel := range []string{".", "sub", "sub/new/file.txt", "newfile", "..foo"} {
		if _, err := f.Resolve(rel); err != nil {
			t.Errorf("Resolve(%q) unexpectedly rejected: %v", rel, err)
		}
	}
	for _, rel := range []string{"../secret", "link", "link/pw.txt", "link/new.txt", "/etc/passwd", "sub/../../secret"} {
		if _, err := f.Resolve(rel); err == nil {
			t.Errorf("Resolve(%q) should be rejected", rel)
		}
	}
}

func TestWriteRefusesSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("keep"), 0o600)
	f := &FS{Root: root}
	// A link inside the root that points inside the root is fine to resolve,
	// but writing through it is refused.
	os.WriteFile(filepath.Join(root, "real.txt"), []byte("a"), 0o644)
	os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "alias.txt"))
	if err := f.Write("alias.txt", []byte("x")); err == nil {
		t.Fatal("wrote through a symlink")
	}
	if got, _ := os.ReadFile(secret); string(got) != "keep" {
		t.Fatal("secret modified")
	}
}

// Every operation resolves beneath the root at the moment of use, so a
// directory replaced by a symlink to outside cannot redirect it.
func TestOperationsNeverFollowSymlinkedDirsOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(outside, "victim"), []byte("keep"), 0o644)
	os.Symlink(outside, filepath.Join(root, "dir"))
	f := &FS{Root: root}
	if err := f.Write("dir/victim", []byte("pwned")); err == nil {
		t.Fatal("wrote through a symlinked directory")
	}
	if err := f.Write("dir/new", []byte("x")); err == nil {
		t.Fatal("created a file through a symlinked directory")
	}
	if _, err := f.Delete("dir/victim"); err == nil {
		t.Fatal("deleted through a symlinked directory")
	}
	if _, _, err := f.Read("dir/victim"); err == nil {
		t.Fatal("read through a symlinked directory")
	}
	if _, _, err := f.SaveUpload("dir", "up.txt", strings.NewReader("x"), 10); err == nil {
		t.Fatal("uploaded through a symlinked directory")
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "victim")); string(b) != "keep" {
		t.Fatalf("outside file changed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(outside, "new")); err == nil {
		t.Fatal("file created outside the root")
	}
}
