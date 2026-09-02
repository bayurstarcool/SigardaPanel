package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"sigardapanel/internal/cli"
)

// System is the CLI group for system management
func (a *App) System(args []string) int {
	if len(args) == 0 {
		a.printSystemHelp()
		return 0
	}
	switch args[0] {
	case "disk-usage":
		return a.systemDiskUsage(args[1:])
	case "updates":
		return a.systemUpdates(args[1:])
	case "help", "--help", "-h":
		a.printSystemHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown system subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) systemDiskUsage(args []string) int {
	flags := flag.NewFlagSet("system disk-usage", flag.ContinueOnError)
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

	// Get latest metrics
	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/metrics/latest", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var metrics struct {
		DiskUsed  int64 `json:"disk_used"`
		DiskTotal int64 `json:"disk_total"`
		MemUsed   int64 `json:"mem_used"`
		MemTotal  int64 `json:"mem_total"`
		CPUPercent float64 `json:"cpu_percent"`
	}
	json.Unmarshal(resp.Data, &metrics)

	if metrics.DiskTotal == 0 {
		fmt.Fprintln(a.stderr, "no disk metrics available for this server")
		return 1
	}

	diskPercent := float64(metrics.DiskUsed) / float64(metrics.DiskTotal) * 100
	diskUsedGB := float64(metrics.DiskUsed) / (1024 * 1024 * 1024)
	diskTotalGB := float64(metrics.DiskTotal) / (1024 * 1024 * 1024)
	diskFreeGB := diskTotalGB - diskUsedGB

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, map[string]any{
			"disk_used_gb":  fmt.Sprintf("%.2f", diskUsedGB),
			"disk_total_gb": fmt.Sprintf("%.2f", diskTotalGB),
			"disk_free_gb":  fmt.Sprintf("%.2f", diskFreeGB),
			"disk_percent":  fmt.Sprintf("%.1f%%", diskPercent),
			"mem_used_gb":   fmt.Sprintf("%.2f", float64(metrics.MemUsed)/(1024*1024*1024)),
			"mem_total_gb":  fmt.Sprintf("%.2f", float64(metrics.MemTotal)/(1024*1024*1024)),
			"cpu_percent":   fmt.Sprintf("%.1f%%", metrics.CPUPercent),
		})
		return 0
	}

	// Table output with status indicator
	statusIcon := "✅"
	if diskPercent >= 90 {
		statusIcon = "🔴 CRITICAL"
	} else if diskPercent >= 80 {
		statusIcon = "🟡 WARNING"
	} else if diskPercent >= 70 {
		statusIcon = "🟠 ELEVATED"
	}

	fmt.Fprintf(a.stdout, "Server Disk Usage: %s\n\n", statusIcon)
	fmt.Fprintf(a.stdout, "  Disk:  %.1f GB / %.1f GB (%.1f%% used, %.1f GB free)\n", diskUsedGB, diskTotalGB, diskPercent, diskFreeGB)
	fmt.Fprintf(a.stdout, "  Mem:   %.1f GB / %.1f GB\n", float64(metrics.MemUsed)/(1024*1024*1024), float64(metrics.MemTotal)/(1024*1024*1024))
	fmt.Fprintf(a.stdout, "  CPU:   %.1f%%\n", metrics.CPUPercent)
	return 0
}

func (a *App) systemUpdates(args []string) int {
	flags := flag.NewFlagSet("system updates", flag.ContinueOnError)
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

	// Execute update check via agent command
	body := map[string]any{
		"command": "bash -c 'apt-get update -qq 2>/dev/null && apt list --upgradable 2>/dev/null | head -20'",
	}
	resp, status, err := client.Post(fmt.Sprintf("/api/v1/servers/%d/command", *serverID), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 && status != 202 {
		return a.printAPIError(resp, status)
	}

	var result struct {
		Output string `json:"output"`
	}
	json.Unmarshal(resp.Data, &result)

	if result.Output == "" {
		fmt.Fprintln(a.stdout, "No updates available or command not executed.")
		return 0
	}

	fmt.Fprintf(a.stdout, "Available updates:\n\n%s\n", result.Output)
	return 0
}

func (a *App) printSystemHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel system <subcommand>

Subcommands:
  disk-usage --server ID [--output json]
                        check disk usage for a server
  updates --server ID   check available system updates
`)
}
