package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"sigardapanel/internal/cli"
)

// Notification is the CLI group for the caller's in-panel notifications
// (the bell in the web UI). Alert channels are managed with "channels".
func (a *App) Notification(args []string) int {
	if len(args) == 0 {
		a.printNotificationHelp()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return a.notificationList(args[1:])
	case "count":
		return a.notificationCount(args[1:])
	case "read":
		return a.notificationRead(args[1:])
	case "delete", "rm":
		return a.notificationDelete(args[1:])
	case "help", "--help", "-h":
		a.printNotificationHelp()
		return exitOK
	default:
		return a.unknownSubcommand("notification", args[0], a.printNotificationHelp)
	}
}

func (a *App) notificationList(args []string) int {
	flags := a.newFlags("notification list", "[--unread] [--limit N] [--offset N] [-o FORMAT]")
	unreadOnly := flags.Bool("unread", false, "only unread notifications (filtered from the fetched page)")
	limit := flags.Int("limit", 50, "maximum number of notifications")
	offset := flags.Int("offset", 0, "skip this many notifications")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}
	q := url.Values{}
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	if *offset > 0 {
		q.Set("offset", strconv.Itoa(*offset))
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/notifications?"+q.Encode(), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Notifications []map[string]any `json:"notifications"`
		Total         int              `json:"total"`
		Unread        int              `json:"unread"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	items := make([]map[string]any, 0, len(data.Notifications))
	for _, n := range data.Notifications {
		if *unreadOnly {
			if read, _ := n["read"].(bool); read {
				continue
			}
		}
		items = append(items, n)
	}
	return a.render(mode, items, func() {
		rows := make([][]string, 0, len(items))
		for _, n := range items {
			read := " "
			if r, _ := n["read"].(bool); !r {
				read = "*"
			}
			rows = append(rows, []string{read, cli.Cell(n["id"]), cli.Cell(n["severity"]), cli.Cell(n["title"]), cli.Cell(n["message"]), cli.Cell(n["created_at"])})
		}
		cli.PrintTable(a.stdout, []string{"", "ID", "SEVERITY", "TITLE", "MESSAGE", "CREATED"}, rows)
		fmt.Fprintf(a.stdout, "%d unread, %d total (* = unread)\n", data.Unread, data.Total)
	})
}

func (a *App) notificationCount(args []string) int {
	flags := a.newFlags("notification count", "")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, "/api/v1/notifications/unread-count", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Count int `json:"count"`
	}
	a.decode(resp, &data)
	fmt.Fprintln(a.stdout, data.Count)
	return exitOK
}

func (a *App) notificationRead(args []string) int {
	flags := a.newFlags("notification read", "<id> | --all")
	all := flags.Bool("all", false, "mark all notifications as read")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if *all {
		if len(pos) > 0 {
			fmt.Fprintln(a.stderr, "pass either <id> or --all")
			return exitUsage
		}
		if _, ok := a.call(client, http.MethodPost, "/api/v1/notifications/read-all", nil, http.StatusOK); !ok {
			return exitError
		}
		fmt.Fprintln(a.stdout, "all notifications marked as read")
		return exitOK
	}
	id, ok := a.idArg(flags, pos, 0, "notification")
	if !ok {
		return exitUsage
	}
	if _, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/notifications/%d/read", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "notification %d marked as read\n", id)
	return exitOK
}

func (a *App) notificationDelete(args []string) int {
	flags := a.newFlags("notification delete", "<id> | --read")
	readOnes := flags.Bool("read", false, "delete all notifications that are already read")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	if *readOnes {
		if len(pos) > 0 {
			fmt.Fprintln(a.stderr, "pass either <id> or --read")
			return exitUsage
		}
		if _, ok := a.call(client, http.MethodDelete, "/api/v1/notifications/read", nil, http.StatusOK); !ok {
			return exitError
		}
		fmt.Fprintln(a.stdout, "read notifications deleted")
		return exitOK
	}
	id, ok := a.idArg(flags, pos, 0, "notification")
	if !ok {
		return exitUsage
	}
	if _, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/notifications/%d", id), nil, http.StatusOK); !ok {
		return exitError
	}
	fmt.Fprintf(a.stdout, "notification %d deleted\n", id)
	return exitOK
}

func (a *App) printNotificationHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel notification <subcommand> [flags]   (alias: notifications)

Your own in-panel notifications (the bell in the web UI).

Subcommands:
  list [--unread] [--limit N] [--offset N] [-o FORMAT]
                        list notifications, newest first
  count                 print the number of unread notifications
  read <id> | --all     mark one or all notifications as read
  delete <id> | --read  delete one notification, or all read ones
`)
}
