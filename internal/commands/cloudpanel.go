package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sigardapanel/internal/db"
	"sigardapanel/internal/store"
)

// CloudPanelSite holds parsed data from a CloudPanel nginx config.
type CloudPanelSite struct {
	Domain       string   `json:"domain"`
	Aliases      []string `json:"aliases,omitempty"`
	RootPath     string   `json:"root_path,omitempty"`
	SSLCertPath  string   `json:"ssl_cert_path,omitempty"`
	SSLKeyPath   string   `json:"ssl_key_path,omitempty"`
	HasSSL       bool     `json:"has_ssl"`
	HasProxyPass bool     `json:"has_proxy_pass"`
	ProxyPassURL string   `json:"proxy_pass_url,omitempty"`
	ConfFile     string   `json:"conf_file"`
	Runtime      string   `json:"runtime"`
}

type CloudPanelScanReport struct {
	Sites             []CloudPanelSite `json:"sites"`
	SSLDomains        []string         `json:"ssl_domains"`
	Total             int              `json:"total"`
	ScanTime          string           `json:"scan_time"`
	CloudPanelVersion string           `json:"cloudpanel_version,omitempty"`
}

var (
	serverBlockRegex = regexp.MustCompile(`(?s)server\s*\{[^}]*\}`)
	serverNameRegex  = regexp.MustCompile(`server_name\s+([^;]+);`)
	rootRegex        = regexp.MustCompile(`root\s+([^;]+);`)
	sslCertRegex     = regexp.MustCompile(`ssl_certificate\s+([^;]+);`)
	sslKeyRegex      = regexp.MustCompile(`ssl_certificate_key\s+([^;]+);`)
	proxyPassRegex   = regexp.MustCompile(`proxy_pass\s+([^;]+);`)
	listenSSLRegex   = regexp.MustCompile(`listen\s+.*ssl`)
	phpSocketRegex   = regexp.MustCompile(`unix:.*php.*\.sock`)
)

// CloudPanel handles the `cloudpanel` subcommand for CloudPanel migration.
func (a *App) CloudPanel(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.stdout, `SigardaPanel CloudPanel Migration Tool

Usage:
  sigardapanel cloudpanel scan              scan CloudPanel sites/SSL
  sigardapanel cloudpanel import-sites      import all sites to SigardaPanel
  sigardapanel cloudpanel import-site <domain>  import single site
  sigardapanel cloudpanel import-ssl        import SSL certificates (dry-run by default)
  sigardapanel cloudpanel doctor            diagnose CloudPanel readiness

Flags (import-sites):
  --dry-run        preview only, no DB writes
  --db-path        path to SigardaPanel DB (default: sigardapanel.db)
  --server-id      target server ID (default: 1)
  --owner-id       target owner user ID (default: 1)
`)
		return 0
	}

	switch args[0] {
	case "scan":
		return a.cloudpanelScan(args[1:])
	case "import-sites":
		return a.cloudpanelImportSites(args[1:])
	case "import-site":
		return a.cloudpanelImportSite(args[1:])
	case "import-ssl":
		return a.cloudpanelImportSSL(args[1:])
	case "doctor":
		return a.cloudpanelDoctor(args[1:])
	default:
		fmt.Fprintf(a.stderr, "unknown cloudpanel subcommand: %s\n", args[0])
		return 2
	}
}

func (a *App) cloudpanelScan(args []string) int {
	report := &CloudPanelScanReport{
		ScanTime: time.Now().UTC().Format(time.RFC3339),
	}

	// Try to detect CloudPanel version
	if data, err := os.ReadFile("/usr/local/cloudpanel/VERSION"); err == nil {
		report.CloudPanelVersion = strings.TrimSpace(string(data))
	} else if data, err := os.ReadFile("/etc/cloudpanel-release"); err == nil {
		report.CloudPanelVersion = strings.TrimSpace(string(data))
	} else if data, err := os.ReadFile("/usr/local/cloudpanel/version.json"); err == nil {
		report.CloudPanelVersion = strings.TrimSpace(string(data))
	} else {
		if _, err := os.Stat("/usr/local/cloudpanel"); err == nil {
			report.CloudPanelVersion = "detected (version unknown)"
		}
	}

	// Scan nginx configs
	scanDirs := []string{
		"/etc/nginx/sites-enabled/",
		"/etc/nginx/sites-available/",
	}

	seen := make(map[string]bool)
	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
				continue
			}
			// Skip default.conf
			if entry.Name() == "default.conf" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			// Avoid duplicate scan
			if seen[path] {
				continue
			}
			seen[path] = true

			sites := parseNginxConfig(path)
			report.Sites = append(report.Sites, sites...)
		}
	}

	// Scan Let's Encrypt SSL domains
	letsDir := "/etc/letsencrypt/live/"
	if entries, err := os.ReadDir(letsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "README" {
				report.SSLDomains = append(report.SSLDomains, e.Name())
			}
		}
	}

	report.Total = len(report.Sites)

	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintln(a.stdout, string(out))
	return 0
}

// parseNginxConfig reads a nginx config file and extracts site blocks.
func parseNginxConfig(path string) []CloudPanelSite {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	blocks := serverBlockRegex.FindAllString(content, -1)
	if len(blocks) == 0 {
		// Maybe single line? Try parsing whole content as one block
		blocks = []string{content}
	}

	var sites []CloudPanelSite
	siteMap := make(map[string]*CloudPanelSite)

	for _, block := range blocks {
		names := extractServerNames(block)
		if len(names) == 0 {
			continue
		}

		primaryDomain := names[0]
		aliases := names[1:]

		rootPath := extractDirective(block, rootRegex)
		sslCert := extractDirective(block, sslCertRegex)
		sslKey := extractDirective(block, sslKeyRegex)
		hasSSL := listenSSLRegex.MatchString(block) || sslCert != ""
		proxyPass := extractDirective(block, proxyPassRegex)
		hasPHP := phpSocketRegex.MatchString(block)
		hasProxyPass := proxyPass != ""

		var runtime string
		if hasPHP {
			runtime = "php"
		} else if hasProxyPass {
			runtime = detectRuntimeFromProxyPass(proxyPass)
		} else if rootPath != "" {
			runtime = "static"
		} else {
			runtime = "redirect"
		}

		if existing, ok := siteMap[primaryDomain]; ok {
			// Merge: prefer SSL info from HTTPS blocks
			if hasSSL && !existing.HasSSL {
				existing.HasSSL = true
				existing.SSLCertPath = sslCert
				existing.SSLKeyPath = sslKey
			}
			if rootPath != "" && existing.RootPath == "" {
				existing.RootPath = rootPath
			}
			if proxyPass != "" && existing.ProxyPassURL == "" {
				existing.ProxyPassURL = proxyPass
				existing.HasProxyPass = true
			}
			existing.Runtime = mergeRuntimes(existing.Runtime, runtime)
			existing.Aliases = mergeAliases(existing.Aliases, aliases)
			continue
		}

		siteMap[primaryDomain] = &CloudPanelSite{
			Domain:       primaryDomain,
			Aliases:      aliases,
			RootPath:     rootPath,
			SSLCertPath:  sslCert,
			SSLKeyPath:   sslKey,
			HasSSL:       hasSSL,
			HasProxyPass: hasProxyPass,
			ProxyPassURL: proxyPass,
			Runtime:      runtime,
			ConfFile:     path,
		}
	}

	for _, s := range siteMap {
		sites = append(sites, *s)
	}
	return sites
}

func extractServerNames(block string) []string {
	match := serverNameRegex.FindStringSubmatch(block)
	if len(match) < 2 {
		return nil
	}
	parts := strings.Fields(match[1])
	var names []string
	for _, p := range parts {
		p = strings.TrimSuffix(p, ";")
		if p != "" && !strings.HasPrefix(p, "_") {
			names = append(names, p)
		}
	}
	return names
}

func extractDirective(block string, re *regexp.Regexp) string {
	match := re.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func detectRuntimeFromProxyPass(url string) string {
	url = strings.TrimSpace(url)
	// Default Node.js port range
	if strings.Contains(url, ":3000") || strings.Contains(url, ":3001") || strings.Contains(url, ":8000") || strings.Contains(url, ":8081") {
		return "node"
	}
	if strings.Contains(url, ":8058") || strings.Contains(url, ":9000") || strings.Contains(url, ":7710") {
		return "go"
	}
	if strings.Contains(url, ":5000") || strings.Contains(url, ":8001") {
		return "python"
	}
	return "proxy"
}

func mergeRuntimes(a, b string) string {
	if a == b {
		return a
	}
	// Prefer more specific runtime
	order := []string{"php", "node", "go", "python", "proxy", "static", "redirect"}
	rank := make(map[string]int)
	for i, r := range order {
		rank[r] = i
	}
	ra := rank[a]
	rb := rank[b]
	if ra < rb {
		return a
	}
	return b
}

func mergeAliases(existing, incoming []string) []string {
	set := make(map[string]bool)
	for _, a := range existing {
		set[a] = true
	}
	for _, a := range incoming {
		if !set[a] {
			existing = append(existing, a)
			set[a] = true
		}
	}
	return existing
}

func systemUserFromRoot(rootPath string) string {
	parts := strings.Split(strings.TrimPrefix(rootPath, "/"), "/")
	if len(parts) >= 2 && parts[0] == "home" {
		return parts[1]
	}
	return ""
}

func (a *App) cloudpanelImportSites(args []string) int {
	flags := flag.NewFlagSet("cloudpanel-import-sites", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dryRun := flags.Bool("dry-run", false, "preview only")
	dbPath := flags.String("db-path", "sigardapanel.db", "SigardaPanel DB path")
	serverID := flags.Int64("server-id", 1, "target server ID")
	ownerID := flags.Int64("owner-id", 1, "target owner user ID")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// First, scan CloudPanel
	report := &CloudPanelScanReport{ScanTime: time.Now().UTC().Format(time.RFC3339)}
	scanDirs := []string{"/etc/nginx/sites-enabled/", "/etc/nginx/sites-available/"}
	seen := make(map[string]bool)
	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") || entry.Name() == "default.conf" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			report.Sites = append(report.Sites, parseNginxConfig(path)...)
		}
	}

	if len(report.Sites) == 0 {
		fmt.Fprintln(a.stdout, "No CloudPanel sites found to import.")
		return 0
	}

	if *dryRun {
		fmt.Fprintf(a.stdout, "DRY RUN — Would import %d sites:\n\n", len(report.Sites))
		for _, s := range report.Sites {
			fmt.Fprintf(a.stdout, "  %-30s runtime=%-8s root=%s\n", s.Domain, s.Runtime, s.RootPath)
		}
		fmt.Fprintln(a.stdout, "\nSSL domains detected:", len(report.SSLDomains))
		return 0
	}

	// Open SigardaPanel DB
	database, err := db.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(a.stderr, "error opening DB %s: %v\n", *dbPath, err)
		return 1
	}
	defer database.Close()

	st := store.New(database)

	// Ensure server exists (create placeholder if not)
	ctx := context.Background()
	server, err := st.Servers.GetByID(ctx, *serverID)
	if err != nil || server == nil {
		srv := &store.Server{
			Name:     "CloudPanel Server (imported)",
			Hostname: "localhost",
			Status:   "active",
		}
		srv.ID = *serverID
		if createErr := st.Servers.Create(ctx, srv); createErr != nil {
			fmt.Fprintf(a.stderr, "error creating target server: %v\n", createErr)
			return 1
		}
		fmt.Fprintf(a.stdout, "Created server ID=%d\n", *serverID)
	}

	imported := 0
	skipped := 0
	errors := 0

	for _, site := range report.Sites {
		if site.Runtime == "redirect" {
			skipped++
			continue
		}

		// Check duplicate
		existing, _ := st.Sites.GetByDomain(ctx, site.Domain)
		if existing != nil {
			fmt.Fprintf(a.stdout, "  SKIP %s (already exists)\n", site.Domain)
			skipped++
			continue
		}

		newSite := &store.Site{
			ServerID:     *serverID,
			Name:         site.Domain,
			Domain:       site.Domain,
			Runtime:      site.Runtime,
			RootPath:     site.RootPath,
			Status:       "active",
			OwnerID:      sql.NullInt64{Int64: *ownerID, Valid: true},
			ManagedBy:    sql.NullString{String: "cloudpanel", Valid: true},
			SystemUser:   sql.NullString{String: systemUserFromRoot(site.RootPath), Valid: systemUserFromRoot(site.RootPath) != ""},
			ConfFile:     sql.NullString{String: site.ConfFile, Valid: site.ConfFile != ""},
			ProxyPassURL: sql.NullString{String: site.ProxyPassURL, Valid: site.ProxyPassURL != ""},
		}

		if site.HasSSL {
			newSite.SSLStatus = sql.NullString{String: "imported", Valid: true}
			newSite.SSLIssuer = sql.NullString{String: "Let's Encrypt (CloudPanel)", Valid: true}
			expiry := guessSSLRenewal(site.Domain)
			newSite.SSLExpiresAt = sql.NullTime{Time: expiry, Valid: true}
		}

		if createErr := st.Sites.Create(ctx, newSite); createErr != nil {
			fmt.Fprintf(a.stderr, "  ERR %s: %v\n", site.Domain, createErr)
			errors++
			continue
		}
		fmt.Fprintf(a.stdout, "  OK  %-30s runtime=%-8s id=%-5d\n", site.Domain, site.Runtime, newSite.ID)
		imported++
	}

	fmt.Fprintf(a.stdout, "\nSummary: %d imported, %d skipped, %d errors\n", imported, skipped, errors)
	return 0
}

func (a *App) cloudpanelImportSite(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(a.stderr, "usage: sigardapanel cloudpanel import-site <domain>")
		return 2
	}
	domain := args[0]

	// Scan and find matching domain
	scanDirs := []string{"/etc/nginx/sites-enabled/", "/etc/nginx/sites-available/"}
	seen := make(map[string]bool)
	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") || entry.Name() == "default.conf" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			sites := parseNginxConfig(path)
			for _, s := range sites {
				if s.Domain == domain || contains(s.Aliases, domain) {
					out, _ := json.MarshalIndent(s, "", "  ")
					fmt.Fprintln(a.stdout, string(out))
					return 0
				}
			}
		}
	}
	fmt.Fprintf(a.stderr, "domain %s not found in CloudPanel configs\n", domain)
	return 1
}

func (a *App) cloudpanelImportSSL(args []string) int {
	flags := flag.NewFlagSet("cloudpanel-import-ssl", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	dbPath := flags.String("db-path", "sigardapanel.db", "SigardaPanel DB path")
	dryRun := flags.Bool("dry-run", true, "dry-run only (default)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	letsDir := "/etc/letsencrypt/live/"
	entries, err := os.ReadDir(letsDir)
	if err != nil {
		fmt.Fprintf(a.stderr, "cannot read %s: %v\n", letsDir, err)
		return 1
	}

	var domains []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "README" {
			domains = append(domains, e.Name())
		}
	}

	if *dryRun {
		fmt.Fprintf(a.stdout, "DRY RUN — %d SSL domains found in Let's Encrypt:\n", len(domains))
		for _, d := range domains {
			certPath := filepath.Join(letsDir, d, "fullchain.pem")
			keyPath := filepath.Join(letsDir, d, "privkey.pem")
			info := checkSSLCert(certPath)
			fmt.Fprintf(a.stdout, "  %-30s cert=%s\n", d, info)
			_ = keyPath
		}
		return 0
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(a.stderr, "error opening DB: %v\n", err)
		return 1
	}
	defer database.Close()

	st := store.New(database)
	ctx := context.Background()
	imported := 0

	for _, domain := range domains {
		site, err := st.Sites.GetByDomain(ctx, domain)
		if err != nil || site == nil {
			// Try www prefix
			site, _ = st.Sites.GetByDomain(ctx, "www."+domain)
		}
		if site == nil {
			fmt.Fprintf(a.stdout, "  SKIP %s (no matching site in SigardaPanel)\n", domain)
			continue
		}

		expiry := guessSSLRenewal(domain)
		if updateErr := st.Sites.UpdateSSL(ctx, site.ID, "active", "Let's Encrypt (CloudPanel)", expiry); updateErr != nil {
			fmt.Fprintf(a.stderr, "  ERR %s: %v\n", domain, updateErr)
			continue
		}
		fmt.Fprintf(a.stdout, "  OK  %s (site ID=%d)\n", domain, site.ID)
		imported++
	}

	fmt.Fprintf(a.stdout, "\nSSL imported for %d sites\n", imported)
	return 0
}

func (a *App) cloudpanelDoctor(args []string) int {
	fmt.Fprintln(a.stdout, "=== CloudPanel Doctor ===")

	checks := []struct {
		name string
		ok   bool
		msg  string
	}{
		{"CloudPanel nginx configs", false, ""},
		{"SigardaPanel database", false, ""},
		{"Let's Encrypt SSL", false, ""},
		{"Site root directories", false, ""},
	}

	// Check 1: CloudPanel nginx
	if entries, err := os.ReadDir("/etc/nginx/sites-enabled/"); err == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") && e.Name() != "default.conf" {
				count++
			}
		}
		checks[0].ok = count > 0
		checks[0].msg = fmt.Sprintf("%d site configs found", count)
	} else {
		checks[0].msg = fmt.Sprintf("cannot read: %v", err)
	}

	// Check 2: DB
	if _, err := os.Stat("sigardapanel.db"); err == nil {
		checks[1].ok = true
		checks[1].msg = "sigardapanel.db exists"
	} else if _, err := os.Stat("/opt/sigardapanel/data/sigardapanel.db"); err == nil {
		checks[1].ok = true
		checks[1].msg = "sigardapanel.db exists at /opt/sigardapanel/data/"
	} else {
		checks[1].msg = "not found (run `sigardapanel init` first)"
	}

	// Check 3: SSL
	if entries, err := os.ReadDir("/etc/letsencrypt/live/"); err == nil {
		count := 0
		for _, e := range entries {
			if e.IsDir() && e.Name() != "README" {
				count++
			}
		}
		checks[2].ok = count > 0
		checks[2].msg = fmt.Sprintf("%d SSL cert dirs found", count)
	} else {
		checks[2].msg = fmt.Sprintf("cannot read: %v", err)
	}

	// Check 4: Site roots
	var rootDirs []string
	scanDirs := []string{"/etc/nginx/sites-enabled/", "/etc/nginx/sites-available/"}
	seen := make(map[string]bool)
	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") || entry.Name() == "default.conf" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			sites := parseNginxConfig(path)
			for _, s := range sites {
				if s.RootPath != "" {
					rootDirs = append(rootDirs, s.RootPath)
				}
			}
		}
	}
	accessible := 0
	inaccessible := 0
	for _, d := range rootDirs {
		if _, err := os.Stat(d); err == nil {
			accessible++
		} else {
			inaccessible++
		}
	}
	checks[3].ok = inaccessible == 0
	checks[3].msg = fmt.Sprintf("%d accessible, %d inaccessible", accessible, inaccessible)

	for _, c := range checks {
		status := "❌"
		if c.ok {
			status = "✅"
		}
		fmt.Fprintf(a.stdout, "  %s  %-40s %s\n", status, c.name, c.msg)
	}

	// Check disk space
	fmt.Fprintln(a.stdout, "\nRelevant disk paths:")
	for _, p := range []string{"/home", "/etc/letsencrypt", "/var/www"} {
		info := diskUsage(p)
		fmt.Fprintf(a.stdout, "  %-25s %s\n", p, info)
	}

	return 0
}

func checkSSLCert(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "not found"
	}
	content := string(data)
	// Try to extract expiry
	notAfterRe := regexp.MustCompile(`Not After\s*:\s*(.+)`)
	if m := notAfterRe.FindStringSubmatch(content); len(m) > 1 {
		return "expires " + strings.TrimSpace(m[1])
	}
	return fmt.Sprintf("%d bytes", len(data))
}

func guessSSLRenewal(domain string) time.Time {
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
	data, err := os.ReadFile(certPath)
	if err != nil {
		return time.Now().AddDate(0, 3, 0) // default 3 months
	}
	notAfterRe := regexp.MustCompile(`Not After\s*:\s*(.+)`)
	if m := notAfterRe.FindStringSubmatch(string(data)); len(m) > 1 {
		if t, err := time.Parse("Jan 2 15:04:05 2006 MST", strings.TrimSpace(m[1])); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", strings.TrimSpace(m[1])); err == nil {
			return t
		}
	}
	return time.Now().AddDate(0, 3, 0)
}

func diskUsage(path string) string {
	var stat os.FileInfo
	var err error
	stat, err = os.Stat(path)
	if err != nil {
		return "not found"
	}
	if stat.IsDir() {
		// Simple size check
		return "exists (" + stat.Mode().String() + ")"
	}
	return fmt.Sprintf("%d bytes", stat.Size())
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
