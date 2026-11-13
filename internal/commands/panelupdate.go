package commands

import (
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
`)
}
