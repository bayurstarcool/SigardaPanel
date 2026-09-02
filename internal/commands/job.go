package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"sigardapanel/internal/cli"
)

// Job is the CLI group for job management
func (a *App) Job(args []string) int {
	if len(args) == 0 {
		a.printJobHelp()
		return 0
	}
	switch args[0] {
	case "list", "ls":
		return a.jobList(args[1:])
	case "watch":
		return a.jobWatch(args[1:])
	case "cancel":
		return a.jobCancel(args[1:])
	case "logs":
		return a.jobLogs(args[1:])
	case "help", "--help", "-h":
		a.printJobHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown job subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) jobList(args []string) int {
	flags := flag.NewFlagSet("job list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	statusFilter := flags.String("status", "", "filter by status")
	typeFilter := flags.String("type", "", "filter by type")
	output := flags.String("output", "", "output format")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := "/api/v1/jobs"
	sep := "?"
	if *statusFilter != "" {
		path += sep + "status=" + *statusFilter
		sep = "&"
	}
	if *typeFilter != "" {
		path += sep + "type=" + *typeFilter
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
		Jobs []map[string]any `json:"jobs"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		if err := cli.PrintJSON(a.stdout, data.Jobs); err != nil {
			fmt.Fprintf(a.stderr, "encode: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(data.Jobs))
	for _, j := range data.Jobs {
		rows = append(rows, []string{
			fmt.Sprintf("%v", j["id"]),
			fmt.Sprintf("%v", j["type"]),
			fmt.Sprintf("%v", j["status"]),
			fmt.Sprintf("%v", j["attempts"]),
			fmt.Sprintf("%v", j["created_at"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "TYPE", "STATUS", "ATTEMPTS", "CREATED"}, rows)
	return 0
}

func (a *App) jobWatch(args []string) int {
	flags := flag.NewFlagSet("job watch", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "job ID (required)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/jobs/%d", *id)
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastStatus string
	for {
		select {
		case <-timeout:
			fmt.Fprintln(a.stderr, "watch timeout (5 min)")
			return 1
		case <-ticker.C:
			resp, status, err := client.Get(path)
			if err != nil {
				fmt.Fprintf(a.stderr, "poll: %v\n", err)
				continue
			}
			if status != 200 {
				fmt.Fprintf(a.stderr, "HTTP %d\n", status)
				continue
			}
			var data struct {
				Status string `json:"status"`
			}
			json.Unmarshal(resp.Data, &data)

			if data.Status != lastStatus {
				fmt.Fprintf(a.stdout, "status: %s\n", data.Status)
				lastStatus = data.Status
			}

			if data.Status == "succeeded" || data.Status == "failed" || data.Status == "canceled" {
				return 0
			}
		}
	}
}

func (a *App) jobLogs(args []string) int {
	flags := flag.NewFlagSet("job logs", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "job ID (required)")
	follow := flags.Bool("follow", false, "follow new lines (poll every 1s)")
	since := flags.Int("since", 0, "start from line N")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	currentSince := *since
	fetch := func() (int, bool) {
		path := fmt.Sprintf("/api/v1/jobs/%d/logs?since=%d", *id, currentSince)
		resp, status, err := client.Get(path)
		if err != nil {
			fmt.Fprintln(a.stderr, err)
			return 0, false
		}
		if status != 200 {
			return 0, false
		}
		var data struct {
			Logs []struct {
				LineNumber int    `json:"line_number"`
				Message    string `json:"message"`
				Stream     string `json:"stream"`
			} `json:"logs"`
			Count int `json:"count"`
		}
		json.Unmarshal(resp.Data, &data)

		for _, l := range data.Logs {
			if l.Stream == "stderr" {
				fmt.Fprintf(a.stdout, "[stderr] %s\n", l.Message)
			} else {
				fmt.Fprintf(a.stdout, "%s\n", l.Message)
			}
			if l.LineNumber > currentSince {
				currentSince = l.LineNumber
			}
		}
		return len(data.Logs), true
	}

	n, ok := fetch()
	if !ok {
		if n == 0 {
			// Could be no logs or error
		}
	}

	if !*follow {
		return 0
	}

	// Follow mode
	fmt.Fprintln(a.stderr, "(following, Ctrl+C to stop)")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute)

	for {
		select {
		case <-timeout:
			fmt.Fprintln(a.stderr, "follow timeout (10 min)")
			return 0
		case <-ticker.C:
			fetch()
		}
	}
}

func (a *App) jobCancel(args []string) int {
	flags := flag.NewFlagSet("job cancel", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "job ID (required)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/jobs/%d/cancel", *id), nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Status string `json:"status"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintf(a.stdout, "job %d cancelled (status: %s)\n", *id, data.Status)
	return 0
}

func (a *App) printJobHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel job <subcommand>

Subcommands:
  list [--status S] [--type T] [--output json]
                        list jobs
  watch --id ID         watch a job until completion (max 5 min)
  cancel --id ID        cancel a running job
  logs --id ID [--follow] [--since N]
                        fetch job log lines
`)
}
