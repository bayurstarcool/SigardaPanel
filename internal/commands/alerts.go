package commands

import (
	"fmt"
	"net/http"

	"sigardapanel/internal/cli"
)

// Alerts is the CLI group for alert history.
func (a *App) Alerts(args []string) int {
	if len(args) == 0 {
		a.printAlertsHelp()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return a.alertList(args[1:])
	case "help", "--help", "-h":
		a.printAlertsHelp()
		return exitOK
	default:
		return a.unknownSubcommand("alerts", args[0], a.printAlertsHelp)
	}
}

func (a *App) alertList(args []string) int {
	flags := a.newFlags("alerts list", "[--limit N] [-o FORMAT]")
	limit := flags.Int("limit", 100, "maximum number of alerts")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}

	path := "/api/v1/alerts"
	if *limit > 0 {
		path += fmt.Sprintf("?limit=%d", *limit)
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
		Alerts []map[string]any `json:"alerts"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Alerts == nil {
		data.Alerts = []map[string]any{}
	}
	return a.render(mode, data.Alerts, func() {
		rows := make([][]string, 0, len(data.Alerts))
		for _, al := range data.Alerts {
			rows = append(rows, []string{
				cli.Cell(al["id"]),
				cli.Cell(al["channel_id"]),
				cli.Cell(al["severity"]),
				cli.Cell(al["subject"]),
				cli.Cell(al["status"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "CHANNEL_ID", "SEVERITY", "SUBJECT", "STATUS"}, rows)
	})
}

func (a *App) printAlertsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel alerts <subcommand> [flags]

Subcommands:
  list [--limit N] [-o FORMAT]
                        alert history sent through alert channels (admin)
`)
}
