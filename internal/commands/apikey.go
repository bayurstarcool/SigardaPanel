package commands

import (
	"fmt"
	"net/http"

	"sigardapanel/internal/cli"
)

// APIKey is the CLI group for personal API keys (/api/v1/api-keys). Keys
// authenticate as their owner and suit CI: export SIGARDAPANEL_TOKEN=<key>.
// Managing keys needs an interactive login session; a key cannot list,
// create or revoke keys (the API answers 403).
func (a *App) APIKey(args []string) int {
	if len(args) == 0 {
		a.printAPIKeyHelp()
		return exitOK
	}
	switch args[0] {
	case "create", "add":
		return a.apiKeyCreate(args[1:])
	case "list", "ls":
		return a.apiKeyList(args[1:])
	case "revoke", "delete", "rm":
		return a.apiKeyRevoke(args[1:])
	case "help", "--help", "-h":
		a.printAPIKeyHelp()
		return exitOK
	default:
		return a.unknownSubcommand("apikey", args[0], a.printAPIKeyHelp)
	}
}

func (a *App) apiKeyCreate(args []string) int {
	flags := a.newFlags("apikey create", "--name NAME [--expires-in-days N] [-o FORMAT]")
	name := flags.String("name", "", "key name, max 64 chars (required)")
	days := flags.Int("expires-in-days", 0, "expire after N days (1-3650; 0 = never)")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if *name == "" && len(pos) == 1 {
		*name = pos[0]
	}
	if *name == "" {
		fmt.Fprintln(a.stderr, "--name is required")
		return exitUsage
	}
	if *days < 0 || *days > 3650 {
		fmt.Fprintln(a.stderr, "--expires-in-days must be between 0 (never) and 3650")
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
	resp, ok := a.call(client, http.MethodPost, "/api/v1/api-keys", map[string]any{"name": *name, "expires_in_days": *days}, http.StatusCreated)
	if !ok {
		return exitError
	}
	var data struct {
		Key   map[string]any `json:"key"`
		Token string         `json:"token"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if mode != cli.OutputTable {
		return a.render(mode, map[string]any{"key": data.Key, "token": data.Token}, nil)
	}
	fmt.Fprintf(a.stdout, "api key created (id=%s, name=%s, expires=%s)\n",
		cli.Cell(data.Key["id"]), cli.Cell(data.Key["name"]), expiryCell(data.Key["expires_at"]))
	fmt.Fprintf(a.stdout, "token: %s\n", data.Token)
	fmt.Fprintln(a.stdout, "\nThis is the only time the key is shown. Use it with:")
	fmt.Fprintln(a.stdout, "  export SIGARDAPANEL_TOKEN=<token>")
	return exitOK
}

func (a *App) apiKeyList(args []string) int {
	flags := a.newFlags("apikey list", "[-o FORMAT]")
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
	resp, ok := a.call(client, http.MethodGet, "/api/v1/api-keys", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Keys  []map[string]any `json:"keys"`
		Count int              `json:"count"`
		Limit int              `json:"limit"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Keys == nil {
		data.Keys = []map[string]any{}
	}
	return a.render(mode, data.Keys, func() {
		rows := make([][]string, 0, len(data.Keys))
		for _, k := range data.Keys {
			expired := ""
			if e, _ := k["expired"].(bool); e {
				expired = " (expired)"
			}
			rows = append(rows, []string{
				cli.Cell(k["id"]),
				cli.Cell(k["name"]),
				cli.Cell(k["prefix"]) + "...",
				cli.Cell(k["created_at"]),
				cli.Cell(k["last_used_at"]),
				expiryCell(k["expires_at"]) + expired,
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "PREFIX", "CREATED", "LAST_USED", "EXPIRES"}, rows)
		fmt.Fprintf(a.stdout, "%d of %d keys\n", data.Count, data.Limit)
	})
}

func (a *App) apiKeyRevoke(args []string) int {
	flags := a.newFlags("apikey revoke", "<id>")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "api key")
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/api-keys/%d", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "api key %d revoked\n", id)
	return exitOK
}

func expiryCell(v any) string {
	if s, _ := v.(string); s != "" {
		return s
	}
	return "never"
}

func (a *App) printAPIKeyHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel apikey <subcommand> [flags]   (alias: api-key)

Personal API keys act as you, with your role, for scripts and CI:
  export SIGARDAPANEL_TOKEN=<key>
Managing keys requires an interactive "sigardapanel login" session; a key
cannot create, list or revoke keys. Keys also cannot use user management,
service tokens, impersonation, audit purge or logout.

Subcommands:
  create --name NAME [--expires-in-days N] [-o FORMAT]
                        create a key (max 20 per user); the key is printed once
  list [-o FORMAT]      list your keys (prefix, created, last used, expiry)
  revoke <id>           revoke one of your keys
`)
}
