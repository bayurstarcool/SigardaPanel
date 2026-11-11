package commands

import (
	"fmt"
	"net/http"

	"sigardapanel/internal/cli"
)

func (a *App) Backup(args []string) int {
	if len(args) == 0 {
		a.printBackupHelp()
		return exitOK
	}
	switch args[0] {
	case "create":
		return a.backupCreate(args[1:])
	case "list", "ls":
		return a.backupList(args[1:])
	case "delete", "rm":
		return a.backupDelete(args[1:])
	case "restore":
		return a.backupRestore(args[1:])
	case "help", "--help", "-h":
		a.printBackupHelp()
		return exitOK
	default:
		return a.unknownSubcommand("backup", args[0], a.printBackupHelp)
	}
}

func (a *App) backupCreate(args []string) int {
	flags := a.newFlags("backup create", "--site ID [--provider ID]")
	siteID := flags.Int64("site", 0, "site ID (required)")
	provider := flags.Int64("provider", 0, "storage provider ID (default: the site's provider, else local)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if len(pos) == 1 && *siteID == 0 {
		if id, ok := a.parseIDArg(pos, 0, "site"); ok {
			*siteID = id
		}
	}
	if *siteID <= 0 {
		fmt.Fprintln(a.stderr, "--site is required")
		return exitUsage
	}

	var body any
	if *provider > 0 {
		body = map[string]any{"storage_provider_id": *provider}
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/backups", *siteID), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return exitError
	}
	if status == http.StatusConflict && resp != nil && resp.Error == nil {
		// Dedupe answer: {"data":{"status":"already_running","message":...}}
		var d struct {
			Message string `json:"message"`
		}
		a.decode(resp, &d)
		fmt.Fprintf(a.stderr, "backup not queued: %s\n", d.Message)
		return exitError
	}
	if status != http.StatusAccepted {
		return a.printAPIErrorFor(client, resp, status)
	}

	var data struct {
		BackupID int64  `json:"backup_id"`
		JobID    int64  `json:"job_id"`
		Storage  string `json:"storage"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "backup queued (backup_id=%d, job_id=%d", data.BackupID, data.JobID)
	if data.Storage != "" {
		fmt.Fprintf(a.stdout, ", storage=%s", data.Storage)
	}
	fmt.Fprintln(a.stdout, ")")
	return exitOK
}

func (a *App) backupList(args []string) int {
	flags := a.newFlags("backup list", "[--site ID] [-o FORMAT]")
	siteID := flags.Int64("site", 0, "only backups of this site")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}

	path := "/api/v1/backups"
	if *siteID > 0 {
		path += fmt.Sprintf("?site_id=%d", *siteID)
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
		Backups []map[string]any `json:"backups"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Backups == nil {
		data.Backups = []map[string]any{}
	}

	return a.render(mode, data.Backups, func() {
		rows := make([][]string, 0, len(data.Backups))
		for _, b := range data.Backups {
			domain := "-"
			if site, ok := b["site"].(map[string]any); ok {
				domain = cli.Cell(site["domain"])
			}
			rows = append(rows, []string{
				cli.Cell(b["id"]),
				cli.Cell(b["site_id"]),
				domain,
				cli.Cell(b["backup_type"]),
				cli.Cell(b["storage_type"]),
				cli.Cell(b["status"]),
				cli.Cell(b["size_bytes"]),
				cli.Cell(b["created_at"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "SITE_ID", "DOMAIN", "TYPE", "STORAGE", "STATUS", "SIZE_BYTES", "CREATED"}, rows)
	})
}

func (a *App) backupRestore(args []string) int {
	flags := a.newFlags("backup restore", "<id> [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "backup")
	if !ok {
		return exitUsage
	}
	if !*yes && !a.confirm(fmt.Sprintf("Restore backup %d? This overwrites the site's current files", id)) {
		return exitError
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/backups/%d/restore", id), nil, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		BackupID int64 `json:"backup_id"`
		JobID    int64 `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "restore job queued (backup_id=%d, job_id=%d)\n", data.BackupID, data.JobID)
	return exitOK
}

func (a *App) backupDelete(args []string) int {
	flags := a.newFlags("backup delete", "<id> [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "backup")
	if !ok {
		return exitUsage
	}
	if !*yes && !a.confirm(fmt.Sprintf("Delete backup %d?", id)) {
		return exitError
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/backups/%d", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "backup %d deleted\n", id)
	return exitOK
}

// confirm asks a yes/no question (default no). It prints "cancelled" and
// returns false when the answer is not yes or stdin is closed.
func (a *App) confirm(question string) bool {
	ok, err := cli.Confirm(question, false, a.stdin, a.stdout)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return false
	}
	if !ok {
		fmt.Fprintln(a.stderr, "cancelled (pass --yes to skip this prompt)")
	}
	return ok
}

func (a *App) printBackupHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel backup <subcommand> [flags]

Subcommands:
  create --site ID [--provider ID]
                        queue a file backup of a site (409 if one is running)
  list [--site ID] [-o FORMAT]
                        list backups you can see (newest first, max 100)
  restore <id> [--yes]  restore a backup over the site's files (asks first)
  delete <id> [--yes]   delete a backup and its archive (asks first)

Prompts default to "no"; without a terminal pass --yes, otherwise the command
is cancelled with exit code 1.
`)
}
