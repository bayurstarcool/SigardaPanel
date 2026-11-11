package commands

import (
	"encoding/json"
	"fmt"
	"sigardapanel/internal/cli"
)

// Firewall is the CLI group for firewall management
func (a *App) Firewall(args []string) int {
	if len(args) == 0 {
		a.printFirewallHelp()
		return exitOK
	}
	switch args[0] {
	case "status":
		return a.firewallStatus(args[1:])
	case "enable":
		return a.firewallToggle(args[1:], "enable")
	case "disable":
		return a.firewallToggle(args[1:], "disable")
	case "allow":
		return a.firewallAllow(args[1:])
	case "deny":
		return a.firewallDeny(args[1:])
	case "rules":
		return a.firewallRules(args[1:])
	case "delete":
		return a.firewallDelete(args[1:])
	case "reset":
		return a.firewallReset(args[1:])
	case "help", "--help", "-h":
		a.printFirewallHelp()
		return exitOK
	default:
		return a.unknownSubcommand("firewall", args[0], a.printFirewallHelp)
	}
}

func (a *App) firewallStatus(args []string) int {
	flags := a.newFlags("firewall status", "--server ID")
	serverID := flags.Int64("server", 0, "server ID (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/firewall/status", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Installed bool   `json:"installed"`
		Active    bool   `json:"active"`
		Raw       string `json:"raw"`
	}
	json.Unmarshal(resp.Data, &data)

	if !data.Installed {
		fmt.Fprintln(a.stdout, "⚠️  UFW is not installed")
		return 0
	}

	if data.Active {
		fmt.Fprintf(a.stdout, "🟢 Firewall: ACTIVE\n\n")
	} else {
		fmt.Fprintf(a.stdout, "🔴 Firewall: INACTIVE\n\n")
	}
	fmt.Fprintln(a.stdout, data.Raw)
	return 0
}

func (a *App) firewallToggle(args []string, action string) int {
	flags := a.newFlags("firewall "+action, "--server ID")
	serverID := flags.Int64("server", 0, "server ID (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/firewall/%s", *serverID, action), nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "firewall %sed\n", action)
	return 0
}

func (a *App) firewallAllow(args []string) int {
	flags := a.newFlags("firewall allow", "--server ID --port PORT [--protocol tcp|udp] [--from IP] [--comment TEXT]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	port := flags.String("port", "", "port (e.g. 80, 443, 22/tcp)")
	protocol := flags.String("protocol", "", "protocol (tcp|udp)")
	from := flags.String("from", "", "source IP (optional)")
	comment := flags.String("comment", "", "rule comment (optional)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 || *port == "" {
		fmt.Fprintln(a.stderr, "--server and --port are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{"port": *port}
	if *protocol != "" {
		body["protocol"] = *protocol
	}
	if *from != "" {
		body["from"] = *from
	}
	if *comment != "" {
		body["comment"] = *comment
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/firewall/allow", *serverID), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "port %s allowed\n", *port)
	return 0
}

func (a *App) firewallDeny(args []string) int {
	flags := a.newFlags("firewall deny", "--server ID --port PORT [--protocol tcp|udp] [--from IP]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	port := flags.String("port", "", "port (e.g. 80, 443, 22/tcp)")
	protocol := flags.String("protocol", "", "protocol (tcp|udp)")
	from := flags.String("from", "", "source IP (optional)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 || *port == "" {
		fmt.Fprintln(a.stderr, "--server and --port are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]string{"port": *port}
	if *protocol != "" {
		body["protocol"] = *protocol
	}
	if *from != "" {
		body["from"] = *from
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/firewall/deny", *serverID), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "port %s denied\n", *port)
	return 0
}

func (a *App) firewallRules(args []string) int {
	flags := a.newFlags("firewall rules", "--server ID [-o FORMAT]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	output := addOutputFlag(flags)

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/firewall/rules", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Rules []struct {
			Number int    `json:"number"`
			Action string `json:"action"`
			From   string `json:"from"`
			To     string `json:"to"`
		} `json:"rules"`
		Total int `json:"total"`
	}
	json.Unmarshal(resp.Data, &data)

	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	if mode != cli.OutputTable {
		return a.render(mode, data.Rules, nil)
	}

	if len(data.Rules) == 0 {
		fmt.Fprintln(a.stdout, "No firewall rules found.")
		return 0
	}

	rows := make([][]string, 0, len(data.Rules))
	for _, r := range data.Rules {
		rows = append(rows, []string{
			fmt.Sprintf("%d", r.Number),
			r.Action,
			r.From,
			r.To,
		})
	}
	cli.PrintTable(a.stdout, []string{"#", "ACTION", "FROM", "TO"}, rows)
	return 0
}

func (a *App) firewallDelete(args []string) int {
	flags := a.newFlags("firewall delete", "--server ID --number N [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	serverID := flags.Int64("server", 0, "server ID (required)")
	number := flags.Int("number", 0, "rule number (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 || *number <= 0 {
		fmt.Fprintln(a.stderr, "--server and --number are required")
		return 2
	}

	if !*yes && !a.confirm(fmt.Sprintf("Delete firewall rule #%d?", *number)) {
		return exitError
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/firewall/delete", *serverID), map[string]int{"number": *number})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "rule #%d deleted\n", *number)
	return 0
}

func (a *App) firewallReset(args []string) int {
	flags := a.newFlags("firewall reset", "--server ID [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	serverID := flags.Int64("server", 0, "server ID (required)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}

	if !*yes && !a.confirm("Reset firewall to defaults? This will remove ALL rules.") {
		return exitError
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/firewall/reset", *serverID), nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintln(a.stdout, "firewall reset to defaults")
	return 0
}

func (a *App) printFirewallHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel firewall <subcommand> [flags]

UFW on a managed server, through its agent (admin).

Subcommands:
  status --server ID    show firewall status
  enable --server ID    enable firewall
  disable --server ID   disable firewall
  allow --server ID --port PORT [--protocol PROTO] [--from IP] [--comment TEXT]
                        allow incoming traffic on port
  deny --server ID --port PORT [--protocol PROTO] [--from IP]
                        deny incoming traffic on port
  rules --server ID [-o FORMAT]
                        list firewall rules
  delete --server ID --number N [--yes]
                        delete a firewall rule by number
  reset --server ID [--yes]
                        reset firewall to defaults (asks first)
`)
}
