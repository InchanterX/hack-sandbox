package queue

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var errStore = errors.New("store unavailable")

type outcome struct {
	ID      uuid.UUID
	Result  float64
	Code    string
	Message string
	At      time.Duration
}

// fakeStore is an in-memory queue. Times are offsets from its creation, which
// inside a synctest bubble are exact managed-time values.
type fakeStore struct {
	mu         sync.Mutex
	start      time.Time
	jobs       []job
	claimErrs  []error
	finishErrs []error
	requeued   int64
	requeueErr error
	claimedAt  []time.Duration
	finishedAt []time.Duration
	outcomes   []outcome
}

var (
	_ claimer  = (*fakeStore)(nil)
	_ recorder = (*fakeStore)(nil)
	_ requeuer = (*fakeStore)(nil)
)

func newFakeStore(expressions ...string) *fakeStore {
	f := &fakeStore{start: time.Now()}
	for i, e := range expressions {
		f.jobs = append(f.jobs, job{id: jobID(i), expression: e})
	}
	return f
}

func jobID(i int) uuid.UUID { return uuid.UUID{15: byte(i + 1)} }

func (f *fakeStore) claim(context.Context) (job, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimedAt = append(f.claimedAt, time.Since(f.start))
	if len(f.claimErrs) > 0 {
		err := f.claimErrs[0]
		f.claimErrs = f.claimErrs[1:]
		return job{}, false, err
	}
	if len(f.jobs) == 0 {
		return job{}, false, nil
	}
	j := f.jobs[0]
	f.jobs = f.jobs[1:]
	return j, true, nil
}

func (f *fakeStore) complete(_ context.Context, id uuid.UUID, result float64) error {
	return f.record(outcome{ID: id, Result: result})
}

func (f *fakeStore) fail(_ context.Context, id uuid.UUID, code, message string) error {
	return f.record(outcome{ID: id, Code: code, Message: message})
}

func (f *fakeStore) record(o outcome) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o.At = time.Since(f.start)
	f.finishedAt = append(f.finishedAt, o.At)
	if len(f.finishErrs) > 0 {
		err := f.finishErrs[0]
		f.finishErrs = f.finishErrs[1:]
		return err
	}
	f.outcomes = append(f.outcomes, o)
	return nil
}

func (f *fakeStore) requeueRunning(context.Context) (int64, error) {
	return f.requeued, f.requeueErr
}

// snapshot returns copies of what the workers did, outcomes sorted by time and id.
func (f *fakeStore) snapshot() (claimed, finished []time.Duration, outcomes []outcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	outcomes = slices.Clone(f.outcomes)
	slices.SortFunc(outcomes, func(a, b outcome) int {
		if c := a.At - b.At; c != 0 {
			return int(c)
		}
		return slices.Compare(a.ID[:], b.ID[:])
	})
	return slices.Clone(f.claimedAt), slices.Clone(f.finishedAt), outcomes
}

// runPool runs p inside a synctest bubble, stops claims at stopAt and, when
// cancelAt is positive, cancels execution at cancelAt. It returns the managed
// time at which Run returned and its error.
func runPool(t *testing.T, p *Pool, stopAt, cancelAt time.Duration) (time.Duration, error) {
	t.Helper()
	start := time.Now()
	claimCtx, stopClaims := context.WithCancel(t.Context())
	defer stopClaims()
	execCtx, cancelExec := context.WithCancel(t.Context())
	defer cancelExec()
	stopTimer := time.AfterFunc(stopAt, stopClaims)
	defer stopTimer.Stop()
	if cancelAt > 0 {
		cancelTimer := time.AfterFunc(cancelAt, cancelExec)
		defer cancelTimer.Stop()
	}
	err := p.Run(claimCtx, execCtx)
	return time.Since(start), err
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

var defaultConfig = Config{Workers: 2, OpDelay: ms(700), PollInterval: ms(300)}

func TestRunTwoWorkersDelayEachBinaryOperation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeStore(
			"2 + 3 * (4 - 1)", // 3 binary operations: 2100 ms
			"(10 - 4) / 3",    // 2 operations: 1400 ms
			"100 - 7",         // 1 operation: 700 ms
			"7 / (2 - 2)",     // the division still waits before failing: 1400 ms
			"--5",             // no binary operation: no delay
		)
		returned, err := runPool(t, newPool(f, f, f, defaultConfig), ms(4000), 0)
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
		if returned != ms(4000) {
			t.Errorf("Run returned at %v, want 4s: idle poll timers must be canceled", returned)
		}
		claimed, _, outcomes := f.snapshot()
		want := []outcome{
			{ID: jobID(1), Result: 2, At: ms(1400)},
			{ID: jobID(0), Result: 11, At: ms(2100)},
			{ID: jobID(2), Result: 93, At: ms(2100)},
			{ID: jobID(4), Result: 5, At: ms(2100)},
			{ID: jobID(3), Code: "division_by_zero", Message: "деление на ноль", At: ms(3500)},
		}
		if diff := cmp.Diff(want, outcomes); diff != "" {
			t.Errorf("outcomes mismatch (-want +got):\n%s", diff)
		}
		// Two workers start together, the second claims again when its task
		// ends; after the queue drains each idle worker polls every 300 ms.
		wantClaims := []time.Duration{0, 0, ms(1400), ms(2100), ms(2100), ms(2100), ms(2400), ms(2700), ms(3000),
			ms(3300), ms(3500), ms(3600), ms(3800), ms(3900)}
		if diff := cmp.Diff(wantClaims, claimed); diff != "" {
			t.Errorf("claim times mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunArithmeticFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		expression string
		want       outcome
	}{
		{
			name:       "division by zero keeps the exact Russian message",
			expression: "7 / (2 - 2)",
			want:       outcome{ID: jobID(0), Code: "division_by_zero", Message: "деление на ноль"},
		},
		{
			name:       "out of range literal fails with overflow",
			expression: "1" + strings.Repeat("0", 400) + " + 1",
			want:       outcome{ID: jobID(0), Code: "overflow", Message: "переполнение"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFakeStore(tc.expression)
				cfg := Config{Workers: 1, PollInterval: ms(300)}
				if _, err := runPool(t, newPool(f, f, f, cfg), ms(100), 0); err != nil {
					t.Fatalf("Run() = %v, want nil", err)
				}
				_, _, outcomes := f.snapshot()
				if diff := cmp.Diff([]outcome{tc.want}, outcomes); diff != "" {
					t.Errorf("outcomes of %q mismatch (-want +got):\n%s", tc.expression, diff)
				}
			})
		})
	}
}

func TestRunIdleWorkerPollsEveryInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeStore()
		cfg := Config{Workers: 1, OpDelay: ms(700), PollInterval: ms(300)}
		returned, err := runPool(t, newPool(f, f, f, cfg), ms(1000), 0)
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
		if returned != ms(1000) {
			t.Errorf("Run returned at %v, want 1s right after claims stop", returned)
		}
		claimed, _, _ := f.snapshot()
		if diff := cmp.Diff([]time.Duration{0, ms(300), ms(600), ms(900)}, claimed); diff != "" {
			t.Errorf("claim times mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunRetriesClaimErrorsAfterPollInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeStore("1 + 1")
		f.claimErrs = []error{errStore, errStore}
		cfg := Config{Workers: 1, OpDelay: ms(100), PollInterval: ms(300)}
		if _, err := runPool(t, newPool(f, f, f, cfg), ms(800), 0); err != nil {
			t.Fatalf("Run() = %v, want nil: store errors must not stop workers", err)
		}
		claimed, _, outcomes := f.snapshot()
		if diff := cmp.Diff([]time.Duration{0, ms(300), ms(600), ms(700)}, claimed); diff != "" {
			t.Errorf("claim times mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]outcome{{ID: jobID(0), Result: 2, At: ms(700)}}, outcomes); diff != "" {
			t.Errorf("outcomes mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunFinishRetries(t *testing.T) {
	for _, tc := range []struct {
		name         string
		finishErrs   []error
		wantFinished []time.Duration
		wantOutcomes []outcome
	}{
		{
			name:         "transient errors are retried after the poll interval",
			finishErrs:   []error{errStore, errStore},
			wantFinished: []time.Duration{0, ms(300), ms(600), ms(600)},
			wantOutcomes: []outcome{{ID: jobID(0), Result: 2, At: ms(600)}, {ID: jobID(1), Result: 3, At: ms(600)}},
		},
		{
			name:         "three failed attempts leave the task running and the worker continues",
			finishErrs:   []error{errStore, errStore, errStore},
			wantFinished: []time.Duration{0, ms(300), ms(600), ms(600)},
			wantOutcomes: []outcome{{ID: jobID(1), Result: 3, At: ms(600)}},
		},
		{
			name:         "a task no longer running is not retried",
			finishErrs:   []error{errNotRunning},
			wantFinished: []time.Duration{0, 0},
			wantOutcomes: []outcome{{ID: jobID(1), Result: 3}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFakeStore("1 + 1", "1 + 2")
				f.finishErrs = tc.finishErrs
				cfg := Config{Workers: 1, PollInterval: ms(300)}
				if _, err := runPool(t, newPool(f, f, f, cfg), ms(1000), 0); err != nil {
					t.Fatalf("Run() = %v, want nil", err)
				}
				_, finished, outcomes := f.snapshot()
				if diff := cmp.Diff(tc.wantFinished, finished); diff != "" {
					t.Errorf("finish attempt times mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantOutcomes, outcomes); diff != "" {
					t.Errorf("outcomes mismatch (-want +got):\n%s", diff)
				}
			})
		})
	}
}

func TestRunStopClaimsLetsActiveTasksFinish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeStore("2 + 3 * (4 - 1)", "100 - 7")
		cfg := Config{Workers: 1, OpDelay: ms(700), PollInterval: ms(300)}
		returned, err := runPool(t, newPool(f, f, f, cfg), ms(1000), 0)
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
		if returned != ms(2100) {
			t.Errorf("Run returned at %v, want 2.1s when the active task ends", returned)
		}
		claimed, _, outcomes := f.snapshot()
		if diff := cmp.Diff([]time.Duration{0}, claimed); diff != "" {
			t.Errorf("claims after stop (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]outcome{{ID: jobID(0), Result: 11, At: ms(2100)}}, outcomes); diff != "" {
			t.Errorf("outcomes mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunCancelExecutionInterruptsDelays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeStore("2 + 3 * (4 - 1)", "(10 - 4) / 3", "100 - 7")
		returned, err := runPool(t, newPool(f, f, f, defaultConfig), ms(500), ms(1000))
		if !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.Canceled) {
			t.Errorf("Run() = %v, want ErrInterrupted and context.Canceled", err)
		}
		if returned != ms(1000) {
			t.Errorf("Run returned at %v, want 1s: delay timers must stop on cancel", returned)
		}
		claimed, finished, _ := f.snapshot()
		if diff := cmp.Diff([]time.Duration{0, 0}, claimed); diff != "" {
			t.Errorf("claim times mismatch (-want +got):\n%s", diff)
		}
		if len(finished) != 0 {
			t.Errorf("interrupted tasks were finished at %v, want them left running", finished)
		}
	})
}

func TestRunRejectsNoWorkers(t *testing.T) {
	f := newFakeStore()
	err := newPool(f, f, f, Config{PollInterval: ms(300)}).Run(t.Context(), t.Context())
	if err == nil {
		t.Error("Run() with zero workers = nil, want an error")
	}
}

func TestRecover(t *testing.T) {
	f := newFakeStore()
	f.requeued = 3
	if err := newPool(f, f, f, defaultConfig).Recover(t.Context()); err != nil {
		t.Errorf("Recover() = %v, want nil", err)
	}
	f.requeueErr = errStore
	if err := newPool(f, f, f, defaultConfig).Recover(t.Context()); !errors.Is(err, errStore) {
		t.Errorf("Recover() = %v, want %v", err, errStore)
	}
}
