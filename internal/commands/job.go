package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"sigardapanel/internal/cli"
)

// Polling intervals; tests shorten them.
var (
	jobWatchInterval  = 2 * time.Second
	jobLogsInterval   = 1 * time.Second
	jobFollowDeadline = 10 * time.Minute
)

// Job is the CLI group for job management
func (a *App) Job(args []string) int {
	if len(args) == 0 {
		a.printJobHelp()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return a.jobList(args[1:])
	case "get", "show":
		return a.jobGet(args[1:])
	case "watch":
		return a.jobWatch(args[1:])
	case "cancel":
		return a.jobCancel(args[1:])
	case "logs":
		return a.jobLogs(args[1:])
	case "help", "--help", "-h":
		a.printJobHelp()
		return exitOK
	default:
		return a.unknownSubcommand("job", args[0], a.printJobHelp)
	}
}

func (a *App) jobList(args []string) int {
	flags := a.newFlags("job list", "[--status S] [--type T] [--server ID] [--site ID] [--limit N] [--offset N] [-o FORMAT]")
	statusFilter := flags.String("status", "", "filter by status (queued|running|succeeded|failed|canceled)")
	typeFilter := flags.String("type", "", "filter by type (e.g. site.create, ssl.renew, backup.create)")
	serverID := flags.Int64("server", 0, "filter by server ID")
	siteID := flags.Int64("site", 0, "filter by site ID")
	limit := flags.Int("limit", 0, "maximum number of jobs (API default 50)")
	offset := flags.Int("offset", 0, "skip this many jobs")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}

	q := url.Values{}
	if *statusFilter != "" {
		q.Set("status", *statusFilter)
	}
	if *typeFilter != "" {
		q.Set("type", *typeFilter)
	}
	if *serverID > 0 {
		q.Set("server_id", strconv.FormatInt(*serverID, 10))
	}
	if *siteID > 0 {
		q.Set("site_id", strconv.FormatInt(*siteID, 10))
	}
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	if *offset > 0 {
		q.Set("offset", strconv.Itoa(*offset))
	}
	path := "/api/v1/jobs"
	if len(q) > 0 {
		path += "?" + q.Encode()
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
		Jobs []map[string]any `json:"jobs"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Jobs == nil {
		data.Jobs = []map[string]any{}
	}

	return a.render(mode, data.Jobs, func() {
		rows := make([][]string, 0, len(data.Jobs))
		for _, j := range data.Jobs {
			rows = append(rows, []string{
				cli.Cell(j["id"]),
				cli.Cell(j["type"]),
				cli.Cell(j["status"]),
				cli.Cell(j["site_id"]),
				fmt.Sprintf("%s/%s", cli.Cell(j["attempts"]), cli.Cell(j["max_attempts"])),
				cli.Cell(j["created_at"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "TYPE", "STATUS", "SITE", "ATTEMPTS", "CREATED"}, rows)
	})
}

func (a *App) jobGet(args []string) int {
	flags := a.newFlags("job get", "<id> [-o FORMAT]")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, 0, "job")
	if !ok {
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/jobs/%d", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var job map[string]any
	if !a.decode(resp, &job) {
		return exitError
	}
	return a.render(mode, job, func() {
		a.printKV(job, "id", "type", "status", "description", "server_id", "site_id", "attempts", "max_attempts",
			"error", "created_at", "started_at", "completed_at", "updated_at")
	})
}

func (a *App) jobWatch(args []string) int {
	flags := a.newFlags("job watch", "<id> [--timeout DURATION]")
	idFlag := flags.Int64("id", 0, "job ID (alternative to the positional argument)")
	timeoutF := flags.Duration("timeout", 5*time.Minute, "give up after this long (exit 1)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "job")
	if !ok {
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}

	path := fmt.Sprintf("/api/v1/jobs/%d", id)
	deadline := time.Now().Add(*timeoutF)
	var lastStatus string
	for {
		resp, status, err := client.Get(path)
		switch {
		case err != nil:
			fmt.Fprintf(a.stderr, "poll: %v\n", err)
		case status == http.StatusNotFound || status == http.StatusUnauthorized || status == http.StatusForbidden:
			return a.printAPIErrorFor(client, resp, status)
		case status != http.StatusOK:
			fmt.Fprintf(a.stderr, "poll: HTTP %d\n", status)
		default:
			var data struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			a.decode(resp, &data)
			if data.Status != lastStatus {
				fmt.Fprintf(a.stdout, "status: %s\n", data.Status)
				lastStatus = data.Status
			}
			switch data.Status {
			case "succeeded":
				return exitOK
			case "failed", "canceled":
				if data.Error != "" {
					fmt.Fprintf(a.stderr, "job %d %s: %s\n", id, data.Status, data.Error)
				}
				return exitError
			}
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(a.stderr, "watch timeout (%s); job still %s\n", *timeoutF, lastStatus)
			return exitError
		}
		time.Sleep(jobWatchInterval)
	}
}

func (a *App) jobLogs(args []string) int {
	flags := a.newFlags("job logs", "<id> [--follow] [--since N]")
	idFlag := flags.Int64("id", 0, "job ID (alternative to the positional argument)")
	follow := flags.Bool("follow", false, "keep polling for new lines until the job finishes (max 10 min)")
	flags.BoolVar(follow, "f", false, "shorthand for --follow")
	since := flags.Int("since", 0, "only lines after line number N")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "job")
	if !ok {
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}

	currentSince := *since
	fetch := func() bool {
		resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/jobs/%d/logs?since=%d", id, currentSince), nil, http.StatusOK)
		if !ok {
			return false
		}
		var data struct {
			Logs []struct {
				LineNumber int    `json:"line_number"`
				Message    string `json:"message"`
				Stream     string `json:"stream"`
			} `json:"logs"`
		}
		if !a.decode(resp, &data) {
			return false
		}
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
		return true
	}

	if !fetch() {
		return exitError
	}
	if !*follow {
		return exitOK
	}

	fmt.Fprintln(a.stderr, "(following until the job finishes, Ctrl+C to stop)")
	deadline := time.Now().Add(jobFollowDeadline)
	for time.Now().Before(deadline) {
		time.Sleep(jobLogsInterval)
		fetch()
		// Stop once the job is finished (one last fetch above caught the tail).
		if resp, status, err := client.Get(fmt.Sprintf("/api/v1/jobs/%d", id)); err == nil && status == http.StatusOK {
			var j struct {
				Status string `json:"status"`
			}
			a.decode(resp, &j)
			if j.Status == "succeeded" || j.Status == "failed" || j.Status == "canceled" {
				fetch()
				return exitOK
			}
		}
	}
	fmt.Fprintln(a.stderr, "follow timeout (10 min)")
	return exitOK
}

func (a *App) jobCancel(args []string) int {
	flags := a.newFlags("job cancel", "<id>")
	idFlag := flags.Int64("id", 0, "job ID (alternative to the positional argument)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "job")
	if !ok {
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/cancel", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "job %d canceled\n", id)
	return exitOK
}

func (a *App) printJobHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel job <subcommand> [flags]

Subcommands:
  list [--status S] [--type T] [--server ID] [--site ID] [--limit N] [--offset N]
       [-o table|json|yaml]
                        list jobs you can see
  get <id> [-o FORMAT]  show one job, including its error (alias: show)
  watch <id> [--timeout 5m]
                        poll until the job finishes; exit 0 if it succeeded,
                        1 if it failed, was canceled or timed out
  cancel <id>           cancel a queued or running job (409 if already finished)
  logs <id> [--follow] [--since N]
                        print job log lines; --follow polls until the job ends

Commands that take <id> also accept --id ID. There is no retry endpoint: re-run
the original command (e.g. site deploy, ssl renew) to queue a new job.
`)
}
