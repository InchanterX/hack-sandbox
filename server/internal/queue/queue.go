// Package queue runs the worker pool that claims queued tasks, evaluates them
// with calc and stores the result.
//
// A worker claims the oldest queued task in one autocommitted statement, so no
// row lock is held while it evaluates. It pauses OpDelay before every binary
// operation and stores done or failed with a conditional update. Cancellation
// and infrastructure failures leave the task running; Recover requeues such
// tasks at the next start.
package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/calc"
)

// Arithmetic failure codes and their exact Russian messages (contracts/schema.sql).
const (
	codeDivisionByZero    = "division_by_zero"
	messageDivisionByZero = "деление на ноль"
	codeOverflow          = "overflow"
	messageOverflow       = "переполнение"
)

// finishAttempts bounds the attempts to store the outcome of one evaluation;
// after them the task stays running for the next Recover.
const finishAttempts = 3

// claimAttempts bounds the consecutive failed claims of one worker, PollInterval
// apart (30 s with the default 300 ms). A store that stays down that long is
// not transient: Run stops and reports it, and the restarted process recovers.
const claimAttempts = 100

// errClaimsFailing reports that a worker exhausted claimAttempts.
var errClaimsFailing = errors.New("claims keep failing")

// ErrInterrupted reports that canceling the execution context aborted active
// evaluations; their tasks stay running until the next Recover.
var ErrInterrupted = errors.New("active tasks interrupted")

// Config configures the worker pool.
type Config struct {
	// Workers is the number of concurrent workers.
	Workers int
	// OpDelay is the pause before each binary operation.
	OpDelay time.Duration
	// PollInterval is the idle wait between empty claims and after store errors.
	PollInterval time.Duration
}

// claimer takes the next queued task.
type claimer interface {
	claim(ctx context.Context) (job, bool, error)
}

// recorder stores the outcome of a claimed task.
type recorder interface {
	complete(ctx context.Context, id uuid.UUID, result float64) error
	fail(ctx context.Context, id uuid.UUID, code, message string) error
}

// requeuer resets interrupted tasks.
type requeuer interface {
	requeueRunning(ctx context.Context) (int64, error)
}

var (
	_ claimer  = (*store)(nil)
	_ recorder = (*store)(nil)
	_ requeuer = (*store)(nil)
)

// Pool is the worker pool over the tasks table.
type Pool struct {
	claims   claimer
	results  recorder
	requeues requeuer
	cfg      Config
}

// New returns a pool that is not running yet.
func New(db *pgxpool.Pool, cfg Config) *Pool {
	s := &store{pool: db}
	return newPool(s, s, s, cfg)
}

func newPool(c claimer, r recorder, q requeuer, cfg Config) *Pool {
	return &Pool{claims: c, results: r, requeues: q, cfg: cfg}
}

// Recover turns interrupted running tasks back into queued ones with all
// execution fields cleared. Call it after migrations, before Run.
func (p *Pool) Recover(ctx context.Context) error {
	n, err := p.requeues.requeueRunning(ctx)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "Recovered interrupted tasks", "count", n)
	return nil
}

// Run starts the workers and blocks until they exit. Canceling claimCtx stops
// new claims; active evaluations continue until they finish or executionCtx is
// canceled. Interrupted tasks stay running for the next Recover and Run
// returns an error matching ErrInterrupted. A failed claim is logged and
// retried after PollInterval up to claimAttempts times in a row; then every
// worker stops claiming, finishes its active task and Run returns the store
// error.
func (p *Pool) Run(claimCtx, executionCtx context.Context) error {
	if p.cfg.Workers < 1 {
		return fmt.Errorf("run queue: workers=%d, want at least 1", p.cfg.Workers)
	}
	claimCtx, stopClaims := context.WithCancelCause(claimCtx)
	defer stopClaims(nil)
	var interrupted atomic.Int64
	var wg sync.WaitGroup
	for i := range p.cfg.Workers {
		log := slog.With("worker", i+1)
		wg.Go(func() {
			failedClaims := 0
			for {
				more, stopped, err := p.next(claimCtx, executionCtx, log, &failedClaims)
				if stopped {
					interrupted.Add(1)
				}
				if err != nil {
					stopClaims(err)
				}
				if !more {
					return
				}
			}
		})
	}
	wg.Wait()
	var errs []error
	if cause := context.Cause(claimCtx); errors.Is(cause, errClaimsFailing) {
		errs = append(errs, cause)
	}
	if n := interrupted.Load(); n > 0 {
		errs = append(errs, fmt.Errorf("%d tasks left running: %w: %w", n, ErrInterrupted, context.Cause(executionCtx)))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("run queue: %w", err)
	}
	return nil
}

// next claims and executes one task, or waits PollInterval when the queue is
// empty or the store fails. more is false once the worker must exit;
// interrupted reports a claimed task left running by executionCtx; err is set
// when the claims failed claimAttempts times in a row, counted in failedClaims.
func (p *Pool) next(claimCtx, executionCtx context.Context, log *slog.Logger, failedClaims *int) (
	more, interrupted bool, err error,
) {
	if claimCtx.Err() != nil {
		return false, false, nil
	}
	// The claim runs under claimCtx, so no claim is issued after stop. A claim
	// in flight at stop may still commit: a returned job is executed as active
	// work, a torn one stays running for Recover.
	j, ok, err := p.claims.claim(claimCtx)
	switch {
	case claimCtx.Err() != nil && !ok:
		if err != nil {
			log.WarnContext(executionCtx, "Claim canceled by stop; a task it claimed stays running for recovery",
				"error", err)
		}
		return false, false, nil
	case err != nil:
		*failedClaims++
		if *failedClaims >= claimAttempts {
			log.ErrorContext(claimCtx, "Claim failed, giving up", "error", err, "attempts", *failedClaims)
			return false, false, fmt.Errorf("%w %d times: %w", errClaimsFailing, *failedClaims, err)
		}
		log.ErrorContext(claimCtx, "Claim failed, retrying", "error", err,
			"attempt", *failedClaims, "retry_in", p.cfg.PollInterval)
		return wait(claimCtx, p.cfg.PollInterval), false, nil
	case !ok:
		*failedClaims = 0
		return wait(claimCtx, p.cfg.PollInterval), false, nil
	}
	*failedClaims = 0
	if p.execute(executionCtx, j, log.With("task_id", j.id)) {
		return false, true, nil
	}
	return claimCtx.Err() == nil, false, nil
}

// execute evaluates a claimed task and stores its outcome. Any failure that is
// not arithmetic leaves the task running; it reports whether ctx interrupted
// the task.
func (p *Pool) execute(ctx context.Context, j job, log *slog.Logger) (interrupted bool) {
	log.InfoContext(ctx, "Task claimed")
	if ctx.Err() != nil {
		log.WarnContext(ctx, "Task interrupted, left running for recovery", "error", context.Cause(ctx))
		return true
	}
	expr, err := calc.Parse(j.expression)
	if err != nil {
		// Expressions are validated on create and update, so this is corrupted
		// data: no failure code fits it and no result may be fabricated.
		log.ErrorContext(ctx, "Stored expression does not parse, left running", "error", err)
		return false
	}
	result, err := expr.Eval(ctx, p.delay)
	var code, message string
	switch {
	case err == nil:
	case errors.Is(err, calc.ErrDivisionByZero):
		code, message = codeDivisionByZero, messageDivisionByZero
	case errors.Is(err, calc.ErrOverflow):
		code, message = codeOverflow, messageOverflow
	case ctx.Err() != nil:
		log.WarnContext(ctx, "Task interrupted, left running for recovery", "error", err)
		return true
	default:
		log.ErrorContext(ctx, "Task evaluation failed, left running", "error", err)
		return false
	}
	err = p.finish(ctx, func(ctx context.Context) error {
		if code == "" {
			return p.results.complete(ctx, j.id, result)
		}
		return p.results.fail(ctx, j.id, code, message)
	})
	if err != nil {
		log.ErrorContext(ctx, "Task outcome not stored, left running for recovery", "error", err)
		return ctx.Err() != nil
	}
	log.InfoContext(ctx, "Task finished", "error_code", code)
	return false
}

// finish stores an outcome with up to finishAttempts tries, PollInterval apart.
func (p *Pool) finish(ctx context.Context, record func(context.Context) error) error {
	var err error
	for attempt := 1; attempt <= finishAttempts; attempt++ {
		err = record(ctx)
		if err == nil || errors.Is(err, errNotRunning) {
			return err
		}
		if attempt == finishAttempts || !wait(ctx, p.cfg.PollInterval) {
			break
		}
	}
	return err
}

// delay is the evaluator callback: one OpDelay pause per binary operation.
func (p *Pool) delay(ctx context.Context) error {
	if p.cfg.OpDelay <= 0 {
		return nil
	}
	if !wait(ctx, p.cfg.OpDelay) {
		return context.Cause(ctx) //nolint:wrapcheck // calc expects the context error itself to tell cancellation apart.
	}
	return nil
}

// wait pauses for d and reports whether ctx is still alive afterwards.
func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
