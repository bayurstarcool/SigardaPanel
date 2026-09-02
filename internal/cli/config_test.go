package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir error: %v", err)
	}
	if dir == "" {
		t.Fatal("ConfigDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("ConfigDir not absolute: %s", dir)
	}
}

func TestConfigDirXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir error: %v", err)
	}
	expected := filepath.Join("/tmp/xdg-test", appName)
	if dir != expected {
		t.Fatalf("expected %q, got %q", expected, dir)
	}
}

func TestConfigPath(t *testing.T) {
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath error: %v", err)
	}
	if filepath.Base(path) != "config.json" {
		t.Fatalf("expected base config.json, got %s", filepath.Base(path))
	}
}

func TestTokenPath(t *testing.T) {
	path, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath error: %v", err)
	}
	if filepath.Base(path) != "token" {
		t.Fatalf("expected base token, got %s", filepath.Base(path))
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := &Config{APIURL: "http://localhost:9999"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	path, _ := ConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if runtime.GOOS != "windows" {
		mode := info.Mode().Perm()
		if mode != 0600 {
			t.Fatalf("config file permission expected 0600, got %#o", mode)
		}
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if loaded.APIURL != cfg.APIURL {
		t.Fatalf("expected APIURL %q, got %q", cfg.APIURL, loaded.APIURL)
	}
}

func TestLoadConfigFallbackEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("SIGARDAPANEL_API_URL", "http://env.local:8080")

	// No config file exists — should fall back to env.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIURL != "http://env.local:8080" {
		t.Fatalf("expected APIURL from env, got %q", cfg.APIURL)
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := SaveToken("my-secret-token"); err != nil {
		t.Fatalf("SaveToken error: %v", err)
	}

	path, _ := TokenPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if runtime.GOOS != "windows" {
		mode := info.Mode().Perm()
		if mode != 0600 {
			t.Fatalf("token file permission expected 0600, got %#o", mode)
		}
	}

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken error: %v", err)
	}
	if token != "my-secret-token" {
		t.Fatalf("expected token %q, got %q", "my-secret-token", token)
	}
}

func TestDeleteToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := SaveToken("token-to-delete"); err != nil {
		t.Fatalf("SaveToken error: %v", err)
	}

	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken error: %v", err)
	}

	_, err := LoadToken()
	if !os.IsNotExist(err) {
		t.Fatalf("expected not exist error, got %v", err)
	}
}

func TestDeleteTokenIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Deleting a non-existent token should not error.
	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken error: %v", err)
	}
}
