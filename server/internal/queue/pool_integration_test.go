//go:build integration

package queue

import (
	"context"
	"errors"
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
	t.Cleanup(func() {
		stopClaims()
		cancelExec()
		<-done
	})
	var err error
	var finished bool
	return stopClaims, cancelExec, func() error {
		if !finished {
			err, finished = <-done, true
		}
		return err
	}
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
