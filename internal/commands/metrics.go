package commands

import (
	"encoding/json"
	"flag"
	"fmt"

	"sigardapanel/internal/cli"
)

func (a *App) Metrics(args []string) int {
	if len(args) == 0 {
		a.printMetricsHelp()
		return 0
	}
	switch args[0] {
	case "latest":
		return a.metricsLatest(args[1:])
	case "list":
		return a.metricsList(args[1:])
	case "gpu":
		return a.metricsGPU(args[1:])
	case "help", "--help", "-h":
		a.printMetricsHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown metrics subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) metricsLatest(args []string) int {
	flags := flag.NewFlagSet("metrics latest", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/servers/%d/metrics/latest", *serverID)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	if err := cli.PrintJSON(a.stdout, json.RawMessage(resp.Data)); err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	return 0
}

func (a *App) metricsList(args []string) int {
	flags := flag.NewFlagSet("metrics list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	rangeStr := flags.String("range", "1h", "time range (e.g. 1h, 24h, 168h)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/servers/%d/metrics?range=%s", *serverID, *rangeStr)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	if err := cli.PrintJSON(a.stdout, json.RawMessage(resp.Data)); err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	return 0
}

func (a *App) metricsGPU(args []string) int {
	flags := flag.NewFlagSet("metrics gpu", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/servers/%d/gpu", *serverID)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	if err := cli.PrintJSON(a.stdout, json.RawMessage(resp.Data)); err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	return 0
}

func (a *App) printMetricsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel metrics <subcommand>

Subcommands:
  latest --server ID   show latest metrics for a server
  list --server ID [--range 1h]
                       list metrics over time range
  gpu --server ID      show latest GPU info for a server
`)
}
