// Command server runs the Calc Queue HTTP API and its queue workers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/auth"
	"calcqueue/internal/queue"
	"calcqueue/internal/server"
	"calcqueue/internal/tasks"
	"calcqueue/migrations"
)

const (
	connectTimeout  = 60 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		logger.Error("Server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := server.LoadConfig(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	slog.Info("Starting server", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := migrations.Apply(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	workers := queue.New(pool, queue.Config{
		Workers:      cfg.Workers,
		OpDelay:      cfg.OpDelay,
		PollInterval: cfg.PollInterval,
	})
	if err := workers.Recover(ctx); err != nil {
		return fmt.Errorf("recover tasks: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           server.NewHandler(pool, auth.NewHandler(pool, cfg.SessionTTL), tasks.NewHandler(pool)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	claimCtx, stopClaims := context.WithCancel(ctx)
	defer stopClaims()
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancelExec()
	workersDone := make(chan error, 1)
	go func() { workersDone <- workers.Run(claimCtx, execCtx) }()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("Listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("serve http: %w", err)
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		stopClaims()
		cancelExec()
		return errors.Join(err, <-workersDone)
	}
	slog.Info("Shutting down")
	stopClaims()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	var errs []error
	if err := srv.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown http: %w", err))
	}
	select {
	case err := <-workersDone:
		errs = append(errs, err)
	case <-shutdownCtx.Done():
		cancelExec()
		errs = append(errs, <-workersDone)
	}
	return errors.Join(errs...)
}

// connect opens the pool and retries the first ping for up to connectTimeout.
func connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		err := pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		slog.Warn("Database not ready", "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, fmt.Errorf("connect to database: %w", err)
		case <-ticker.C:
		}
	}
}
