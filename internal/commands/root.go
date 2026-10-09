package commands

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	agentserver "sigardapanel/internal/agentserver"
	backendserver "sigardapanel/internal/backendserver"
	"sigardapanel/internal/config"
	_ "modernc.org/sqlite"
)

// Version is overridden at release time: -ldflags "-X sigardapanel/internal/commands.Version=1.2.3"
var Version = "1.1.0"

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
	case "version", "--version", "-v":
		fmt.Fprintln(a.stdout, Version)
		return 0
	case "api":
		return a.runService("api", backendserver.Run)
	case "agent":
		return a.runService("agent", agentserver.Run)
	case "dev":
		return a.runDev()
	case "doctor":
		return a.doctor(args[1:])
	case "init":
		return a.Init(args[1:])
	case "install":
		return a.Install(args[1:])
	case "login":
		return a.Login(args[1:])
	case "logout":
		return a.Logout(args[1:])
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
	case "channels":
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
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	apiURL := flags.String("api-url", os.Getenv("SIGARDAPANEL_API_URL"), "SigardaPanel API URL")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	problems := 0
	fmt.Fprintln(a.stdout, "SigardaPanel CLI doctor")
	fmt.Fprintf(a.stdout, "version: %s\n", Version)
	if *apiURL == "" {
		fmt.Fprintln(a.stdout, "  api_url: not configured")
		problems++
	} else {
		fmt.Fprintf(a.stdout, "  api_url: %s\n", *apiURL)
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
	fmt.Fprint(a.stdout, `SigardaPanel CLI

Usage:
  sigardapanel <command> [flags]

Commands:
  api                          run API server
  agent                        run agent service
  dev                          run API + agent locally
  install                      setup wizard (interactive)
  init                         create admin user (direct DB)
  login                        authenticate CLI
  logout                       revoke current CLI token
  server add|list|update|remove|doctor
                               manage servers
  site create|list|update|delete|deploy
                               manage sites
  ssl issue|renew|status       manage SSL
  logs tail                    tail logs
  job list|watch|cancel|logs   manage jobs
  backup create|list|delete|restore
                               manage backups
  db create|list|update|delete manage databases
  db user create|list|rotate-password|delete
                               manage database users
  channels add|list|remove     manage alert channels
  alerts list                  view alert history
  metrics latest|list          view server metrics
  user create|list|reset-password|delete
                               manage users
  system disk-usage|updates   system management
  docker ps|start|stop|restart|rm|logs|images|compose
                               Docker container management
  git branches|log|rollback|checkout
                               Git deploy management
  firewall status|enable|disable|allow|deny|rules|delete|reset
                               Firewall (UFW) management
  redis stats|info|flush|flushdb
                               Redis management
  cloudpanel scan|import-sites|import-ssl|doctor
                               CloudPanel migration
  doctor                       diagnose CLI configuration
  version                      print version

Global environment:
  SIGARDAPANEL_API_ADDR        API listen addr, default :7700
  SIGARDAPANEL_AGENT_ADDR      agent listen addr, default :7710
  SIGARDAPANEL_API_URL         API base URL
  SIGARDAPANEL_TOKEN           API token
  SIGARDAPANEL_AGENT_TOKEN     agent bearer token (auto-synced from DB if empty)
  SIGARDAPANEL_OUTPUT          table|json|yaml
`)
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
