//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

// row is the execution state of a task as stored.
type row struct {
	Status       string
	Expression   string
	Result       *float64
	ErrorCode    *string
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

type env struct {
	pool   *pgxpool.Pool
	store  *store
	userID uuid.UUID
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool, _ := testdb.NewSchema(t)
	if err := migrations.Apply(t.Context(), pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	var userID uuid.UUID
	if err := pool.QueryRow(t.Context(), `INSERT INTO users (login, password_hash) VALUES ('alice', 'hash')
RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return env{pool: pool, store: &store{pool: pool}, userID: userID}
}

// insert adds a queued task created at the given time.
func (e env) insert(t *testing.T, expression string, createdAt time.Time) uuid.UUID {
	t.Helper()
	return e.insertID(t, uuid.New(), expression, createdAt)
}

func (e env) insertID(t *testing.T, id uuid.UUID, expression string, createdAt time.Time) uuid.UUID {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO tasks (id, user_id, expression, created_at)
VALUES ($1, $2, $3, $4)`, id, e.userID, expression, createdAt); err != nil {
		t.Fatalf("insert task %q: %v", expression, err)
	}
	return id
}

func (e env) row(t *testing.T, id uuid.UUID) row {
	t.Helper()
	var r row
	if err := e.pool.QueryRow(t.Context(), `SELECT status, expression, result, error_code, error_message,
  started_at, finished_at FROM tasks WHERE id = $1`, id).Scan(
		&r.Status, &r.Expression, &r.Result, &r.ErrorCode, &r.ErrorMessage, &r.StartedAt, &r.FinishedAt,
	); err != nil {
		t.Fatalf("read task %s: %v", id, err)
	}
	return r
}

func (e env) claim(t *testing.T) (job, bool) {
	t.Helper()
	j, ok, err := e.store.claim(t.Context())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return j, ok
}

func ptr[T any](v T) *T { return &v }

var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestStoreClaimsInCreatedAtThenIDOrder(t *testing.T) {
	e := newEnv(t)
	third := e.insert(t, "3", base.Add(time.Second))
	second := e.insertID(t, uuid.MustParse("ffffffff-0000-0000-0000-000000000000"), "2", base)
	first := e.insertID(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), "1", base)

	var got []uuid.UUID
	for {
		j, ok := e.claim(t)
		if !ok {
			break
		}
		got = append(got, j.id)
	}
	if diff := cmp.Diff([]uuid.UUID{first, second, third}, got); diff != "" {
		t.Errorf("claim order mismatch (-want +got):\n%s", diff)
	}
}

func TestStoreConcurrentClaimsSkipLockedRows(t *testing.T) {
	e := newEnv(t)
	first := e.insert(t, "1 + 1", base)
	second := e.insert(t, "2 + 2", base.Add(time.Second))
	third := e.insert(t, "3 + 3", base.Add(2*time.Second))

	// Another worker is inside its claim and still holds the first row.
	tx, err := e.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() // No-op after Commit.
	var other uuid.UUID
	var expression string
	if err := tx.QueryRow(t.Context(), claimSQL).Scan(&other, &expression); err != nil {
		t.Fatalf("claim in transaction: %v", err)
	}
	if other != first {
		t.Fatalf("transaction claimed %s, want the oldest %s", other, first)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	j, ok, err := e.store.claim(ctx)
	if err != nil || !ok {
		t.Fatalf("claim while the oldest row is locked = %v, %v; want the next task without blocking", ok, err)
	}
	if j.id != second || j.expression != "2 + 2" {
		t.Errorf("claim = %s %q, want %s %q", j.id, j.expression, second, "2 + 2")
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	if j, _ := e.claim(t); j.id != third {
		t.Errorf("third claim = %s, want %s", j.id, third)
	}
	if _, ok := e.claim(t); ok {
		t.Error("claim on an empty queue reported a task")
	}
}

func TestStoreClaimReturnsCurrentExpressionAndReleasesLock(t *testing.T) {
	e := newEnv(t)
	id := e.insert(t, "1 + 1", base)
	// An accepted PATCH changes the expression before the claim.
	if _, err := e.pool.Exec(t.Context(), `UPDATE tasks SET expression = '2 * 3' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	j, ok := e.claim(t)
	if !ok || j.id != id || j.expression != "2 * 3" {
		t.Fatalf("claim = %s %q %v, want %s %q", j.id, j.expression, ok, id, "2 * 3")
	}
	got := e.row(t, id)
	if got.Status != "running" || got.StartedAt == nil || got.FinishedAt != nil {
		t.Errorf("claimed row = %+v, want running with started_at only", got)
	}
	assertUnlocked(t, e.pool, id)
}

// assertUnlocked fails when another transaction holds the row lock of id.
func assertUnlocked(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() // Read-only lock probe.
	if _, err := tx.Exec(t.Context(), `SELECT id FROM tasks WHERE id = $1 FOR UPDATE NOWAIT`, id); err != nil {
		t.Errorf("row %s is still locked: %v", id, err)
	}
}

func TestStoreFinishOnlyRunningTasks(t *testing.T) {
	e := newEnv(t)
	done := e.insert(t, "1 + 1", base)
	failed := e.insert(t, "1 / 0", base.Add(time.Second))
	queued := e.insert(t, "2 + 2", base.Add(2*time.Second))
	e.claim(t)
	e.claim(t)

	if err := e.store.complete(t.Context(), done, 2); err != nil {
		t.Fatalf("complete running task: %v", err)
	}
	if err := e.store.fail(t.Context(), failed, codeDivisionByZero, messageDivisionByZero); err != nil {
		t.Fatalf("fail running task: %v", err)
	}
	if err := e.store.complete(t.Context(), done, 3); !errors.Is(err, errNotRunning) {
		t.Errorf("complete finished task = %v, want errNotRunning", err)
	}
	if err := e.store.fail(t.Context(), queued, codeOverflow, messageOverflow); !errors.Is(err, errNotRunning) {
		t.Errorf("fail queued task = %v, want errNotRunning", err)
	}

	gotDone, gotFailed := e.row(t, done), e.row(t, failed)
	if gotDone.Status != "done" || gotDone.Result == nil || *gotDone.Result != 2 || gotDone.FinishedAt == nil {
		t.Errorf("completed row = %+v, want done with result 2 and finished_at", gotDone)
	}
	if gotFailed.Status != "failed" || gotFailed.Result != nil || gotFailed.FinishedAt == nil ||
		cmp.Diff(ptr("деление на ноль"), gotFailed.ErrorMessage) != "" {
		t.Errorf("failed row = %+v, want failed with the Russian message and finished_at", gotFailed)
	}
	if got := e.row(t, queued).Status; got != "queued" {
		t.Errorf("queued row status = %s after a stray fail, want queued", got)
	}
}

func TestRecoverRequeuesOnlyRunningTasks(t *testing.T) {
	e := newEnv(t)
	queued := e.insert(t, "1 + 1", base)
	running := e.insert(t, "2 + 2", base.Add(time.Second))
	done := e.insert(t, "3 + 3", base.Add(2*time.Second))
	failed := e.insert(t, "1 / 0", base.Add(3*time.Second))
	if _, err := e.pool.Exec(t.Context(), `
UPDATE tasks SET status = 'running', started_at = now() WHERE id = $1;`, running); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE tasks SET status = 'done', result = 6, started_at = now(),
  finished_at = now() WHERE id = $1`, done); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE tasks SET status = 'failed', error_code = 'division_by_zero',
  error_message = 'деление на ноль', started_at = now(), finished_at = now() WHERE id = $1`, failed); err != nil {
		t.Fatal(err)
	}
	before := map[uuid.UUID]row{queued: e.row(t, queued), done: e.row(t, done), failed: e.row(t, failed)}

	if err := New(e.pool, defaultConfig).Recover(t.Context()); err != nil {
		t.Fatalf("Recover() = %v", err)
	}

	if diff := cmp.Diff(row{Status: "queued", Expression: "2 + 2"}, e.row(t, running)); diff != "" {
		t.Errorf("recovered row mismatch (-want +got):\n%s", diff)
	}
	for id, want := range before {
		if diff := cmp.Diff(want, e.row(t, id)); diff != "" {
			t.Errorf("row %s changed by Recover (-want +got):\n%s", want.Expression, diff)
		}
	}
}
