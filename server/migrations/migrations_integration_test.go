//go:build integration

package migrations_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

func TestApplyIsIdempotent(t *testing.T) {
	pool, schema := testdb.NewSchema(t)
	for i := range 2 {
		if err := migrations.Apply(t.Context(), pool); err != nil {
			t.Fatalf("Apply() run %d error = %v", i+1, err)
		}
	}

	rows, err := pool.Query(t.Context(),
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = $1 AND table_name <> 'goose_db_version' ORDER BY table_name`, schema)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if diff := cmp.Diff([]string{"sessions", "tasks", "users"}, tables); diff != "" {
		t.Errorf("tables mismatch (-want +got):\n%s", diff)
	}

	wantIndexes := map[string]string{
		"tasks_queue_idx":        "CREATE INDEX tasks_queue_idx ON %s.tasks USING btree (status, created_at)",
		"tasks_user_created_idx": "CREATE INDEX tasks_user_created_idx ON %s.tasks USING btree (user_id, created_at DESC)",
	}
	for name, def := range wantIndexes {
		var got string
		err := pool.QueryRow(t.Context(),
			`SELECT indexdef FROM pg_indexes WHERE schemaname = $1 AND indexname = $2`, schema, name).Scan(&got)
		if err != nil {
			t.Errorf("index %s: %v", name, err)
			continue
		}
		if want := fmt.Sprintf(def, schema); got != want {
			t.Errorf("index %s = %q, want %q", name, got, want)
		}
	}

	var versions int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM goose_db_version WHERE version_id > 0`).Scan(&versions); err != nil {
		t.Fatalf("count applied versions: %v", err)
	}
	if versions != 1 {
		t.Errorf("applied migration versions = %d, want 1 after two runs", versions)
	}
}

func TestTaskStateConstraints(t *testing.T) {
	pool, _ := testdb.NewSchema(t)
	if err := migrations.Apply(t.Context(), pool); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var userID string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO users (login, password_hash) VALUES ('alice', 'hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	const insert = `INSERT INTO tasks (user_id, expression, status, result, error_code, error_message, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	now := "2026-10-27T12:00:00Z"
	tests := []struct {
		name    string
		args    []any
		wantErr bool
	}{
		{name: "queued without execution fields", args: []any{"1+1", "queued", nil, nil, nil, nil, nil}},
		{name: "running with start time", args: []any{"1+1", "running", nil, nil, nil, now, nil}},
		{name: "done with result", args: []any{"1+1", "done", 2.0, nil, nil, now, now}},
		{name: "failed division by zero", args: []any{"1/0", "failed", nil, "division_by_zero", "деление на ноль", now, now}},
		{name: "failed overflow", args: []any{"9*9", "failed", nil, "overflow", "переполнение", now, now}},
		{name: "unknown status", args: []any{"1+1", "paused", nil, nil, nil, nil, nil}, wantErr: true},
		{name: "queued with result", args: []any{"1+1", "queued", 2.0, nil, nil, nil, nil}, wantErr: true},
		{name: "running without start time", args: []any{"1+1", "running", nil, nil, nil, nil, nil}, wantErr: true},
		{name: "done without result", args: []any{"1+1", "done", nil, nil, nil, now, now}, wantErr: true},
		{name: "done with infinite result", args: []any{"1+1", "done", "Infinity", nil, nil, now, now}, wantErr: true},
		{name: "done with NaN result", args: []any{"1+1", "done", "NaN", nil, nil, now, now}, wantErr: true},
		{name: "failed with mismatched message", args: []any{"1/0", "failed", nil, "division_by_zero", "переполнение", now, now}, wantErr: true},
		{name: "failed with unknown code", args: []any{"1/0", "failed", nil, "boom", "деление на ноль", now, now}, wantErr: true},
		{name: "failed without finish time", args: []any{"1/0", "failed", nil, "division_by_zero", "деление на ноль", now, nil}, wantErr: true},
		{name: "empty expression", args: []any{"", "queued", nil, nil, nil, nil, nil}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := exec(t, pool, insert, append([]any{userID}, tc.args...)...)
			if !tc.wantErr {
				if err != nil {
					t.Errorf("insert %v error = %v, want success", tc.args, err)
				}
				return
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Errorf("insert %v error = %v, want check_violation 23514", tc.args, err)
			}
		})
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	return err
}
