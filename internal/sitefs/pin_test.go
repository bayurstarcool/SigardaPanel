package sitefs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// userLink makes a symlink that looks planted by a site user (owned by a
// non-root uid when the test runs as root).
func userLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Lchown(link, 4242, 4242); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSymlinkedRootRefused(t *testing.T) {
	base := t.TempDir()
	secret := filepath.Join(base, "secret")
	os.MkdirAll(secret, 0o755)
	os.WriteFile(filepath.Join(secret, "shadow"), []byte("root:hash"), 0o600)

	// ~/htdocs replaced by a link: the root itself and anything below it.
	home := filepath.Join(base, "home", "bob")
	os.MkdirAll(home, 0o755)
	userLink(t, secret, filepath.Join(home, "htdocs"))
	root := filepath.Join(home, "htdocs")

	if err := CheckNoUserSymlinks(filepath.Join(root, "x.com")); err == nil {
		t.Fatal("path through a user symlink accepted")
	}
	if _, err := PinDir(root); err == nil {
		t.Fatal("PinDir followed a user symlink")
	}
	f := &FS{Root: root}
	if _, _, err := f.Read("shadow"); err == nil {
		t.Fatal("file manager read through a symlinked root")
	}
	if err := f.Write("passwd", []byte("x")); err == nil {
		t.Fatal("file manager wrote through a symlinked root")
	}
	if _, err := os.Stat(filepath.Join(secret, "passwd")); err == nil {
		t.Fatal("file created in the link target")
	}
}

func TestRemoveTreeNeverFollowsLinks(t *testing.T) {
	base := t.TempDir()
	victim := filepath.Join(base, "victim")
	os.MkdirAll(victim, 0o755)
	os.WriteFile(filepath.Join(victim, "keep"), []byte("x"), 0o644)
	parent := filepath.Join(base, "htdocs")
	os.MkdirAll(parent, 0o755)

	// Root replaced by a link: refused, target kept.
	userLink(t, victim, filepath.Join(parent, "a.com"))
	if err := RemoveTree(filepath.Join(parent, "a.com")); err == nil {
		t.Fatal("removed through a symlinked root")
	}
	// Parent replaced by a link: refused, target kept.
	userLink(t, base, filepath.Join(base, "lnk"))
	if err := RemoveTree(filepath.Join(base, "lnk", "victim")); err == nil {
		t.Fatal("removed through a symlinked parent")
	}
	if _, err := os.Stat(filepath.Join(victim, "keep")); err != nil {
		t.Fatal("link target was deleted")
	}
	// A real root is removed; a missing one is fine.
	real := filepath.Join(parent, "b.com")
	os.MkdirAll(filepath.Join(real, "sub"), 0o755)
	if err := RemoveTree(real); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(real); !os.IsNotExist(err) {
		t.Fatal("root not removed")
	}
	if err := RemoveTree(filepath.Join(parent, "gone.com")); err != nil {
		t.Fatalf("missing root: %v", err)
	}
}

func TestWriteNewFileDoesNotFollowOrOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "nologin")
	userLink(t, target, filepath.Join(dir, "index.html"))
	if err := WriteNewFile(dir, "index.html", []byte("x"), 0o644); err == nil {
		t.Fatal("followed a planted symlink")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("link target created")
	}
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("user code"), 0o644)
	_ = WriteNewFile(dir, "app.js", []byte("starter"), 0o644)
	if b, _ := os.ReadFile(filepath.Join(dir, "app.js")); string(b) != "user code" {
		t.Fatal("existing file overwritten")
	}
}

func TestChownTreeNeverFollowsLinks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	outside := filepath.Join(t.TempDir(), "etc-file")
	os.WriteFile(outside, []byte("x"), 0o644)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "d"), 0o755)
	os.WriteFile(filepath.Join(root, "d", "f"), []byte("x"), 0o644)
	os.Symlink(outside, filepath.Join(root, "link"))
	p, err := PinDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := ChownTree(p, 4343, 4343); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".", "d", "d/f"} {
		fi, _ := os.Lstat(filepath.Join(root, rel))
		if st := fi.Sys().(*syscall.Stat_t); st.Uid != 4343 {
			t.Fatalf("%s not chowned", rel)
		}
	}
	fi, _ := os.Stat(outside)
	if st := fi.Sys().(*syscall.Stat_t); st.Uid == 4343 {
		t.Fatal("link target chowned")
	}
}

// A FIFO planted in the site must not hang the agent.
func TestReadRefusesFIFO(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skip("mkfifo unavailable")
	}
	f := &FS{Root: root}
	done := make(chan error, 2)
	go func() { _, _, err := f.Read("pipe"); done <- err }()
	go func() {
		fh, _, err := f.OpenDownload("pipe")
		if fh != nil {
			fh.Close()
		}
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("FIFO accepted")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("opening a FIFO blocked")
		}
	}
}
