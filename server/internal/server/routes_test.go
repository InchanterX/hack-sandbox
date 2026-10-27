package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"calcqueue/internal/auth"
	"calcqueue/internal/server"
	"calcqueue/internal/tasks"
)

func TestNewMuxRegistersOpenAPIOperations(t *testing.T) {
	t.Parallel()
	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	mux := server.NewMux(health, auth.NewHandler(nil, time.Hour), tasks.NewHandler(nil))

	tests := []struct {
		method, path, pattern string
	}{
		{http.MethodPost, "/api/auth/register", "POST /api/auth/register"},
		{http.MethodPost, "/api/auth/login", "POST /api/auth/login"},
		{http.MethodPost, "/api/auth/logout", "POST /api/auth/logout"},
		{http.MethodGet, "/api/me", "GET /api/me"},
		{http.MethodPost, "/api/tasks", "POST /api/tasks"},
		{http.MethodGet, "/api/tasks", "GET /api/tasks"},
		{http.MethodGet, "/api/tasks/00000000-0000-4000-8000-000000000002", "GET /api/tasks/{id}"},
		{http.MethodPatch, "/api/tasks/not-a-uuid", "PATCH /api/tasks/{id}"},
		{http.MethodDelete, "/api/tasks/x", "DELETE /api/tasks/{id}"},
		{http.MethodGet, "/api/healthz", "GET /api/healthz"},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			_, got := mux.Handler(httptest.NewRequest(tc.method, tc.path, nil))
			if got != tc.pattern {
				t.Errorf("route pattern for %s %s = %q, want %q", tc.method, tc.path, got, tc.pattern)
			}
		})
	}
}

func TestNewMuxProtectsPrivateRoutes(t *testing.T) {
	t.Parallel()
	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	mux := server.NewMux(health, auth.NewHandler(nil, time.Hour), tasks.NewHandler(nil))

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/auth/logout", http.StatusUnauthorized},
		{http.MethodGet, "/api/me", http.StatusUnauthorized},
		{http.MethodPost, "/api/tasks", http.StatusUnauthorized},
		{http.MethodGet, "/api/tasks", http.StatusUnauthorized},
		{http.MethodGet, "/api/tasks/x", http.StatusUnauthorized},
		{http.MethodPatch, "/api/tasks/x", http.StatusUnauthorized},
		{http.MethodDelete, "/api/tasks/x", http.StatusUnauthorized},
		{http.MethodGet, "/api/healthz", http.StatusTeapot},
		{http.MethodGet, "/api/unknown", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.want {
				t.Errorf("%s %s without token: status = %d, want %d", tc.method, tc.path, w.Code, tc.want)
			}
		})
	}
}
