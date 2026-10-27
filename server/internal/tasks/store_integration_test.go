//go:build integration

package tasks

import (
	"errors"
	"strings"
	"testing"

	"calcqueue/internal/testdb"
	"calcqueue/migrations"

	"github.com/google/uuid"
)

func TestStoreLifecycle(t *testing.T) {
	pool, _ := testdb.NewSchema(t)
	ctx := t.Context()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var user, other uuid.UUID
	for i, id := range []*uuid.UUID{&user, &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (login,password_hash) VALUES ($1,'hash') RETURNING id`, []string{"alice", "other"}[i]).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	s := &store{pool: pool}
	svc := &service{reader: s, mutator: s}
	items, err := s.list(ctx, user)
	if err != nil || len(items) != 0 {
		t.Fatalf("empty list: %v %v", items, err)
	}
	for _, expression := range []string{"", strings.Repeat("1", 201), "1+", "1\t+2"} {
		if _, err := svc.create(ctx, user, expression); err == nil {
			t.Fatalf("accepted %q", expression)
		}
	}
	first, err := svc.create(ctx, user, "1+2")
	if err != nil {
		t.Fatal(err)
	}
	if first.status != "queued" || first.result != nil || first.startedAt != nil || first.finishedAt != nil || first.errorCode != nil {
		t.Fatalf("initial task: %+v", first)
	}
	second, err := svc.create(ctx, user, "3")
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.list(ctx, user)
	if err != nil || len(items) != 2 || items[0].id != second.id {
		t.Fatalf("list: %+v %v", items, err)
	}
	if _, err := s.get(ctx, other, first.id); !errors.Is(err, errNotFound) {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := svc.update(ctx, other, first.id, ""); !errors.Is(err, errNotFound) {
		t.Fatalf("foreign update: %v", err)
	}
	if _, err := svc.update(ctx, user, first.id, "1+"); !errors.Is(err, errSyntax) {
		t.Fatalf("invalid update: %v", err)
	}
	updated, err := svc.update(ctx, user, first.id, "4+5")
	if err != nil || updated.id != first.id || !updated.createdAt.Equal(first.createdAt) {
		t.Fatalf("update: %+v %v", updated, err)
	}
	var expression string
	if err := pool.QueryRow(ctx, `UPDATE tasks SET status='running', started_at=now() WHERE id=(SELECT id FROM tasks WHERE status='queued' ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING expression`).Scan(&expression); err != nil || expression != "4+5" {
		t.Fatalf("claim: %q %v", expression, err)
	}
	if _, err := svc.update(ctx, user, first.id, ""); !errors.Is(err, errNotQueued) {
		t.Fatalf("claimed update: %v", err)
	}
	if err := s.delete(ctx, user, first.id); !errors.Is(err, errRunning) {
		t.Fatalf("claimed delete: %v", err)
	}
	if err := s.delete(ctx, user, second.id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.get(ctx, user, second.id); !errors.Is(err, errNotFound) {
		t.Fatalf("deleted get: %v", err)
	}
}
