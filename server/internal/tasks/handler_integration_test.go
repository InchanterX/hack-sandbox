//go:build integration

package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"calcqueue/internal/auth"
	"calcqueue/internal/openapi"
	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

func TestHTTPTasks(t *testing.T) {
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
	call := func(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	register := func(login string) string {
		w := call(http.HandlerFunc(a.Register), "POST", "/api/auth/register", `{"login":"`+login+`","password":"12345678"}`, "")
		if w.Code != 201 {
			t.Fatalf("register: %d %s", w.Code, w.Body.String())
		}
		var session openapi.AuthSession
		if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		return session.Token
	}
	token, other := register("alice"), register("other")
	if w := call(mux, "GET", "/api/tasks", "", token); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
		t.Fatalf("empty list: %d %s", w.Code, w.Body.String())
	}
	w := call(mux, "POST", "/api/tasks", `{"expression":"1+2"}`, token)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var task openapi.Task
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "queued" || task.Result != nil || task.Error != nil || task.StartedAt != nil || task.FinishedAt != nil {
		t.Fatalf("task: %+v", task)
	}
	path := "/api/tasks/" + task.Id.String()
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		for _, tc := range []struct {
			path, token string
			status      int
		}{{path, other, 404}, {"/api/tasks/invalid", token, 404}, {path, "", 401}} {
			w := call(mux, method, tc.path, `{"expression":"2"}`, tc.token)
			if w.Code != tc.status {
				t.Errorf("%s %s: %d %s", method, tc.path, w.Code, w.Body.String())
			}
		}
	}
	if w := call(mux, "PATCH", path, `{"expression":"5"}`, token); w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	if w := call(mux, "DELETE", path, "", token); w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
}
