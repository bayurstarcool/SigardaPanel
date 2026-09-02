package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"sigardapanel/internal/cli"
)

// Docker is the CLI group for Docker container management
func (a *App) Docker(args []string) int {
	if len(args) == 0 {
		a.printDockerHelp()
		return 0
	}
	switch args[0] {
	case "ps", "list":
		return a.dockerList(args[1:])
	case "start":
		return a.dockerContainerAction(args[1:], "start")
	case "stop":
		return a.dockerContainerAction(args[1:], "stop")
	case "restart":
		return a.dockerContainerAction(args[1:], "restart")
	case "rm", "remove":
		return a.dockerContainerRemove(args[1:])
	case "logs":
		return a.dockerLogs(args[1:])
	case "images":
		return a.dockerImages(args[1:])
	case "compose":
		return a.dockerCompose(args[1:])
	case "help", "--help", "-h":
		a.printDockerHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown docker subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) dockerList(args []string) int {
	flags := flag.NewFlagSet("docker list", flag.ContinueOnError)
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/docker/containers", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Containers []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Image string `json:"image"`
			State string `json:"state"`
			Ports string `json:"ports"`
		} `json:"containers"`
		Total int `json:"total"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Containers)
		return 0
	}

	if len(data.Containers) == 0 {
		fmt.Fprintln(a.stdout, "No containers found.")
		return 0
	}

	rows := make([][]string, 0, len(data.Containers))
	for _, c := range data.Containers {
		state := c.State
		if state == "running" {
			state = "🟢 " + state
		} else {
			state = "🔴 " + state
		}
		rows = append(rows, []string{c.ID[:12], c.Name, c.Image, state, c.Ports})
	}
	cli.PrintTable(a.stdout, []string{"ID", "NAME", "IMAGE", "STATE", "PORTS"}, rows)
	fmt.Fprintf(a.stdout, "\nTotal: %d container(s)\n", data.Total)
	return 0
}

func (a *App) dockerContainerAction(args []string, action string) int {
	flags := flag.NewFlagSet("docker "+action, flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	containerID := flags.String("id", "", "container ID or name (required)")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 || *containerID == "" {
		fmt.Fprintln(a.stderr, "--server and --id are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post(
		fmt.Sprintf("/api/v1/servers/%d/docker/container/%s", *serverID, action),
		map[string]string{"container_id": *containerID},
	)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "container %s %sed\n", *containerID, action)
	return 0
}

func (a *App) dockerContainerRemove(args []string) int {
	flags := flag.NewFlagSet("docker remove", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	containerID := flags.String("id", "", "container ID or name (required)")
	force := flags.Bool("force", false, "force remove")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 || *containerID == "" {
		fmt.Fprintln(a.stderr, "--server and --id are required")
		return 2
	}

	confirmed, err := cli.Confirm(fmt.Sprintf("Remove container %s?", *containerID), false, a.stdin, a.stdout)
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

	resp, status, err := client.Post(
		fmt.Sprintf("/api/v1/servers/%d/docker/container/remove", *serverID),
		map[string]any{"container_id": *containerID, "force": *force},
	)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "container %s removed\n", *containerID)
	return 0
}

func (a *App) dockerLogs(args []string) int {
	flags := flag.NewFlagSet("docker logs", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	containerID := flags.String("id", "", "container ID or name (required)")
	lines := flags.Int("lines", 100, "number of log lines")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *serverID <= 0 || *containerID == "" {
		fmt.Fprintln(a.stderr, "--server and --id are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/servers/%d/docker/container/logs?container_id=%s&lines=%d", *serverID, *containerID, *lines)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Logs string `json:"logs"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintln(a.stdout, data.Logs)
	return 0
}

func (a *App) dockerImages(args []string) int {
	flags := flag.NewFlagSet("docker images", flag.ContinueOnError)
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

	resp, status, err := client.Get(fmt.Sprintf("/api/v1/servers/%d/docker/images", *serverID))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Images []map[string]string `json:"images"`
		Total  int                 `json:"total"`
	}
	json.Unmarshal(resp.Data, &data)

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		cli.PrintJSON(a.stdout, data.Images)
		return 0
	}

	if len(data.Images) == 0 {
		fmt.Fprintln(a.stdout, "No images found.")
		return 0
	}

	rows := make([][]string, 0, len(data.Images))
	for _, img := range data.Images {
		rows = append(rows, []string{img["repository"], img["tag"], img["size"], img["created"]})
	}
	cli.PrintTable(a.stdout, []string{"REPOSITORY", "TAG", "SIZE", "CREATED"}, rows)
	return 0
}

func (a *App) dockerCompose(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: docker compose up|down --server ID [--path PATH]")
		return 2
	}

	flags := flag.NewFlagSet("docker compose", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	path := flags.String("path", "docker-compose.yml", "compose file path")
	build := flags.Bool("build", false, "build images before starting (up only)")

	if err := flags.Parse(args[1:]); err != nil {
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

	action := args[0]
	var endpoint string
	var body map[string]any

	if action == "up" {
		endpoint = fmt.Sprintf("/api/v1/servers/%d/docker/compose/up", *serverID)
		body = map[string]any{"path": *path, "detached": true, "build": *build}
	} else if action == "down" {
		endpoint = fmt.Sprintf("/api/v1/servers/%d/docker/compose/down", *serverID)
		body = map[string]any{"path": *path}
	} else {
		fmt.Fprintf(a.stderr, "unknown compose action: %s (use up or down)\n", action)
		return 2
	}

	resp, status, err := client.Post(endpoint, body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var result struct {
		Message string `json:"message"`
		Output  string `json:"output"`
	}
	json.Unmarshal(resp.Data, &result)

	fmt.Fprintf(a.stdout, "%s\n", result.Message)
	if result.Output != "" {
		fmt.Fprintln(a.stdout, result.Output)
	}
	return 0
}

func (a *App) printDockerHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel docker <subcommand>

Subcommands:
  ps --server ID        list all containers
  start --server ID --id CID
                        start a container
  stop --server ID --id CID
                        stop a container
  restart --server ID --id CID
                        restart a container
  rm --server ID --id CID [--force]
                        remove a container
  logs --server ID --id CID [--lines N]
                        view container logs
  images --server ID    list Docker images
  compose up --server ID [--path PATH] [--build]
                        start compose services
  compose down --server ID [--path PATH]
                        stop compose services
`)
}
