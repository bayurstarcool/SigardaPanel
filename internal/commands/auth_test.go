package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/cli"
	"sigardapanel/internal/db"
	"sigardapanel/internal/store"
)

func TestInitSuccess(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Init([]string{
		"--user", "admintest",
		"--password", "supersecret123",
		"--email", "admin@test.local",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "admintest") {
		t.Fatalf("expected username in output, got %q", stdout.String())
	}

	// Verify DB entry.
	database, err := db.Open(tmpDB)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	s := store.New(database)
	user, err := s.Users.GetByUsername(t.Context(), "admintest")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user == nil {
		t.Fatal("user not found in database")
	}
	if user.Email != "admin@test.local" {
		t.Fatalf("expected email admin@test.local, got %s", user.Email)
	}
	if user.Role != "admin" {
		t.Fatalf("expected role admin, got %s", user.Role)
	}
}

func TestInitDuplicateUser(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Init([]string{
		"--user", "dupuser",
		"--password", "supersecret123",
	})
	if code != 0 {
		t.Fatalf("first init expected exit 0, got %d\nstderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Init([]string{
		"--user", "dupuser",
		"--password", "supersecret123",
	})
	if code != 1 {
		t.Fatalf("second init expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("expected duplicate error, got %q", stderr.String())
	}
}

func TestInitShortPassword(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Init([]string{
		"--user", "shortuser",
		"--password", "short",
	})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "at least 8 characters") {
		t.Fatalf("expected short password error, got %q", stderr.String())
	}
}

func TestInitPromptPassword(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	// Simulate interactive password input: password + newline + confirm + newline.
	stdin := strings.NewReader("mypassword123\nmypassword123\n")
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, stdin)

	code := app.Init([]string{
		"--user", "promptuser",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	// Verify user exists.
	database, err := db.Open(tmpDB)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	s := store.New(database)
	user, err := s.Users.GetByUsername(t.Context(), "promptuser")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user == nil {
		t.Fatal("user not found after prompt init")
	}
}

func TestInitPromptPasswordMismatch(t *testing.T) {
	tmpDB := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", tmpDB)

	stdin := strings.NewReader("password123\ndifferent456\n")
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, stdin)

	code := app.Init([]string{
		"--user", "mismatchuser",
	})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "do not match") {
		t.Fatalf("expected mismatch error, got %q", stderr.String())
	}
}

func TestLoginSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/auth/login" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if body["username"] != "testuser" || body["password"] != "testpass123" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"token": "mock-token-abc123",
				"user": map[string]string{
					"username": "testuser",
				},
			},
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Login([]string{
		"--api-url", server.URL,
		"--user", "testuser",
		"--password", "testpass123",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as testuser") {
		t.Fatalf("expected login success message, got %q", stdout.String())
	}

	// Verify token saved.
	token, err := cli.LoadToken()
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token != "mock-token-abc123" {
		t.Fatalf("expected token mock-token-abc123, got %q", token)
	}

	cfg, err := cli.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIURL != server.URL {
		t.Fatalf("expected APIURL %s, got %s", server.URL, cfg.APIURL)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Login([]string{
		"--api-url", server.URL,
		"--user", "baduser",
		"--password", "badpass",
	})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "invalid credentials") {
		t.Fatalf("expected invalid credentials error, got %q", stderr.String())
	}
}

func TestLoginPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["username"] == "promptname" && body["password"] == "promptpass" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"token": "prompt-token",
					"user":  map[string]string{"username": "promptname"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// stdin provides username and password interactively.
	stdin := strings.NewReader("promptname\npromptpass\n")
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, stdin)

	code := app.Login([]string{
		"--api-url", server.URL,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	token, _ := cli.LoadToken()
	if token != "prompt-token" {
		t.Fatalf("expected prompt-token, got %q", token)
	}
}

func TestLogoutSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/logout" && r.Method == http.MethodPost {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer my-token" {
				revoked = true
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]bool{"ok": true}})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	// Pre-populate config and token.
	cli.SaveConfig(&cli.Config{APIURL: server.URL})
	cli.SaveToken("my-token")

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Logout([]string{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged out") {
		t.Fatalf("expected logged out message, got %q", stdout.String())
	}
	if !revoked {
		t.Fatal("expected token to be revoked server-side")
	}

	_, err := cli.LoadToken()
	if !os.IsNotExist(err) {
		t.Fatalf("expected token file removed, got err=%v", err)
	}
}

func TestLogoutNoToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Logout([]string{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "not logged in") {
		t.Fatalf("expected not logged in message, got %q", stdout.String())
	}
}

func TestLogoutAPIDown(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Save config pointing to a non-existent server.
	cli.SaveConfig(&cli.Config{APIURL: "http://127.0.0.1:59999"})
	cli.SaveToken("tok")

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))

	code := app.Logout([]string{})
	if code != 0 {
		t.Fatalf("expected exit 0 even when API down, got %d\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged out") {
		t.Fatalf("expected logged out message, got %q", stdout.String())
	}

	_, err := cli.LoadToken()
	if !os.IsNotExist(err) {
		t.Fatalf("expected token removed even when API down, got err=%v", err)
	}
}
