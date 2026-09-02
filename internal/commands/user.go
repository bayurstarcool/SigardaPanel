package commands

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/cli"
	"sigardapanel/internal/config"

	_ "modernc.org/sqlite"
)

// User is the CLI group for user management.
func (a *App) User(args []string) int {
	if len(args) == 0 {
		a.printUserHelp()
		return 0
	}
	switch args[0] {
	case "create":
		return a.userCreate(args[1:])
	case "list", "ls":
		return a.userList(args[1:])
	case "reset-password":
		return a.userResetPassword(args[1:])
	case "delete", "rm":
		return a.userDelete(args[1:])
	case "help", "--help", "-h":
		a.printUserHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown user subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) userCreate(args []string) int {
	flags := flag.NewFlagSet("user create", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	username := flags.String("username", "", "username")
	email := flags.String("email", "", "email address")
	password := flags.String("password", "", "password (prompt if empty)")
	role := flags.String("role", "user", "role: super_admin, admin, user")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *username == "" {
		name, err := cli.Prompt("Username", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "input error:", err)
			return 2
		}
		*username = name
	}
	if *username == "" {
		fmt.Fprintln(a.stderr, "username is required")
		return 2
	}

	if *password == "" {
		pw, err := cli.PromptPassword("Password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "input error:", err)
			return 2
		}
		*password = pw
	}
	if *password == "" {
		fmt.Fprintln(a.stderr, "password is required")
		return 2
	}

	switch *role {
	case "super_admin", "admin", "user":
	default:
		fmt.Fprintf(a.stderr, "invalid role %q: must be super_admin, admin, or user\n", *role)
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{
		"username": *username,
		"email":    *email,
		"password": *password,
		"role":     *role,
	}

	resp, status, err := client.Post("/api/v1/users", body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 201 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.stdout, "user created (id=%d)\n", data.ID)
	fmt.Fprintf(a.stdout, "username: %s\n", data.Username)
	fmt.Fprintf(a.stdout, "email: %s\n", data.Email)
	fmt.Fprintf(a.stdout, "role: %s\n", data.Role)
	return 0
}

func (a *App) userList(args []string) int {
	flags := flag.NewFlagSet("user list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	output := flags.String("output", "", "output format (table|json)")
	direct := flags.Bool("direct", false, "bypass API, read straight from local DB")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *direct {
		return a.userListDirect(*output)
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Get("/api/v1/users")
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Users []struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			Role      string `json:"role"`
			CreatedAt string `json:"created_at"`
		} `json:"users"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}

	var users []userRow
	for _, u := range data.Users {
		users = append(users, userRow{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			CreatedAt: u.CreatedAt,
		})
	}

	return a.printUsers(*output, users, data.Count)
}

// userListDirect lists users directly from the panel SQLite DB, bypassing the API.
func (a *App) userListDirect(output string) int {
	dbPath := resolveDBPath()
	if dbPath == "" {
		fmt.Fprintln(a.stderr, "database not found")
		return 1
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(a.stderr, "open database: %v\n", err)
		return 1
	}
	defer db.Close()

	rows, err := db.Query(
		"SELECT id, username, email, role, COALESCE(created_at,'') FROM users ORDER BY id")
	if err != nil {
		fmt.Fprintf(a.stderr, "query failed: %v\n", err)
		return 1
	}
	defer rows.Close()

	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			fmt.Fprintf(a.stderr, "scan failed: %v\n", err)
			return 1
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintf(a.stderr, "rows error: %v\n", err)
		return 1
	}

	return a.printUsers(output, users, len(users))
}

type userRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func (a *App) printUsers(output string, users []userRow, count int) int {
	mode := cli.GetOutputMode()
	if output == "json" {
		mode = cli.OutputJSON
	} else if output == "table" {
		mode = cli.OutputTable
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, users)
		return 0
	}

	rows := make([][]string, 0, len(users))
	for _, u := range users {
		rows = append(rows, []string{
			fmt.Sprintf("%d", u.ID),
			u.Username,
			u.Email,
			u.Role,
			u.CreatedAt,
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "USERNAME", "EMAIL", "ROLE", "CREATED"}, rows)
	fmt.Fprintf(a.stdout, "total: %d\n", count)
	return 0
}

func (a *App) userResetPassword(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: user reset-password <username|id> [--password <pass>] [--direct]")
		return 2
	}

	target := args[0]
	password := ""
	direct := false
	for i := 1; i < len(args); i++ {
		if args[i] == "--password" && i+1 < len(args) {
			password = args[i+1]
			i++
		} else if args[i] == "--direct" {
			direct = true
		} else if args[i] == "--help" || args[i] == "-h" {
			a.printUserHelp()
			return 0
		}
	}

	if password == "" {
		pw, err := cli.PromptPassword("New password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "input error:", err)
			return 2
		}
		password = pw
	}
	if password == "" {
		fmt.Fprintln(a.stderr, "password is required")
		return 2
	}

	// Direct DB mode — bypasses API, works even when API is down
	if direct {
		return a.userResetPasswordDirect(target, password)
	}

	// API mode — resolve username/email to id first, then PATCH
	id := target
	if _, err := strconv.ParseInt(target, 10, 64); err != nil {
		resolved, err := a.resolveUserID(target)
		if err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		id = resolved
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{"password": password}
	resp, status, err := client.Patch("/api/v1/users/"+id, body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "password updated for user %s\n", target)
	return 0
}

// resolveUserID resolves a username or email to a user id via the API.
func (a *App) resolveUserID(target string) (string, error) {
	client, err := cli.NewClient()
	if err != nil {
		return "", err
	}
	resp, status, err := client.Get("/api/v1/users")
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("list users failed (status %d)", status)
	}

	var data struct {
		Users []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"users"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode users: %v", err)
	}

	lower := strings.ToLower(strings.TrimSpace(target))
	for _, u := range data.Users {
		if strings.ToLower(u.Username) == lower || strings.ToLower(u.Email) == lower {
			return strconv.FormatInt(u.ID, 10), nil
		}
	}
	return "", fmt.Errorf("user %q not found", target)
}

// resolveDBPath returns the panel SQLite DB path.
func resolveDBPath() string {
	dbPath := config.DBPath()
	if dbPath == "" || dbPath == "sigardapanel.db" {
		for _, p := range []string{
			"/var/lib/sigardapanel/sigardapanel.db",
			"/opt/sigardapanel/sigardapanel.db",
			"sigardapanel.db",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return ""
	}
	return dbPath
}

// userResetPasswordDirect writes directly to the panel SQLite DB, bypassing the API.
// target may be a numeric id, a username, or an email.
func (a *App) userResetPasswordDirect(target, password string) int {
	dbPath := resolveDBPath()
	if dbPath == "" {
		fmt.Fprintln(a.stderr, "database not found")
		return 1
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(a.stderr, "open database: %v\n", err)
		return 1
	}
	defer db.Close()

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(a.stderr, "hash password: %v\n", err)
		return 1
	}

	var result sql.Result
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		// numeric id
		result, err = db.Exec(
			"UPDATE users SET password_hash = ?, password_changed_at = CURRENT_TIMESTAMP WHERE id = ?",
			hash, id,
		)
		if err != nil {
			fmt.Fprintf(a.stderr, "update failed: %v\n", err)
			return 1
		}
	} else {
		// username or email — resolve row id
		var uid int64
		lower := strings.ToLower(strings.TrimSpace(target))
		err := db.QueryRow(
			"SELECT id FROM users WHERE LOWER(username) = ? OR LOWER(email) = ?",
			lower, lower,
		).Scan(&uid)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Fprintf(a.stderr, "user %q not found\n", target)
			} else {
				fmt.Fprintf(a.stderr, "lookup failed: %v\n", err)
			}
			return 1
		}
		result, err = db.Exec(
			"UPDATE users SET password_hash = ?, password_changed_at = CURRENT_TIMESTAMP WHERE id = ?",
			hash, uid,
		)
		if err != nil {
			fmt.Fprintf(a.stderr, "update failed: %v\n", err)
			return 1
		}
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		fmt.Fprintf(a.stderr, "user %q not found\n", target)
		return 1
	}

	fmt.Fprintf(a.stdout, "password updated for user %s (direct)\n", target)
	return 0
}

func (a *App) userDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: user delete <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Delete("/api/v1/users/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "user %s deleted\n", id)
	return 0
}

func (a *App) printUserHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel user <subcommand>

Subcommands:
  create [--username <user>] [--email <email>] [--password <pass>] [--role <role>]
                        create a new user (role: super_admin, admin, user)
  list [--output json] [--direct]
                        list all users (--direct bypasses API, reads local DB)
  reset-password <username|id> [--password <pass>] [--direct]
                        reset password by username, email, or id
                        (--direct bypasses API, writes straight to DB)
  delete <id>           delete a user

Note: quote passwords containing special characters:
  sigardapanel user reset-password admin --password "#pass#word" --direct
`)
}
