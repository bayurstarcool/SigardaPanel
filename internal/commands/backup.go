package commands

import (
	"encoding/json"
	"flag"
	"fmt"

	"sigardapanel/internal/cli"
)

func (a *App) Backup(args []string) int {
	if len(args) == 0 {
		a.printBackupHelp()
		return 0
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
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown backup subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) backupCreate(args []string) int {
	flags := flag.NewFlagSet("backup create", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
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

	path := fmt.Sprintf("/api/v1/sites/%d/backups", *siteID)
	resp, status, err := client.Post(path, nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		BackupID int64 `json:"backup_id"`
		JobID    int64 `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "backup queued (backup_id=%d, job_id=%d)\n", data.BackupID, data.JobID)
	return 0
}

func (a *App) backupList(args []string) int {
	flags := flag.NewFlagSet("backup list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "filter by site ID")
	output := flags.String("output", "", "output format (table|json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := "/api/v1/backups"
	if *siteID > 0 {
		path += fmt.Sprintf("?site_id=%d", *siteID)
	}

	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Backups []map[string]any `json:"backups"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		if err := cli.PrintJSON(a.stdout, data.Backups); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(data.Backups))
	for _, b := range data.Backups {
		rows = append(rows, []string{
			fmt.Sprintf("%v", b["id"]),
			fmt.Sprintf("%v", b["site_id"]),
			fmt.Sprintf("%v", b["status"]),
			fmt.Sprintf("%v", b["created_at"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "SITE_ID", "STATUS", "CREATED"}, rows)
	return 0
}

func (a *App) backupRestore(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: backup restore <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post("/api/v1/backups/"+id+"/restore", nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		BackupID int64 `json:"backup_id"`
		JobID    int64 `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "restore job queued (backup_id=%d, job_id=%d)\n", data.BackupID, data.JobID)
	return 0
}

func (a *App) backupDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: backup delete <id>")
		return 2
	}
	id := args[0]

	// Confirmation prompt
	confirmed, err := cli.Confirm(fmt.Sprintf("Delete backup %s?", id), false, a.stdin, a.stdout)
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

	resp, status, err := client.Delete("/api/v1/backups/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "backup %s deleted\n", id)
	return 0
}

func (a *App) printBackupHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel backup <subcommand>

Subcommands:
  create --site ID      create a new backup
  list [--site ID]      list backups
  delete <id>           delete a backup
  restore <id>          restore from backup
`)
}
