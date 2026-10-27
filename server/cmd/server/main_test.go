//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	"calcqueue/internal/server"
	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

func TestMain(m *testing.M) {
	// The os/signal dispatcher started by signal.NotifyContext lives forever.
	goleak.VerifyTestMain(m, goleak.IgnoreAnyFunction("os/signal.loop"))
}

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

// harness is an isolated schema: pool for seeding and inspection, cfg for the
// server under test.
type harness struct {
	pool *pgxpool.Pool
	cfg  server.Config
}

func newHarness(t *testing.T) harness {
	t.Helper()
	pool, schema := testdb.NewSchema(t)
	u, err := url.Parse(testdb.URL())
	if err != nil {
		t.Fatalf("parse test database url: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return harness{pool: pool, cfg: server.Config{
		Port:         8080,
		DatabaseURL:  u.String(),
		Workers:      2,
		OpDelay:      10 * time.Millisecond,
		PollInterval: 20 * time.Millisecond,
		SessionTTL:   time.Hour,
	}}
}

// seed applies migrations ahead of the server and inserts a user.
func (h harness) seed(t *testing.T) uuid.UUID {
	t.Helper()
	if err := migrations.Apply(t.Context(), h.pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	var userID uuid.UUID
	if err := h.pool.QueryRow(t.Context(), `INSERT INTO users (login, password_hash) VALUES ('seeded', 'hash')
RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}

var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// insert adds a task created at base+offset and runs extra, if any, on it.
func (h harness) insert(t *testing.T, userID uuid.UUID, expression string, offset time.Duration, extra string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := h.pool.Exec(t.Context(), `INSERT INTO tasks (id, user_id, expression, created_at)
VALUES ($1, $2, $3, $4)`, id, userID, expression, base.Add(offset)); err != nil {
		t.Fatalf("insert task %q: %v", expression, err)
	}
	if extra != "" {
		if _, err := h.pool.Exec(t.Context(), extra, id); err != nil {
			t.Fatalf("update task %q: %v", expression, err)
		}
	}
	return id
}

func (h harness) row(t *testing.T, id uuid.UUID) row {
	t.Helper()
	var r row
	if err := h.pool.QueryRow(t.Context(), `SELECT status, expression, result, error_code, error_message,
  started_at, finished_at FROM tasks WHERE id = $1`, id).Scan(
		&r.Status, &r.Expression, &r.Result, &r.ErrorCode, &r.ErrorMessage, &r.StartedAt, &r.FinishedAt,
	); err != nil {
		t.Fatalf("read task %s: %v", id, err)
	}
	return r
}

// startApp runs start with a short connect budget and closes the app at cleanup.
func startApp(t *testing.T, cfg server.Config) *app {
	t.Helper()
	a, err := start(t.Context(), cfg, 10*time.Second)
	if err != nil {
		t.Fatalf("start() = %v", err)
	}
	t.Cleanup(a.close)
	return a
}

// serveApp serves a on a loopback port; stop cancels the serve context as a
// stop signal does and returns the error of serve.
func serveApp(t *testing.T, a *app, grace time.Duration) (baseURL string, stop func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, ln, grace) }()
	stop = sync.OnceValue(func() error {
		cancel()
		return <-done
	})
	t.Cleanup(func() { _ = stop() }) // A test that cares checks stop itself.
	return "http://" + ln.Addr().String(), stop
}

// waitFor polls cond every 10 ms until it holds or timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func ptr[T any](v T) *T { return &v }

func TestLifecycleBudgets(t *testing.T) {
	if connectTimeout != 60*time.Second {
		t.Errorf("connectTimeout = %v, want 60s startup budget", connectTimeout)
	}
	if shutdownGrace != 10*time.Second {
		t.Errorf("shutdownGrace = %v, want 10s for active work", shutdownGrace)
	}
	// HTTP and workers share the grace, then canceled workers get stopTimeout.
	if total := shutdownGrace + stopTimeout; total >= 30*time.Second {
		t.Errorf("worst-case stop = %v, want under the 30s pod grace", total)
	}
}

func TestConnectRetriesWithinBudget(t *testing.T) {
	budget := 1500 * time.Millisecond
	began := time.Now()
	pool, err := connect(t.Context(), "postgres://calc:s3cret@127.0.0.1:1/calc?sslmode=disable", budget)
	elapsed := time.Since(began)
	if err == nil {
		pool.Close()
		t.Fatal("connect() to a closed port = nil error, want a timeout")
	}
	if elapsed < budget-100*time.Millisecond || elapsed > budget+3*time.Second {
		t.Errorf("connect() gave up after %v, want about the %v budget", elapsed, budget)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("connect() error leaks the password: %v", err)
	}

	h := newHarness(t)
	pool, err = connect(t.Context(), h.cfg.DatabaseURL, budget)
	if err != nil {
		t.Fatalf("connect() to the test database = %v", err)
	}
	pool.Close()
}

func TestStartMigratesThenRecoversBeforeServing(t *testing.T) {
	h := newHarness(t)
	// An empty schema: recovery can only succeed after migrations created tasks.
	startApp(t, h.cfg).close()

	userID := h.seed(t) // Idempotent: the migrations are already applied.
	queued := h.insert(t, userID, "100 - 7", 0, "")
	running := h.insert(t, userID, "2 + 3 * (4 - 1)", time.Second,
		`UPDATE tasks SET status = 'running', started_at = now() WHERE id = $1`)
	done := h.insert(t, userID, "1 + 1", 2*time.Second,
		`UPDATE tasks SET status = 'done', result = 2, started_at = now(), finished_at = now() WHERE id = $1`)
	failed := h.insert(t, userID, "7 / (2 - 2)", 3*time.Second, `UPDATE tasks SET status = 'failed',
  error_code = 'division_by_zero', error_message = 'деление на ноль', started_at = now(), finished_at = now()
WHERE id = $1`)
	terminal := map[uuid.UUID]row{done: h.row(t, done), failed: h.row(t, failed)}

	a := startApp(t, h.cfg)
	// start has returned, no worker runs yet: the interrupted row is queued
	// with every execution field cleared, the others are untouched.
	if diff := cmp.Diff(row{Status: "queued", Expression: "2 + 3 * (4 - 1)"}, h.row(t, running)); diff != "" {
		t.Errorf("recovered row before serving (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(row{Status: "queued", Expression: "100 - 7"}, h.row(t, queued)); diff != "" {
		t.Errorf("queued row before serving (-want +got):\n%s", diff)
	}

	_, stop := serveApp(t, a, time.Second)
	waitFor(t, 10*time.Second, "queued and recovered tasks to finish", func() bool {
		return h.row(t, queued).Status == "done" && h.row(t, running).Status == "done"
	})
	if err := stop(); err != nil {
		t.Errorf("serve() = %v, want nil after stop", err)
	}
	if got := h.row(t, running).Result; got == nil || *got != 11 {
		t.Errorf("recovered task result = %v, want 11", got)
	}
	if got := h.row(t, queued).Result; got == nil || *got != 93 {
		t.Errorf("queued task result = %v, want 93", got)
	}
	for id, want := range terminal {
		if diff := cmp.Diff(want, h.row(t, id)); diff != "" {
			t.Errorf("terminal row %q changed (-want +got):\n%s", want.Expression, diff)
		}
	}
}

// client calls the API of the server under test.
type client struct {
	t     *testing.T
	base  string
	http  *http.Client
	token string
}

func newClient(t *testing.T, baseURL string) *client {
	t.Helper()
	// No keep-alive: no idle connection goroutines outlive the test.
	return &client{t: t, base: baseURL, http: &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}}
}

// do sends a JSON request and decodes a JSON response body into out, if any.
func (c *client) do(method, path string, body, out any) int {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }() // Read-only response.
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

type session struct {
	Token string `json:"token"`
}

type task struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Result *float64 `json:"result"`
}

func TestServeAnswersTheTenRoutes(t *testing.T) {
	h := newHarness(t)
	baseURL, stop := serveApp(t, startApp(t, h.cfg), time.Second)
	c := newClient(t, baseURL)

	var health struct{ Status string }
	if got := c.do(http.MethodGet, "/api/healthz", nil, &health); got != http.StatusOK || health.Status != "ok" {
		t.Fatalf("GET /api/healthz = %d %+v, want 200 ok", got, health)
	}
	creds := map[string]string{"login": "alice", "password": "correct horse"}
	var s session
	if got := c.do(http.MethodPost, "/api/auth/register", creds, &s); got != http.StatusCreated || s.Token == "" {
		t.Fatalf("POST /api/auth/register = %d %+v, want 201 with a token", got, s)
	}
	if got := c.do(http.MethodPost, "/api/auth/login", creds, &s); got != http.StatusOK || s.Token == "" {
		t.Fatalf("POST /api/auth/login = %d %+v, want 200 with a token", got, s)
	}
	c.token = s.Token
	var me struct{ Login string }
	if got := c.do(http.MethodGet, "/api/me", nil, &me); got != http.StatusOK || me.Login != "alice" {
		t.Errorf("GET /api/me = %d %+v, want 200 alice", got, me)
	}

	var created task
	if got := c.do(http.MethodPost, "/api/tasks", map[string]string{"expression": "2 + 3 * (4 - 1)"}, &created); got != http.StatusCreated {
		t.Fatalf("POST /api/tasks = %d, want 201", got)
	}
	var list struct{ Items []task }
	if got := c.do(http.MethodGet, "/api/tasks", nil, &list); got != http.StatusOK || len(list.Items) != 1 {
		t.Errorf("GET /api/tasks = %d with %d items, want 200 with 1", got, len(list.Items))
	}
	path := "/api/tasks/" + created.ID
	var got task
	waitFor(t, 10*time.Second, "the task to be done", func() bool {
		if code := c.do(http.MethodGet, path, nil, &got); code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, code)
		}
		return got.Status == "done" || got.Status == "failed"
	})
	if diff := cmp.Diff(task{ID: created.ID, Status: "done", Result: ptr(11.0)}, got); diff != "" {
		t.Errorf("executed task mismatch (-want +got):\n%s", diff)
	}
	if code := c.do(http.MethodPatch, path, map[string]string{"expression": "1 + 1"}, nil); code != http.StatusConflict {
		t.Errorf("PATCH done task = %d, want 409", code)
	}
	if code := c.do(http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Errorf("DELETE done task = %d, want 204", code)
	}
	if code := c.do(http.MethodGet, path, nil, nil); code != http.StatusNotFound {
		t.Errorf("GET deleted task = %d, want 404", code)
	}
	if code := c.do(http.MethodPost, "/api/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Errorf("POST /api/auth/logout = %d, want 204", code)
	}
	if code := c.do(http.MethodGet, "/api/me", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("GET /api/me after logout = %d, want 401", code)
	}

	if err := stop(); err != nil {
		t.Errorf("serve() = %v, want nil after stop", err)
	}
}

func TestLogConfigOmitsTheDatabasePassword(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	logConfig(t.Context(), server.Config{Port: 8080, DatabaseURL: "postgres://calc:s3cret@db:5432/calc",
		Workers: 2, OpDelay: 700 * time.Millisecond, PollInterval: 300 * time.Millisecond, SessionTTL: time.Hour})
	if out := buf.String(); strings.Contains(out, "s3cret") || !strings.Contains(out, `"workers":2`) {
		t.Errorf("config log = %s, want the settings without the password", out)
	}
}

// The grace tests run the real shutdownGrace, in parallel to save wall time.

func TestStopLetsActiveWorkFinishWithinGrace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	userID := h.seed(t)
	active := h.insert(t, userID, "2 + 3 * (4 - 1)", 0, "")
	cfg := h.cfg
	// One worker: after stop it is busy with the active task, so nothing else
	// may claim the task created after stop. Three 2 s operations: the task
	// needs about 6 s after the stop, inside the 10 s grace.
	cfg.Workers, cfg.OpDelay = 1, 2*time.Second
	_, stop := serveApp(t, startApp(t, cfg), shutdownGrace)

	waitFor(t, 5*time.Second, "the task to run", func() bool { return h.row(t, active).Status == "running" })
	began := time.Now()
	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	late := h.insert(t, userID, "100 - 7", time.Second, "")
	if err := <-stopped; err != nil {
		t.Errorf("serve() = %v, want nil when active work finishes in time", err)
	}
	if elapsed := time.Since(began); elapsed < 4*time.Second || elapsed >= shutdownGrace {
		t.Errorf("stop took %v, want the active task to run on and end before the %v grace", elapsed, shutdownGrace)
	}
	if got := h.row(t, active); got.Status != "done" || got.Result == nil || *got.Result != 11 {
		t.Errorf("active row = %+v, want done with 11", got)
	}
	if got := h.row(t, late).Status; got != "queued" {
		t.Errorf("task created after stop is %s, want queued: no claims after stop", got)
	}
}

func TestStopCancelsWorkAfterGraceAndLeavesItRecoverable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	userID := h.seed(t)
	active := h.insert(t, userID, "2 + 3 * (4 - 1)", 0, "")
	cfg := h.cfg
	cfg.Workers, cfg.OpDelay = 1, time.Hour
	_, stop := serveApp(t, startApp(t, cfg), shutdownGrace)

	waitFor(t, 5*time.Second, "the task to run", func() bool { return h.row(t, active).Status == "running" })
	began := time.Now()
	if err := stop(); err != nil {
		t.Errorf("serve() = %v, want nil: interrupted tasks are recovered, not a failed stop", err)
	}
	if elapsed := time.Since(began); elapsed < shutdownGrace || elapsed > shutdownGrace+stopTimeout {
		t.Errorf("stop took %v, want the %v grace and then cancellation", elapsed, shutdownGrace)
	}
	if got := h.row(t, active); got.Status != "running" || got.ErrorCode != nil || got.FinishedAt != nil {
		t.Errorf("interrupted row = %+v, want still running, not failed", got)
	}

	// The next start recovers the row before serving and then completes it.
	next := startApp(t, h.cfg)
	if diff := cmp.Diff(row{Status: "queued", Expression: "2 + 3 * (4 - 1)"}, h.row(t, active)); diff != "" {
		t.Errorf("recovered row mismatch (-want +got):\n%s", diff)
	}
	_, stopNext := serveApp(t, next, time.Second)
	waitFor(t, 10*time.Second, "the recovered task to finish", func() bool { return h.row(t, active).Status == "done" })
	if err := stopNext(); err != nil {
		t.Errorf("second serve() = %v, want nil", err)
	}
	if got := h.row(t, active).Result; got == nil || *got != 11 {
		t.Errorf("recovered task result = %v, want 11", got)
	}
}

func TestRunExitsOnSIGTERM(t *testing.T) {
	h := newHarness(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"DATABASE_URL":      h.cfg.DatabaseURL,
		"PORT":              strconv.Itoa(port),
		"WORKERS":           "2",
		"OP_DELAY_MS":       "10",
		"POLL_INTERVAL_MS":  "20",
		"SESSION_TTL_HOURS": "1",
	}

	ctx, stopSignals := notifyContext()
	defer stopSignals()
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(key string) string { return env[key] }) }()

	c := newClient(t, "http://127.0.0.1:"+strconv.Itoa(port))
	c.http.Timeout = time.Second
	waitFor(t, 30*time.Second, "the server to become healthy", func() bool {
		select {
		case err := <-done:
			t.Fatalf("run() returned before SIGTERM: %v", err)
		default:
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, c.base+"/api/healthz", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close() // Only the status matters.
		return resp.StatusCode == http.StatusOK
	})

	began := time.Now()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() after SIGTERM = %v, want nil", err)
		}
	case <-timer.C:
		t.Fatal("run() did not return within the 30s pod grace after SIGTERM")
	}
	if elapsed := time.Since(began); elapsed > shutdownGrace {
		t.Errorf("idle server stopped after %v, want well inside the %v grace", elapsed, shutdownGrace)
	}
}
