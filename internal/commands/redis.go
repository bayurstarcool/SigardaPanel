package commands

import (
	"encoding/json"
	"fmt"
	"sigardapanel/internal/cli"
)

// Redis is the CLI group for Redis management
func (a *App) Redis(args []string) int {
	if len(args) == 0 {
		a.printRedisHelp()
		return exitOK
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
		return exitOK
	default:
		return a.unknownSubcommand("redis", args[0], a.printRedisHelp)
	}

}

func (a *App) redisStats(args []string) int {
	flags := a.newFlags("redis stats", "--server ID [-o FORMAT]")
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

	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	if mode != cli.OutputTable {
		return a.render(mode, data.Stats, nil)
	}

	fmt.Fprintln(a.stdout, "Redis Statistics:")
	fmt.Fprintln(a.stdout, "─────────────────────────────────────")
	for k, v := range data.Stats {
		fmt.Fprintf(a.stdout, "  %-30s %s\n", k, v)
	}
	return 0
}

func (a *App) redisInfo(args []string) int {
	flags := a.newFlags("redis info", "--server ID")
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
	flags := a.newFlags("redis flush", "--server ID [--yes]")
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

	if !*yes && !a.confirm("Flush ALL Redis data on this server?") {
		return exitError
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
	flags := a.newFlags("redis flushdb", "--server ID --db N [--yes]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	flags.BoolVar(yes, "y", false, "shorthand for --yes")
	serverID := flags.Int64("server", 0, "server ID (required)")
	db := flags.Int("db", 0, "database number (0-15)")

	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 {
		fmt.Fprintln(a.stderr, "--server is required")
		return 2
	}
	if *db < 0 || *db > 15 {
		fmt.Fprintln(a.stderr, "--db must be 0-15")
		return 2
	}

	if !*yes && !a.confirm(fmt.Sprintf("Flush Redis database %d?", *db)) {
		return exitError
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
	fmt.Fprint(a.stdout, `Usage: sigardapanel redis <subcommand> [flags]

Redis on a managed server, through its agent (admin).

Subcommands:
  stats --server ID [-o FORMAT]
                        show Redis statistics
  info --server ID     show detailed Redis info
  flush --server ID [--yes]
                        flush ALL Redis data (asks first)
  flushdb --server ID --db N [--yes]
                        flush a specific database (0-15)
`)
}
