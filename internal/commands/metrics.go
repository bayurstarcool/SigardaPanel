package commands

import (
	"fmt"
	"net/http"
	"net/url"

	"sigardapanel/internal/cli"
)

func (a *App) Metrics(args []string) int {
	if len(args) == 0 {
		a.printMetricsHelp()
		return exitOK
	}
	switch args[0] {
	case "latest":
		return a.metricsLatest(args[1:], "latest", "/metrics/latest")
	case "gpu":
		return a.metricsLatest(args[1:], "gpu", "/gpu")
	case "list":
		return a.metricsList(args[1:])
	case "help", "--help", "-h":
		a.printMetricsHelp()
		return exitOK
	default:
		return a.unknownSubcommand("metrics", args[0], a.printMetricsHelp)
	}
}

// serverIDFlag resolves --server or a positional server ID.
func (a *App) serverIDFlag(pos []string, flagVal int64) (int64, bool) {
	if flagVal == 0 && len(pos) == 1 {
		if id, ok := a.parseIDArg(pos, 0, "server"); ok {
			return id, true
		}
		return 0, false
	}
	if flagVal <= 0 || len(pos) > 0 {
		fmt.Fprintln(a.stderr, "--server required")
		return 0, false
	}
	return flagVal, true
}

func (a *App) metricsLatest(args []string, name, suffix string) int {
	flags := a.newFlags("metrics "+name, "--server ID [-o FORMAT]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.serverIDFlag(pos, *serverID)
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/servers/%d%s", id, suffix), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data map[string]any
	if !a.decode(resp, &data) {
		return exitError
	}
	return a.render(mode, data, func() {
		a.printKV(data, "server_id", "collected_at", "cpu_percent", "load_1", "load_5", "load_15",
			"mem_used", "mem_total", "disk_used", "disk_total", "gpu_detected", "gpu_name")
	})
}

func (a *App) metricsList(args []string) int {
	flags := a.newFlags("metrics list", "--server ID [--range 1h|24h|7d] [-o FORMAT]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	rangeStr := flags.String("range", "1h", "time range, e.g. 1h, 24h, 7d (max 720 points)")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.serverIDFlag(pos, *serverID)
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/servers/%d/metrics?range=%s", id, url.QueryEscape(*rangeStr)), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Metrics []map[string]any `json:"metrics"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Metrics == nil {
		data.Metrics = []map[string]any{}
	}
	return a.render(mode, data.Metrics, func() {
		rows := make([][]string, 0, len(data.Metrics))
		for _, m := range data.Metrics {
			rows = append(rows, []string{
				cli.Cell(m["collected_at"]),
				pct(m["cpu_percent"]),
				ratio(m["mem_used"], m["mem_total"]),
				ratio(m["disk_used"], m["disk_total"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"COLLECTED", "CPU%", "MEM%", "DISK%"}, rows)
	})
}

func pct(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.1f", f)
	}
	return "-"
}

func ratio(used, total any) string {
	u, ok1 := used.(float64)
	t, ok2 := total.(float64)
	if !ok1 || !ok2 || t == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", u/t*100)
}

func (a *App) printMetricsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel metrics <subcommand> [flags]

Subcommands:
  latest --server ID [-o FORMAT]
                        latest metrics sample of a server (404 if none yet)
  list --server ID [--range 1h] [-o FORMAT]
                        samples over a time range (1h, 24h, 7d; max 720 points)
  gpu --server ID [-o FORMAT]
                        latest GPU info of a server
`)
}
