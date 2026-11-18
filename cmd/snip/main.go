// Command snip runs the URL shortener HTTP server.
//
// Configuration (environment variables):
//
//	PORT          port to listen on (default 8080)
//	DATABASE_URL  postgres://user:pass@host:5432/db?sslmode=disable
//	              (unset = in-memory store; links are lost on restart)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weeeijin/snip/internal/server"
	"github.com/weeeijin/snip/internal/store"
)

func main() {
	// JSON logs: one object per line, easy for Loki or CloudWatch to parse.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// ctx is cancelled on Ctrl+C or SIGTERM (what Docker and Kubernetes send to stop a container).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		pg, err := store.NewPostgres(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer pg.Close()
		st = pg
		logger.Info("using postgres store")
	} else {
		st = store.NewMemory()
		logger.Warn("DATABASE_URL not set, using in-memory store (links are lost on restart)")
	}

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:    addr,
		Handler: server.New(st, logger),
		// Timeouts stop slow or stuck clients from holding connections forever.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err // e.g. port already in use
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting new requests and let in-flight ones
	// finish, so a deploy or scale-down doesn't cut users off mid-request.
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("stopped cleanly")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
