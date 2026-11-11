package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
	agentserver "sigardapanel/internal/agentserver"
	backendserver "sigardapanel/internal/backendserver"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/cli"
	"sigardapanel/internal/config"
)

type App struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

func New(stdout io.Writer, stderr io.Writer, stdin io.Reader) *App {
	return &App{stdout: stdout, stderr: stderr, stdin: stdin}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		a.printHelp()
		return 0
	case "--version", "-v":
		fmt.Fprintln(a.stdout, buildinfo.Version)
		return 0
	case "version":
		return a.Version(args[1:])
	case "whoami":
		return a.Whoami(args[1:])
	case "apikey", "api-key", "apikeys", "api-keys":
		return a.APIKey(args[1:])
	case "audit":
		return a.Audit(args[1:])
	case "notification", "notifications":
		return a.Notification(args[1:])
	case "api", "agent", "dev", "install":
		// These start long-running services or an interactive wizard that
		// writes the DB: never run them when the user only asked for help.
		if isHelpArg(args[1:]) {
			a.printServiceHelp(args[0])
			return 0
		}
		if len(args) > 1 {
			fmt.Fprintf(a.stderr, "%s takes no arguments (configure it with SIGARDAPANEL_* environment variables)\n", args[0])
			return 2
		}
		switch args[0] {
		case "api":
			return a.runService("api", backendserver.Run)
		case "agent":
			return a.runService("agent", agentserver.Run)
		case "dev":
			return a.runDev()
		default:
			return a.Install(args[1:])
		}
	case "doctor":
		return a.doctor(args[1:])
	case "init":
		return a.Init(args[1:])
	case "login":
		return a.Login(args[1:])
	case "logout":
		return a.Logout(args[1:])
	case "passwd":
		return a.Passwd(args[1:])
	case "server":
		return a.Server(args[1:])
	case "site":
		return a.Site(args[1:])
	case "ssl":
		return a.SSL(args[1:])
	case "job":
		return a.Job(args[1:])
	case "logs":
		return a.Logs(args[1:])
	case "backup":
		return a.Backup(args[1:])
	case "metrics":
		return a.Metrics(args[1:])
	case "channels", "channel":
		return a.Channels(args[1:])
	case "alerts":
		return a.Alerts(args[1:])
	case "cloudpanel":
		return a.CloudPanel(args[1:])
	case "user":
		return a.User(args[1:])
	case "db":
		return a.DB(args[1:])
	case "system":
		return a.System(args[1:])
	case "docker":
		return a.Docker(args[1:])
	case "git":
		return a.Git(args[1:])
	case "firewall":
		return a.Firewall(args[1:])
	case "redis":
		return a.Redis(args[1:])
	default:
		fmt.Fprintf(a.stderr, "unknown command: %s\n", args[0])
		fmt.Fprintln(a.stderr, "run 'sigardapanel help' for the list of commands")
		return 2
	}
}

func (a *App) runService(name string, run func(context.Context) error) int {
	// Two-way sync: env → DB (when env is set) or DB → env (when env is empty).
	// This ensures SIGARDAPANEL_AGENT_TOKEN and DB servers.agent_token never drift.
	if os.Getenv("SIGARDAPANEL_AGENT_TOKEN") == "" {
		if token, err := devSyncAgentToken(); err == nil {
			os.Setenv("SIGARDAPANEL_AGENT_TOKEN", token)
			fmt.Fprintf(a.stdout, "%s: auto-set SIGARDAPANEL_AGENT_TOKEN from DB\n", name)
		}
	} else if name == "api" || name == "agent" {
		if err := syncAgentTokenToDB(os.Getenv("SIGARDAPANEL_AGENT_TOKEN")); err != nil {
			fmt.Fprintf(a.stderr, "%s: WARN failed to sync agent token to DB: %v\n", name, err)
		} else {
			fmt.Fprintf(a.stdout, "%s: synced SIGARDAPANEL_AGENT_TOKEN -> DB servers.agent_token\n", name)
		}
	}

	// Check port availability before starting
	if name == "api" || name == "agent" {
		addr := ""
		if name == "api" {
			addr = config.CheckPortWarn("API", config.BackendServer().Addr)
		} else {
			addr = config.CheckPortWarn("Agent", config.AgentServer().Addr)
		}
		if addr == "" {
			fmt.Fprintf(os.Stderr, "%s: port already in use. Set %s_ADDR to a different port.\n", name, strings.ToUpper(name))
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error(name+" failed", "error", err)
		return 1
	}
	return 0
}

func (a *App) runDev() int {
	// Two-way sync for dev mode
	if os.Getenv("SIGARDAPANEL_AGENT_TOKEN") == "" {
		token, err := devSyncAgentToken()
		if err != nil {
			fmt.Fprintf(os.Stderr, "dev: failed to sync agent token: %v\n", err)
		} else {
			os.Setenv("SIGARDAPANEL_AGENT_TOKEN", token)
			fmt.Fprintf(a.stdout, "dev: auto-set SIGARDAPANEL_AGENT_TOKEN from DB\n")
		}
	} else {
		if err := syncAgentTokenToDB(os.Getenv("SIGARDAPANEL_AGENT_TOKEN")); err != nil {
			fmt.Fprintf(a.stderr, "dev: WARN failed to sync agent token to DB: %v\n", err)
		} else {
			fmt.Fprintf(a.stdout, "dev: synced SIGARDAPANEL_AGENT_TOKEN -> DB servers.agent_token\n")
		}
	}

	// Check port availability before starting
	apiAddr := config.CheckPortWarn("API", config.BackendServer().Addr)
	agentAddr := config.CheckPortWarn("Agent", config.AgentServer().Addr)
	if apiAddr == "" || agentAddr == "" {
		fmt.Fprintf(os.Stderr, "dev: port conflict detected. Set SIGARDAPANEL_API_ADDR / SIGARDAPANEL_AGENT_ADDR to different ports.\n")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- backendserver.Run(ctx)
	}()

	go func() {
		defer wg.Done()
		errCh <- agentserver.Run(ctx)
	}()

	fmt.Fprintln(a.stdout, "dev mode started: api + agent")
	err := <-errCh
	stop()
	wg.Wait()

	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("dev mode failed", "error", err)
		return 1
	}
	return 0
}

func (a *App) doctor(args []string) int {
	flags := a.newFlags("doctor", "[--api-url URL]")
	apiURL := flags.String("api-url", "", "API URL to check (default: the one the CLI would use)")
	if _, code, ok := a.parseFlags(flags, args); !ok {
		return code
	}

	problems := 0
	fmt.Fprintln(a.stdout, "SigardaPanel CLI doctor")
	fmt.Fprintf(a.stdout, "version: %s\n", buildinfo.Version)
	if cfgPath, err := cli.ConfigPath(); err == nil {
		fmt.Fprintf(a.stdout, "  config: %s\n", cfgPath)
	}

	client, _ := a.apiClient()
	if *apiURL != "" {
		client.BaseURL = strings.TrimRight(*apiURL, "/")
	}
	client.HTTP = &http.Client{Timeout: 5 * time.Second}
	fmt.Fprintf(a.stdout, "  api_url: %s\n", client.BaseURL)
	if _, status, err := client.Get("/api/v1/health"); err != nil {
		fmt.Fprintf(a.stdout, "  api health: UNREACHABLE (%v)\n", err)
		problems++
	} else if status != http.StatusOK {
		fmt.Fprintf(a.stdout, "  api health: HTTP %d\n", status)
		problems++
	} else {
		fmt.Fprintln(a.stdout, "  api health: ok")
		switch client.TokenSource {
		case "":
			fmt.Fprintln(a.stdout, "  auth: no token (run: sigardapanel login)")
		default:
			src := "token file"
			if client.TokenSource == "env" {
				src = "SIGARDAPANEL_TOKEN"
			}
			resp, status, err := client.Get("/api/v1/auth/me")
			if err == nil && status == http.StatusOK {
				var me struct {
					Username string `json:"username"`
					Role     string `json:"role"`
				}
				a.decode(resp, &me)
				fmt.Fprintf(a.stdout, "  auth: ok as %s (%s) via %s\n", me.Username, me.Role, src)
			} else {
				fmt.Fprintf(a.stdout, "  auth: token from %s rejected (HTTP %d)\n", src, status)
				problems++
			}
		}
	}

	// Agent token consistency check
	envTok := os.Getenv("SIGARDAPANEL_AGENT_TOKEN")
	if envTok != "" {
		fmt.Fprintf(a.stdout, "  env SIGARDAPANEL_AGENT_TOKEN: set (%d chars)\n", len(envTok))
		// Check DB match
		dbPath := config.DBPath()
		if _, err := os.Stat(dbPath); err == nil {
			db, err := sql.Open("sqlite", dbPath+"?mode=ro")
			if err == nil {
				defer db.Close()
				var dbTok sql.NullString
				if err := db.QueryRow("SELECT agent_token FROM servers WHERE agent_token IS NOT NULL AND agent_token != '' LIMIT 1").Scan(&dbTok); err == nil && dbTok.Valid {
					if dbTok.String == envTok {
						fmt.Fprintln(a.stdout, "  DB servers.agent_token: MATCH")
					} else {
						fmt.Fprintf(a.stdout, "  DB servers.agent_token: MISMATCH (%d chars, prefix %s...)\n", len(dbTok.String), dbTok.String[:8])
						fmt.Fprintln(a.stdout, "  FIX: run: sigardapanel server token --id 1")
						problems++
					}
				}
			}
		}
	} else {
		fmt.Fprintln(a.stdout, "  env SIGARDAPANEL_AGENT_TOKEN: not set")
		fmt.Fprintln(a.stdout, "  (will auto-sync from DB on startup)")
	}

	if problems > 0 {
		fmt.Fprintf(a.stdout, "\n%d problem(s) found\n", problems)
		return 1
	}
	fmt.Fprintln(a.stdout, "all checks passed")
	return 0
}

// syncAgentTokenToDB writes the agent token to the servers table in the DB,
// ensuring the local server row always matches the running env var.
func syncAgentTokenToDB(token string) error {
	dbPath := config.DBPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found at %s", dbPath)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	_, err = db.Exec("UPDATE servers SET agent_token = ? WHERE id = 1 AND (agent_token IS NULL OR agent_token != ?)", token, token)
	if err != nil {
		return fmt.Errorf("update agent_token: %w", err)
	}
	return nil
}

func (a *App) printHelp() {
	fmt.Fprint(a.stdout, `SigardaPanel `+buildinfo.Version+` — panel API, agent and CLI in one binary

Usage:
  sigardapanel <command> [subcommand] [flags]
  sigardapanel <command> --help        flags of a command or subcommand

Services (run on the panel host / managed servers):
  api                          run the panel API server
  agent                        run the server agent
  dev                          run API + agent in one process (development)
  install                      interactive setup wizard (writes the local DB)
  init [--user U] [--role R]   create an admin user directly in the local DB

Session:
  login [--api-url URL]        log in and save the session token
  logout                       revoke and delete the saved session token
  passwd                       change your own password (also: user passwd)
  whoami                       show the authenticated user
  version [--client]           CLI version and the server's version
  doctor                       check CLI config, API reachability and token

Resources:
  server     add|list|get|health|update|remove|token|doctor
  site       list|get|create|update|delete|deploy|suspend|resume|reload-nginx|setup-logrotate
  ssl        status|issue|renew|renew-all
  job        list|get|watch|cancel|logs
  backup     create|list|restore|delete
  db         create|list|update|delete
  db user    create|list|rotate-password|delete
  logs       tail
  metrics    latest|list|gpu
  system     disk-usage|updates
  user       create|list|update|reset-password|passwd|delete
  apikey     create|list|revoke                personal API keys
  audit      list|purge
  notification list|count|read|delete          your in-panel notifications
  channels   add|list|remove                   alert channels
  alerts     list                              alert history
  docker     ps|start|stop|restart|rm|logs|images|compose
  git        branches|log|rollback|checkout
  firewall   status|enable|disable|allow|deny|rules|delete|reset
  redis      stats|info|flush|flushdb
  cloudpanel scan|import-sites|import-site|import-ssl|doctor

Common flags:
  -o, --output table|json|yaml   output format of list/get commands
  -y, --yes                      skip confirmation prompts (destructive commands)

Environment (CLI):
  SIGARDAPANEL_API_URL         API base URL; overrides the URL saved by login
                               (default `+cli.DefaultAPIURL+`)
  SIGARDAPANEL_TOKEN           bearer token (e.g. a personal API key); overrides
                               the token saved by login
  SIGARDAPANEL_OUTPUT          default output format: table|json|yaml
  SIGARDAPANEL_USER / SIGARDAPANEL_PASSWORD   defaults for login
  XDG_CONFIG_HOME              config dir is $XDG_CONFIG_HOME/sigardapanel
                               (default ~/.config/sigardapanel)

Environment (services): SIGARDAPANEL_API_ADDR (:7700), SIGARDAPANEL_AGENT_ADDR (:7710),
  SIGARDAPANEL_DB_PATH, SIGARDAPANEL_AGENT_TOKEN, SIGARDAPANEL_ENCRYPTION_KEY, ...

Exit codes: 0 success, 1 error (API/network error, failed job, declined prompt),
2 usage error. Full reference: docs/CLI.md
`)
}

func (a *App) printServiceHelp(name string) {
	desc := map[string]string{
		"api":     "Run the panel API server (listens on SIGARDAPANEL_API_ADDR, default :7700).",
		"agent":   "Run the server agent (listens on SIGARDAPANEL_AGENT_ADDR, default :7710; auth: SIGARDAPANEL_AGENT_TOKEN).",
		"dev":     "Run API and agent in one process for development (SIGARDAPANEL_DEV=true disables rate limits).",
		"install": "Interactive setup wizard: migrates the DB at SIGARDAPANEL_DB_PATH and creates the first super_admin.",
	}[name]
	fmt.Fprintf(a.stdout, "usage: sigardapanel %s\n\n%s\nConfigured through SIGARDAPANEL_* environment variables; see docs/CLI.md.\n", name, desc)
}

// devSyncAgentToken reads the agent token from the database.
// In dev mode, both backend and agent run in the same process.
// The agent validates against SIGARDAPANEL_AGENT_TOKEN env var,
// so this function ensures the env var matches the DB server token.
func devSyncAgentToken() (string, error) {
	dbPath := config.DBPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database not found at %s", dbPath)
	}
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return "", fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	var token sql.NullString
	if err := db.QueryRow("SELECT agent_token FROM servers WHERE agent_token IS NOT NULL AND agent_token != '' LIMIT 1").Scan(&token); err != nil {
		return "", fmt.Errorf("query agent_token: %w", err)
	}
	if !token.Valid || token.String == "" {
		return "", fmt.Errorf("no agent_token found in servers table")
	}
	return token.String, nil
}
