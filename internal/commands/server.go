package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"sigardapanel/internal/cli"
)

// Server is the CLI group for server management
func (a *App) Server(args []string) int {
	if len(args) == 0 {
		a.printServerHelp()
		return 0
	}
	switch args[0] {
	case "add":
		return a.serverAdd(args[1:])
	case "list", "ls":
		return a.serverList(args[1:])
	case "update", "edit":
		return a.serverUpdate(args[1:])
	case "remove", "rm":
		return a.serverRemove(args[1:])
	case "doctor":
		return a.serverDoctor(args[1:])
	case "token":
		return a.serverToken(args[1:])
	case "help", "--help", "-h":
		a.printServerHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown server subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) serverAdd(args []string) int {
	flags := flag.NewFlagSet("server add", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	name := flags.String("name", "", "server name")
	hostname := flags.String("hostname", "", "server hostname")
	ip := flags.String("ip", "", "server IP address (optional)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *name == "" || *hostname == "" {
		fmt.Fprintln(a.stderr, "name and hostname are required")
		fmt.Fprintln(a.stderr, "usage: server add --name <name> --hostname <host> [--ip <ip>]")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{"name": *name, "hostname": *hostname}
	if *ip != "" {
		body["ip_address"] = *ip
	}

	resp, status, err := client.Post("/api/v1/servers", body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 201 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Hostname   string `json:"hostname"`
		AgentToken string `json:"agent_token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.stdout, "server created (id=%d)\n", data.ID)
	fmt.Fprintf(a.stdout, "name: %s\n", data.Name)
	fmt.Fprintf(a.stdout, "hostname: %s\n", data.Hostname)
	fmt.Fprintf(a.stdout, "agent_token: %s\n", data.AgentToken)
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "save this token — it won't be shown again")
	return 0
}

func (a *App) serverList(args []string) int {
	flags := flag.NewFlagSet("server list", flag.ContinueOnError)
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

	resp, status, err := client.Get("/api/v1/servers")
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	} else if *output == "table" {
		mode = cli.OutputTable
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Servers)
		return 0
	}

	rows := make([][]string, 0, len(data.Servers))
	for _, s := range data.Servers {
		rows = append(rows, []string{
			fmt.Sprintf("%v", s["id"]),
			fmt.Sprintf("%v", s["name"]),
			fmt.Sprintf("%v", s["hostname"]),
			fmt.Sprintf("%v", s["status"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "NAME", "HOSTNAME", "STATUS"}, rows)
	return 0
}

func (a *App) serverRemove(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: server remove <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Delete("/api/v1/servers/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "server %s deleted\n", id)
	return 0
}

func (a *App) serverDoctor(args []string) int {
	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	_, status, err := client.Get("/api/v1/health")
	if err != nil {
		fmt.Fprintf(a.stderr, "panel health: %v\n", err)
		return 1
	}
	if status != 200 {
		fmt.Fprintf(a.stderr, "panel health: HTTP %d\n", status)
		return 1
	}

	fmt.Fprintf(a.stdout, "panel: ok (url: %s)\n", client.BaseURL)
	return 0
}

func (a *App) serverUpdate(args []string) int {
	flags := flag.NewFlagSet("server update", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "server ID (required)")
	name := flags.String("name", "", "server name")
	hostname := flags.String("hostname", "", "server hostname")
	ip := flags.String("ip", "", "server IP address")
	statusFlag := flags.String("status", "", "server status (active|inactive|maintenance)")
	agentURL := flags.String("agent-url", "", "agent URL (http://host:7710)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		fmt.Fprintln(a.stderr, "usage: server update --id <id> [--name N] [--hostname H] [--ip I] [--status S] [--agent-url U]")
		return 2
	}

	// Check at least one field to update
	if *name == "" && *hostname == "" && *ip == "" && *statusFlag == "" && *agentURL == "" {
		fmt.Fprintln(a.stderr, "at least one field is required")
		fmt.Fprintln(a.stderr, "usage: server update --id <id> [--name N] [--hostname H] [--ip I] [--status S] [--agent-url U]")
		return 2
	}

	if *statusFlag != "" && *statusFlag != "active" && *statusFlag != "inactive" && *statusFlag != "maintenance" {
		fmt.Fprintf(a.stderr, "invalid status %q: must be active, inactive, or maintenance\n", *statusFlag)
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := make(map[string]any)
	if *name != "" {
		body["name"] = *name
	}
	if *hostname != "" {
		body["hostname"] = *hostname
	}
	if *ip != "" {
		body["ip_address"] = *ip
	}
	if *statusFlag != "" {
		body["status"] = *statusFlag
	}
	if *agentURL != "" {
		body["agent_url"] = *agentURL
	}

	resp, status, err := client.Patch(fmt.Sprintf("/api/v1/servers/%d", *id), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintf(a.stdout, "server updated (id=%d)\n", data.ID)
	if data.Name != "" {
		fmt.Fprintf(a.stdout, "  name: %s\n", data.Name)
	}
	if data.Hostname != "" {
		fmt.Fprintf(a.stdout, "  hostname: %s\n", data.Hostname)
	}
	return 0
}

// serverToken regenerates the agent token for a server via the API and
// synchronizes it everywhere: DB (API side), and the local env files
// (agent.env + panel.env) so all three never drift.
func (a *App) serverToken(args []string) int {
	flags := flag.NewFlagSet("server token", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 1, "server ID (default 1)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	// 1. Regenerate via API (updates DB servers.agent_token)
	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/token", *id), nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		AgentToken string `json:"agent_token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil || data.AgentToken == "" {
		fmt.Fprintln(a.stderr, "no agent_token in response")
		return 1
	}
	token := data.AgentToken

	// 2. Sync to local env files so the agent process picks it up on restart
	envFiles := []string{}
	if _, err := os.Stat("/etc/sigardapanel/agent.env"); err == nil {
		envFiles = append(envFiles, "/etc/sigardapanel/agent.env")
	}
	if _, err := os.Stat("/etc/sigardapanel/panel.env"); err == nil {
		envFiles = append(envFiles, "/etc/sigardapanel/panel.env")
	}
	for _, ef := range envFiles {
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

	fmt.Fprintf(a.stdout, "agent token regenerated for server %d\n", *id)
	fmt.Fprintf(a.stdout, "agent_token: %s\n", token)
	fmt.Fprintln(a.stdout, "NOTE: restart sigardapanel-agent + sigardapanel-api to apply")
	return 0
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
	fmt.Fprint(a.stdout, `Usage: sigardapanel server <subcommand>

Subcommands:
  add --name <name> --hostname <host> [--ip <ip>]
                        register a new server
  list [--output json]  list all servers
  update --id <id> [--name N] [--hostname H] [--ip I] [--status S] [--agent-url U]
                        update server settings
  remove <id>           remove a server
  token [--id <id>]     regenerate agent token (syncs DB + agent.env + panel.env)
  doctor                health check
`)
}

func (a *App) printAPIError(resp *cli.APIResponse, status int) int {
	if resp != nil && resp.Error != nil {
		fmt.Fprintf(a.stderr, "API error (%d): %s — %s\n", status, resp.Error.Code, resp.Error.Message)
	} else {
		fmt.Fprintf(a.stderr, "HTTP %d\n", status)
	}
	return 1
}

// silence unused import warning if strings unused
var _ = strings.TrimSpace