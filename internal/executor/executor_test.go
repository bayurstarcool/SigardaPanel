package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunShell_Allowed(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	out, err := e.RunShell(ctx, "echo", "hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hello world\n"
	if out != expected {
		t.Fatalf("unexpected output: got %q, want %q", out, expected)
	}
}

func TestRunShell_NotAllowed(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	_, err := e.RunShell(ctx, "rm", "-rf", "/")
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
	want := "command not allowed: rm"
	if err.Error() != want {
		t.Fatalf("unexpected error: got %q, want %q", err.Error(), want)
	}
}

func TestMkdir(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "subdir", "nested")
	if err := e.Mkdir(ctx, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		t.Fatalf("expected dir to exist: %v", err)
	}
}

func TestWriteFile(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.txt")
	content := "hello world"
	if err := e.WriteFile(ctx, path, content, 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected error reading file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("unexpected content: got %q, want %q", string(data), content)
	}
}

func TestRemoveAll(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "delete_me")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveAll(ctx, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("expected dir to not exist")
	}
}

func TestStat_ExistingPath(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	dir := t.TempDir()
	exists, err := e.Stat(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected path to exist")
	}
}

func TestStat_NonExistingPath(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	exists, err := e.Stat(ctx, filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected path to not exist")
	}
}

func TestEnsureDir_Exists(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	dir := t.TempDir()
	if err := e.EnsureDir(ctx, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureDir_NotExists(t *testing.T) {
	e := &Executor{Timeout: 5 * time.Second}
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "new_dir", "nested")
	if err := e.EnsureDir(ctx, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		t.Fatalf("expected dir to exist: %v", err)
	}
}
