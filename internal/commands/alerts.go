package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"strconv"

	"sigardapanel/internal/cli"
)

// Alerts is the CLI group for alert history.
func (a *App) Alerts(args []string) int {
	if len(args) == 0 {
		a.printAlertsHelp()
		return 0
	}
	switch args[0] {
	case "list", "ls":
		return a.alertList(args[1:])
	case "help", "--help", "-h":
		a.printAlertsHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown alert subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) alertList(args []string) int {
	flags := flag.NewFlagSet("alert list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	output := flags.String("output", "", "output format (table|json)")
	limit := flags.Int("limit", 100, "maximum number of alerts")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := "/api/v1/alerts"
	if *limit > 0 {
		path += fmt.Sprintf("?limit=%d", *limit)
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
		Alerts []map[string]any `json:"alerts"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Alerts)
		return 0
	}

	rows := make([][]string, 0, len(data.Alerts))
	for _, al := range data.Alerts {
		chID := fmt.Sprintf("%v", al["channel_id"])
		if v, ok := al["channel_id"].(float64); ok {
			chID = strconv.FormatInt(int64(v), 10)
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", al["id"]),
			chID,
			fmt.Sprintf("%v", al["severity"]),
			fmt.Sprintf("%v", al["subject"]),
			fmt.Sprintf("%v", al["status"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "CHANNEL_ID", "SEVERITY", "SUBJECT", "STATUS"}, rows)
	return 0
}

func (a *App) printAlertsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel alerts <subcommand>

Subcommands:
  list [--limit N] [--output json]  list alert history
`)
}
