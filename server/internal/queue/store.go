package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// errNotRunning reports that a final update found no running row with the id:
// another process recovered or finished it, so this result must not be stored.
var errNotRunning = errors.New("task is not running")

// job is a claimed task: the expression is the one committed at claim time.
type job struct {
	id         uuid.UUID
	expression string
}

// store is the SQL access of the queue to the tasks table.
type store struct {
	pool *pgxpool.Pool
}

// claimSQL is the atomic claim of contracts/schema.sql. It runs as a single
// autocommitted statement, so the row lock is released when it returns.
const claimSQL = `UPDATE tasks SET status = 'running', started_at = now()
WHERE id = (
  SELECT id FROM tasks WHERE status = 'queued'
  ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
) RETURNING id, expression`

// claim marks the oldest unlocked queued task running and returns it; ok is
// false when there is nothing to claim.
func (s *store) claim(ctx context.Context) (job, bool, error) {
	var j job
	err := s.pool.QueryRow(ctx, claimSQL).Scan(&j.id, &j.expression)
	if errors.Is(err, pgx.ErrNoRows) {
		return job{}, false, nil
	}
	if err != nil {
		return job{}, false, fmt.Errorf("claim task: %w", err)
	}
	return j, true, nil
}

// complete stores a successful result of a running task.
func (s *store) complete(ctx context.Context, id uuid.UUID, result float64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE tasks SET status = 'done', result = $2, finished_at = now()
WHERE id = $1 AND status = 'running'`, id, result)
	if err != nil {
		return fmt.Errorf("complete task %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete task %s: %w", id, errNotRunning)
	}
	return nil
}

// fail stores an arithmetic failure of a running task.
func (s *store) fail(ctx context.Context, id uuid.UUID, code, message string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE tasks SET status = 'failed', error_code = $2, error_message = $3,
  finished_at = now()
WHERE id = $1 AND status = 'running'`, id, code, message)
	if err != nil {
		return fmt.Errorf("fail task %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("fail task %s: %w", id, errNotRunning)
	}
	return nil
}

// requeueRunning turns every running task back into a queued one with all
// execution fields cleared and returns how many rows changed.
func (s *store) requeueRunning(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE tasks SET status = 'queued', started_at = NULL, finished_at = NULL,
  result = NULL, error_code = NULL, error_message = NULL
WHERE status = 'running'`)
	if err != nil {
		return 0, fmt.Errorf("requeue running tasks: %w", err)
	}
	return tag.RowsAffected(), nil
}
