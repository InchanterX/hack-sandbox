//go:build integration

package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/auth"
	"calcqueue/internal/openapi"
	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

type httpEnv struct {
	t    *testing.T
	pool *pgxpool.Pool
	auth *auth.Handler
	mux  *http.ServeMux
}

func newHTTPEnv(t *testing.T) *httpEnv {
	t.Helper()
	pool, _ := testdb.NewSchema(t)
	if err := migrations.Apply(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	a := auth.NewHandler(pool, time.Hour)
	h := NewHandler(pool)
	mux := http.NewServeMux()
	mux.Handle("POST /api/tasks", a.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/tasks", a.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/tasks/{id}", a.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /api/tasks/{id}", a.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/tasks/{id}", a.RequireAuth(http.HandlerFunc(h.Delete)))
	return &httpEnv{t: t, pool: pool, auth: a, mux: mux}
}

func (e *httpEnv) call(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func (e *httpEnv) do(method, path, body, token string) *httptest.ResponseRecorder {
	return e.call(e.mux, method, path, body, token)
}

func (e *httpEnv) register(login string) string {
	e.t.Helper()
	w := e.call(http.HandlerFunc(e.auth.Register), "POST", "/api/auth/register", `{"login":"`+login+`","password":"12345678"}`, "")
	if w.Code != 201 {
		e.t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var session openapi.AuthSession
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		e.t.Fatal(err)
	}
	return session.Token
}

// create posts a valid expression and returns the decoded task.
func (e *httpEnv) create(token, expression string) openapi.Task {
	e.t.Helper()
	w := e.do("POST", "/api/tasks", `{"expression":"`+expression+`"}`, token)
	if w.Code != 201 {
		e.t.Fatalf("create %q: %d %s", expression, w.Code, w.Body.String())
	}
	return decodeTask(e.t, w)
}

func (e *httpEnv) count() int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.t.Context(), `SELECT count(*) FROM tasks`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *httpEnv) expression(id uuid.UUID) string {
	e.t.Helper()
	var v string
	if err := e.pool.QueryRow(e.t.Context(), `SELECT expression FROM tasks WHERE id=$1`, id).Scan(&v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

// setStatus moves a task into a state allowed by tasks_state_fields.
func (e *httpEnv) setStatus(id uuid.UUID, status string) {
	e.t.Helper()
	query := map[string]string{
		"running": `UPDATE tasks SET status='running', started_at=now() WHERE id=$1`,
		"done":    `UPDATE tasks SET status='done', result=3, started_at=now(), finished_at=now() WHERE id=$1`,
		"failed":  `UPDATE tasks SET status='failed', error_code='division_by_zero', error_message='деление на ноль', started_at=now(), finished_at=now() WHERE id=$1`,
	}[status]
	if _, err := e.pool.Exec(e.t.Context(), query, id); err != nil {
		e.t.Fatal(err)
	}
}

// decodeTask checks the OpenAPI wire shape: exactly the Task keys, nullable ones present.
func decodeTask(t *testing.T, w *httptest.ResponseRecorder) openapi.Task {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	keys := []string{"id", "expression", "status", "result", "error", "created_at", "started_at", "finished_at"}
	if len(raw) != len(keys) {
		t.Fatalf("task keys: %s", w.Body.String())
	}
	for _, k := range keys {
		if _, ok := raw[k]; !ok {
			t.Fatalf("task key %q missing: %s", k, w.Body.String())
		}
	}
	var task openapi.Task
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	return task
}

func assertQueuedShape(t *testing.T, w *httptest.ResponseRecorder) openapi.Task {
	t.Helper()
	task := decodeTask(t, w)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	for _, k := range []string{"result", "error", "started_at", "finished_at"} {
		if string(raw[k]) != "null" {
			t.Fatalf("queued %s = %s, want null", k, raw[k])
		}
	}
	if task.Status != openapi.Queued {
		t.Fatalf("status %q", task.Status)
	}
	return task
}

func assertError(t *testing.T, w *httptest.ResponseRecorder, status int, code openapi.ErrorCode) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var env openapi.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("envelope %s: %v", w.Body.String(), err)
	}
	if env.Error.Code != code || env.Error.Message == "" {
		t.Fatalf("error %+v, want code %s", env.Error, code)
	}
}

func TestHTTPCreateListGet(t *testing.T) {
	e := newHTTPEnv(t)
	token, other := e.register("alice"), e.register("other")
	if w := e.do("GET", "/api/tasks", "", token); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
		t.Fatalf("empty list: %d %s", w.Code, w.Body.String())
	}
	w := e.do("POST", "/api/tasks", `{"expression":"1 + 2"}`, token)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	first := assertQueuedShape(t, w)
	if first.Expression != "1 + 2" || first.Id == uuid.Nil || first.CreatedAt.IsZero() {
		t.Fatalf("created: %+v", first)
	}
	foreign := e.create(other, "7")
	second := e.create(token, "3")

	w = e.do("GET", "/api/tasks/"+first.Id.String(), "", token)
	if w.Code != 200 {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	got := assertQueuedShape(t, w)
	if got.Id != first.Id || got.Expression != first.Expression || !got.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("get: %+v, want %+v", got, first)
	}

	w = e.do("GET", "/api/tasks", "", token)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var list openapi.TaskList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Items[0].Id != second.Id || list.Items[1].Id != first.Id {
		t.Fatalf("list must hold own tasks newest first: %s", w.Body.String())
	}
	for _, item := range list.Items {
		if item.Id == foreign.Id {
			t.Fatalf("list leaked foreign task: %s", w.Body.String())
		}
	}
	w = e.do("GET", "/api/tasks", "", other)
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 1 || list.Items[0].Id != foreign.Id {
		t.Fatalf("other list: %s %v", w.Body.String(), err)
	}
}

func TestHTTPNotFoundAndUnauthorized(t *testing.T) {
	e := newHTTPEnv(t)
	token, other := e.register("alice"), e.register("other")
	own := e.create(token, "1+2")
	foreign := e.create(other, "3")
	ownPath := "/api/tasks/" + own.Id.String()
	cases := map[string]string{
		"foreign": "/api/tasks/" + foreign.Id.String(),
		"missing": "/api/tasks/" + uuid.New().String(),
		"invalid": "/api/tasks/not-a-uuid",
	}
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		for name, path := range cases {
			t.Run(method+" "+name, func(t *testing.T) {
				assertError(t, e.do(method, path, `{"expression":"2"}`, token), 404, openapi.ErrorCodeNotFound)
			})
		}
	}
	if got := e.expression(foreign.Id); got != "3" || e.count() != 2 {
		t.Fatalf("foreign task changed: %q count %d", got, e.count())
	}

	// Without a valid token every route answers 401 regardless of the target.
	paths := []string{ownPath, cases["foreign"], cases["missing"], cases["invalid"]}
	requests := []struct{ method, path string }{{"POST", "/api/tasks"}, {"GET", "/api/tasks"}}
	for _, p := range paths {
		for _, m := range []string{"GET", "PATCH", "DELETE"} {
			requests = append(requests, struct{ method, path string }{m, p})
		}
	}
	for _, token := range []string{"", "wrong-token"} {
		for _, r := range requests {
			t.Run("401 "+r.method+" "+r.path+" "+token, func(t *testing.T) {
				assertError(t, e.do(r.method, r.path, `{"expression":"bad("}`, token), 401, openapi.ErrorCodeUnauthorized)
			})
		}
	}
	if e.count() != 2 || e.expression(own.Id) != "1+2" {
		t.Fatal("unauthenticated requests changed tasks")
	}
}

var invalidExpressions = []struct {
	name, body string
	code       openapi.ErrorCode
}{
	{"empty", `{"expression":""}`, openapi.ErrorCodeValidationError},
	{"201 chars", `{"expression":"` + strings.Repeat("1", 201) + `"}`, openapi.ErrorCodeValidationError},
	{"201 code points", `{"expression":"` + strings.Repeat("й", 201) + `"}`, openapi.ErrorCodeValidationError},
	{"tab", `{"expression":"1\t+2"}`, openapi.ErrorCodeSyntaxError},
	{"letter", `{"expression":"1+x"}`, openapi.ErrorCodeSyntaxError},
	{"unicode", `{"expression":"й"}`, openapi.ErrorCodeSyntaxError},
	{"dangling operator", `{"expression":"1+"}`, openapi.ErrorCodeSyntaxError},
	{"unbalanced", `{"expression":"(1+2"}`, openapi.ErrorCodeSyntaxError},
}

func TestHTTPCreateValidation(t *testing.T) {
	e := newHTTPEnv(t)
	token := e.register("alice")
	e.create(token, "1")
	for _, tc := range invalidExpressions {
		t.Run(tc.name, func(t *testing.T) {
			assertError(t, e.do("POST", "/api/tasks", tc.body, token), 422, tc.code)
			if n := e.count(); n != 1 {
				t.Fatalf("row count %d after rejected create", n)
			}
		})
	}
	if task := e.create(token, "11"+strings.Repeat("+1", 99)); len(task.Expression) != 200 {
		t.Fatalf("200 chars rejected: %+v", task)
	}
}

func TestHTTPUpdate(t *testing.T) {
	e := newHTTPEnv(t)
	token := e.register("alice")
	task := e.create(token, "1+2")
	path := "/api/tasks/" + task.Id.String()
	for _, tc := range invalidExpressions {
		t.Run(tc.name, func(t *testing.T) {
			assertError(t, e.do("PATCH", path, tc.body, token), 422, tc.code)
			if got := e.expression(task.Id); got != "1+2" || e.count() != 1 {
				t.Fatalf("stored %q count %d after rejected patch", got, e.count())
			}
		})
	}
	w := e.do("PATCH", path, `{"expression":"4 * (5 - 1)"}`, token)
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	updated := assertQueuedShape(t, w)
	if updated.Id != task.Id || !updated.CreatedAt.Equal(task.CreatedAt) || updated.Expression != "4 * (5 - 1)" {
		t.Fatalf("patched %+v from %+v", updated, task)
	}
	if got := e.expression(task.Id); got != "4 * (5 - 1)" {
		t.Fatalf("stored %q", got)
	}

	for _, status := range []string{"running", "done", "failed"} {
		t.Run(status, func(t *testing.T) {
			task := e.create(token, "8")
			e.setStatus(task.Id, status)
			path := "/api/tasks/" + task.Id.String()
			// Status errors take precedence over expression validation.
			for _, body := range []string{`{"expression":"9"}`, `{"expression":""}`, `{"expression":"1+"}`} {
				assertError(t, e.do("PATCH", path, body, token), 409, openapi.ErrorCodeNotQueued)
			}
			if got := e.expression(task.Id); got != "8" {
				t.Fatalf("stored %q after 409", got)
			}
		})
	}
}

func TestHTTPDelete(t *testing.T) {
	e := newHTTPEnv(t)
	token := e.register("alice")
	running := e.create(token, "1")
	e.setStatus(running.Id, "running")
	assertError(t, e.do("DELETE", "/api/tasks/"+running.Id.String(), "", token), 409, openapi.ErrorCodeTaskRunning)
	if w := e.do("GET", "/api/tasks/"+running.Id.String(), "", token); w.Code != 200 {
		t.Fatalf("running task gone: %d", w.Code)
	}
	for _, status := range []string{"queued", "done", "failed"} {
		t.Run(status, func(t *testing.T) {
			task := e.create(token, "2")
			if status != "queued" {
				e.setStatus(task.Id, status)
			}
			path := "/api/tasks/" + task.Id.String()
			w := e.do("DELETE", path, "", token)
			if w.Code != 204 || w.Body.Len() != 0 {
				t.Fatalf("delete %s: %d %q", status, w.Code, w.Body.String())
			}
			assertError(t, e.do("GET", path, "", token), 404, openapi.ErrorCodeNotFound)
			assertError(t, e.do("DELETE", path, "", token), 404, openapi.ErrorCodeNotFound)
		})
	}
	if e.count() != 1 {
		t.Fatalf("row count %d, want only running", e.count())
	}
}
