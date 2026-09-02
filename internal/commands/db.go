package commands

import (
	"encoding/json"
	"flag"
	"fmt"

	"sigardapanel/internal/cli"
)

// DB is the CLI group for database management
func (a *App) DB(args []string) int {
	if len(args) == 0 {
		a.printDBHelp()
		return 0
	}
	switch args[0] {
	case "create":
		return a.dbCreate(args[1:])
	case "list", "ls":
		return a.dbList(args[1:])
	case "update", "edit":
		return a.dbUpdate(args[1:])
	case "delete", "rm":
		return a.dbDelete(args[1:])
	case "user":
		return a.dbUser(args[1:])
	case "help", "--help", "-h":
		a.printDBHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown db subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) dbCreate(args []string) int {
	flags := flag.NewFlagSet("db create", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	name := flags.String("name", "", "database name")
	dbType := flags.String("type", "mysql", "database type (mysql|postgres)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *siteID <= 0 || *name == "" {
		fmt.Fprintln(a.stderr, "--site and --name required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/sites/%d/databases", *siteID)
	body := map[string]string{"name": *name, "type": *dbType}

	resp, status, err := client.Post(path, body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		DatabaseID int64 `json:"database_id"`
		JobID      int64 `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "database queued (id=%d, job_id=%d)\n", data.DatabaseID, data.JobID)
	return 0
}

func (a *App) dbList(args []string) int {
	flags := flag.NewFlagSet("db list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	output := flags.String("output", "", "output format (table|json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *siteID <= 0 {
		fmt.Fprintln(a.stderr, "--site is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/sites/%d/databases", *siteID)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Databases []map[string]any `json:"databases"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		if err := cli.PrintJSON(a.stdout, data.Databases); err != nil {
			fmt.Fprintf(a.stderr, "encode: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(data.Databases))
	for _, db := range data.Databases {
		rows = append(rows, []string{
			fmt.Sprintf("%v", db["id"]),
			fmt.Sprintf("%v", db["name"]),
			fmt.Sprintf("%v", db["type"]),
			fmt.Sprintf("%v", db["charset"]),
			fmt.Sprintf("%v", db["size_bytes"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "NAME", "TYPE", "CHARSET", "SIZE"}, rows)
	return 0
}

func (a *App) dbUpdate(args []string) int {
	flags := flag.NewFlagSet("db update", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "database ID (required)")
	name := flags.String("name", "", "database name")
	charset := flags.String("charset", "", "database charset (utf8, utf8mb4, etc.)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		fmt.Fprintln(a.stderr, "usage: db update --id <id> [--name N] [--charset C]")
		return 2
	}

	if *name == "" && *charset == "" {
		fmt.Fprintln(a.stderr, "at least one field is required")
		fmt.Fprintln(a.stderr, "usage: db update --id <id> [--name N] [--charset C]")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := make(map[string]any)
	if *name != "" {
		body["name"] = *name
	}
	if *charset != "" {
		body["charset"] = *charset
	}

	resp, status, err := client.Patch(fmt.Sprintf("/api/v1/databases/%d", *id), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "database updated (id=%d)\n", *id)
	return 0
}

func (a *App) dbDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: db delete <id>")
		return 2
	}
	id := args[0]

	// Confirmation prompt
	confirmed, err := cli.Confirm(fmt.Sprintf("Delete database %s? This cannot be undone.", id), false, a.stdin, a.stdout)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(a.stdout, "cancelled")
		return 0
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Delete("/api/v1/databases/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "database %s deleted (cleanup job queued)\n", id)
	return 0
}

// dbUser is the CLI group for database user management
func (a *App) dbUser(args []string) int {
	if len(args) == 0 {
		a.printDBUserHelp()
		return 0
	}
	switch args[0] {
	case "create":
		return a.dbUserCreate(args[1:])
	case "list", "ls":
		return a.dbUserList(args[1:])
	case "rotate-password":
		return a.dbUserRotatePassword(args[1:])
	case "delete", "rm":
		return a.dbUserDelete(args[1:])
	case "help", "--help", "-h":
		a.printDBUserHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown db user subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) dbUserCreate(args []string) int {
	flags := flag.NewFlagSet("db user create", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dbID := flags.Int64("database", 0, "database ID")
	username := flags.String("username", "", "username")
	password := flags.String("password", "", "password (prompt if empty)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *dbID <= 0 || *username == "" {
		fmt.Fprintln(a.stderr, "--database and --username required")
		return 2
	}

	pw := *password
	if pw == "" {
		var err error
		pw, err = cli.PromptPassword("password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "prompt error:", err)
			return 1
		}
	}
	if len(pw) < 8 {
		fmt.Fprintln(a.stderr, "password must be at least 8 chars")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/databases/%d/users", *dbID)
	body := map[string]string{
		"username": *username,
		"password": pw,
	}

	resp, status, err := client.Post(path, body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
		JobID    int64  `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "db user queued (user_id=%d, username=%s, job_id=%d)\n", data.UserID, data.Username, data.JobID)
	return 0
}

func (a *App) dbUserList(args []string) int {
	flags := flag.NewFlagSet("db user list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dbID := flags.Int64("database", 0, "database ID")
	output := flags.String("output", "", "output format (table|json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *dbID <= 0 {
		fmt.Fprintln(a.stderr, "--database is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/databases/%d/users", *dbID)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Users []map[string]any `json:"users"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		if err := cli.PrintJSON(a.stdout, data.Users); err != nil {
			fmt.Fprintf(a.stderr, "encode: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(data.Users))
	for _, u := range data.Users {
		rows = append(rows, []string{
			fmt.Sprintf("%v", u["id"]),
			fmt.Sprintf("%v", u["username"]),
			fmt.Sprintf("%v", u["host_pattern"]),
			fmt.Sprintf("%v", u["privileges"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "USERNAME", "HOST", "PRIVILEGES"}, rows)
	return 0
}

func (a *App) dbUserDelete(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(a.stderr, "usage: db user delete <database_id> <user_id>")
		return 2
	}

	dbID := args[0]
	userID := args[1]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/databases/%s/users/%s", dbID, userID)
	resp, status, err := client.Delete(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "db user %s deleted (cleanup job queued)\n", userID)
	return 0
}

func (a *App) dbUserRotatePassword(args []string) int {
	flags := flag.NewFlagSet("db user rotate-password", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dbID := flags.Int64("database", 0, "database ID (required)")
	userID := flags.Int64("user", 0, "user ID (required)")
	password := flags.String("password", "", "new password (prompt if empty)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *dbID <= 0 || *userID <= 0 {
		fmt.Fprintln(a.stderr, "--database and --user are required")
		fmt.Fprintln(a.stderr, "usage: db user rotate-password --database DB_ID --user USER_ID [--password P]")
		return 2
	}

	pw := *password
	if pw == "" {
		var err error
		pw, err = cli.PromptPassword("new password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "prompt error:", err)
			return 1
		}
	}
	if len(pw) < 8 {
		fmt.Fprintln(a.stderr, "password must be at least 8 chars")
		return 2
	}

	// Confirm
	confirmed, err := cli.Confirm(fmt.Sprintf("Rotate password for DB user %d on database %d?", *userID, *dbID), false, a.stdin, a.stdout)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(a.stdout, "cancelled")
		return 0
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/databases/%d/users/%d/rotate-password", *dbID, *userID)
	resp, status, err := client.Post(path, map[string]string{"password": pw})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 && status != 202 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "password rotated for user %d on database %d\n", *userID, *dbID)
	return 0
}

func (a *App) printDBHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel db <subcommand>

Subcommands:
  create --site ID --name X [--type mysql|postgres]
                        create database
  list --site ID        list databases for a site
  update --id ID [--name N] [--charset C]
                        update database settings
  delete <id>           delete database
  user create|list|rotate-password|delete
                        manage database users
`)
}

func (a *App) printDBUserHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel db user <subcommand>

Subcommands:
  create --database ID --username U [--password P]
                        create db user (prompts password if not provided)
  list --database ID    list db users for a database
  rotate-password --database DB_ID --user USER_ID [--password P]
                        rotate db user password
  delete <database_id> <user_id>
                        delete db user
`)
}
