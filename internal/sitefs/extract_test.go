package sitefs

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string]string, extra func(zw *zip.Writer)) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if extra != nil {
		extra(zw)
	}
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListSortedFoldersFirstAndSymlinks(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(root, "A.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(root, "zdir"), 0o755)
	os.Mkdir(filepath.Join(root, "adir"), 0o755)
	os.Symlink("adir", filepath.Join(root, "link-in"))
	os.Symlink("/etc", filepath.Join(root, "link-out"))
	f := &FS{Root: root}
	got, err := f.List(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	byName := map[string]Entry{}
	for _, e := range got {
		names = append(names, e.Name)
		byName[e.Name] = e
	}
	want := []string{"adir", "link-in", "zdir", "A.txt", "b.txt", "link-out"}
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order = %v, want %v", names, want)
		}
	}
	if e := byName["link-in"]; !e.IsSymlink || !e.IsDir {
		t.Fatalf("link-in = %+v, want symlink to a dir", e)
	}
	if e := byName["link-out"]; !e.IsSymlink || e.IsDir {
		t.Fatalf("link-out = %+v, want a symlink that cannot be followed", e)
	}
}

func TestReadFollowsLinksOnlyInsideRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "real.txt"), []byte("hello"), 0o644)
	os.Symlink("real.txt", filepath.Join(root, "alias.txt"))
	os.Symlink("/etc/hostname", filepath.Join(root, "escape.txt"))
	f := &FS{Root: root}
	if data, _, err := f.Read("alias.txt"); err != nil || string(data) != "hello" {
		t.Fatalf("read via link = %q, %v", data, err)
	}
	if _, _, err := f.Read("escape.txt"); err == nil {
		t.Fatal("a link leaving the root was followed")
	}
}

func TestIsText(t *testing.T) {
	if !IsText([]byte("héllo\r\nworld")) {
		t.Fatal("utf-8 text reported as binary")
	}
	if IsText([]byte{0x89, 'P', 'N', 'G', 0}) || IsText([]byte{0xe9, 't', 'e'}) {
		t.Fatal("binary or latin-1 reported as text")
	}
}

func TestChmod(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "run.sh")
	os.WriteFile(p, []byte("#!/bin/sh"), 0o644)
	os.Symlink("run.sh", filepath.Join(root, "lnk"))
	f := &FS{Root: root}
	if err := f.Chmod("run.sh", "755"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	for _, bad := range []string{"4755", "7777", "abc", "9", "1000"} {
		if err := f.Chmod("run.sh", bad); err == nil {
			t.Fatalf("mode %q accepted", bad)
		}
	}
	if err := f.Chmod("lnk", "600"); err == nil {
		t.Fatal("chmod through a symlink accepted")
	}
	if err := f.Chmod(".", "777"); err == nil {
		t.Fatal("chmod of the site root accepted")
	}
}

func TestExtract(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "keep.txt"), []byte("original"), 0o644)
	writeZip(t, filepath.Join(root, "a.zip"), map[string]string{
		"dir/one.txt":        "1",
		"two.txt":            "2",
		"keep.txt":           "overwritten?",
		"../escape.txt":      "zip slip",
		"dir/../../evil.txt": "zip slip 2",
	}, func(zw *zip.Writer) {
		h := &zip.FileHeader{Name: "link"}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := zw.CreateHeader(h)
		w.Write([]byte("/etc/passwd"))
	})
	f := &FS{Root: root}
	written, skipped, err := f.Extract("a.zip", ".")
	if err != nil {
		t.Fatal(err)
	}
	if written != 2 || skipped != 4 {
		t.Fatalf("written=%d skipped=%d, want 2 and 4", written, skipped)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "dir/one.txt")); string(b) != "1" {
		t.Fatal("dir/one.txt not extracted")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "keep.txt")); string(b) != "original" {
		t.Fatal("existing file overwritten")
	}
	if _, err := os.Lstat(filepath.Join(root, "link")); err == nil {
		t.Fatal("symlink entry extracted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Fatal("zip slip wrote outside the root")
	}
	if _, _, err := f.Extract("keep.txt", "."); err == nil {
		t.Fatal("non-zip accepted")
	}
}

func TestExtractRejectsBomb(t *testing.T) {
	root := t.TempDir()
	// A zip whose headers claim a small size but whose stored size is over
	// the limit is caught by the declared total; here the total is checked.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "big.bin", Method: zip.Deflate}
	h.UncompressedSize64 = MaxExtractBytes + 1
	w, _ := zw.CreateRaw(h)
	w.Write([]byte{})
	zw.Close()
	os.WriteFile(filepath.Join(root, "bomb.zip"), buf.Bytes(), 0o644)
	f := &FS{Root: root}
	if _, _, err := f.Extract("bomb.zip", "."); err == nil {
		t.Fatal("oversized archive accepted")
	}
}
