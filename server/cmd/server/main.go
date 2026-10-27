// Command server runs the Calc Queue HTTP API and its queue workers.
//
// Startup order: config, database connect (retried for up to 60 s),
// migrations, recovery of interrupted tasks, then HTTP and workers. On SIGTERM
// it stops new claims and HTTP intake at once, gives active work up to 10 s,
// then cancels execution; interrupted tasks stay running and are recovered by
// the next start. The whole stop stays well inside the 30 s pod grace.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	// connectTimeout bounds the wait for the database at startup.
	connectTimeout = 60 * time.Second
	// connectRetry is the pause between failed startup pings.
	connectRetry = time.Second
	// shutdownGrace is the time active HTTP requests and evaluations get after
	// a stop signal before execution is canceled.
	shutdownGrace = 10 * time.Second
	// stopTimeout bounds the wait for workers after execution is canceled.
	stopTimeout = 5 * time.Second
	// readHeaderTimeout bounds slow request headers.
	readHeaderTimeout = 5 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := notifyContext()
	err := run(ctx, os.Getenv)
	stop()
	if err != nil {
		slog.Error("Server stopped", "error", err)
		os.Exit(1)
	}
	slog.Info("Server stopped")
}

// notifyContext returns a context canceled by SIGINT or SIGTERM.
func notifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

// run reads the configuration through getenv, starts the server and blocks
// until ctx is canceled and the shutdown has finished.
func run(ctx context.Context, getenv func(string) string) error {
	cfg, err := server.LoadConfig(getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	slog.InfoContext(ctx, "Starting server", "config", cfg)

	a, err := start(ctx, cfg, connectTimeout)
	if err != nil {
		return err
	}
	defer a.close()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr(), err)
	}
	return a.serve(ctx, ln, shutdownGrace)
}

// app is the started server: migrated, recovered and ready to serve.
type app struct {
	pool    *pgxpool.Pool
	workers *queue.Pool
	handler http.Handler
}

// start connects to the database within connectBudget, applies migrations and
// then recovers interrupted tasks. No worker runs before it returns.
func start(ctx context.Context, cfg server.Config, connectBudget time.Duration) (*app, error) {
	pool, err := connect(ctx, cfg.DatabaseURL, connectBudget)
	if err != nil {
		return nil, err
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	workers := queue.New(pool, queue.Config{
		Workers:      cfg.Workers,
		OpDelay:      cfg.OpDelay,
		PollInterval: cfg.PollInterval,
	})
	if err := workers.Recover(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("recover tasks: %w", err)
	}
	return &app{
		pool:    pool,
		workers: workers,
		handler: server.NewHandler(pool, auth.NewHandler(pool, cfg.SessionTTL), tasks.NewHandler(pool)),
	}, nil
}

// close releases the database pool.
func (a *app) close() { a.pool.Close() }

// serve runs HTTP on ln and the workers until ctx is canceled. Then it stops
// new claims and HTTP intake, waits up to grace for active requests and
// evaluations, cancels execution and waits up to stopTimeout for the workers.
// Tasks interrupted this way stay running for the next start; that is a
// warning, not a failed stop.
func (a *app) serve(ctx context.Context, ln net.Listener, grace time.Duration) error {
	srv := &http.Server{Handler: a.handler, ReadHeaderTimeout: readHeaderTimeout}

	claimCtx, stopClaims := context.WithCancel(ctx)
	defer stopClaims()
	// Execution outlives ctx by up to grace; it is canceled explicitly.
	execCtx, cancelExec := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelExec()
	workersDone := make(chan error, 1)
	go func() { workersDone <- a.workers.Run(claimCtx, execCtx) }()

	serveErr := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Listening", "addr", ln.Addr().String())
		serveErr <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		stopClaims()
		cancelExec()
		return errors.Join(fmt.Errorf("serve http: %w", err), waitWorkers(workersDone))
	}
	slog.InfoContext(ctx, "Shutting down", "grace", grace)
	stopClaims()

	graceCtx, cancelGrace := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancelGrace()
	var errs []error
	if err := srv.Shutdown(graceCtx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown http: %w", err))
		if err := srv.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close http: %w", err))
		}
	}
	select {
	case err := <-workersDone:
		errs = append(errs, workerError(err))
	case <-graceCtx.Done():
		slog.WarnContext(ctx, "Shutdown grace expired, canceling active tasks", "grace", grace)
		cancelExec()
		errs = append(errs, waitWorkers(workersDone))
	}
	return errors.Join(errs...)
}

// waitWorkers waits up to stopTimeout for the result of queue.Pool.Run.
func waitWorkers(done <-chan error) error {
	timer := time.NewTimer(stopTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return workerError(err)
	case <-timer.C:
		return fmt.Errorf("queue workers did not stop within %v", stopTimeout)
	}
}

// workerError maps the result of queue.Pool.Run to the stop error: tasks
// interrupted by the shutdown are expected and recovered at the next start.
func workerError(err error) error {
	if errors.Is(err, queue.ErrInterrupted) {
		slog.Warn("Active tasks left running for recovery at the next start", "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("run queue workers: %w", err)
	}
	return nil
}

// connect opens the pool and retries the first ping for up to budget.
func connect(ctx context.Context, dsn string, budget time.Duration) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	ticker := time.NewTicker(connectRetry)
	defer ticker.Stop()
	for {
		err := pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		slog.WarnContext(ctx, "Database not ready", "error", err, "retry_in", connectRetry)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, fmt.Errorf("connect to database within %v: %w", budget, err)
		case <-ticker.C:
		}
	}
}
