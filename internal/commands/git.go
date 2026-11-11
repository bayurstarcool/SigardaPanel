package commands

import (
	"encoding/json"
	"fmt"
	"sigardapanel/internal/cli"
)

// Git is the CLI group for Git deploy management
func (a *App) Git(args []string) int {
	if len(args) == 0 {
		a.printGitHelp()
		return exitOK
	}
	switch args[0] {
	case "branches":
		return a.gitBranches(args[1:])
	case "log":
		return a.gitLog(args[1:])
	case "rollback":
		return a.gitRollback(args[1:])
	case "checkout":
		return a.gitCheckout(args[1:])
	case "help", "--help", "-h":
		a.printGitHelp()
		return exitOK
	default:
		return a.unknownSubcommand("git", args[0], a.printGitHelp)
	}
}

func (a *App) gitBranches(args []string) int {
	flags := a.newFlags("git branches", "--site ID [-o FORMAT]")
	siteID := flags.Int64("site", 0, "site ID (required)")
	output := addOutputFlag(flags)

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/sites/%d/git/branches", *siteID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Branches []string `json:"branches"`
		Current  string   `json:"current"`
	}
	json.Unmarshal(resp.Data, &data)

	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	if mode != cli.OutputTable {
		return a.render(mode, map[string]any{"branches": data.Branches, "current": data.Current}, nil)
	}

	fmt.Fprintf(a.stdout, "Current branch: %s\n\n", data.Current)
	for _, b := range data.Branches {
		marker := "  "
		if b == data.Current {
			marker = "* "
		}
		fmt.Fprintf(a.stdout, "%s%s\n", marker, b)
	}
	return 0
}

func (a *App) gitLog(args []string) int {
	flags := a.newFlags("git log", "--site ID [--lines N] [-o FORMAT]")
	siteID := flags.Int64("site", 0, "site ID (required)")
	lines := flags.Int("lines", 20, "number of commits")
	output := addOutputFlag(flags)

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/sites/%d/git/log?lines=%d", *siteID, *lines))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Commits []struct {
			Hash    string `json:"hash"`
			Short   string `json:"short"`
			Message string `json:"message"`
			Author  string `json:"author"`
			Date    string `json:"date"`
		} `json:"commits"`
	}
	json.Unmarshal(resp.Data, &data)

	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	if mode != cli.OutputTable {
		return a.render(mode, data.Commits, nil)
	}

	if len(data.Commits) == 0 {
		fmt.Fprintln(a.stdout, "No commits found.")
		return 0
	}

	rows := make([][]string, 0, len(data.Commits))
	for _, c := range data.Commits {
		date := c.Date
		if len(date) > 10 {
			date = date[:10]
		}
		rows = append(rows, []string{c.Short, c.Message, c.Author, date})
	}
	cli.PrintTable(a.stdout, []string{"COMMIT", "MESSAGE", "AUTHOR", "DATE"}, rows)
	return 0
}

func (a *App) gitRollback(args []string) int {
	flags := a.newFlags("git rollback", "--site ID --commit HASH")
	siteID := flags.Int64("site", 0, "site ID (required)")
	commit := flags.String("commit", "", "commit hash to rollback to (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *siteID <= 0 || *commit == "" {
		fmt.Fprintln(a.stderr, "--site and --commit are required")
		return 2
	}

	confirmed, err := cli.Confirm(fmt.Sprintf("Rollback site %d to commit %s?", *siteID, *commit), false, a.stdin, a.stdout)
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

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/git/rollback", *siteID), map[string]string{"commit": *commit})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var result struct {
		Message      string `json:"message"`
		NewHead      string `json:"new_head"`
		BackupBranch string `json:"backup_branch"`
	}
	json.Unmarshal(resp.Data, &result)

	fmt.Fprintf(a.stdout, "%s\n", result.Message)
	if result.BackupBranch != "" {
		fmt.Fprintf(a.stdout, "  backup branch: %s\n", result.BackupBranch)
	}
	return 0
}

func (a *App) gitCheckout(args []string) int {
	flags := a.newFlags("git checkout", "--site ID --branch NAME")
	siteID := flags.Int64("site", 0, "site ID (required)")
	branch := flags.String("branch", "", "branch name (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *siteID <= 0 || *branch == "" {
		fmt.Fprintln(a.stderr, "--site and --branch are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/git/checkout", *siteID), map[string]string{"branch": *branch})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var result struct {
		Message string `json:"message"`
		Pull    string `json:"pull"`
	}
	json.Unmarshal(resp.Data, &result)

	fmt.Fprintf(a.stdout, "%s\n", result.Message)
	if result.Pull != "" {
		fmt.Fprintf(a.stdout, "  pull: %s\n", result.Pull)
	}
	return 0
}

func (a *App) printGitHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel git <subcommand> [flags]

Git working copy of a site (rollback/checkout: admin).

Subcommands:
  branches --site ID [-o FORMAT]
                        list branches
  log --site ID [--lines N] [-o FORMAT]
                        view commit history
  rollback --site ID --commit HASH
                        rollback to a specific commit
  checkout --site ID --branch NAME
                        switch to a branch
`)
}
