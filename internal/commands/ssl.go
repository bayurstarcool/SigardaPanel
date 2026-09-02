package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"sigardapanel/internal/cli"
)

// SSL is the CLI group for SSL certificate management
func (a *App) SSL(args []string) int {
	if len(args) == 0 {
		a.printSSLHelp()
		return 0
	}
	switch args[0] {
	case "status":
		return a.sslStatus(args[1:])
	case "issue":
		return a.sslIssue(args[1:])
	case "renew":
		return a.sslRenew(args[1:])
	case "renew-all":
		return a.sslRenewAll(args[1:])
	case "help", "--help", "-h":
		a.printSSLHelp()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown ssl subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) sslStatus(args []string) int {
	flags := flag.NewFlagSet("ssl status", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *siteID <= 0 {
		fmt.Fprintln(a.stderr, "--site is required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/sites/%d/ssl", *siteID)
	resp, status, err := client.Get(path)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return 1
	}
	if err := cli.PrintJSON(a.stdout, data); err != nil {
		fmt.Fprintf(a.stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) sslIssue(args []string) int {
	flags := flag.NewFlagSet("ssl issue", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	email := flags.String("email", "", "contact email for Let's Encrypt")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *siteID <= 0 || *email == "" {
		fmt.Fprintln(a.stderr, "--site and --email required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/sites/%d/ssl/issue", *siteID)
	resp, status, err := client.Post(path, map[string]string{"email": *email})
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct{ JobID int64 `json:"job_id"` }
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "ssl issue job queued (job_id=%d)\n", data.JobID)
	return 0
}

func (a *App) sslRenew(args []string) int {
	flags := flag.NewFlagSet("ssl renew", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	siteID := flags.Int64("site", 0, "site ID (required)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *siteID <= 0 {
		fmt.Fprintln(a.stderr, "--site required")
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	path := fmt.Sprintf("/api/v1/sites/%d/ssl/renew", *siteID)
	resp, status, err := client.Post(path, nil)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 202 {
		return a.printAPIError(resp, status)
	}

	var data struct{ JobID int64 `json:"job_id"` }
	json.Unmarshal(resp.Data, &data)
	fmt.Fprintf(a.stdout, "ssl renew job queued (job_id=%d)\n", data.JobID)
	return 0
}

func (a *App) sslRenewAll(args []string) int {
	flags := flag.NewFlagSet("ssl renew-all", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dryRun := flags.Bool("dry-run", false, "preview only, don't queue jobs")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	client, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}

	// Get all sites
	resp, status, err := client.Get("/api/v1/sites")
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return 1
	}
	if status != 200 {
		return a.printAPIError(resp, status)
	}

	var siteData struct {
		Sites []struct {
			ID          int64  `json:"id"`
			Domain      string `json:"domain"`
			SSLStatus   string `json:"ssl_status"`
			SSLExpires  string `json:"ssl_expires_at"`
		} `json:"sites"`
	}
	json.Unmarshal(resp.Data, &siteData)

	// Filter sites with SSL expiring within 30 days
	var expiring []struct {
		ID     int64
		Domain string
		Expiry string
	}
	for _, s := range siteData.Sites {
		if s.SSLStatus == "active" && s.SSLExpires != "" {
			// Parse expiry date
			expiry, err := time.Parse(time.RFC3339, s.SSLExpires)
			if err != nil {
				continue
			}
			daysLeft := int(time.Until(expiry).Hours() / 24)
			if daysLeft <= 30 {
				expiring = append(expiring, struct {
					ID     int64
					Domain string
					Expiry string
				}{s.ID, s.Domain, fmt.Sprintf("%s (%d days left)", s.SSLExpires[:10], daysLeft)})
			}
		}
	}

	if len(expiring) == 0 {
		fmt.Fprintln(a.stdout, "No SSL certificates expiring within 30 days.")
		return 0
	}

	// Display expiring certs
	fmt.Fprintf(a.stdout, "SSL certificates expiring within 30 days (%d):\n\n", len(expiring))
	rows := make([][]string, 0, len(expiring))
	for _, e := range expiring {
		rows = append(rows, []string{
			fmt.Sprintf("%d", e.ID),
			e.Domain,
			e.Expiry,
		})
	}
	cli.PrintTable(a.stdout, []string{"ID", "DOMAIN", "EXPIRES"}, rows)

	if *dryRun {
		fmt.Fprintln(a.stdout, "\nDry run — no jobs queued.")
		return 0
	}

	// Queue renew jobs
	queued := 0
	for _, e := range expiring {
		_, status, err := client.Post(fmt.Sprintf("/api/v1/sites/%d/ssl/renew", e.ID), nil)
		if err != nil {
			fmt.Fprintf(a.stderr, "  %s: %v\n", e.Domain, err)
			continue
		}
		if status != 202 {
			fmt.Fprintf(a.stderr, "  %s: HTTP %d\n", e.Domain, status)
			continue
		}
		queued++
		fmt.Fprintf(a.stdout, "  ✓ %s — renew job queued\n", e.Domain)
	}

	fmt.Fprintf(a.stdout, "\n%d renewal job(s) queued.\n", queued)
	return 0
}

func (a *App) printSSLHelp() {
	fmt.Fprint(a.stdout, `Usage: sigardapanel ssl <subcommand>

Subcommands:
  status --site ID       show SSL status for a site
  issue --site ID --email E
                         issue Let's Encrypt certificate
  renew --site ID        renew certificate
  renew-all [--dry-run]  renew all expiring certificates (≤30 days)
`)
}
