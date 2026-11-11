package commands

import (
	"fmt"
	"net/http"

	"sigardapanel/internal/cli"
)

// DB is the CLI group for database management
func (a *App) DB(args []string) int {
	if len(args) == 0 {
		a.printDBHelp()
		return exitOK
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
		return exitOK
	default:
		return a.unknownSubcommand("db", args[0], a.printDBHelp)
	}
}

func (a *App) dbCreate(args []string) int {
	flags := a.newFlags("db create", "--site ID --name NAME [--type mysql|postgres] [--charset C] [--collation C]")
	siteID := flags.Int64("site", 0, "site ID (required)")
	name := flags.String("name", "", "database name (required)")
	dbType := flags.String("type", "mysql", "database type: mysql|postgres")
	charset := flags.String("charset", "", "character set (default utf8mb4)")
	collation := flags.String("collation", "", "collation")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *siteID <= 0 || *name == "" {
		fmt.Fprintln(a.stderr, "--site and --name required")
		return exitUsage
	}

	body := map[string]string{"name": *name, "type": *dbType}
	if *charset != "" {
		body["charset"] = *charset
	}
	if *collation != "" {
		body["collation"] = *collation
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/sites/%d/databases", *siteID), body, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		DatabaseID int64 `json:"database_id"`
		JobID      int64 `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "database queued (id=%d, job_id=%d)\n", data.DatabaseID, data.JobID)
	return exitOK
}

func (a *App) dbList(args []string) int {
	flags := a.newFlags("db list", "[--site ID] [-o FORMAT]")
	siteID := flags.Int64("site", 0, "only databases of this site (default: all you can access)")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}

	path := "/api/v1/databases"
	if *siteID > 0 {
		path = fmt.Sprintf("/api/v1/sites/%d/databases", *siteID)
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, path, nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Databases []map[string]any `json:"databases"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Databases == nil {
		data.Databases = []map[string]any{}
	}

	return a.render(mode, data.Databases, func() {
		rows := make([][]string, 0, len(data.Databases))
		for _, db := range data.Databases {
			rows = append(rows, []string{
				cli.Cell(db["id"]),
				cli.Cell(db["name"]),
				cli.Cell(db["type"]),
				cli.Cell(db["site_id"]),
				cli.Cell(db["charset"]),
				cli.Cell(db["size_bytes"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "TYPE", "SITE_ID", "CHARSET", "SIZE_BYTES"}, rows)
	})
}

func (a *App) dbUpdate(args []string) int {
	flags := a.newFlags("db update", "<id> [--charset C] [--collation C]")
	idFlag := flags.Int64("id", 0, "database ID (alternative to the positional argument)")
	charset := flags.String("charset", "", "character set (e.g. utf8mb4)")
	collation := flags.String("collation", "", "collation (e.g. utf8mb4_unicode_ci)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, *idFlag, "database")
	if !ok {
		return exitUsage
	}
	if *charset == "" && *collation == "" {
		fmt.Fprintln(a.stderr, "at least one of --charset or --collation is required (databases cannot be renamed)")
		return exitUsage
	}

	body := make(map[string]any)
	if *charset != "" {
		body["charset"] = *charset
	}
	if *collation != "" {
		body["collation"] = *collation
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodPatch, fmt.Sprintf("/api/v1/databases/%d", id), body, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "database updated (id=%d)\n", id)
	return exitOK
}

func (a *App) dbDelete(args []string) int {
	flags := a.newFlags("db delete", "<id> [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "database")
	if !ok {
		return exitUsage
	}
	if !*yes && !a.confirm(fmt.Sprintf("Delete database %d? This cannot be undone.", id)) {
		return exitError
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/databases/%d", id), nil, http.StatusAccepted, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "database %d deleted (cleanup job queued)\n", id)
	return exitOK
}

// dbUser is the CLI group for database user management
func (a *App) dbUser(args []string) int {
	if len(args) == 0 {
		a.printDBUserHelp()
		return exitOK
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
		return exitOK
	default:
		return a.unknownSubcommand("db user", args[0], a.printDBUserHelp)
	}
}

func (a *App) dbUserCreate(args []string) int {
	flags := a.newFlags("db user create", "--database ID --username U [--password P] [--host H] [--privileges P]")
	dbID := flags.Int64("database", 0, "database ID (required)")
	username := flags.String("username", "", "username (required)")
	password := flags.String("password", "", "password (prompted if empty)")
	host := flags.String("host", "", "host pattern (default localhost)")
	privileges := flags.String("privileges", "", "privileges (default ALL)")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *dbID <= 0 || *username == "" {
		fmt.Fprintln(a.stderr, "--database and --username required")
		return exitUsage
	}

	pw := *password
	if pw == "" {
		var err error
		pw, err = cli.PromptPassword("password", a.stdin, a.stderr)
		if err != nil {
			fmt.Fprintln(a.stderr, "prompt error:", err)
			return exitError
		}
	}
	if len(pw) < 8 {
		fmt.Fprintln(a.stderr, "password must be at least 8 chars")
		return exitUsage
	}

	body := map[string]string{"username": *username, "password": pw}
	if *host != "" {
		body["host_pattern"] = *host
	}
	if *privileges != "" {
		body["privileges"] = *privileges
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/databases/%d/users", *dbID), body, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
		JobID    int64  `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "db user queued (user_id=%d, username=%s, job_id=%d)\n", data.UserID, data.Username, data.JobID)
	return exitOK
}

func (a *App) dbUserList(args []string) int {
	flags := a.newFlags("db user list", "--database ID [-o FORMAT]")
	dbID := flags.Int64("database", 0, "database ID (required)")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *dbID <= 0 {
		fmt.Fprintln(a.stderr, "--database is required")
		return exitUsage
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/databases/%d/users", *dbID), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Users []map[string]any `json:"users"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Users == nil {
		data.Users = []map[string]any{}
	}
	return a.render(mode, data.Users, func() {
		rows := make([][]string, 0, len(data.Users))
		for _, u := range data.Users {
			rows = append(rows, []string{
				cli.Cell(u["id"]),
				cli.Cell(u["username"]),
				cli.Cell(u["host_pattern"]),
				cli.Cell(u["privileges"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "USERNAME", "HOST", "PRIVILEGES"}, rows)
	})
}

func (a *App) dbUserDelete(args []string) int {
	flags := a.newFlags("db user delete", "<database-id> <user-id>")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if len(pos) != 2 {
		fmt.Fprintln(a.stderr, "usage: sigardapanel db user delete <database-id> <user-id>")
		return exitUsage
	}
	dbID, ok1 := a.parseIDArg(pos[:1], 0, "database")
	userID, ok2 := a.parseIDArg(pos[1:], 0, "db user")
	if !ok1 || !ok2 {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/databases/%d/users/%d", dbID, userID), nil, http.StatusAccepted, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "db user %d deleted (cleanup job queued)\n", userID)
	return exitOK
}

// dbUserRotatePassword asks the API to generate a new random password for a
// database user; the API does not accept a caller-chosen password here.
func (a *App) dbUserRotatePassword(args []string) int {
	flags := a.newFlags("db user rotate-password", "--database ID --user ID [--yes]")
	dbID := flags.Int64("database", 0, "database ID (required)")
	userID := flags.Int64("user", 0, "database user ID (required)")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *dbID <= 0 || *userID <= 0 {
		fmt.Fprintln(a.stderr, "--database and --user are required")
		fmt.Fprintln(a.stderr, "usage: sigardapanel db user rotate-password --database DB_ID --user USER_ID [--yes]")
		return exitUsage
	}
	if !*yes && !a.confirm(fmt.Sprintf("Rotate password for DB user %d on database %d? Applications using it will break until updated.", *userID, *dbID)) {
		return exitError
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/databases/%d/users/%d/rotate-password", *dbID, *userID), nil, http.StatusOK, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		Username string `json:"username"`
		Password string `json:"password"`
		JobID    int64  `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "password rotated for %s (user %d, database %d)\n", data.Username, *userID, *dbID)
	if data.Password != "" {
		fmt.Fprintf(a.stdout, "new password: %s\n", data.Password)
	}
	return exitOK
}

func (a *App) printDBHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel db <subcommand> [flags]

Subcommands:
  create --site ID --name NAME [--type mysql|postgres] [--charset C] [--collation C]
                        create a database (queues a job)
  list [--site ID] [-o FORMAT]
                        list databases (all you can access, or one site's)
  update <id> [--charset C] [--collation C]
                        change charset/collation
  delete <id> [--yes]   delete a database (asks first)
  user ...              manage database users (see: sigardapanel db user help)
`)
}

func (a *App) printDBUserHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel db user <subcommand> [flags]

Subcommands:
  create --database ID --username U [--password P] [--host H] [--privileges P]
                        create a database user (password prompted if omitted)
  list --database ID [-o FORMAT]
                        list users of a database
  rotate-password --database ID --user ID [--yes]
                        generate a new random password and print it
  delete <database-id> <user-id>
                        delete a database user
`)
}
