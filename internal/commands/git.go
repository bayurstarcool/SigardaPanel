package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"sigardapanel/internal/cli"
)

// Git is the CLI group for Git deploy management
func (a *App) Git(args []string) int {
	if len(args) == 0 {
		a.printGitHelp()
		return 0
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
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown git subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) gitBranches(args []string) int {
	flags := flag.NewFlagSet("git branches", flag.ContinueOnError)
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

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, map[string]any{"branches": data.Branches, "current": data.Current})
		return 0
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
	flags := flag.NewFlagSet("git log", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	lines := flags.Int("lines", 20, "number of commits")
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

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Commits)
		return 0
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
	flags := flag.NewFlagSet("git rollback", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	commit := flags.String("commit", "", "commit hash to rollback to (required)")

	if err := flags.Parse(args); err != nil {
		return 2
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
		Message       string `json:"message"`
		NewHead       string `json:"new_head"`
		BackupBranch  string `json:"backup_branch"`
	}
	json.Unmarshal(resp.Data, &result)

	fmt.Fprintf(a.stdout, "%s\n", result.Message)
	if result.BackupBranch != "" {
		fmt.Fprintf(a.stdout, "  backup branch: %s\n", result.BackupBranch)
	}
	return 0
}

func (a *App) gitCheckout(args []string) int {
	flags := flag.NewFlagSet("git checkout", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	branch := flags.String("branch", "", "branch name (required)")

	if err := flags.Parse(args); err != nil {
		return 2
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
	fmt.Fprint(a.stdout, `Usage: sigardapanel git <subcommand>

Subcommands:
  branches --site ID [--output json]
                        list branches
  log --site ID [--lines N] [--output json]
                        view commit history
  rollback --site ID --commit HASH
                        rollback to a specific commit
  checkout --site ID --branch NAME
                        switch to a branch
`)
}
