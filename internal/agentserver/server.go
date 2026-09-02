package agenthttpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	agenthttp "sigardapanel/internal/agenthttp"
	"sigardapanel/internal/config"
)

func Run(ctx context.Context) error {
	cfg := config.AgentServer()
	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      agenthttp.NewRouter(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			slog.Info("starting SigardaPanel agent with TLS", "addr", cfg.Addr)
			if err := server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
		} else {
			slog.Info("starting SigardaPanel agent", "addr", cfg.Addr)
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
		slog.Info("agent stopped")
		return nil
	case err := <-errCh:
		return err
	}
}
