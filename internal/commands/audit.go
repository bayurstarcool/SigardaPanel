package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"sigardapanel/internal/cli"
)

// Audit is the CLI group for the panel audit log.
func (a *App) Audit(args []string) int {
	if len(args) == 0 {
		a.printAuditHelp()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return a.auditList(args[1:])
	case "purge":
		return a.auditPurge(args[1:])
	case "help", "--help", "-h":
		a.printAuditHelp()
		return exitOK
	default:
		return a.unknownSubcommand("audit", args[0], a.printAuditHelp)
	}
}

func (a *App) auditList(args []string) int {
	flags := a.newFlags("audit list", "[--user ID] [--resource R] [--action A] [--limit N] [--offset N] [-o FORMAT]")
	userID := flags.Int64("user", 0, "only entries by this user ID")
	resource := flags.String("resource", "", "only this resource type (e.g. site, server, user, api_key)")
	action := flags.String("action", "", "only this action (e.g. site_suspend, api_key.create, login)")
	limit := flags.Int("limit", 100, "maximum entries, 1-1000")
	offset := flags.Int("offset", 0, "skip this many entries")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *limit < 1 || *limit > 1000 {
		fmt.Fprintln(a.stderr, "--limit must be between 1 and 1000")
		return exitUsage
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(*limit))
	if *offset > 0 {
		q.Set("offset", strconv.Itoa(*offset))
	}
	if *userID > 0 {
		q.Set("user_id", strconv.FormatInt(*userID, 10))
	}
	if *resource != "" {
		q.Set("resource", *resource)
	}
	if *action != "" {
		q.Set("action", *action)
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/audit-logs?"+q.Encode(), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Entries []map[string]any `json:"entries"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Entries == nil {
		data.Entries = []map[string]any{}
	}
	return a.render(mode, data.Entries, func() {
		rows := make([][]string, 0, len(data.Entries))
		for _, e := range data.Entries {
			details := cli.Cell(e["details"])
			if len(details) > 60 {
				details = details[:57] + "..."
			}
			rows = append(rows, []string{
				cli.Cell(e["id"]),
				cli.Cell(e["created_at"]),
				cli.Cell(e["user_id"]),
				cli.Cell(e["action"]),
				cli.Cell(e["resource"]),
				cli.Cell(e["resource_id"]),
				details,
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "TIME", "USER", "ACTION", "RESOURCE", "RESOURCE_ID", "DETAILS"}, rows)
	})
}

func (a *App) auditPurge(args []string) int {
	flags := a.newFlags("audit purge", "--older-than-days N [--yes]")
	days := flags.Int("older-than-days", 0, "delete entries older than N days, 1-3650 (required)")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *days < 1 || *days > 3650 {
		fmt.Fprintln(a.stderr, "--older-than-days is required (1-3650)")
		return exitUsage
	}
	if !*yes && !a.confirm(fmt.Sprintf("Permanently delete audit entries older than %d days?", *days)) {
		return exitError
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/audit-logs?older_than_days=%d", *days), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Deleted int64 `json:"deleted"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "deleted %d audit entries older than %d days (the purge itself was audited)\n", data.Deleted, *days)
	return exitOK
}

func (a *App) printAuditHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel audit <subcommand> [flags]

Subcommands:
  list [--user ID] [--resource R] [--action A] [--limit N] [--offset N] [-o FORMAT]
                        recent audit entries, newest first (admin)
  purge --older-than-days N [--yes]
                        delete entries older than N days (super_admin, interactive
                        login only; the purge is recorded as a new entry)
`)
}
