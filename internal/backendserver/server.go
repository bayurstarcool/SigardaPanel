package backendserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	"sigardapanel/internal/publicip"
	"sigardapanel/internal/secrets"
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

	// Pick the encryption key before anything is encrypted: generates a key
	// file on new installs, warns loudly when the weak fallback is in use.
	secrets.InitEncryptionKey(ctx, database)

	// Initialize store
	s := store.New(database)

	// TOTP secrets written by older builds are plaintext; encrypt them now
	// that the encryption key is settled.
	if n, err := s.TwoFactor.EncryptLegacyTOTPSecrets(ctx); err != nil {
		slog.Warn("2fa: encrypt legacy TOTP secrets", "error", err)
	} else if n > 0 {
		slog.Info("2fa: encrypted legacy plaintext TOTP secrets", "count", n)
	}

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

	// Start security collector (threat rules every 10 minutes)
	securityCollector := sjobs.NewSecurityCollector(s)
	securityCollector.Start(ctx)
	defer securityCollector.Stop()

	// Look for new panel releases (notifies super admins; installs them
	// when auto-update is enabled under Settings).
	apihttp.StartPanelUpdateChecker(ctx, s, database)

	// Check in with the master panel about daily (license, version).
	apihttp.StartCheckin(ctx, s)

	// Check users' disk usage against their limits (notifies at 80% / 100%).
	apihttp.StartDiskQuotaChecker(ctx, s)

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

	// Member area (PostgreSQL, separate from the panel DB) is optional: it is
	// enabled only when MEMBER_DB_HOST is set. Once configured, a connection
	// failure is fatal so a misconfiguration is never silently ignored.
	var memberDB *sql.DB
	if host := Getenv("MEMBER_DB_HOST", ""); host != "" {
		dbName := Getenv("MEMBER_DB_NAME", "sigardapanel_member")
		memberDB, err = member.ConnectPostgreSQL(
			host,
			Getenv("MEMBER_DB_PORT", "5432"),
			Getenv("MEMBER_DB_USER", "sigardapanel"),
			Getenv("MEMBER_DB_PASSWORD", ""),
			dbName,
		)
		if err != nil {
			return err
		}
		defer memberDB.Close()

		memberStore := member.NewStore(memberDB)
		if err := memberStore.Migrate(); err != nil {
			return err
		}
		slog.Info("member database connected", "host", host, "db", dbName)
	} else {
		slog.Info("member database not configured (MEMBER_DB_HOST unset), member area disabled")
	}

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: apihttp.NewRouter(s, database, memberDB),
		// Bodies (file uploads, backups) may take minutes; the headers may not.
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
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
			go fixLocalServerIP(context.WithoutCancel(ctx), s, server.ID)
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
	go fixLocalServerIP(context.WithoutCancel(ctx), s, server.ID)
	return nil
}

// publicURLIPv4 is the host of SIGARDAPANEL_PUBLIC_URL when it is a public
// IPv4 literal ("" for a domain or anything else).
func publicURLIPv4() string {
	u, err := url.Parse(strings.TrimSpace(os.Getenv("SIGARDAPANEL_PUBLIC_URL")))
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && publicip.IsPublicIPv4(ip) {
		return ip.To4().String()
	}
	return ""
}

// detectLocalPublicIP is replaced in tests.
var detectLocalPublicIP = func(ctx context.Context) (string, error) {
	r, err := publicip.Detect(ctx, nil)
	return r.IP, err
}

// fixLocalServerIP replaces the panel host's address on the Servers page
// with its public IPv4 when it holds a private (or no) address: cloud VMs
// only see the private one on their interfaces, and site DNS points at
// this address. A public address (set by hand or SIGARDAPANEL_LOCAL_IP) is
// never changed.
func fixLocalServerIP(ctx context.Context, s *store.Store, serverID int64) {
	if config.LocalServerIP() != "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	srv, err := s.Servers.GetByID(ctx, serverID)
	if err != nil || srv == nil {
		return
	}
	if ip := net.ParseIP(strings.TrimSpace(srv.IPAddress.String)); srv.IPAddress.Valid && publicip.IsPublicIPv4(ip) {
		return
	}
	// The address the installer chose for the panel URL is the one people
	// use: take it when it is a public IPv4 (a host with several addresses,
	// or a NAT whose outbound address differs, could detect another one).
	pub, err := publicURLIPv4(), error(nil)
	if pub == "" {
		pub, err = detectLocalPublicIP(ctx)
	}
	if err != nil || !publicip.IsPublicIPv4(net.ParseIP(pub)) {
		slog.Warn("local server: public IP not detected; set it on the Servers page", "server_id", serverID, "error", err)
		return
	}
	old := srv.IPAddress.String
	srv.IPAddress = sql.NullString{String: pub, Valid: true}
	if err := s.Servers.Update(ctx, srv); err != nil {
		slog.Warn("local server: store public IP", "server_id", serverID, "error", err)
		return
	}
	slog.Info("local server: address set to its public IP", "server_id", serverID, "old", old, "ip", pub)
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
