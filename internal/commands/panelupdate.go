package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PanelUpdate is the CLI group for the panel self-update (super_admin).
func (a *App) PanelUpdate(args []string) int {
	if len(args) == 0 {
		return a.panelUpdateStatus(nil, false)
	}
	switch args[0] {
	case "status":
		return a.panelUpdateStatus(args[1:], false)
	case "check":
		return a.panelUpdateStatus(args[1:], true)
	case "apply":
		return a.panelUpdateApply(args[1:])
	case "auto":
		return a.panelUpdateAuto(args[1:])
	case "backups":
		return a.panelUpdateBackups(args[1:])
	case "rollback":
		return a.panelUpdateRollback(args[1:])
	case "agents":
		return a.panelUpdateAgents(args[1:])
	case "update-agents":
		return a.panelUpdateAgentsApply(args[1:])
	case "agents-auto":
		return a.panelUpdateAgentsAuto(args[1:])
	case "help", "--help", "-h":
		a.printPanelUpdateHelp()
		return exitOK
	default:
		return a.unknownSubcommand("panel-update", args[0], a.printPanelUpdateHelp)
	}
}

type panelUpdateInfo struct {
	CurrentVersion  string `json:"current_version"`
	UpdateAvailable bool   `json:"update_available"`
	AutoUpdate      bool   `json:"auto_update"`
	AutoWindow      *struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"auto_update_window"`
	CheckedAt  string `json:"checked_at"`
	CheckError string `json:"check_error"`
	AgentError string `json:"agent_error"`
	Latest     *struct {
		Tag string `json:"tag"`
		URL string `json:"url"`
	} `json:"latest"`
	Status *struct {
		State       string `json:"state"`
		Tag         string `json:"tag"`
		Message     string `json:"message"`
		Log         string `json:"log"`
		Supported   bool   `json:"supported"`
		Unsupported string `json:"unsupported_reason"`
	} `json:"status"`
}

func (a *App) panelUpdateStatus(args []string, refresh bool) int {
	name := "panel-update status"
	if refresh {
		name = "panel-update check"
	}
	flags := a.newFlags(name, "[-o FORMAT]")
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
	path := "/api/v1/system/panel-update"
	if refresh {
		path += "?refresh=1"
	}
	resp, ok := a.call(client, http.MethodGet, path, nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var info panelUpdateInfo
	if !a.decode(resp, &info) {
		return exitError
	}
	var raw any
	a.decode(resp, &raw)
	return a.render(mode, raw, func() {
		latest := "unknown"
		if info.Latest != nil {
			latest = info.Latest.Tag
		}
		fmt.Fprintf(a.stdout, "Installed:  %s\n", info.CurrentVersion)
		fmt.Fprintf(a.stdout, "Latest:     %s", latest)
		if info.UpdateAvailable {
			fmt.Fprint(a.stdout, "  (update available: sigardapanel panel-update apply)")
		}
		fmt.Fprintln(a.stdout)
		if info.CheckError != "" {
			fmt.Fprintf(a.stdout, "Check:      failed: %s\n", info.CheckError)
		}
		auto := "off"
		if info.AutoUpdate {
			auto = "on"
			if w := info.AutoWindow; w != nil {
				auto += fmt.Sprintf(" (between %02d:00 and %02d:00 server time)", w.Start, w.End)
			}
		}
		fmt.Fprintf(a.stdout, "Auto:       %s\n", auto)
		switch {
		case info.AgentError != "":
			fmt.Fprintf(a.stdout, "Agent:      %s\n", info.AgentError)
		case info.Status != nil:
			last := info.Status.State
			if info.Status.Message != "" {
				last += ": " + info.Status.Message
			}
			fmt.Fprintf(a.stdout, "Last run:   %s\n", last)
			if !info.Status.Supported && info.Status.Unsupported != "" {
				fmt.Fprintf(a.stdout, "Note:       %s\n", info.Status.Unsupported)
			}
		}
	})
}

func (a *App) panelUpdateApply(args []string) int {
	flags := a.newFlags("panel-update apply", "[--yes] [--no-wait]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	noWait := flags.Bool("no-wait", false, "start the update and return without following it")
	timeout := flags.Duration("timeout", 45*time.Minute, "how long to follow the update")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/system/panel-update?refresh=1", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var info panelUpdateInfo
	if !a.decode(resp, &info) {
		return exitError
	}
	if !info.UpdateAvailable || info.Latest == nil {
		fmt.Fprintf(a.stdout, "Already on the latest release (%s).\n", info.CurrentVersion)
		return exitOK
	}
	if !*yes && !a.confirm(fmt.Sprintf("Update SigardaPanel %s -> %s? The panel restarts for about a minute.", info.CurrentVersion, info.Latest.Tag)) {
		return exitError
	}
	if _, ok := a.call(client, http.MethodPost, "/api/v1/system/panel-update", map[string]any{}, http.StatusAccepted); !ok {
		return exitError
	}
	tag := info.Latest.Tag
	fmt.Fprintf(a.stdout, "Update to %s started.\n", tag)
	if *noWait {
		return exitOK
	}
	return a.followPanelUpdate(tag, *timeout)
}

// panelUpdatePoll is the interval between status polls (tests shorten it).
var panelUpdatePoll = 3 * time.Second

// followPanelUpdate polls until the agent reports a result and, on success,
// the API runs the new version. Failed requests are expected while the
// services restart.
func (a *App) followPanelUpdate(tag string, timeout time.Duration) int {
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	deadline := time.Now().Add(timeout)
	printed := 0
	for time.Now().Before(deadline) {
		time.Sleep(panelUpdatePoll)
		resp, status, err := client.Request(http.MethodGet, "/api/v1/system/panel-update", nil)
		if err == nil && status == http.StatusNotFound {
			// An older release without the update endpoint (after a rollback):
			// the version alone tells whether it is running.
			if v, vs, verr := client.Request(http.MethodGet, "/api/v1/version", nil); verr == nil && vs == http.StatusOK {
				var ver struct {
					Version string `json:"version"`
				}
				if json.Unmarshal(v.Data, &ver) == nil && "v"+strings.TrimPrefix(ver.Version, "v") == tag {
					fmt.Fprintf(a.stdout, "SigardaPanel is now %s.\n", ver.Version)
					return exitOK
				}
			}
			continue
		}
		if err != nil || status != http.StatusOK {
			continue // restarting
		}
		var info panelUpdateInfo
		if !a.decode(resp, &info) || info.Status == nil {
			continue
		}
		lines := strings.Split(strings.TrimRight(info.Status.Log, "\n"), "\n")
		if len(lines) < printed {
			printed = 0 // the log was truncated at the front
		}
		for _, l := range lines[printed:] {
			if l != "" {
				fmt.Fprintln(a.stdout, "  "+l)
			}
		}
		printed = len(lines)
		switch info.Status.State {
		case "failed":
			fmt.Fprintf(a.stderr, "update failed: %s\n", info.Status.Message)
			return exitError
		case "succeeded":
			if "v"+strings.TrimPrefix(info.CurrentVersion, "v") == tag {
				fmt.Fprintf(a.stdout, "SigardaPanel is now %s.\n", info.CurrentVersion)
				return exitOK
			}
		}
	}
	fmt.Fprintf(a.stderr, "gave up following the update after %s; check: sigardapanel panel-update status\n", timeout)
	return exitError
}

func (a *App) panelUpdateAuto(args []string) int {
	flags := a.newFlags("panel-update auto", "on|off [--window START-END|any]")
	window := flags.String("window", "", "hours (server time) automatic updates may start in, e.g. 2-5; 'any' clears it")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	body := map[string]any{}
	if len(pos) > 0 {
		switch pos[0] {
		case "on":
			body["auto_update"] = true
		case "off":
			body["auto_update"] = false
		default:
			fmt.Fprintf(a.stderr, "expected on or off, got %q\n", pos[0])
			return exitUsage
		}
	}
	if *window != "" {
		if *window == "any" {
			body["auto_update_window"] = nil
		} else {
			parts := strings.SplitN(*window, "-", 2)
			start, err1 := strconv.Atoi(parts[0])
			var end int
			var err2 error = fmt.Errorf("missing end")
			if len(parts) == 2 {
				end, err2 = strconv.Atoi(parts[1])
			}
			if err1 != nil || err2 != nil {
				fmt.Fprintln(a.stderr, "--window must look like 2-5 (hours 0-23) or any")
				return exitUsage
			}
			body["auto_update_window"] = map[string]int{"start": start, "end": end}
		}
	}
	if len(body) == 0 {
		fmt.Fprintln(a.stderr, "usage: sigardapanel panel-update auto on|off [--window START-END|any]")
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodPut, "/api/v1/system/panel-update/settings", body, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintln(a.stdout, "Saved.")
	return exitOK
}

func (a *App) panelUpdateBackups(args []string) int {
	flags := a.newFlags("panel-update backups", "[-o FORMAT]")
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
	resp, ok := a.call(client, http.MethodGet, "/api/v1/system/panel-update/backups", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Backups []struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Kind        string `json:"kind"`
			HasDatabase bool   `json:"has_database"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"backups"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	return a.render(mode, data, func() {
		if len(data.Backups) == 0 {
			fmt.Fprintln(a.stdout, "No update backups yet (one is made before every update).")
			return
		}
		fmt.Fprintf(a.stdout, "%-18s %-10s %-13s %-9s %s\n", "BACKUP", "VERSION", "KIND", "DATABASE", "SIZE")
		for _, b := range data.Backups {
			db := "no"
			if b.HasDatabase {
				db = "yes"
			}
			fmt.Fprintf(a.stdout, "%-18s %-10s %-13s %-9s %.1f MB\n", b.Name, b.Version, b.Kind, db, float64(b.SizeBytes)/(1<<20))
		}
	})
}

func (a *App) panelUpdateRollback(args []string) int {
	flags := a.newFlags("panel-update rollback", "BACKUP [--yes] [--no-wait]")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	noWait := flags.Bool("no-wait", false, "start the rollback and return without following it")
	timeout := flags.Duration("timeout", 45*time.Minute, "how long to follow the rollback")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		fmt.Fprintln(a.stderr, "usage: sigardapanel panel-update rollback BACKUP [--yes]  (list them with: panel-update backups)")
		return exitUsage
	}
	if !*yes && !a.confirm("Roll back to backup "+pos[0]+"? The binary, web app and DATABASE are restored: changes made since that backup are lost.") {
		return exitError
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, "/api/v1/system/panel-update/rollback", map[string]string{"backup": pos[0]}, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var started struct {
		Tag string `json:"tag"`
	}
	a.decode(resp, &started)
	fmt.Fprintf(a.stdout, "Rollback to %s started.\n", started.Tag)
	if *noWait || started.Tag == "" {
		return exitOK
	}
	return a.followPanelUpdate(started.Tag, *timeout)
}

func (a *App) panelUpdateAgents(args []string) int {
	flags := a.newFlags("panel-update agents", "[-o FORMAT]")
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
	resp, ok := a.call(client, http.MethodGet, "/api/v1/system/agent-updates", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Target     string `json:"target_version"`
		AutoUpdate bool   `json:"auto_update"`
		Servers    []struct {
			ServerID        int64  `json:"server_id"`
			Name            string `json:"name"`
			IsLocal         bool   `json:"is_local"`
			AgentVersion    string `json:"agent_version"`
			UpdateAvailable bool   `json:"update_available"`
			Error           string `json:"error"`
			Update          *struct {
				State   string `json:"state"`
				Message string `json:"message"`
			} `json:"update"`
		} `json:"servers"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	return a.render(mode, data, func() {
		auto := "off"
		if data.AutoUpdate {
			auto = "on"
		}
		fmt.Fprintf(a.stdout, "Target version: %s   automatic agent updates: %s\n\n", data.Target, auto)
		fmt.Fprintf(a.stdout, "%-5s %-24s %-10s %s\n", "ID", "SERVER", "AGENT", "STATUS")
		for _, r := range data.Servers {
			status := "up to date"
			switch {
			case r.IsLocal:
				status = "panel host (updated with the panel)"
			case r.Update != nil && r.Update.State == "running":
				status = "updating…"
			case r.UpdateAvailable:
				status = "update available"
			}
			if r.Error != "" {
				status += " — " + r.Error
			} else if r.Update != nil && r.Update.State == "failed" {
				status += " — last update failed: " + r.Update.Message
			}
			fmt.Fprintf(a.stdout, "%-5d %-24s %-10s %s\n", r.ServerID, r.Name, r.AgentVersion, status)
		}
	})
}

func (a *App) panelUpdateAgentsApply(args []string) int {
	flags := a.newFlags("panel-update update-agents", "--all | --server ID [--server ID...] [--yes]")
	all := flags.Bool("all", false, "every remote server whose agent is older than the panel")
	yes := flags.Bool("yes", false, "do not ask for confirmation")
	var ids int64List
	flags.Var(&ids, "server", "server ID (repeatable)")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if !*all && len(ids) == 0 {
		fmt.Fprintln(a.stderr, "usage: sigardapanel panel-update update-agents --all | --server ID [--server ID...]")
		return exitUsage
	}
	if !*yes && !a.confirm("Update the selected agents to the panel's version? Each agent restarts for a few seconds.") {
		return exitError
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	body := map[string]any{"all": *all}
	if len(ids) > 0 {
		body["server_ids"] = []int64(ids)
	}
	resp, ok := a.call(client, http.MethodPost, "/api/v1/system/agent-updates", body, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Target  string `json:"target_version"`
		Results []struct {
			Name    string `json:"name"`
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"results"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if len(data.Results) == 0 {
		fmt.Fprintln(a.stdout, "All agents are up to date.")
		return exitOK
	}
	failed := 0
	for _, r := range data.Results {
		mark := "started"
		if !r.OK {
			mark, failed = "FAILED", failed+1
		}
		fmt.Fprintf(a.stdout, "  %-24s %s: %s\n", r.Name, mark, r.Message)
	}
	fmt.Fprintln(a.stdout, "Follow progress with: sigardapanel panel-update agents")
	if failed > 0 {
		return exitError
	}
	return exitOK
}

func (a *App) panelUpdateAgentsAuto(args []string) int {
	flags := a.newFlags("panel-update agents-auto", "on|off")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	if len(pos) != 1 || (pos[0] != "on" && pos[0] != "off") {
		fmt.Fprintln(a.stderr, "usage: sigardapanel panel-update agents-auto on|off")
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if _, ok := a.call(client, http.MethodPut, "/api/v1/system/agent-updates/settings", map[string]bool{"auto_update": pos[0] == "on"}, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintln(a.stdout, "Saved.")
	return exitOK
}

// int64List is a repeatable int64 flag.
type int64List []int64

func (l *int64List) String() string { return fmt.Sprint(*l) }
func (l *int64List) Set(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid server ID %q", v)
	}
	*l = append(*l, n)
	return nil
}

func (a *App) printPanelUpdateHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel panel-update <subcommand> [flags]   (super_admin)

Subcommands:
  status [-o FORMAT]    installed and latest release, auto-update, last run (default)
  check [-o FORMAT]     like status, but looks GitHub up again now
  apply [--yes] [--no-wait] [--timeout 45m]
                        install the latest signed release (backup + automatic
                        rollback) and follow it until it finishes
  auto on|off [--window START-END|any]
                        install new releases automatically, optionally only
                        between those hours (server time, e.g. --window 2-5)
  backups [-o FORMAT]   the backups made before each update
  rollback BACKUP [--yes] [--no-wait]
                        restore binary, web app and database from a backup
                        (changes since that backup are lost; the current state
                        is backed up first)
  agents [-o FORMAT]    agent version and update state of every server
  update-agents --all | --server ID [--server ID...] [--yes]
                        update remote agents to the panel's version
  agents-auto on|off    update remote agents automatically after the panel
`)
}
