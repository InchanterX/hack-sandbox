package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	pool *pgxpool.Pool
	// locked, when set, runs after a mutation locks its row; tests use it to hold the lock.
	locked func()
}

var _ reader = (*store)(nil)
var _ mutator = (*store)(nil)

const columns = `id, expression, status, result, error_code, error_message, created_at, started_at, finished_at`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (task, error) {
	var t task
	err := row.Scan(&t.id, &t.expression, &t.status, &t.result, &t.errorCode, &t.errorMessage, &t.createdAt, &t.startedAt, &t.finishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return task{}, errNotFound
	}
	if err != nil {
		return task{}, fmt.Errorf("scan task: %w", err)
	}
	return t, nil
}
func (s *store) create(ctx context.Context, user uuid.UUID, expression string) (task, error) {
	return scan(s.pool.QueryRow(ctx, `INSERT INTO tasks (user_id, expression) VALUES ($1,$2) RETURNING `+columns, user, expression))
}
func (s *store) list(ctx context.Context, user uuid.UUID) ([]task, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM tasks WHERE user_id=$1 ORDER BY created_at DESC, id DESC`, user)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	result := make([]task, 0)
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return result, nil
}
func (s *store) get(ctx context.Context, user, id uuid.UUID) (task, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE id=$1 AND user_id=$2`, id, user))
}
func (s *store) update(ctx context.Context, user, id uuid.UUID, expression string, validate func(string) error) (task, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return task{}, fmt.Errorf("begin update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // Commit or the original error determines the outcome.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM tasks WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, user).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return task{}, errNotFound
	}
	if err != nil {
		return task{}, fmt.Errorf("lock task: %w", err)
	}
	if s.locked != nil {
		s.locked()
	}
	if status != "queued" {
		return task{}, errNotQueued
	}
	if err := validate(expression); err != nil {
		return task{}, err
	}
	t, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET expression=$3 WHERE id=$1 AND user_id=$2 AND status='queued' RETURNING `+columns, id, user, expression))
	if err != nil {
		return task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return task{}, fmt.Errorf("commit update: %w", err)
	}
	return t, nil
}
func (s *store) delete(ctx context.Context, user, id uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // Commit or the original error determines the outcome.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM tasks WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, user).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return fmt.Errorf("lock task: %w", err)
	}
	if s.locked != nil {
		s.locked()
	}
	if status == "running" {
		return errRunning
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id=$1 AND user_id=$2 AND status <> 'running'`, id, user)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errRunning
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	return nil
}
