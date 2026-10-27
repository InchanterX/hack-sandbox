//go:build integration

package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

// waitFor polls cond every 10 ms until it holds or timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-deadline.C:
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		case <-tick.C:
		}
	}
}

// started runs p until the returned stop functions are called; wait returns
// the error of Run.
func started(t *testing.T, p *Pool) (stopClaims, cancelExec context.CancelFunc, wait func() error) {
	t.Helper()
	claimCtx, stopClaims := context.WithCancel(t.Context())
	execCtx, cancelExec := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- p.Run(claimCtx, execCtx) }()
	// Tests may wait explicitly before cleanup. Share the received result so
	// cleanup still joins the workers without waiting for a second send.
	wait = sync.OnceValue(func() error { return <-done })
	t.Cleanup(func() {
		stopClaims()
		cancelExec()
		_ = wait() // Tests that care about the result check it explicitly.
	})
	return stopClaims, cancelExec, wait
}

func TestPoolExecutesTasksThroughAllStates(t *testing.T) {
	e := newEnv(t)
	ids := map[string]uuid.UUID{}
	for i, expression := range []string{"2 + 3 * (4 - 1)", "(10 - 4) / 3", "100 - 7", "7 / (2 - 2)"} {
		ids[expression] = e.insert(t, expression, base.Add(time.Duration(i)*time.Second))
	}
	slow := ids["2 + 3 * (4 - 1)"]
	if got := e.row(t, slow); got.Status != "queued" || got.StartedAt != nil {
		t.Fatalf("new row = %+v, want queued without started_at", got)
	}

	p := New(e.pool, Config{Workers: 2, OpDelay: 100 * time.Millisecond, PollInterval: 20 * time.Millisecond})
	stopClaims, _, wait := started(t, p)

	waitFor(t, 5*time.Second, "the first task to run", func() bool { return e.row(t, slow).Status == "running" })
	if got := e.row(t, slow); got.StartedAt == nil || got.FinishedAt != nil || got.Result != nil || got.ErrorCode != nil {
		t.Errorf("running row = %+v, want only started_at set", got)
	}
	waitFor(t, 10*time.Second, "all tasks to finish", func() bool {
		for _, id := range ids {
			if s := e.row(t, id).Status; s != "done" && s != "failed" {
				return false
			}
		}
		return true
	})
	stopClaims()
	if err := wait(); err != nil {
		t.Errorf("Run() = %v, want nil after claims stop", err)
	}

	want := map[string]row{
		"2 + 3 * (4 - 1)": {Status: "done", Result: ptr(11.0)},
		"(10 - 4) / 3":    {Status: "done", Result: ptr(2.0)},
		"100 - 7":         {Status: "done", Result: ptr(93.0)},
		"7 / (2 - 2)":     {Status: "failed", ErrorCode: ptr("division_by_zero"), ErrorMessage: ptr("деление на ноль")},
	}
	for expression, w := range want {
		got := e.row(t, ids[expression])
		if got.StartedAt == nil || got.FinishedAt == nil || got.FinishedAt.Before(*got.StartedAt) {
			t.Errorf("%q: started_at %v, finished_at %v; want both set in order", expression, got.StartedAt, got.FinishedAt)
		}
		w.Expression = expression
		got.StartedAt, got.FinishedAt = nil, nil
		if diff := cmp.Diff(w, got); diff != "" {
			t.Errorf("%q: row mismatch (-want +got):\n%s", expression, diff)
		}
	}
}

func TestPoolStopLeavesInterruptedTaskRecoverable(t *testing.T) {
	e := newEnv(t)
	active := e.insert(t, "2 + 3 * (4 - 1)", base)
	p := New(e.pool, Config{Workers: 2, OpDelay: time.Hour, PollInterval: 20 * time.Millisecond})
	stopClaims, cancelExec, wait := started(t, p)

	waitFor(t, 5*time.Second, "the task to run", func() bool { return e.row(t, active).Status == "running" })
	// The worker is inside its first one-hour delay: the claim has committed.
	assertUnlocked(t, e.pool, active)

	stopClaims()
	late := e.insert(t, "100 - 7", base.Add(time.Second))
	cancelExec()
	if err := wait(); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run() = %v, want ErrInterrupted", err)
	}
	if got := e.row(t, late).Status; got != "queued" {
		t.Errorf("task created after stop is %s, want queued: no claims after stop", got)
	}
	if got := e.row(t, active); got.Status != "running" || got.ErrorCode != nil || got.FinishedAt != nil {
		t.Errorf("interrupted row = %+v, want still running, not failed", got)
	}

	// The next start recovers the row before its workers run, then completes it.
	next := New(e.pool, Config{Workers: 2, PollInterval: 20 * time.Millisecond})
	if err := next.Recover(t.Context()); err != nil {
		t.Fatalf("Recover() = %v", err)
	}
	if diff := cmp.Diff(row{Status: "queued", Expression: "2 + 3 * (4 - 1)"}, e.row(t, active)); diff != "" {
		t.Errorf("recovered row mismatch (-want +got):\n%s", diff)
	}
	stopNext, _, waitNext := started(t, next)
	waitFor(t, 5*time.Second, "recovered tasks to finish", func() bool {
		return e.row(t, active).Status == "done" && e.row(t, late).Status == "done"
	})
	stopNext()
	if err := waitNext(); err != nil {
		t.Errorf("second Run() = %v, want nil", err)
	}
	if got := e.row(t, active).Result; got == nil || *got != 11 {
		t.Errorf("recovered task result = %v, want 11", got)
	}
}

// recordingClaimer records every task the wrapped store claims.
type recordingClaimer struct {
	*store
	mu      sync.Mutex
	claimed []job
}

func (r *recordingClaimer) claim(ctx context.Context) (job, bool, error) {
	j, ok, err := r.store.claim(ctx)
	if ok {
		r.mu.Lock()
		r.claimed = append(r.claimed, j)
		r.mu.Unlock()
	}
	return j, ok, err
}

func (r *recordingClaimer) jobs() []job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]job(nil), r.claimed...)
}

func TestPoolTwoWorkersClaimDistinctTasksInOrder(t *testing.T) {
	e := newEnv(t)
	// Same created_at for the first pair: id breaks the tie.
	first := e.insertID(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), "1 + 1", base)
	second := e.insertID(t, uuid.MustParse("ffffffff-0000-0000-0000-000000000000"), "2 + 2", base)
	third := e.insert(t, "3 + 3", base.Add(time.Second))
	fourth := e.insert(t, "4 + 4", base.Add(2*time.Second))
	// An accepted PATCH before the claim: the worker evaluates the new text.
	if _, err := e.pool.Exec(t.Context(), `UPDATE tasks SET expression = '5 * 5' WHERE id = $1`, second); err != nil {
		t.Fatal(err)
	}

	rec := &recordingClaimer{store: e.store}
	p := newPool(rec, e.store, e.store, Config{Workers: 2, OpDelay: time.Second, PollInterval: 20 * time.Millisecond})
	stopClaims, _, wait := started(t, p)

	// Both workers are inside their one-second delay at the same time.
	waitFor(t, 5*time.Second, "both workers to run a task", func() bool { return len(rec.jobs()) == 2 })
	gotFirst := rec.jobs()
	if gotFirst[0].id == gotFirst[1].id {
		t.Fatalf("both workers claimed %s", gotFirst[0].id)
	}
	for _, id := range []uuid.UUID{first, second} {
		if got := e.row(t, id).Status; got != "running" {
			t.Errorf("oldest task %s is %s while both workers are busy, want running", id, got)
		}
		// The claims committed: no row lock is held during the delays.
		assertUnlocked(t, e.pool, id)
	}
	for _, id := range []uuid.UUID{third, fourth} {
		if got := e.row(t, id).Status; got != "queued" {
			t.Errorf("newer task %s is %s while both workers are busy, want queued", id, got)
		}
	}

	waitFor(t, 10*time.Second, "all tasks to finish", func() bool {
		return e.row(t, third).Status == "done" && e.row(t, fourth).Status == "done"
	})
	stopClaims()
	if err := wait(); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}

	got := rec.jobs()
	ids := make(map[uuid.UUID]string, len(got))
	for _, j := range got {
		if _, dup := ids[j.id]; dup {
			t.Errorf("task %s was claimed twice", j.id)
		}
		ids[j.id] = j.expression
	}
	want := map[uuid.UUID]string{first: "1 + 1", second: "5 * 5", third: "3 + 3", fourth: "4 + 4"}
	if diff := cmp.Diff(want, ids); diff != "" {
		t.Errorf("claimed tasks and expressions mismatch (-want +got):\n%s", diff)
	}
	// FIFO across workers: the oldest pair first, in either worker order.
	if pair := map[uuid.UUID]bool{gotFirst[0].id: true, gotFirst[1].id: true}; !pair[first] || !pair[second] {
		t.Errorf("first claims %v, want the two oldest tasks %s and %s", gotFirst, first, second)
	}
	if r := e.row(t, second); r.Result == nil || *r.Result != 25 {
		t.Errorf("patched task result = %v, want 25 from the current expression", r.Result)
	}
}

func TestStoreParallelClaimsNeverShareATask(t *testing.T) {
	e := newEnv(t)
	const tasks = 40
	for i := range tasks {
		e.insert(t, "1 + 1", base.Add(time.Duration(i)*time.Millisecond))
	}
	var mu sync.Mutex
	seen := map[uuid.UUID]int{}
	var wg sync.WaitGroup
	for worker := range 2 {
		wg.Go(func() {
			for {
				j, ok, err := e.store.claim(t.Context())
				if err != nil {
					t.Errorf("worker %d: claim: %v", worker, err)
					return
				}
				if !ok {
					return
				}
				mu.Lock()
				seen[j.id]++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(seen) != tasks {
		t.Errorf("claimed %d distinct tasks, want %d", len(seen), tasks)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("task %s claimed %d times, want once", id, n)
		}
	}
}
