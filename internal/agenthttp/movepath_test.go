package agenthttp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMovePath_SameFilesystem(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := movePath(src, dst); err != nil {
		t.Fatalf("movePath: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be gone, got err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil || string(got) != "world" {
		t.Fatalf("expected nested file preserved, got %q err=%v", got, err)
	}
}

func TestCopyPath_PreservesTreeAndModes(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("data"), 0640); err != nil {
		t.Fatal(err)
	}

	if err := copyPath(src, dst); err != nil {
		t.Fatalf("copyPath: %v", err)
	}
	// src must still exist (copy, not move)
	if _, err := os.Stat(filepath.Join(src, "f.txt")); err != nil {
		t.Fatalf("src should remain after copy: %v", err)
	}
	info, err := os.Stat(filepath.Join(dst, "f.txt"))
	if err != nil {
		t.Fatalf("dst file: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("expected mode 0640, got %v", info.Mode().Perm())
	}
	got, _ := os.ReadFile(filepath.Join(dst, "f.txt"))
	if string(got) != "data" {
		t.Fatalf("expected content 'data', got %q", got)
	}
}

func TestCopyPath_Symlink(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "link")
	dst := filepath.Join(base, "link-copy")
	if err := os.Symlink("/target/path", src); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := copyPath(src, dst); err != nil {
		t.Fatalf("copyPath symlink: %v", err)
	}
	target, err := os.Readlink(dst)
	if err != nil || target != "/target/path" {
		t.Fatalf("expected symlink preserved, got %q err=%v", target, err)
	}
}
