package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"sigardapanel/internal/cli"
)

// Server is the CLI group for server management
func (a *App) Server(args []string) int {
	if len(args) == 0 {
		a.printServerHelp()
		return exitOK
	}
	switch args[0] {
	case "add":
		return a.serverAdd(args[1:])
	case "list", "ls":
		return a.serverList(args[1:])
	case "get", "show":
		return a.serverGet(args[1:])
	case "health":
		return a.serverHealth(args[1:])
	case "update", "edit":
		return a.serverUpdate(args[1:])
	case "remove", "rm", "delete":
		return a.serverRemove(args[1:])
	case "doctor":
		return a.serverDoctor(args[1:])
	case "token":
		return a.serverToken(args[1:])
	case "help", "--help", "-h":
		a.printServerHelp()
		return exitOK
	default:
		return a.unknownSubcommand("server", args[0], a.printServerHelp)
	}
}

func (a *App) serverAdd(args []string) int {
	flags := a.newFlags("server add", "--name NAME --hostname HOST [--ip IP] [--agent-url URL] [--ssh-port N] [--description TEXT]")
	name := flags.String("name", "", "server name (required)")
	hostname := flags.String("hostname", "", "server hostname (required)")
	ip := flags.String("ip", "", "server IP address")
	agentURL := flags.String("agent-url", "", "agent URL, e.g. http://10.0.0.5:7710")
	sshPort := flags.Int("ssh-port", 0, "SSH port (default 22)")
	description := flags.String("description", "", "free-text description")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *name == "" || *hostname == "" {
		fmt.Fprintln(a.stderr, "--name and --hostname are required")
		fmt.Fprintln(a.stderr, "usage: sigardapanel server add --name <name> --hostname <host> [--ip <ip>] [--agent-url <url>]")
		return exitUsage
	}

	body := map[string]any{"name": *name, "hostname": *hostname}
	if *ip != "" {
		body["ip_address"] = *ip
	}
	if *agentURL != "" {
		body["agent_url"] = *agentURL
	}
	if *sshPort > 0 {
		body["ssh_port"] = *sshPort
	}
	if *description != "" {
		body["description"] = *description
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, "/api/v1/servers", body, http.StatusCreated)
	if !ok {
		return exitError
	}
	var data struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Hostname   string `json:"hostname"`
		AgentToken string `json:"agent_token"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}

	fmt.Fprintf(a.stdout, "server created (id=%d)\n", data.ID)
	fmt.Fprintf(a.stdout, "name: %s\n", data.Name)
	fmt.Fprintf(a.stdout, "hostname: %s\n", data.Hostname)
	fmt.Fprintf(a.stdout, "agent_token: %s\n", data.AgentToken)
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "save this token — it won't be shown again")
	return exitOK
}

func (a *App) serverList(args []string) int {
	flags := a.newFlags("server list", "[-o FORMAT]")
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
	resp, ok := a.call(client, http.MethodGet, "/api/v1/servers", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Servers []map[string]any `json:"servers"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Servers == nil {
		data.Servers = []map[string]any{}
	}
	return a.render(mode, data.Servers, func() {
		rows := make([][]string, 0, len(data.Servers))
		for _, s := range data.Servers {
			rows = append(rows, []string{
				cli.Cell(s["id"]),
				cli.Cell(s["name"]),
				cli.Cell(s["hostname"]),
				cli.Cell(s["ip_address"]),
				cli.Cell(s["status"]),
				cli.Cell(s["health_status"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "HOSTNAME", "IP", "STATUS", "HEALTH"}, rows)
	})
}

func (a *App) serverGet(args []string) int {
	flags := a.newFlags("server get", "<id> [-o FORMAT]")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "server")
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/servers/%d", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var srv map[string]any
	if !a.decode(resp, &srv) {
		return exitError
	}
	delete(srv, "agent_token") // never echo secrets in table/structured output
	return a.render(mode, srv, func() {
		a.printKV(srv, "id", "name", "hostname", "ip_address", "ipv6", "agent_url", "ssh_port", "status",
			"health_status", "health_latency_ms", "last_health_check", "provider", "description", "created_at", "updated_at")
	})
}

// serverHealth shows the last health check of one server, or of all.
func (a *App) serverHealth(args []string) int {
	flags := a.newFlags("server health", "[<id>] [-o FORMAT]")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	path := "/api/v1/servers/health"
	single := len(pos) > 0
	if single {
		id, ok := a.idArg(flags, pos, 0, "server")
		if !ok {
			return exitUsage
		}
		path = fmt.Sprintf("/api/v1/servers/%d/health", id)
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, path, nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var servers []map[string]any
	if single {
		var one map[string]any
		if !a.decode(resp, &one) {
			return exitError
		}
		servers = []map[string]any{one}
	} else {
		var data struct {
			Servers []map[string]any `json:"servers"`
		}
		if !a.decode(resp, &data) {
			return exitError
		}
		servers = data.Servers
	}
	if servers == nil {
		servers = []map[string]any{}
	}
	var out any = servers
	if single {
		out = servers[0]
	}
	return a.render(mode, out, func() {
		rows := make([][]string, 0, len(servers))
		for _, s := range servers {
			rows = append(rows, []string{
				cli.Cell(s["id"]), cli.Cell(s["name"]), cli.Cell(s["health_status"]),
				cli.Cell(s["health_latency_ms"]), healthTime(s["last_health_check"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "HEALTH", "LATENCY_MS", "LAST_CHECK"}, rows)
	})
}

// healthTime renders last_health_check, which the API returns either as a
// time string or as a sql.NullTime object {Time, Valid}.
func healthTime(v any) string {
	if m, ok := v.(map[string]any); ok {
		if valid, _ := m["Valid"].(bool); !valid {
			return "-"
		}
		return cli.Cell(m["Time"])
	}
	return cli.Cell(v)
}

func (a *App) serverRemove(args []string) int {
	flags := a.newFlags("server remove", "<id>")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, 0, "server")
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/servers/%d", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "server %d deleted\n", id)
	return exitOK
}

func (a *App) serverDoctor(args []string) int {
	flags := a.newFlags("server doctor", "")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	_, status, err := client.Get("/api/v1/health")
	if err != nil {
		fmt.Fprintf(a.stderr, "panel health: %v\n", err)
		return exitError
	}
	if status != http.StatusOK {
		fmt.Fprintf(a.stderr, "panel health: HTTP %d\n", status)
		return exitError
	}
	fmt.Fprintf(a.stdout, "panel: ok (url: %s)\n", client.BaseURL)
	return exitOK
}

func (a *App) serverUpdate(args []string) int {
	flags := a.newFlags("server update", "<id> [--name N] [--hostname H] [--ip IP] [--ssh-port N] [--description TEXT]")
	idFlag := flags.Int64("id", 0, "server ID (alternative to the positional argument)")
	name := flags.String("name", "", "server name")
	hostname := flags.String("hostname", "", "server hostname")
	ip := flags.String("ip", "", "server IP address (\"none\" clears it)")
	sshPort := flags.Int("ssh-port", 0, "SSH port")
	description := flags.String("description", "", "free-text description")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(flags, pos, *idFlag, "server")
	if !ok {
		return exitUsage
	}

	body := make(map[string]any)
	if *name != "" {
		body["name"] = *name
	}
	if *hostname != "" {
		body["hostname"] = *hostname
	}
	if *ip == "none" {
		body["ip_address"] = ""
	} else if *ip != "" {
		body["ip_address"] = *ip
	}
	if *sshPort > 0 {
		body["ssh_port"] = *sshPort
	}
	if *description != "" {
		body["description"] = *description
	}
	if len(body) == 0 {
		fmt.Fprintln(a.stderr, "at least one of --name, --hostname, --ip, --ssh-port, --description is required")
		fmt.Fprintln(a.stderr, "(status is maintained by health checks; the agent URL cannot be changed through the API)")
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPatch, fmt.Sprintf("/api/v1/servers/%d", id), body, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "server updated (id=%d)\n", data.ID)
	if data.Name != "" {
		fmt.Fprintf(a.stdout, "  name: %s\n", data.Name)
	}
	if data.Hostname != "" {
		fmt.Fprintf(a.stdout, "  hostname: %s\n", data.Hostname)
	}
	return exitOK
}

// serverToken regenerates the agent token for a server via the API and, unless
// --no-env-sync is given, writes it into the local env files (agent.env +
// panel.env under /etc/sigardapanel) so DB and services never drift.
func (a *App) serverToken(args []string) int {
	flags := a.newFlags("server token", "[<id>] [--no-env-sync]")
	idFlag := flags.Int64("id", 0, "server ID (alternative to the positional argument; default 1)")
	noSync := flags.Bool("no-env-sync", false, "do not rewrite /etc/sigardapanel/{agent,panel}.env on this machine")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id := int64(1)
	if len(pos) > 0 || *idFlag > 0 {
		if id, ok = a.idArg(flags, pos, *idFlag, "server"); !ok {
			return exitUsage
		}
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}

	// Only the panel host's own agent reads /etc/sigardapanel/*.env on this
	// machine; writing a remote server's token there would break the local
	// agent. Decide from the server's agent_url before regenerating.
	local := false
	if sresp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/servers/%d", id), nil, http.StatusOK); ok {
		var srv struct {
			AgentURL string `json:"agent_url"`
		}
		a.decode(sresp, &srv)
		local = isLocalAgentURL(srv.AgentURL)
	} else {
		return exitError
	}

	// 1. Regenerate via API (updates DB servers.agent_token)
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/servers/%d/token", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		AgentToken string `json:"agent_token"`
	}
	if !a.decode(resp, &data) || data.AgentToken == "" {
		fmt.Fprintln(a.stderr, "no agent_token in response")
		return exitError
	}
	token := data.AgentToken

	// 2. Sync to local env files so the agent process picks it up on restart
	if !local {
		fmt.Fprintf(a.stdout, "agent token regenerated for server %d (remote: local env files left untouched)\n", id)
		fmt.Fprintf(a.stdout, "agent_token: %s\n", token)
		fmt.Fprintln(a.stdout, "NOTE: on that server set SIGARDAPANEL_AGENT_TOKEN=<token> in /etc/sigardapanel/agent.env and restart sigardapanel-agent")
		return exitOK
	}
	if !*noSync {
		for _, ef := range localEnvFiles {
			if _, err := os.Stat(ef); err != nil {
				continue
			}
			content, err := os.ReadFile(ef)
			if err != nil {
				fmt.Fprintf(a.stderr, "  WARN: read %s: %v\n", ef, err)
				continue
			}
			updated := updateEnvToken(string(content), token)
			if err := os.WriteFile(ef, []byte(updated), 0o600); err != nil {
				fmt.Fprintf(a.stderr, "  WARN: write %s: %v\n", ef, err)
				continue
			}
			fmt.Fprintf(a.stdout, "  env: %s updated\n", ef)
		}
	}

	fmt.Fprintf(a.stdout, "agent token regenerated for server %d\n", id)
	fmt.Fprintf(a.stdout, "agent_token: %s\n", token)
	fmt.Fprintln(a.stdout, "NOTE: restart sigardapanel-agent + sigardapanel-api to apply")
	return exitOK
}

// localEnvFiles are the panel host's env files that carry the local agent
// token (a variable so tests never touch the real files).
var localEnvFiles = []string{"/etc/sigardapanel/agent.env", "/etc/sigardapanel/panel.env"}

// isLocalAgentURL reports whether an agent URL points at this machine
// (loopback host), i.e. the panel host's own auto-registered agent.
func isLocalAgentURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch h := u.Hostname(); h {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return strings.HasPrefix(h, "127.")
	}
}

// updateEnvToken replaces the SIGARDAPANEL_AGENT_TOKEN value in an env file
// (handles existing line, appends if missing).
func updateEnvToken(content, token string) string {
	lines := strings.Split(content, "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "SIGARDAPANEL_AGENT_TOKEN=") {
			lines[i] = "SIGARDAPANEL_AGENT_TOKEN=" + token
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, "SIGARDAPANEL_AGENT_TOKEN="+token)
	}
	return strings.Join(lines, "\n")
}

func (a *App) printServerHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel server <subcommand> [flags]

Subcommands:
  add --name NAME --hostname HOST [--ip IP] [--agent-url URL] [--ssh-port N]
      [--description TEXT]
                        register a server (admin); prints its agent token once
  list [-o FORMAT]      list servers
  get <id> [-o FORMAT]  show one server (alias: show)
  health [<id>] [-o FORMAT]
                        last health check of one server, or of all servers
  update <id> [--name N] [--hostname H] [--ip IP|none] [--ssh-port N]
         [--description TEXT]
                        update server settings (admin)
  remove <id>           remove a server (admin; alias: rm, delete)
  token [<id>] [--no-env-sync]
                        regenerate the agent token (admin; default server 1). For the
                        local server (agent_url on localhost) it is also written to
                        /etc/sigardapanel/{agent,panel}.env; for a remote server it is
                        only printed, to be set in that server's agent.env
  doctor                check that the panel API answers
`)
}
