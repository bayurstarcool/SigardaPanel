package commands

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"sigardapanel/internal/cli"
)

// Site is the CLI group for site management
func (a *App) Site(args []string) int {
	if len(args) == 0 {
		a.printSiteHelp()
		return exitOK
	}
	switch args[0] {
	case "list", "ls":
		return a.siteList(args[1:])
	case "get", "show":
		return a.siteGet(args[1:])
	case "create":
		return a.siteCreate(args[1:])
	case "update", "edit", "config":
		return a.siteUpdate(args[0], args[1:])
	case "delete", "rm":
		return a.siteDelete(args[1:])
	case "deploy":
		return a.siteAction(args[1:], "deploy", "deploy", "deploy job queued")
	case "suspend":
		return a.siteAction(args[1:], "suspend", "suspend", "suspend job queued")
	case "resume":
		return a.siteAction(args[1:], "resume", "resume", "resume job queued")
	case "reload-nginx":
		return a.siteAction(args[1:], "reload-nginx", "reload-nginx", "nginx reload job queued")
	case "setup-logrotate":
		return a.siteSetupLogrotate(args[1:])
	case "help", "--help", "-h":
		a.printSiteHelp()
		return exitOK
	default:
		return a.unknownSubcommand("site", args[0], a.printSiteHelp)
	}
}

func (a *App) siteList(args []string) int {
	flags := a.newFlags("site list", "[--server ID] [--status S] [--runtime R] [--search Q] [--owner ID] [--limit N] [--offset N] [-o FORMAT]")
	serverID := flags.Int64("server", 0, "filter by server ID")
	statusF := flags.String("status", "", "filter by status (pending|active|suspended|error|deleting)")
	runtime := flags.String("runtime", "", "filter by runtime")
	search := flags.String("search", "", "filter by name/domain substring")
	owner := flags.Int64("owner", 0, "filter by owner user ID")
	limit := flags.Int("limit", 0, "maximum number of sites (default: API default)")
	offset := flags.Int("offset", 0, "skip this many sites")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
	}

	q := url.Values{}
	if *serverID > 0 {
		q.Set("server_id", strconv.FormatInt(*serverID, 10))
	}
	if *statusF != "" {
		q.Set("status", *statusF)
	}
	if *runtime != "" {
		q.Set("runtime", *runtime)
	}
	if *search != "" {
		q.Set("search", *search)
	}
	if *owner > 0 {
		q.Set("owner_id", strconv.FormatInt(*owner, 10))
	}
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	if *offset > 0 {
		q.Set("offset", strconv.Itoa(*offset))
	}
	path := "/api/v1/sites"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodGet, path, nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		Sites      []map[string]any `json:"sites"`
		TotalCount int              `json:"total_count"`
	}
	if !a.decode(resp, &data) {
		return exitError
	}
	if data.Sites == nil {
		data.Sites = []map[string]any{}
	}

	return a.render(mode, data.Sites, func() {
		rows := make([][]string, 0, len(data.Sites))
		for _, s := range data.Sites {
			rows = append(rows, []string{
				cli.Cell(s["id"]),
				cli.Cell(s["name"]),
				cli.Cell(s["domain"]),
				cli.Cell(s["runtime"]),
				cli.Cell(s["status"]),
				cli.Cell(s["ssl_status"]),
				cli.Cell(s["server_id"]),
			})
		}
		cli.PrintTable(a.stdout, []string{"ID", "NAME", "DOMAIN", "RUNTIME", "STATUS", "SSL", "SERVER"}, rows)
		if data.TotalCount > len(data.Sites) {
			fmt.Fprintf(a.stdout, "showing %d of %d (use --limit/--offset)\n", len(data.Sites), data.TotalCount)
		}
	})
}

func (a *App) siteGet(args []string) int {
	flags := a.newFlags("site get", "<id> [-o FORMAT]")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, 0, "site")
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/sites/%d", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var site map[string]any
	if !a.decode(resp, &site) {
		return exitError
	}
	return a.render(mode, site, func() {
		a.printKV(site, "id", "name", "domain", "status", "runtime", "server_id", "owner_id",
			"system_user", "root_path", "php_version", "proxy_pass_url", "git_repo", "git_branch", "git_commit",
			"ssl_status", "ssl_issuer", "ssl_expires_at", "ssl_last_check", "created_at", "updated_at")
	})
}

func (a *App) siteCreate(args []string) int {
	flags := a.newFlags("site create", "--server ID --name NAME --domain DOMAIN [--runtime R] [flags]")
	serverID := flags.Int64("server", 0, "server ID (required)")
	name := flags.String("name", "", "site name (required; 1-64 chars, alphanumeric/_-)")
	domain := flags.String("domain", "", "site domain (required)")
	runtime := flags.String("runtime", "static", "runtime: static|php|node|go|python")
	rootPath := flags.String("root", "", "document root (default /home/<system_user>/htdocs/<domain>)")
	phpVersion := flags.String("php-version", "", "PHP version for php sites (e.g. 8.3)")
	proxyURL := flags.String("proxy-url", "", "upstream URL for reverse-proxied apps (proxy_pass_url)")
	systemUser := flags.String("system-user", "", "unix account owning the files (admins only; default: your username)")
	output := addOutputFlag(flags)
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}
	if *serverID <= 0 || *name == "" || *domain == "" {
		fmt.Fprintln(a.stderr, "--server, --name and --domain are required")
		return exitUsage
	}
	mode, ok := a.outputMode(*output)
	if !ok {
		return exitUsage
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
	if *phpVersion != "" {
		body["php_version"] = *phpVersion
	}
	if *proxyURL != "" {
		body["proxy_pass_url"] = *proxyURL
	}
	if *systemUser != "" {
		body["system_user"] = *systemUser
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, "/api/v1/sites", body, http.StatusCreated)
	if !ok {
		return exitError
	}
	var raw map[string]any
	if !a.decode(resp, &raw) {
		return exitError
	}
	if mode != cli.OutputTable {
		return a.render(mode, raw, nil)
	}
	var data struct {
		Site map[string]any `json:"site"`
		Job  struct {
			ID int64 `json:"id"`
		} `json:"job"`
		CloudflareDNS map[string]any `json:"cloudflare_dns,omitempty"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "site created (id=%s, domain=%s, status=%s)\n",
		cli.Cell(data.Site["id"]), cli.Cell(data.Site["domain"]), cli.Cell(data.Site["status"]))
	fmt.Fprintf(a.stdout, "provisioning job_id: %d (follow with: sigardapanel job watch %d)\n", data.Job.ID, data.Job.ID)
	if data.CloudflareDNS != nil {
		if p, _ := data.CloudflareDNS["provisioned"].(bool); p {
			fmt.Fprintf(a.stdout, "cloudflare DNS: record created in zone %s\n", cli.Cell(data.CloudflareDNS["zone"]))
		} else if e, _ := data.CloudflareDNS["error"].(string); e != "" {
			fmt.Fprintf(a.stdout, "cloudflare DNS: not provisioned (%s)\n", e)
		}
	}
	return exitOK
}

// siteUpdate changes the settings the API lets you edit after creation
// (POST /sites/:id/config): git repository/branch, reverse-proxy upstream and
// backup provider. Name, domain, runtime and root path are fixed at creation.
func (a *App) siteUpdate(verb string, args []string) int {
	flags := a.newFlags("site "+verb, "<id> [--git-repo URL] [--git-branch B] [--proxy-url URL] [--backup-provider ID]")
	idFlag := flags.Int64("id", 0, "site ID (alternative to the positional argument)")
	repo := flags.String("git-repo", "", "git repository (HTTPS URL)")
	branch := flags.String("git-branch", "", "git branch (default main when --git-repo is set)")
	proxyURL := flags.String("proxy-url", "", "reverse-proxy upstream URL (\"none\" clears it)")
	backupProvider := flags.Int64("backup-provider", -1, "backup provider ID for this site (0 = default/local)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "site")
	if !ok {
		return exitUsage
	}
	if *branch != "" && *repo == "" {
		fmt.Fprintln(a.stderr, "--git-branch requires --git-repo (the API only updates git settings together)")
		return exitUsage
	}

	body := map[string]any{}
	if *repo != "" {
		body["git_repo"] = *repo
		if *branch != "" {
			body["git_branch"] = *branch
		}
	}
	if *proxyURL != "" {
		if *proxyURL == "none" {
			body["proxy_pass_url"] = ""
		} else {
			body["proxy_pass_url"] = *proxyURL
		}
	}
	if *backupProvider >= 0 {
		body["backup_provider_id"] = *backupProvider
	}
	if len(body) == 0 {
		fmt.Fprintln(a.stderr, "nothing to update: pass --git-repo, --proxy-url or --backup-provider")
		fmt.Fprintln(a.stderr, "(name, domain, runtime and root path cannot be changed after creation)")
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/sites/%d/config", id), body, http.StatusOK)
	if !ok {
		return exitError
	}
	var data struct {
		WebhookSecret string `json:"webhook_secret"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "site %d updated\n", id)
	if data.WebhookSecret != "" {
		fmt.Fprintf(a.stdout, "webhook_secret: %s (shown once; deploy webhook: POST /api/v1/webhooks/deploy/%d)\n", data.WebhookSecret, id)
	}
	return exitOK
}

func (a *App) siteDelete(args []string) int {
	flags := a.newFlags("site delete", "<id>")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, 0, "site")
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodDelete, fmt.Sprintf("/api/v1/sites/%d", id), nil, http.StatusOK, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		JobID int64 `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "site %d deleted (cleanup job_id=%d)\n", id, data.JobID)
	return exitOK
}

// siteAction runs a POST /sites/:id/<endpoint> that queues a job.
func (a *App) siteAction(args []string, name, endpoint, done string) int {
	flags := a.newFlags("site "+name, "<id>")
	idFlag := flags.Int64("id", 0, "site ID (alternative to the positional argument)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "site")
	if !ok {
		return exitUsage
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/sites/%d/%s", id, endpoint), nil, http.StatusOK, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		JobID int64 `json:"job_id"`
	}
	a.decode(resp, &data)
	fmt.Fprintf(a.stdout, "%s for site %d (job_id=%d)\n", done, id, data.JobID)
	return exitOK
}

func (a *App) siteSetupLogrotate(args []string) int {
	flags := a.newFlags("site setup-logrotate", "<id>")
	idFlag := flags.Int64("id", 0, "site ID (alternative to the positional argument)")
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	id, ok := a.idArg(pos, *idFlag, "site")
	if !ok {
		return exitUsage
	}

	client, ok := a.apiClient()
	if !ok {
		return exitError
	}

	// Get site details to find domain
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/sites/%d", id), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var site struct {
		Domain   string `json:"domain"`
		ServerID int64  `json:"server_id"`
	}
	if !a.decode(resp, &site) {
		return exitError
	}

	// Execute logrotate setup via agent command (admin only).
	body := map[string]any{
		"command": fmt.Sprintf("bash -c 'cat > /etc/logrotate.d/sigarda_%s << \"EOF\"\n/var/log/nginx/%s.access.log /var/log/nginx/%s.error.log {\n    daily\n    missingok\n    rotate 14\n    compress\n    delaycompress\n    notifempty\n    create 0640 www-data adm\n    sharedscripts\n    postrotate\n        [ -f /var/run/nginx.pid ] && kill -USR1 $(cat /var/run/nginx.pid)\n    endscript\n}\nEOF'", site.Domain, site.Domain, site.Domain),
	}
	resp, ok = a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/servers/%d/command", site.ServerID), body, http.StatusOK, http.StatusAccepted)
	if !ok {
		return exitError
	}
	if !a.commandSucceeded(resp) {
		return exitError
	}

	fmt.Fprintf(a.stdout, "logrotate config created for %s\n", site.Domain)
	fmt.Fprintf(a.stdout, "  path: /etc/logrotate.d/sigarda_%s\n", site.Domain)
	return exitOK
}

// commandSucceeded checks the agent's {ok, output, error} result of
// POST /servers/:id/command and reports a failure.
func (a *App) commandSucceeded(resp *cli.APIResponse) bool {
	var r struct {
		OK     *bool  `json:"ok"`
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	a.decode(resp, &r)
	if r.OK != nil && !*r.OK {
		fmt.Fprintf(a.stderr, "command failed on server: %s\n", r.Error)
		if r.Output != "" {
			fmt.Fprintln(a.stderr, r.Output)
		}
		return false
	}
	return true
}

func (a *App) printSiteHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel site <subcommand> [flags]

Subcommands:
  list [--server ID] [--status S] [--runtime R] [--search Q] [--owner ID]
       [--limit N] [--offset N] [-o table|json|yaml]
                        list sites you can access
  get <id> [-o FORMAT]  show one site (alias: show)
  create --server ID --name NAME --domain DOMAIN [--runtime static|php|node|go|python]
         [--root PATH] [--php-version V] [--proxy-url URL] [--system-user U]
                        create a site (queues a provisioning job)
  update <id> [--git-repo URL [--git-branch B]] [--proxy-url URL|none]
         [--backup-provider ID]
                        change git / proxy / backup settings (alias: config, edit)
  delete <id>           delete a site and its files/databases (queues cleanup)
  deploy <id>           git pull + build (queues a deploy job)
  suspend <id>          take an active site offline, keeping its data (admin)
  resume <id>           bring a suspended site back online (admin)
  reload-nginx <id>     regenerate and reload the site's nginx vhost
  setup-logrotate <id>  install a logrotate config for the site's nginx logs (admin)

Commands that take <id> also accept --id ID. Run "sigardapanel site <subcommand> --help"
for all flags.
`)
}
