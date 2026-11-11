package commands

import (
	"fmt"
	"net/http"
	"time"

	"sigardapanel/internal/cli"
)

// SSL is the CLI group for SSL certificate management
func (a *App) SSL(args []string) int {
	if len(args) == 0 {
		a.printSSLHelp()
		return exitOK
	}
	switch args[0] {
	case "status":
		return a.sslStatus(args[1:])
	case "issue":
		return a.sslJob(args[1:], "issue")
	case "renew":
		return a.sslJob(args[1:], "renew")
	case "renew-all":
		return a.sslRenewAll(args[1:])
	case "help", "--help", "-h":
		a.printSSLHelp()
		return exitOK
	default:
		return a.unknownSubcommand("ssl", args[0], a.printSSLHelp)
	}
}

func (a *App) sslStatus(args []string) int {
	flags := a.newFlags("ssl status", "<site-id> [-o FORMAT]")
	siteFlag := flags.Int64("site", 0, "site ID (alternative to the positional argument)")
	output := addOutputFlag(flags)
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	siteID, ok := a.idArg(flags, pos, *siteFlag, "site")
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
	resp, ok := a.call(client, http.MethodGet, fmt.Sprintf("/api/v1/sites/%d/ssl", siteID), nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var data map[string]any
	if !a.decode(resp, &data) {
		return exitError
	}
	return a.render(mode, data, func() {
		a.printKV(data, "site_id", "domain", "ssl_status", "ssl_issuer", "ssl_expires_at", "ssl_last_check")
	})
}

// sslJob queues an ssl.issue or ssl.renew job for one site.
func (a *App) sslJob(args []string, action string) int {
	synopsis := "<site-id>"
	if action == "issue" {
		synopsis = "<site-id> [--email E]"
	}
	flags := a.newFlags("ssl "+action, synopsis)
	siteFlag := flags.Int64("site", 0, "site ID (alternative to the positional argument)")
	var email *string
	if action == "issue" {
		email = flags.String("email", "", "Let's Encrypt contact email (default: site owner's, then your account email)")
	}
	pos, code, ok := a.parseFlags(flags, args)
	if !ok {
		return code
	}
	siteID, ok := a.idArg(flags, pos, *siteFlag, "site")
	if !ok {
		return exitUsage
	}

	var body any
	if email != nil && *email != "" {
		body = map[string]string{"email": *email}
	}
	client, ok := a.apiClient()
	if !ok {
		return exitError
	}
	resp, ok := a.call(client, http.MethodPost, fmt.Sprintf("/api/v1/sites/%d/ssl/%s", siteID, action), body, http.StatusAccepted)
	if !ok {
		return exitError
	}
	var data struct {
		JobID int64  `json:"job_id"`
		Email string `json:"email"`
	}
	a.decode(resp, &data)
	if data.Email != "" {
		fmt.Fprintf(a.stdout, "ssl %s job queued (job_id=%d, email=%s)\n", action, data.JobID, data.Email)
	} else {
		fmt.Fprintf(a.stdout, "ssl %s job queued (job_id=%d)\n", action, data.JobID)
	}
	return exitOK
}

// sslRenewAll asks the API to queue renewals for every certificate that
// expires within 30 days (POST /api/v1/ssl/renew-all, admin). --dry-run only
// lists the candidates from the sites you can see.
func (a *App) sslRenewAll(args []string) int {
	flags := a.newFlags("ssl renew-all", "[--dry-run] [-o FORMAT]")
	dryRun := flags.Bool("dry-run", false, "only list certificates expiring within 30 days; queue nothing")
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

	if !*dryRun {
		resp, ok := a.call(client, http.MethodPost, "/api/v1/ssl/renew-all", nil, http.StatusOK)
		if !ok {
			return exitError
		}
		var data map[string]any
		if !a.decode(resp, &data) {
			return exitError
		}
		return a.render(mode, data, func() {
			fmt.Fprintf(a.stdout, "%s certificate(s) expire within 30 days: %s renewal job(s) queued, %s skipped (inactive site or renewal already pending)\n",
				cli.Cell(data["total"]), cli.Cell(data["queued"]), cli.Cell(data["skipped"]))
		})
	}

	resp, ok := a.call(client, http.MethodGet, "/api/v1/sites?limit=1000", nil, http.StatusOK)
	if !ok {
		return exitError
	}
	var siteData struct {
		Sites []struct {
			ID         int64  `json:"id"`
			Domain     string `json:"domain"`
			Status     string `json:"status"`
			SSLStatus  string `json:"ssl_status"`
			SSLExpires string `json:"ssl_expires_at"`
		} `json:"sites"`
	}
	if !a.decode(resp, &siteData) {
		return exitError
	}

	type candidate struct {
		ID         int64  `json:"site_id"`
		Domain     string `json:"domain"`
		SiteStatus string `json:"site_status"`
		ExpiresAt  string `json:"ssl_expires_at"`
		DaysLeft   int    `json:"days_left"`
	}
	expiring := []candidate{}
	for _, s := range siteData.Sites {
		if s.SSLExpires == "" {
			continue
		}
		expiry, err := time.Parse(time.RFC3339, s.SSLExpires)
		if err != nil {
			continue
		}
		daysLeft := int(time.Until(expiry).Hours() / 24)
		if daysLeft <= 30 {
			expiring = append(expiring, candidate{s.ID, s.Domain, s.Status, expiry.UTC().Format("2006-01-02"), daysLeft})
		}
	}

	return a.render(mode, expiring, func() {
		if len(expiring) == 0 {
			fmt.Fprintln(a.stdout, "No SSL certificates expire within 30 days.")
			return
		}
		rows := make([][]string, 0, len(expiring))
		for _, e := range expiring {
			rows = append(rows, []string{fmt.Sprint(e.ID), e.Domain, e.SiteStatus, e.ExpiresAt, fmt.Sprint(e.DaysLeft)})
		}
		cli.PrintTable(a.stdout, []string{"SITE", "DOMAIN", "STATUS", "EXPIRES", "DAYS_LEFT"}, rows)
		fmt.Fprintln(a.stdout, "\nDry run: no jobs queued. Only active sites are renewed.")
	})
}

func (a *App) printSSLHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel ssl <subcommand> [flags]

Subcommands:
  status <site-id> [-o FORMAT]
                        show certificate status, issuer and expiry for a site
  issue <site-id> [--email E]
                        issue a Let's Encrypt certificate (queues a job; email
                        defaults to the site owner's, then yours)
  renew <site-id>       renew the site's certificate (queues a job)
  renew-all [--dry-run] [-o FORMAT]
                        queue renewals for all certificates expiring within 30
                        days (admin); --dry-run only lists them

Commands that take <site-id> also accept --site ID.
`)
}
