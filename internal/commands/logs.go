package commands

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"sigardapanel/internal/cli"
)

// Logs is the CLI group for log management
func (a *App) Logs(args []string) int {
	if len(args) == 0 {
		a.printLogsHelp()
		return exitOK
	}
	switch args[0] {
	case "tail":
		return a.logsTail(args[1:])
	case "help", "--help", "-h":
		a.printLogsHelp()
		return exitOK
	default:
		return a.unknownSubcommand("logs", args[0], a.printLogsHelp)
	}
}

var (
	followInterval = 2 * time.Second
	followTimeout  = 10 * time.Minute
)

// errReported marks an error that was already printed to stderr.
var errReported = errors.New("error already reported")

type siteLogEntry struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func (a *App) logsTail(args []string) int {
	flags := a.newFlags("logs tail", "--site ID [--type access|error|php] [--lines N] [--follow] [-o FORMAT]")
	siteID := flags.Int64("site", 0, "site ID (required)")
	logType := flags.String("type", "access", "log type: access|error|php")
	lines := flags.Int("lines", 100, "number of lines, 1-5000")
	follow := flags.Bool("follow", false, "keep printing new lines (poll every 2s, max 10 min)")
	flags.BoolVar(follow, "f", false, "shorthand for --follow")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if *siteID == 0 && len(pos) == 1 {
		if id, ok := a.parseIDArg(pos, 0, "site"); ok {
			*siteID = id
		}
	}
	if *siteID <= 0 {
		fmt.Fprintln(a.stderr, "--site is required")
		return exitUsage
	}
	if *lines < 1 || *lines > 5000 {
		fmt.Fprintln(a.stderr, "--lines must be between 1 and 5000")
		return exitUsage
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	if mode != cli.OutputTable && *follow {
		fmt.Fprintln(a.stderr, "--follow only works with table (plain) output")
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	path := fmt.Sprintf("/api/v1/sites/%d/logs?type=%s&limit=%d", *siteID, *logType, *lines)

	// The API returns the newest *lines* entries, newest first.
	fetch := func() ([]siteLogEntry, map[string]any, error) {
		resp, status, err := client.Get(path)
		if err != nil {
			return nil, nil, err
		}
		if status != http.StatusOK {
			a.printAPIErrorFor(client, resp, status)
			return nil, nil, errReported
		}
		var data struct {
			Logs   []siteLogEntry `json:"logs"`
			Exists bool           `json:"exists"`
			Path   string         `json:"path"`
			Error  string         `json:"error"`
		}
		var raw map[string]any
		a.decode(resp, &data)
		a.decode(resp, &raw)
		if data.Error != "" {
			return nil, raw, fmt.Errorf("%s: %s", data.Path, data.Error)
		}
		return data.Logs, raw, nil
	}

	entries, raw, err := fetch()
	if err != nil {
		if err != errReported {
			fmt.Fprintln(a.stderr, err)
		}
		return exitError
	}
	if mode != cli.OutputTable {
		return a.render(mode, raw, nil)
	}
	if exists, _ := raw["exists"].(bool); !exists {
		fmt.Fprintf(a.stderr, "log file %s does not exist (yet)\n", cli.Cell(raw["path"]))
	}
	for i := len(entries) - 1; i >= 0; i-- {
		fmt.Fprintln(a.stdout, entries[i].Message)
	}
	if !*follow {
		return exitOK
	}

	last := ""
	if len(entries) > 0 {
		last = entries[0].Message
	}
	fmt.Fprintln(a.stderr, "(following, Ctrl+C to stop)")
	ticker := time.NewTicker(followInterval)
	defer ticker.Stop()
	timeout := time.After(followTimeout)
	for {
		select {
		case <-timeout:
			fmt.Fprintln(a.stderr, "follow timeout (10 min)")
			return exitOK
		case <-ticker.C:
			entries, _, err := fetch()
			if err != nil {
				if err != errReported {
					fmt.Fprintln(a.stderr, err)
				}
				continue
			}
			// Print only entries newer than the last one printed.
			n := len(entries)
			for i, e := range entries {
				if last != "" && e.Message == last {
					n = i
					break
				}
			}
			for i := n - 1; i >= 0; i-- {
				fmt.Fprintln(a.stdout, entries[i].Message)
			}
			if len(entries) > 0 {
				last = entries[0].Message
			}
		}
	}
}

func (a *App) printLogsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel logs <subcommand> [flags]

Subcommands:
  tail --site ID [--type access|error|php] [--lines N] [--follow] [-o FORMAT]
                        print the last N lines of a site's nginx access/error log
                        or the PHP-FPM log (oldest first); --follow keeps polling
`)
}
