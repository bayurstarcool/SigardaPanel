package commands

import (
	"encoding/json"
	"fmt"
	"net/http"

	"sigardapanel/internal/cli"
)

// Channels is the CLI group for alert channel management.
func (a *App) Channels(args []string) int {
	if len(args) == 0 {
		a.printChannelsHelp()
		return exitOK
	}
	switch args[0] {
	case "add", "create":
		return a.channelAdd(args[1:])
	case "list", "ls":
		return a.channelList(args[1:])
	case "remove", "rm", "delete":
		return a.channelRemove(args[1:])
	case "help", "--help", "-h":
		a.printChannelsHelp()
		return exitOK
	default:
		return a.unknownSubcommand("channels", args[0], a.printChannelsHelp)
	}
}

func (a *App) channelAdd(args []string) int {
	flags := a.newFlags("channels add", "--name NAME --type telegram|discord|email|webhook --config JSON")
	name := flags.String("name", "", "channel name (required)")
	chType := flags.String("type", "", "channel type: telegram|discord|email|webhook (required)")
	config := flags.String("config", "", "channel config as a JSON object (required), e.g. '{\"bot_token\":\"..\",\"chat_id\":\"..\"}'")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *name == "" || *chType == "" || *config == "" {
		fmt.Fprintln(a.stderr, "name, type, and config are required")
		fmt.Fprintln(a.stderr, "usage: sigardapanel channels add --name <name> --type <type> --config '{...}'")
		return exitUsage
	}
	// The API expects config as a JSON object, not a string containing JSON.
	var cfgObj map[string]any
	if err := json.Unmarshal([]byte(*config), &cfgObj); err != nil || cfgObj == nil {
		fmt.Fprintln(a.stderr, "--config must be a JSON object, e.g. '{\"url\":\"https://example.com/hook\"}'")
		return exitUsage
	}

	body := map[string]any{
		"name":   *name,
		"type":   *chType,
		"config": json.RawMessage(*config),
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, "/api/v1/alert-channels", body, http.StatusCreated)
	if !ok {
		return exitError
	}
	var data struct {
		ID int64 `json:"id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "channel created (id=%d)\n", data.ID)
	return exitOK
}

func (a *App) channelList(args []string) int {
	flags := a.newFlags("channels list", "[-o FORMAT]")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/alert-channels", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Channels []map[string]any `json:"channels"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Channels == nil {
		data.Channels = []map[string]any{}
	}
	return a.render(mode, data.Channels, func() {
		rows := make([][]string, 0, len(data.Channels))
		for _, ch := range data.Channels {
			rows = append(rows, []string{
				cli.Cell(ch["id"]),
				cli.Cell(ch["name"]),
				cli.Cell(ch["type"]),
				cli.Cell(ch["enabled"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "TYPE", "ENABLED"}, rows)
	})
}

func (a *App) channelRemove(args []string) int {
	flags := a.newFlags("channels remove", "<id>")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "channel")
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/alert-channels/%d", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "channel %d deleted\n", id)
	return exitOK
}

func (a *App) printChannelsHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel channels <subcommand> [flags]

Alert channels receive server/site alerts (admin only).

Subcommands:
  add --name NAME --type TYPE --config JSON
                        create a channel; config per type:
                          telegram  {"bot_token":"..","chat_id":".."}
                          discord   {"webhook_url":"https://discord.com/api/webhooks/.."}
                          email     {"to_address":"ops@example.com","from_address":"..","smtp_tls":"starttls|ssl|none"}
                          webhook   {"url":"https://example.com/hook"}
  list [-o FORMAT]      list channels (secrets are redacted by the API)
  remove <id>           delete a channel
`)
}
