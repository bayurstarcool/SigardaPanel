package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/cli"
	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

const (
	defaultAdminUsername = "admin"
	defaultAdminRole     = auth.RoleAdmin
)

func dbPath() string {
	return config.DBPath()
}

func defaultAPIURL() string {
	return "http://localhost:7700"
}

func tokenPathDisplay() string {
	path, _ := cli.TokenPath()
	return path
}

func (a *App) context() context.Context {
	return context.Background()
}

// Init bootstraps the admin user locally via direct DB access.
func (a *App) Init(args []string) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(a.stderr)

	username := flags.String("user", os.Getenv("SIGARDAPANEL_ADMIN_USER"), "admin username")
	password := flags.String("password", os.Getenv("SIGARDAPANEL_ADMIN_PASSWORD"), "admin password (prompt if empty)")
	email := flags.String("email", "", "admin email")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *username == "" {
		*username = defaultAdminUsername
	}

	if *password == "" {
		pw, err := cli.PromptPassword("admin password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to read password: %v\n", err)
			return 1
		}
		*password = pw

		pw2, err := cli.PromptPassword("confirm password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to read password: %v\n", err)
			return 1
		}
		if pw != pw2 {
			fmt.Fprintln(a.stderr, "passwords do not match")
			return 1
		}
	}

	if *email == "" {
		*email = *username + "@localhost"
	}

	if len(*password) < 8 {
		fmt.Fprintln(a.stderr, "password must be at least 8 characters")
		return 1
	}

	// Open database.
	database, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(a.stderr, "open database: %v\n", err)
		return 1
	}
	defer database.Close()

	// Run migrations if not yet applied.
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		fmt.Fprintf(a.stderr, "run migrations: %v\n", err)
		return 1
	}

	s := store.New(database)

	// Check for duplicate user.
	existing, _ := s.Users.GetByUsername(a.context(), *username)
	if existing != nil {
		fmt.Fprintf(a.stderr, "user %q already exists\n", *username)
		return 1
	}

	// Create user. PasswordHash holds the plaintext here because Create hashes it.
	user := &store.User{
		Username:     *username,
		Email:        *email,
		Role:         auth.RoleAdmin,
		PasswordHash: *password,
	}

	if err := s.Users.Create(a.context(), user); err != nil {
		fmt.Fprintf(a.stderr, "create user: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.stdout, "admin user %q created (id=%d)\n", user.Username, user.ID)
	fmt.Fprintf(a.stdout, "next: sigardapanel login --api-url %s\n", defaultAPIURL())
	return 0
}

// Login authenticates against the API and saves the token locally.
func (a *App) Login(args []string) int {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(a.stderr)

	apiURL := flags.String("api-url", os.Getenv("SIGARDAPANEL_API_URL"), "API base URL")
	username := flags.String("user", os.Getenv("SIGARDAPANEL_USER"), "username")
	password := flags.String("password", os.Getenv("SIGARDAPANEL_PASSWORD"), "password (prompt if empty)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *apiURL == "" {
		*apiURL = defaultAPIURL()
	}

	if *username == "" {
		u, err := cli.Prompt("username", a.stdin, a.stdout)
		if err != nil {
			fmt.Fprintf(a.stderr, "read username: %v\n", err)
			return 1
		}
		*username = u
	}

	if *password == "" {
		pw, err := cli.PromptPassword("password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "read password: %v\n", err)
			return 1
		}
		*password = pw
	}

	body, _ := json.Marshal(map[string]string{
		"username": *username,
		"password": *password,
	})
	resp, err := http.Post(*apiURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(a.stderr, "login request: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(a.stderr, "invalid credentials")
		return 1
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(a.stderr, "login failed: %s\n", resp.Status)
		return 1
	}

	var result struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(a.stderr, "decode response: %v\n", err)
		return 1
	}

	if result.Data.Token == "" {
		fmt.Fprintln(a.stderr, "empty token in response")
		return 1
	}

	if err := cli.SaveConfig(&cli.Config{APIURL: *apiURL}); err != nil {
		fmt.Fprintf(a.stderr, "save config: %v\n", err)
		return 1
	}
	if err := cli.SaveToken(result.Data.Token); err != nil {
		fmt.Fprintf(a.stderr, "save token: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.stdout, "logged in as %s\n", *username)
	fmt.Fprintf(a.stdout, "token saved to %s\n", tokenPathDisplay())
	return 0
}

// Logout revokes the token server-side (best-effort) and always deletes the local token.
func (a *App) Logout(args []string) int {
	flags := flag.NewFlagSet("logout", flag.ContinueOnError)
	flags.SetOutput(a.stderr)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	cfg, err := cli.LoadConfig()
	if err != nil {
		fmt.Fprintln(a.stdout, "not logged in")
		return 0
	}

	token, err := cli.LoadToken()
	if err != nil {
		fmt.Fprintln(a.stdout, "not logged in")
		return 0
	}

	// Best-effort server-side revoke.
	req, _ := http.NewRequest("POST", cfg.APIURL+"/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}

	// Always delete local token regardless of API result.
	if err := cli.DeleteToken(); err != nil {
		fmt.Fprintf(a.stderr, "delete token: %v\n", err)
		return 1
	}

	fmt.Fprintln(a.stdout, "logged out")
	return 0
}
