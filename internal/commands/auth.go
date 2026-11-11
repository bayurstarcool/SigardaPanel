package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/cli"
	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

const defaultAdminUsername = "admin"

func dbPath() string {
	return config.DBPath()
}

func tokenPathDisplay() string {
	path, _ := cli.TokenPath()
	return path
}

func (a *App) context() context.Context {
	return context.Background()
}

// Init bootstraps an admin user locally via direct DB access (no API).
func (a *App) Init(args []string) int {
	flags := a.newFlags("init", "[--user NAME] [--password P] [--email E] [--role super_admin|admin] [--must-change-password]")
	username := flags.String("user", os.Getenv("SIGARDAPANEL_ADMIN_USER"), "admin username (env SIGARDAPANEL_ADMIN_USER, default admin)")
	password := flags.String("password", os.Getenv("SIGARDAPANEL_ADMIN_PASSWORD"), "admin password, min 8 chars (env SIGARDAPANEL_ADMIN_PASSWORD; prompted if empty)")
	email := flags.String("email", "", "admin email (default <user>@localhost)")
	role := flags.String("role", "", "role: super_admin or admin (default super_admin if none exists yet, else admin)")
	mustChange := flags.Bool("must-change-password", false, "the password is temporary: the account must choose its own (web Profile or 'sigardapanel passwd') before using the API")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}

	if *username == "" {
		*username = defaultAdminUsername
	}
	if *role != "" && *role != auth.RoleSuperAdmin && *role != auth.RoleAdmin {
		fmt.Fprintf(a.stderr, "invalid --role %q: must be super_admin or admin\n", *role)
		return exitUsage
	}

	if *password == "" {
		pw, err := cli.PromptPassword("admin password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to read password: %v\n", err)
			return exitError
		}
		*password = pw

		pw2, err := cli.PromptPassword("confirm password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "failed to read password: %v\n", err)
			return exitError
		}
		if pw != pw2 {
			fmt.Fprintln(a.stderr, "passwords do not match")
			return exitError
		}
	}

	if *email == "" {
		*email = *username + "@localhost"
	}

	if len(*password) < 8 {
		fmt.Fprintln(a.stderr, "password must be at least 8 characters")
		return exitError
	}

	database, err := db.Open(dbPath())
	if err != nil {
		fmt.Fprintf(a.stderr, "open database: %v\n", err)
		return exitError
	}
	defer database.Close()

	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		fmt.Fprintf(a.stderr, "run migrations: %v\n", err)
		return exitError
	}

	s := store.New(database)

	existing, _ := s.Users.GetByUsername(a.context(), *username)
	if existing != nil {
		fmt.Fprintf(a.stderr, "user %q already exists\n", *username)
		return exitError
	}

	if *role == "" {
		// The first account must be able to do everything (see all sites,
		// purge audit logs, manage the DB server), like the install wizard's.
		var supers int
		_ = database.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", auth.RoleSuperAdmin).Scan(&supers)
		if supers == 0 {
			*role = auth.RoleSuperAdmin
		} else {
			*role = auth.RoleAdmin
		}
	}

	// PasswordHash holds the plaintext here because Create hashes it.
	user := &store.User{
		Username:           *username,
		Email:              *email,
		Role:               *role,
		PasswordHash:       *password,
		MustChangePassword: *mustChange,
	}
	if err := s.Users.Create(a.context(), user); err != nil {
		fmt.Fprintf(a.stderr, "create user: %v\n", err)
		return exitError
	}

	fmt.Fprintf(a.stdout, "%s user %q created (id=%d)\n", user.Role, user.Username, user.ID)
	fmt.Fprintf(a.stdout, "next: sigardapanel login --api-url %s --user %s\n", cli.DefaultAPIURL, user.Username)
	return exitOK
}

// cliHTTPClient is used for login/logout calls so an unreachable API cannot
// hang the CLI forever.
var cliHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Login authenticates against the API and saves the API URL and session
// token under the config dir.
func (a *App) Login(args []string) int {
	flags := a.newFlags("login", "[--api-url URL] [--user NAME] [--password P] [--totp CODE | --recovery-code CODE]")
	apiURL := flags.String("api-url", "", "API base URL (default $SIGARDAPANEL_API_URL, then the saved URL, then "+cli.DefaultAPIURL+")")
	username := flags.String("user", os.Getenv("SIGARDAPANEL_USER"), "username (env SIGARDAPANEL_USER; prompted if empty)")
	password := flags.String("password", os.Getenv("SIGARDAPANEL_PASSWORD"), "password (env SIGARDAPANEL_PASSWORD; prompted if empty)")
	totp := flags.String("totp", "", "two-factor code (prompted when the account requires one)")
	recovery := flags.String("recovery-code", "", "one-time 2FA recovery code instead of --totp")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}

	if *apiURL == "" {
		*apiURL = cli.ResolveAPIURL()
	}
	*apiURL = strings.TrimRight(*apiURL, "/")

	if *username == "" {
		u, err := cli.Prompt("username", a.stdin, a.stdout)
		if err != nil {
			fmt.Fprintf(a.stderr, "read username: %v\n", err)
			return exitError
		}
		*username = u
	}

	if *password == "" {
		pw, err := cli.PromptPassword("password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "read password: %v\n", err)
			return exitError
		}
		*password = pw
	}

	body := map[string]string{"username": *username, "password": *password}
	if *totp != "" {
		body["totp_code"] = *totp
	}
	if *recovery != "" {
		body["recovery_code"] = *recovery
	}

	status, result, apiErr, err := a.postLogin(*apiURL, body)
	if err != nil {
		fmt.Fprintf(a.stderr, "login request: %v\n", err)
		return exitError
	}
	if status == http.StatusUnauthorized && apiErr != nil && apiErr.Code == "2fa_required" {
		code, err := cli.Prompt("two-factor code", a.stdin, a.stderr)
		if err != nil || code == "" {
			fmt.Fprintln(a.stderr, "two-factor authentication code required (use --totp or --recovery-code)")
			return exitError
		}
		body["totp_code"] = code
		status, result, apiErr, err = a.postLogin(*apiURL, body)
		if err != nil {
			fmt.Fprintf(a.stderr, "login request: %v\n", err)
			return exitError
		}
	}
	if status != http.StatusOK {
		msg := http.StatusText(status)
		if apiErr != nil && apiErr.Message != "" {
			msg = apiErr.Message
		}
		if status == http.StatusUnauthorized && (apiErr == nil || apiErr.Code == "" || apiErr.Code == "unauthorized") {
			msg = "invalid credentials"
		}
		fmt.Fprintf(a.stderr, "login failed (%d): %s\n", status, msg)
		return exitError
	}

	if result.Data.Token == "" {
		fmt.Fprintln(a.stderr, "empty token in response")
		return exitError
	}

	if err := cli.SaveConfig(&cli.Config{APIURL: *apiURL}); err != nil {
		fmt.Fprintf(a.stderr, "save config: %v\n", err)
		return exitError
	}
	if err := cli.SaveToken(result.Data.Token); err != nil {
		fmt.Fprintf(a.stderr, "save token: %v\n", err)
		return exitError
	}

	fmt.Fprintf(a.stdout, "logged in as %s", *username)
	if result.Data.User.Role != "" {
		fmt.Fprintf(a.stdout, " (%s)", result.Data.User.Role)
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprintf(a.stdout, "token saved to %s\n", tokenPathDisplay())
	if result.Data.User.PasswordChangeRequired {
		fmt.Fprintln(a.stderr, "password change required: this account still uses a password someone else chose (installer or administrator).")
		fmt.Fprintln(a.stderr, "Until you choose your own, the API refuses every other command (and this account's API keys).")
		fmt.Fprintln(a.stderr, "Change it now with:  sigardapanel passwd   (or in the web panel under Profile)")
	}
	if os.Getenv("SIGARDAPANEL_TOKEN") != "" {
		fmt.Fprintln(a.stderr, "warning: SIGARDAPANEL_TOKEN is set and overrides the saved token; unset it to use this login")
	}
	return exitOK
}

type loginResult struct {
	Data struct {
		Token string `json:"token"`
		User  struct {
			Username               string `json:"username"`
			Role                   string `json:"role"`
			PasswordChangeRequired bool   `json:"password_change_required"`
		} `json:"user"`
	} `json:"data"`
	Error *cli.APIError `json:"error"`
}

func (a *App) postLogin(apiURL string, body map[string]string) (int, *loginResult, *cli.APIError, error) {
	b, _ := json.Marshal(body)
	resp, err := cliHTTPClient.Post(apiURL+"/api/v1/auth/login", "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("cannot reach API at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	var result loginResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil && resp.StatusCode == http.StatusOK {
		return resp.StatusCode, nil, nil, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, &result, result.Error, nil
}

// Logout revokes the saved session server-side (best-effort) and always
// deletes the local token file. SIGARDAPANEL_TOKEN is never revoked.
func (a *App) Logout(args []string) int {
	flags := a.newFlags("logout", "")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}

	token, err := cli.LoadToken()
	token = strings.TrimSpace(token)
	if err != nil || token == "" {
		fmt.Fprintln(a.stdout, "not logged in")
		if os.Getenv("SIGARDAPANEL_TOKEN") != "" {
			fmt.Fprintln(a.stderr, "note: SIGARDAPANEL_TOKEN is set; logout does not revoke it (use: sigardapanel apikey revoke <id>)")
		}
		return exitOK
	}

	// Revoke at the API that issued the token (the URL saved by login).
	apiURL := cli.ResolveAPIURL()
	if cfg, err := cli.LoadConfig(); err == nil && cfg != nil && cfg.APIURL != "" {
		apiURL = strings.TrimRight(cfg.APIURL, "/")
	}
	revoked := false
	if req, err := http.NewRequest(http.MethodPost, apiURL+"/api/v1/auth/logout", nil); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := cliHTTPClient.Do(req); err == nil {
			revoked = resp.StatusCode == http.StatusOK
			resp.Body.Close()
		}
	}

	// Always delete local token regardless of API result.
	if err := cli.DeleteToken(); err != nil {
		fmt.Fprintf(a.stderr, "delete token: %v\n", err)
		return exitError
	}

	fmt.Fprintln(a.stdout, "logged out")
	if !revoked {
		fmt.Fprintf(a.stderr, "warning: could not revoke the session at %s; the local token was deleted anyway\n", apiURL)
	}
	return exitOK
}

// Passwd changes the password of the logged-in account through
// PUT /api/v1/auth/profile. It is the CLI way out of password_change_required.
func (a *App) Passwd(args []string) int {
	flags := a.newFlags("passwd", "[--current P] [--new P]")
	current := flags.String("current", "", "current password (prompted if empty)")
	newPass := flags.String("new", "", "new password, min 8 chars (prompted twice if empty)")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if client.Token == "" {
		fmt.Fprintln(a.stderr, "not logged in: run sigardapanel login first")
		return exitError
	}
	if client.TokenSource == "env" {
		fmt.Fprintln(a.stderr, "note: SIGARDAPANEL_TOKEN is set; changing a password needs a login session (unset it and run sigardapanel login)")
	}

	if *current == "" {
		pw, err := cli.PromptPassword("current password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "read password: %v\n", err)
			return exitError
		}
		*current = pw
	}
	if *newPass == "" {
		pw, err := cli.PromptPassword("new password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "read password: %v\n", err)
			return exitError
		}
		pw2, err := cli.PromptPassword("confirm new password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintf(a.stderr, "read password: %v\n", err)
			return exitError
		}
		if pw != pw2 {
			fmt.Fprintln(a.stderr, "passwords do not match")
			return exitError
		}
		*newPass = pw
	}
	if *current == "" {
		fmt.Fprintln(a.stderr, "current password is required")
		return exitUsage
	}
	if len(*newPass) < 8 {
		fmt.Fprintln(a.stderr, "new password must be at least 8 characters")
		return exitUsage
	}
	if *newPass == *current {
		fmt.Fprintln(a.stderr, "new password must differ from the current one")
		return exitUsage
	}

	if _, ok := a.call(client, http.MethodPut, "/api/v1/auth/profile",
		map[string]string{"old_password": *current, "new_password": *newPass}, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintln(a.stdout, "password changed")
	return exitOK
}

// Whoami shows who the current token authenticates as.
func (a *App) Whoami(args []string) int {
	flags := a.newFlags("whoami", "[-o FORMAT]")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if client.Token == "" {
		fmt.Fprintln(a.stderr, "not logged in: run sigardapanel login, or set SIGARDAPANEL_TOKEN")
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/auth/me", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var me map[string]any
	if !a.decode(resp, &me) {
		return exitError
	}
	me["api_url"] = client.BaseURL
	me["token_source"] = map[string]string{"env": "SIGARDAPANEL_TOKEN", "file": tokenPathDisplay()}[client.TokenSource]
	return a.render(mode, me, func() {
		a.printKV(me, "id", "username", "email", "role", "api_url", "token_source")
	})
}

// Version prints the CLI version and, when reachable, the API's version.
func (a *App) Version(args []string) int {
	flags := a.newFlags("version", "[--client] [-o FORMAT]")
	clientOnly := flags.Bool("client", false, "print only the CLI version (no network)")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *clientOnly {
		fmt.Fprintln(a.stdout, buildinfo.Version)
		return exitOK
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	out := map[string]any{"client": buildinfo.Version}
	client, _ := a.apiClient()
	client.HTTP = &http.Client{Timeout: 3 * time.Second}
	out["api_url"] = client.BaseURL
	resp, status, err := client.Get("/api/v1/version")
	switch {
	case err != nil:
		out["server_error"] = err.Error()
	case status != http.StatusOK:
		out["server_error"] = fmt.Sprintf("HTTP %d", status)
	default:
		var v struct {
			Version string `json:"version"`
		}
		a.decode(resp, &v)
		out["server"] = v.Version
	}
	return a.render(mode, out, func() {
		fmt.Fprintf(a.stdout, "client: %s\n", buildinfo.Version)
		if s, ok := out["server"].(string); ok {
			fmt.Fprintf(a.stdout, "server: %s (%s)\n", s, client.BaseURL)
			if s != buildinfo.Version {
				fmt.Fprintln(a.stdout, "note: client and server versions differ")
			}
		} else {
			fmt.Fprintf(a.stdout, "server: unavailable (%s: %v)\n", client.BaseURL, out["server_error"])
		}
	})
}
