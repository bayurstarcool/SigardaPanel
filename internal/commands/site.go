package commands

import (
	"encoding/json"
	"flag"
	"fmt"

	"sigardapanel/internal/cli"
)

// Site is the CLI group for site management
func (a *App) Site(args []string) int {
	if len(args) == 0 {
		a.printSiteHelp()
		return 0
	}
	switch args[0] {
	case "list", "ls":
		return a.siteList(args[1:])
	case "create":
		return a.siteCreate(args[1:])
	case "update", "edit":
		return a.siteUpdate(args[1:])
	case "delete", "rm":
		return a.siteDelete(args[1:])
	case "deploy":
		return a.siteDeploy(args[1:])
	case "config":
		return a.siteConfig(args[1:])
	case "setup-logrotate":
		return a.siteSetupLogrotate(args[1:])
	case "help", "--help", "-h":
		a.printSiteHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown site subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) siteList(args []string) int {
	flags := flag.NewFlagSet("site list", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "filter by server ID")
	output := flags.String("output", "", "output format (table|json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := "/api/v1/sites"
	if *serverID > 0 {
		path += fmt.Sprintf("?server_id=%d", *serverID)
	}

	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}

	mode := cli.GetOutputMode()
	if *output == "json" {
		mode = cli.OutputJSON
	}

	if mode == cli.OutputJSON {
		if err := cli.PrintJSON(a.stdout, data.Sites); err != nil {
			fmt.Fprintf(a.stderr, "encode: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([][]string, 0, len(data.Sites))
	for _, s := range data.Sites {
		rows = append(rows, []string{
			fmt.Sprintf("%v", s["id"]),
			fmt.Sprintf("%v", s["name"]),
			fmt.Sprintf("%v", s["domain"]),
			fmt.Sprintf("%v", s["runtime"]),
			fmt.Sprintf("%v", s["status"]),
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "NAME", "DOMAIN", "RUNTIME", "STATUS"}, rows)
	return 0
}

func (a *App) siteCreate(args []string) int {
	flags := flag.NewFlagSet("site create", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	serverID := flags.Int64("server", 0, "server ID (required)")
	name := flags.String("name", "", "site name")
	domain := flags.String("domain", "", "site domain")
	runtime := flags.String("runtime", "static", "runtime (static|php|node|go|python)")
	rootPath := flags.String("root", "", "root path (default /home/<system_user>/htdocs/<domain>)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *serverID <= 0 || *name == "" || *domain == "" {
		fmt.Fprintln(a.stderr, "server, name, domain are required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]any{
		"server_id": *serverID,
		"name":      *name,
		"domain":    *domain,
		"runtime":   *runtime,
	}
	if *rootPath != "" {
		body["root_path"] = *rootPath
	}

	resp, status, err := client.Post("/api/v1/sites", body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 201 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		ID     int64  `json:"id"`
		Domain string `json:"domain"`
		JobID  int64  `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintf(a.stdout, "site created (id=%d, domain=%s)\n", data.ID, data.Domain)
	fmt.Fprintf(a.stdout, "job_id: %d\n", data.JobID)
	return 0
}

func (a *App) siteDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: site delete <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Delete("/api/v1/sites/" + id)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	fmt.Fprintf(a.stdout, "site %s deleted (cleanup job queued)\n", id)
	return 0
}

func (a *App) siteDeploy(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.stderr, "usage: site deploy <id>")
		return 2
	}
	id := args[0]

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	resp, status, err := client.Post("/api/v1/sites/"+id+"/deploy", nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct {
		JobID int64 `json:"job_id"`
	}
	json.Unmarshal(resp.Data, &data)

	fmt.Fprintf(a.stdout, "deploy job queued (job_id=%d)\n", data.JobID)
	return 0
}

func (a *App) siteConfig(args []string) int {
	flags := flag.NewFlagSet("site config", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "site ID")
	repo := flags.String("git-repo", "", "git repository URL")
	branch := flags.String("git-branch", "main", "git branch")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *id <= 0 {
		fmt.Fprintln(a.stderr, "id is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	body := map[string]any{
		"git_repo":   *repo,
		"git_branch": *branch,
	}

	resp, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/config", *id), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data map[string]any
	json.Unmarshal(resp.Data, &data)
	if secret, ok := data["webhook_secret"].(string); ok && secret != "" {
		fmt.Fprintf(a.stdout, "webhook_secret: %s\n", secret)
	}
	fmt.Fprintln(a.stdout, "config updated")
	return 0
}

func (a *App) siteUpdate(args []string) int {
	flags := flag.NewFlagSet("site update", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "site ID (required)")
	name := flags.String("name", "", "site name")
	domain := flags.String("domain", "", "site domain")
	runtime := flags.String("runtime", "", "runtime (static|php|node|go|python)")
	rootPath := flags.String("root", "", "root path")
	gitRepo := flags.String("git-repo", "", "git repository URL")
	gitBranch := flags.String("git-branch", "", "git branch")
	statusFlag := flags.String("status", "", "site status (active|pending|error)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		fmt.Fprintln(a.stderr, "usage: site update --id <id> [--name N] [--domain D] [--runtime R] [--root PATH] [--git-repo URL] [--git-branch B] [--status S]")
		return 2
	}

	// Check at least one field to update
	if *name == "" && *domain == "" && *runtime == "" && *rootPath == "" && *gitRepo == "" && *gitBranch == "" && *statusFlag == "" {
		fmt.Fprintln(a.stderr, "at least one field is required")
		fmt.Fprintln(a.stderr, "usage: site update --id <id> [--name N] [--domain D] [--runtime R] [--root PATH] [--git-repo URL] [--git-branch B] [--status S]")
		return 2
	}

	if *runtime != "" && *runtime != "static" && *runtime != "php" && *runtime != "node" && *runtime != "go" && *runtime != "python" {
		fmt.Fprintf(a.stderr, "invalid runtime %q: must be static, php, node, go, or python\n", *runtime)
		return 2
	}

	if *statusFlag != "" && *statusFlag != "active" && *statusFlag != "pending" && *statusFlag != "error" {
		fmt.Fprintf(a.stderr, "invalid status %q: must be active, pending, or error\n", *statusFlag)
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	// Build update payload — use config endpoint for git, PATCH for basic fields
	body := make(map[string]any)
	if *name != "" {
		body["name"] = *name
	}
	if *domain != "" {
		body["domain"] = *domain
	}
	if *runtime != "" {
		body["runtime"] = *runtime
	}
	if *rootPath != "" {
		body["root_path"] = *rootPath
	}
	if *statusFlag != "" {
		body["status"] = *statusFlag
	}

	// Update basic fields via PATCH
	if len(body) > 0 {
		resp, status, err := client.Patch(fmt.Sprintf("/api/v1/sites/%d", *id), body)
		if err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		if status != 200 {
			return a.printAPIError(resp, status)
		}
	}

	// Update git config if provided
	if *gitRepo != "" || *gitBranch != "" {
		gitBody := make(map[string]any)
		if *gitRepo != "" {
			gitBody["git_repo"] = *gitRepo
		}
		if *gitBranch != "" {
			gitBody["git_branch"] = *gitBranch
		}
		resp, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/config", *id), gitBody)
		if err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		if status != 200 {
			return a.printAPIError(resp, status)
		}

		var configData struct {
			WebhookSecret string `json:"webhook_secret"`
		}
		json.Unmarshal(resp.Data, &configData)
		if configData.WebhookSecret != "" {
			fmt.Fprintf(a.stdout, "  webhook_secret: %s\n", configData.WebhookSecret)
		}
	}

	fmt.Fprintf(a.stdout, "site updated (id=%d)\n", *id)
	return 0
}

func (a *App) siteSetupLogrotate(args []string) int {
	flags := flag.NewFlagSet("site setup-logrotate", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	id := flags.Int64("id", 0, "site ID (required)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *id <= 0 {
		fmt.Fprintln(a.stderr, "--id is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	// Get site details to find domain
	resp, status, err := client.Get(fmt.Sprintf("/api/v1/sites/%d", *id))
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var site struct {
		Domain   string `json:"domain"`
		ServerID int64  `json:"server_id"`
	}
	json.Unmarshal(resp.Data, &site)

	// Execute logrotate setup via agent command
	body := map[string]any{
		"command": fmt.Sprintf("bash -c 'cat > /etc/logrotate.d/sigarda_%s << \"EOF\"\n/var/log/nginx/%s.access.log /var/log/nginx/%s.error.log {\n    daily\n    missingok\n    rotate 14\n    compress\n    delaycompress\n    notifempty\n    create 0640 www-data adm\n    sharedscripts\n    postrotate\n        [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)\n    endscript\n}\nEOF'", site.Domain, site.Domain, site.Domain),
	}
	_, status, err = client.Post(fmt.Sprintf("/api/v1/servers/%d/command", site.ServerID), body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 && status != 202 {
		return a.printAPIError(nil, status)
	}

	fmt.Fprintf(a.stdout, "logrotate config created for %s\n", site.Domain)
	fmt.Fprintf(a.stdout, "  path: /etc/logrotate.d/sigarda_%s\n", site.Domain)
	return 0
}

func (a *App) printSiteHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel site <subcommand>

Subcommands:
  list [--server ID] [--output json]
                        list sites
  create --server ID --name X --domain Y [--runtime R] [--root PATH]
                        create site
  update --id ID [--name N] [--domain D] [--runtime R] [--root PATH]
          [--git-repo URL] [--git-branch B] [--status S]
                        update site settings
  delete <id>           delete site
  deploy <id>           deploy site
  config --id ID --git-repo URL [--git-branch BRANCH]
                        update site git config
  setup-logrotate --id ID
                        setup log rotation for a site
`)
}
