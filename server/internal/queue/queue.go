// Package queue runs the worker pool that claims queued tasks, evaluates them
// with calc and stores the result.
//
// Scaffold stub (T01): Recover does nothing and Run only waits for claimCtx
// until T05 implements the pool behind these stable signatures.
package queue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config configures the worker pool.
type Config struct {
	// Workers is the number of concurrent workers.
	Workers int
	// OpDelay is the pause before each binary operation.
	OpDelay time.Duration
	// PollInterval is the idle wait between empty claims and after store errors.
	PollInterval time.Duration
}

// Pool is the worker pool over the tasks table.
type Pool struct {
	db  *pgxpool.Pool
	cfg Config
}

// New returns a pool that is not running yet.
func New(db *pgxpool.Pool, cfg Config) *Pool {
	return &Pool{db: db, cfg: cfg}
}

// Recover turns interrupted running tasks back into queued ones with all
// execution fields cleared. Call it after migrations, before Run.
func (p *Pool) Recover(_ context.Context) error {
	return nil
}

// Run starts the workers and blocks until they exit. Canceling claimCtx stops
// new claims; canceling executionCtx aborts active evaluations, which stay
// running for the next Recover.
func (p *Pool) Run(claimCtx, _ context.Context) error {
	<-claimCtx.Done()
	return nil
}
