package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"sigardapanel/internal/cli"
)

// Logs is the CLI group for log management
func (a *App) Logs(args []string) int {
	if len(args) == 0 {
		a.printLogsHelp()
		return 0
	}
	switch args[0] {
	case "tail":
		return a.logsTail(args[1:])
	case "help", "--help", "-h":
		a.printLogsHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown logs subcommand: %s\n", args[0])
		return 2
	}
}

var (
	followInterval = 2 * time.Second
	followTimeout  = 10 * time.Minute
)

func (a *App) logsTail(args []string) int {
	flags := flag.NewFlagSet("logs tail", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	logType := flags.String("type", "access", "log type (access|error)")
	follow := flags.Bool("follow", false, "follow log output (poll every 2s)")
	lines := flags.Int("lines", 100, "lines to fetch")
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

	path := fmt.Sprintf("/api/v1/sites/%d/logs?type=%s&lines=%d", *siteID, *logType, *lines)

	fetch := func() error {
		resp, status, err := client.Get(path)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("HTTP %d", status)
		}
		var data struct {
			Message string   `json:"message"`
			Lines   []string `json:"lines"`
		}
		if json.Unmarshal(resp.Data, &data); err == nil {
			for _, line := range data.Lines {
				fmt.Fprintln(a.stdout, line)
			}
		}
		return nil
	}

	if !*follow {
		if err := fetch(); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		return 0
	}

	// Follow mode
	ticker := time.NewTicker(followInterval)
	defer ticker.Stop()
	timeout := time.After(followTimeout)

	fmt.Fprintln(a.stderr, "(following, Ctrl+C to stop)")

	for {
		select {
		case <-timeout:
			fmt.Fprintln(a.stderr, "follow timeout (10 min)")
			return 0
		case <-ticker.C:
			if err := fetch(); err != nil {
				fmt.Fprintln(a.stderr, err)
			}
		}
	}
}

func (a *App) printLogsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel logs <subcommand>

Subcommands:
  tail --site ID [--type access|error] [--follow] [--lines N]
                        tail logs for a site
`)
}
