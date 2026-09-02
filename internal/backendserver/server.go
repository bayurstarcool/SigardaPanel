package backendserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sigardapanel/internal/member"
	"strings"
	"time"

	"sigardapanel/internal/agentclient"
	apihttp "sigardapanel/internal/apihttp"
	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/dbserver"
	sjobs "sigardapanel/internal/jobs"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

// Run starts the backend API server.
func Run(ctx context.Context) error {
	cfg := config.BackendServer()

	// Open database
	database, err := db.Open(config.DBPath())
	if err != nil {
		return err
	}
	defer db.Close(database)

	slog.Info("database opened", "path", config.DBPath())

	// Run migrations
	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		return err
	}
	slog.Info("migrations applied")

	// Initialize store
	s := store.New(database)

	// Register panel host as local managed server on first start.
	if err := ensureLocalServer(ctx, s); err != nil {
		return err
	}

	// Seed default database server if missing (from CloudPanel credentials)
	seedDatabaseServer(s)

	// Start worker
	agentClient := agentclient.New(config.BackendAgentURL(), config.BackendAgentToken())
	worker := sjobs.NewWorker(s, agentClient)
	worker.Start(ctx)
	defer worker.Stop()
	// Start backup scheduler (checks due schedules every 60 seconds)
	sjobs.RunScheduler(ctx, s, 60*time.Second)

	// Start agent health checker (checks every 60 seconds)
	healthChecker := sjobs.NewHealthChecker(s)
	healthChecker.Start(ctx)
	defer healthChecker.Stop()

	// Start metrics collector (queries each server's agent every 60s)
	metricsCollector := sjobs.NewMetricsCollector(s)
	metricsCollector.Start(ctx)
	defer metricsCollector.Stop()

	// Start periodic session cleanup (every 1 hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Sessions.CleanupExpired(context.Background()); err != nil {
					slog.Warn("session cleanup failed", "error", err)
				} else {
					slog.Debug("expired sessions cleaned up")
				}
			}
		}
	}()

	// Open member database (PostgreSQL, separate from panel)
	memberDB, err := member.ConnectPostgreSQL(
		Getenv("MEMBER_DB_HOST", "localhost"),
		Getenv("MEMBER_DB_PORT", "5432"),
		Getenv("MEMBER_DB_USER", "sigardapanel"),
		Getenv("MEMBER_DB_PASSWORD", "sigardapanel123"),
		Getenv("MEMBER_DB_NAME", "sigardapanel_member"),
	)
	if err != nil {
		return err
	}
	defer memberDB.Close()

	memberStore := member.NewStore(memberDB)
	if err := memberStore.Migrate(); err != nil {
		return err
	}
	slog.Info("member database connected", "host", "localhost", "db", "sigardapanel_member")

	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      apihttp.NewRouter(s, database, memberDB),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			slog.Info("starting SigardaPanel API with TLS", "addr", cfg.Addr)
			if err := server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
		} else {
			slog.Info("starting SigardaPanel API", "addr", cfg.Addr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		slog.Info("api stopped")
		return nil
	case err := <-errCh:
		return err
	}
}

// seedDatabaseServer creates the default database server entry from CloudPanel credentials.
func seedDatabaseServer(s *store.Store) {
	ctx := context.Background()

	// Check if already seeded
	existing, err := s.DBStore.GetDefault(ctx)
	if err != nil {
		slog.Warn("dbserver: check default failed", "error", err)
		return
	}
	if existing != nil {
		slog.Info("dbserver: default already exists", "id", existing.ID, "engine", existing.Engine)
		return
	}

	// Use CloudPanel MySQL root credentials
	password := config.MySQLRootPassword()
	if password == "" {
		slog.Warn("dbserver: MYSQL_ROOT_PASSWORD not set, skipping seed")
		return
	}

	svr := &dbserver.Server{
		Name:      "Local MySQL (CloudPanel)",
		Engine:    "MariaDB",
		Host:      "127.0.0.1",
		Port:      3306,
		Username:  "root",
		IsActive:  true,
		IsDefault: true,
	}

	encPassword, err := s.DBStore.EncryptPassword(password)
	if err != nil {
		slog.Warn("dbserver: encrypt failed", "error", err)
		return
	}
	svr.PasswordEncrypted = encPassword

	if err := s.DBStore.SetServer(ctx, svr); err != nil {
		slog.Warn("dbserver: seed failed", "error", err)
		return
	}
	slog.Info("dbserver: default database server seeded", "id", svr.ID)
}

// Getenv returns env value or default.
func Getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ensureLocalServer adds the panel host as its first managed server.
// Existing entries are preserved; matching uses hostname or agent URL.
func ensureLocalServer(ctx context.Context, s *store.Store) error {
	servers, err := s.Servers.List(ctx)
	if err != nil {
		return fmt.Errorf("list servers for local detection: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "localhost"
	}
	agentURL := strings.TrimRight(config.AgentServerURL(), "/")
	for _, server := range servers {
		if server.Hostname == hostname || (server.AgentURL.Valid && strings.TrimRight(server.AgentURL.String, "/") == agentURL) {
			return nil
		}
	}

	token := config.AgentToken()
	if token == "" {
		return fmt.Errorf("local server not registered: SIGARDAPANEL_AGENT_TOKEN is empty")
	}

	localIP := config.LocalServerIP()
	if localIP == "" {
		localIP = detectLocalIPv4()
	}
	for _, server := range servers {
		if localIP != "" && server.IPAddress.Valid && server.IPAddress.String == localIP {
			return nil
		}
	}

	server := &store.Server{
		Name:        "Local Server",
		Hostname:    hostname,
		AgentURL:    sql.NullString{String: agentURL, Valid: true},
		IPAddress:   sql.NullString{String: localIP, Valid: localIP != ""},
		SSHPort:     22,
		Description: "Panel host auto-detected as primary local server",
		Status:      "active",
		AgentToken:  token,
		Provider:    sql.NullString{String: "Local", Valid: true},
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		return fmt.Errorf("register local server: %w", err)
	}
	slog.Info("local server auto-registered", "server_id", server.ID, "hostname", hostname, "agent_url", agentURL)
	return nil
}

func detectLocalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip != nil && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}
