package commands

import (
	"encoding/json"
	"flag"
	"fmt"

	"sigardapanel/internal/cli"
)

// Channels is the CLI group for alert channel management.
func (a *App) Channels(args []string) int {
	if len(args) == 0 {
		a.printChannelsHelp()
		return 0
	}
	switch args[0] {
	case "add":
		return a.channelAdd(args[1:])
	case "list", "ls":
		return a.channelList(args[1:])
	case "remove", "rm":
		return a.channelRemove(args[1:])
	case "help", "--help", "-h":
		a.printChannelsHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown channel subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) channelAdd(args []string) int {
	flags := flag.NewFlagSet("channel add", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	name := flags.String("name", "", "channel name")
	chType := flags.String("type", "", "channel type (telegram|discord|email|webhook)")
	config := flags.String("config", "", "channel config JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *name == "" || *chType == "" || *config == "" {
		fmt.Fprintln(a.stderr, "name, type, and config are required")
		fmt.Fprintln(a.stderr, "usage: channel add --name <name> --type <type> --config '{...}'")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{
		"name":   *name,
		"type":   *chType,
		"config": *config,
	}

	resp, status, err := client.Post("/api/v1/alert-channels", body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 201 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "channel created (id=%d)\n", data.ID)
	return 0
}

func (a *App) channelList(args []string) int {
	flags := flag.NewFlagSet("channel list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	output := flags.String("output", "", "output format (table|json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Get("/api/v1/alert-channels")
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Channels []map[string]any `json:"channels"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Channels)
		return 0
	}

	rows := make([][]string, 0, len(data.Channels))
	for _, ch := range data.Channels {
		rows = append(rows, []string{
			fmt.Sprintf("%v", ch["id"]),
			fmt.Sprintf("%v", ch["name"]),
			fmt.Sprintf("%v", ch["type"]),
			fmt.Sprintf("%v", ch["enabled"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "NAME", "TYPE", "ENABLED"}, rows)
	return 0
}

func (a *App) channelRemove(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: channel remove <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Delete("/api/v1/alert-channels/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "channel %s deleted\n", id)
	return 0
}

func (a *App) printChannelsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel channels <subcommand>

Subcommands:
  add --name <name> --type <type> --config '{...}'
                        create a new alert channel
  list [--output json]  list all alert channels
  remove <id>           remove an alert channel
`)
}
