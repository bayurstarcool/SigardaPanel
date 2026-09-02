package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"sigardapanel/internal/cli"
)

// Redis is the CLI group for Redis management
func (a *App) Redis(args []string) int {
	if len(args) == 0 {
		a.printRedisHelp()
		return 0
	}
	switch args[0] {
	case "stats":
		return a.redisStats(args[1:])
	case "info":
		return a.redisInfo(args[1:])
	case "flush":
		return a.redisFlush(args[1:])
	case "flushdb":
		return a.redisFlushDB(args[1:])
	case "help", "--help", "-h":
		a.printRedisHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown redis subcommand: %s\n", args[0])
		return 2
	}

}

func (a *App) redisStats(args []string) int {
	flags := flag.NewFlagSet("redis stats", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	output := flags.String("output", "", "output format (table|json)")

	if err := flags.Parse(args); err != nil {
		return 2
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/redis/stats", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Stats map[string]string `json:"stats"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Stats)
		return 0
	}

	fmt.Fprintln(a.stdout, "Redis Statistics:")
	fmt.Fprintln(a.stdout, "─────────────────────────────────────")
	for k, v := range data.Stats {
		fmt.Fprintf(a.stdout, "  %-30s %s\n", k, v)
	}
	return 0
}

func (a *App) redisInfo(args []string) int {
	flags := flag.NewFlagSet("redis info", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")

	if err := flags.Parse(args); err != nil {
		return 2
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/redis/info", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Info string `json:"info"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintln(a.stdout, data.Info)
	return 0
}

func (a *App) redisFlush(args []string) int {
	flags := flag.NewFlagSet("redis flush", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}

	confirmed, err := cli.Confirm("Flush ALL Redis data on this server?", false, a.stdin, a.stdout)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(a.stdout, "cancelled")
		return 0
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/redis/flush", *serverID), nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintln(a.stdout, "Redis data flushed")
	return 0
}

func (a *App) redisFlushDB(args []string) int {
	flags := flag.NewFlagSet("redis flushdb", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	db := flags.Int("db", 0, "database number (0-15)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}
	if *db < 0 || *db > 15 {
		fmt.Fprintln(a.stderr, "--db must be 0-15")
		return 2
	}

	confirmed, err := cli.Confirm(fmt.Sprintf("Flush Redis database %d?", *db), false, a.stdin, a.stdout)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(a.stdout, "cancelled")
		return 0
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/redis/flushdb", *serverID), map[string]int{"db": *db})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "Redis database %d flushed\n", *db)
	return 0
}

func (a *App) printRedisHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel redis <subcommand>

Subcommands:
  stats --server ID [--output json]
                        show Redis statistics
  info --server ID     show detailed Redis info
  flush --server ID    flush ALL Redis data (with confirmation)
  flushdb --server ID --db N
                        flush a specific database (0-15)
`)
}
