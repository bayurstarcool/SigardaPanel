package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"
	"member/internal/handler"
	"member/internal/store"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Run() error {
	dbHost := getenv("DB_HOST", "localhost")
	dbPort := getenv("DB_PORT", "5432")
	dbUser := getenv("DB_USER", "sigardapanel")
	dbPass := getenv("DB_PASSWORD", "sigardapanel123")
	dbName := getenv("DB_NAME", "sigardapanel_member")

	dsn := "host=" + dbHost + " port=" + dbPort + " user=" + dbUser + " password=" + dbPass + " dbname=" + dbName + " sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	if err := db.Ping(); err != nil {
		return err
	}
	slog.Info("postgresql connected", "host", dbHost, "db", dbName)

	s := store.New(db)
	if err := s.Migrate(); err != nil {
		return err
	}

	h := handler.New(s)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"https://panel.sigarda.dev", "http://localhost:7720"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
	}))
	e.Use(middleware.RequestID())

	// API routes
	api := e.Group("/api")
	api.POST("/register", h.Register)
	api.POST("/login", h.Login)
	api.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	auth := api.Group("")
	auth.Use(requireAuth(s))
	auth.GET("/me", h.Me)
	auth.PUT("/profile", h.UpdateProfile)
	auth.PUT("/plan", h.UpdatePlan)

	license := api.Group("/license")
	license.POST("/validate", h.ValidateLicense)
	license.POST("/activate", h.ActivateLicense)
	license.GET("/info", h.LicenseInfo)

	// SPA - serve index.html for all non-API routes
	indexPath := filepath.Join("static", "index.html")
	
	// Try to find the right path
	if _, err := os.Stat(indexPath); err != nil {
		// Try from binary location
		exe, _ := os.Executable()
		dir := filepath.Dir(exe)
		indexPath = filepath.Join(dir, "static", "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			// Try absolute path
			indexPath = "/root/SigardaPanel-Enterprise/member/static/index.html"
		}
	}
	
	slog.Info("serving index", "path", indexPath)
	
	serveIndex := func(c echo.Context) error {
		return c.File(indexPath)
	}

	e.GET("/", serveIndex)
	e.GET("/login", serveIndex)
	e.GET("/register", serveIndex)
	e.GET("/dashboard", serveIndex)
	e.GET("/pricing", serveIndex)

	addr := ":" + getenv("PORT", "7800")
	slog.Info("member portal starting", "addr", addr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: addr, Handler: e}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func requireAuth(s *store.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(401, "missing authorization header")
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				return echo.NewHTTPError(401, "invalid authorization format")
			}
			sess, err := s.GetSession(token)
			if err != nil {
				return echo.NewHTTPError(401, "invalid or expired session")
			}
			c.Set("member_id", sess.MemberID)
			return next(c)
		}
	}
}
