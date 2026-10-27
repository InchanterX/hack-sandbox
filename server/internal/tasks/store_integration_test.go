//go:build integration

package tasks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

// claimSQL is the atomic claim from contracts/schema.sql used by the queue.
const claimSQL = `UPDATE tasks SET status = 'running', started_at = now()
WHERE id = (
  SELECT id FROM tasks WHERE status = 'queued'
  ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
) RETURNING id, expression`

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func claim(t *testing.T, q queryRower) (uuid.UUID, string, bool) {
	t.Helper()
	var id uuid.UUID
	var expression string
	err := q.QueryRow(t.Context(), claimSQL).Scan(&id, &expression)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false
	}
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return id, expression, true
}

func newStoreEnv(t *testing.T) (*pgxpool.Pool, *store, *service, uuid.UUID, uuid.UUID) {
	t.Helper()
	pool, _ := testdb.NewSchema(t)
	ctx := t.Context()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var ids [2]uuid.UUID
	for i, login := range []string{"alice", "other"} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (login,password_hash) VALUES ($1,'hash') RETURNING id`, login).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	s := &store{pool: pool}
	return pool, s, &service{reader: s, mutator: s}, ids[0], ids[1]
}

func storedExpression(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (string, string) {
	t.Helper()
	var expression, status string
	if err := pool.QueryRow(t.Context(), `SELECT expression, status FROM tasks WHERE id=$1`, id).Scan(&expression, &status); err != nil {
		t.Fatalf("stored task %s: %v", id, err)
	}
	return expression, status
}

func rowCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStoreLifecycle(t *testing.T) {
	pool, s, svc, user, other := newStoreEnv(t)
	ctx := t.Context()
	items, err := s.list(ctx, user)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list: %#v %v", items, err)
	}
	invalid := map[string]error{"": errValidation, strings.Repeat("1", 201): errValidation, "1+": errSyntax, "1\t+2": errSyntax, "a": errSyntax}
	for expression, want := range invalid {
		if _, err := svc.create(ctx, user, expression); !errors.Is(err, want) {
			t.Fatalf("create %q: %v, want %v", expression, err, want)
		}
	}
	if n := rowCount(t, pool); n != 0 {
		t.Fatalf("rejected creates stored %d rows", n)
	}
	first, err := svc.create(ctx, user, "1+2")
	if err != nil {
		t.Fatal(err)
	}
	if first.status != "queued" || first.result != nil || first.startedAt != nil || first.finishedAt != nil || first.errorCode != nil || first.errorMessage != nil {
		t.Fatalf("initial task: %+v", first)
	}
	foreign, err := svc.create(ctx, other, "7")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.create(ctx, user, "3")
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.list(ctx, user)
	if err != nil || len(items) != 2 || items[0].id != second.id || items[1].id != first.id {
		t.Fatalf("own list newest first: %+v %v", items, err)
	}
	if got, err := s.get(ctx, user, first.id); err != nil || got.id != first.id || got.expression != "1+2" {
		t.Fatalf("own get: %+v %v", got, err)
	}
	for _, id := range []uuid.UUID{foreign.id, uuid.New()} {
		if _, err := s.get(ctx, user, id); !errors.Is(err, errNotFound) {
			t.Fatalf("get %s: %v", id, err)
		}
		if _, err := svc.update(ctx, user, id, "1"); !errors.Is(err, errNotFound) {
			t.Fatalf("update %s: %v", id, err)
		}
		if err := s.delete(ctx, user, id); !errors.Is(err, errNotFound) {
			t.Fatalf("delete %s: %v", id, err)
		}
	}
	for expression, want := range invalid {
		if _, err := svc.update(ctx, user, first.id, expression); !errors.Is(err, want) {
			t.Fatalf("update %q: %v, want %v", expression, err, want)
		}
	}
	if expression, _ := storedExpression(t, pool, first.id); expression != "1+2" || rowCount(t, pool) != 3 {
		t.Fatalf("rejected updates changed %q", expression)
	}
	updated, err := svc.update(ctx, user, first.id, "4+5")
	if err != nil || updated.id != first.id || !updated.createdAt.Equal(first.createdAt) || updated.expression != "4+5" || updated.status != "queued" {
		t.Fatalf("update: %+v %v", updated, err)
	}

	states := map[string]string{
		"running": `UPDATE tasks SET status='running', started_at=now() WHERE id=$1`,
		"done":    `UPDATE tasks SET status='done', result=1, started_at=now(), finished_at=now() WHERE id=$1`,
		"failed":  `UPDATE tasks SET status='failed', error_code='overflow', error_message='переполнение', started_at=now(), finished_at=now() WHERE id=$1`,
	}
	for status, query := range states {
		task, err := svc.create(ctx, user, "8")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, query, task.id); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.update(ctx, user, task.id, ""); !errors.Is(err, errNotQueued) {
			t.Fatalf("update %s: %v", status, err)
		}
		err = s.delete(ctx, user, task.id)
		if status == "running" {
			if !errors.Is(err, errRunning) {
				t.Fatalf("delete running: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("delete %s: %v", status, err)
		}
		if _, err := s.get(ctx, user, task.id); !errors.Is(err, errNotFound) {
			t.Fatalf("deleted %s get: %v", status, err)
		}
	}
}

// holdLock makes the next store mutation pause right after it locks its row.
// It returns a channel closed once the lock is held and a release function.
func holdLock(s *store) (<-chan struct{}, func()) {
	locked, release := make(chan struct{}), make(chan struct{})
	s.locked = func() {
		close(locked)
		<-release
	}
	return locked, func() { close(release) }
}

func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("mutation did not finish")
		return nil
	}
}

func assertBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("mutation finished while claim held the row: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestLockOrderPatchBeforeClaim(t *testing.T) {
	_, s, svc, user, _ := newStoreEnv(t)
	task, err := svc.create(t.Context(), user, "1")
	if err != nil {
		t.Fatal(err)
	}
	locked, release := holdLock(s)
	done := make(chan error, 1)
	go func() {
		_, err := svc.update(t.Context(), user, task.id, "2+3")
		done <- err
	}()
	<-locked
	// The claim runs while PATCH holds the row: it must not take the old expression.
	if id, expression, ok := claim(t, s.pool); ok {
		t.Fatalf("claim took locked row %s %q", id, expression)
	}
	release()
	if err := waitErr(t, done); err != nil {
		t.Fatalf("update: %v", err)
	}
	s.locked = nil
	id, expression, ok := claim(t, s.pool)
	if !ok || id != task.id || expression != "2+3" {
		t.Fatalf("later claim: %s %q %v, want accepted PATCH", id, expression, ok)
	}
}

func TestLockOrderClaimBeforeMutation(t *testing.T) {
	for _, op := range []string{"patch", "delete"} {
		t.Run(op, func(t *testing.T) {
			pool, s, svc, user, _ := newStoreEnv(t)
			ctx := t.Context()
			task, err := svc.create(ctx, user, "1")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }() // Already committed on the success path.
			if id, _, ok := claim(t, tx); !ok || id != task.id {
				t.Fatalf("claim: %s %v", id, ok)
			}
			done := make(chan error, 1)
			go func() {
				if op == "patch" {
					_, err := svc.update(ctx, user, task.id, "2")
					done <- err
					return
				}
				done <- s.delete(ctx, user, task.id)
			}()
			assertBlocked(t, done)
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			want := map[string]error{"patch": errNotQueued, "delete": errRunning}[op]
			if err := waitErr(t, done); !errors.Is(err, want) {
				t.Fatalf("%s after claim: %v, want %v", op, err, want)
			}
			if expression, status := storedExpression(t, pool, task.id); expression != "1" || status != "running" {
				t.Fatalf("claimed task changed: %q %s", expression, status)
			}
		})
	}
}

func TestLockOrderDeleteBeforeClaim(t *testing.T) {
	pool, s, svc, user, _ := newStoreEnv(t)
	ctx := t.Context()
	oldest, err := svc.create(ctx, user, "1")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := svc.create(ctx, user, "2")
	if err != nil {
		t.Fatal(err)
	}
	locked, release := holdLock(s)
	done := make(chan error, 1)
	go func() { done <- s.delete(ctx, user, oldest.id) }()
	<-locked
	// While DELETE holds the oldest row the claim skips it.
	if id, _, ok := claim(t, pool); !ok || id != newer.id {
		t.Fatalf("claim during delete: %s %v, want %s", id, ok, newer.id)
	}
	release()
	if err := waitErr(t, done); err != nil {
		t.Fatalf("delete: %v", err)
	}
	s.locked = nil
	if id, _, ok := claim(t, pool); ok {
		t.Fatalf("claim took deleted row %s", id)
	}

	// Sequential delete-first: the deleted row is never claimable.
	third, err := svc.create(ctx, user, "3")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.delete(ctx, user, third.id); err != nil {
		t.Fatal(err)
	}
	if id, _, ok := claim(t, pool); ok {
		t.Fatalf("claim took deleted row %s", id)
	}
	if n := rowCount(t, pool); n != 1 {
		t.Fatalf("row count %d, want only the claimed task", n)
	}
}
