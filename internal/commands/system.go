package commands

import (
	"fmt"
	"net/http"
)

// System is the CLI group for system management
func (a *App) System(args []string) int {
	if len(args) == 0 {
		a.printSystemHelp()
		return exitOK
	}
	switch args[0] {
	case "disk-usage":
		return a.systemDiskUsage(args[1:])
	case "updates":
		return a.systemUpdates(args[1:])
	case "help", "--help", "-h":
		a.printSystemHelp()
		return exitOK
	default:
		return a.unknownSubcommand("system", args[0], a.printSystemHelp)
	}
}

func (a *App) systemDiskUsage(args []string) int {
	flags := a.newFlags("system disk-usage", "--server ID [-o FORMAT]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.serverIDFlag(pos, *serverID)
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/servers/%d/metrics/latest", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var metrics struct {
		DiskUsed   int64   `json:"disk_used"`
		DiskTotal  int64   `json:"disk_total"`
		MemUsed    int64   `json:"mem_used"`
		MemTotal   int64   `json:"mem_total"`
		CPUPercent float64 `json:"cpu_percent"`
	}
	a.decode(resp, &metrics)
	if metrics.DiskTotal == 0 {
		fmt.Fprintln(a.stderr, "no disk metrics available for this server")
		return exitError
	}

	const gb = 1024 * 1024 * 1024
	diskPercent := float64(metrics.DiskUsed) / float64(metrics.DiskTotal) * 100
	diskUsedGB := float64(metrics.DiskUsed) / gb
	diskTotalGB := float64(metrics.DiskTotal) / gb
	diskFreeGB := diskTotalGB - diskUsedGB
	level := "ok"
	switch {
	case diskPercent >= 90:
		level = "critical"
	case diskPercent >= 80:
		level = "warning"
	case diskPercent >= 70:
		level = "elevated"
	}

	out := map[string]any{
		"disk_used_gb":  round2(diskUsedGB),
		"disk_total_gb": round2(diskTotalGB),
		"disk_free_gb":  round2(diskFreeGB),
		"disk_percent":  round2(diskPercent),
		"mem_used_gb":   round2(float64(metrics.MemUsed) / gb),
		"mem_total_gb":  round2(float64(metrics.MemTotal) / gb),
		"cpu_percent":   round2(metrics.CPUPercent),
		"level":         level,
	}
	return a.render(mode, out, func() {
		fmt.Fprintf(a.stdout, "Server disk usage: %s\n\n", level)
		fmt.Fprintf(a.stdout, "  Disk:  %.1f GB / %.1f GB (%.1f%% used, %.1f GB free)\n", diskUsedGB, diskTotalGB, diskPercent, diskFreeGB)
		fmt.Fprintf(a.stdout, "  Mem:   %.1f GB / %.1f GB\n", float64(metrics.MemUsed)/gb, float64(metrics.MemTotal)/gb)
		fmt.Fprintf(a.stdout, "  CPU:   %.1f%%\n", metrics.CPUPercent)
	})
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func (a *App) systemUpdates(args []string) int {
	flags := a.newFlags("system updates", "--server ID")
	serverID := flags.Int64("server", 0, "server ID (required)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.serverIDFlag(pos, *serverID)
	if !ok {
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	// Runs apt on the server through the agent (admin only).
	body := map[string]any{
		"command": "bash -c 'apt-get update -qq 2>/dev/null && apt list --upgradable 2>/dev/null | head -20'",
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/servers/%d/command", id), body, http.StatusOK, http.StatusAccepted)
	if !ok {
		return exitError
	}
	if !a.commandSucceeded(resp) {
		return exitError
	}
	var result struct {
		Output string `json:"output"`
	}
	a.decode(resp, &result)
	if result.Output == "" {
		fmt.Fprintln(a.stdout, "No updates available.")
		return exitOK
	}
	fmt.Fprintf(a.stdout, "Available updates (first 20):\n\n%s\n", result.Output)
	return exitOK
}

func (a *App) printSystemHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel system <subcommand> [flags]

Subcommands:
  disk-usage --server ID [-o FORMAT]
                        disk/memory/CPU from the latest metrics sample
  updates --server ID   list upgradable apt packages (runs apt on the server; admin)
`)
}
