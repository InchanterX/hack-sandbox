// Package server composes the HTTP route table, the health check and the
// runtime configuration of the API server.
package server

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/auth"
	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
	"calcqueue/internal/tasks"
)

// NewHandler returns the route table of the ten OpenAPI operations.
// Register, login and healthz are public; every other route is wrapped with
// authHandler.RequireAuth.
func NewHandler(pool *pgxpool.Pool, authHandler *auth.Handler, tasksHandler *tasks.Handler) http.Handler {
	return newMux(NewHealthHandler(pool, healthTimeout), authHandler, tasksHandler)
}

func newMux(health http.Handler, a *auth.Handler, t *tasks.Handler) *http.ServeMux {
	protected := func(h http.HandlerFunc) http.Handler { return a.RequireAuth(h) }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", a.Register)
	mux.HandleFunc("POST /api/auth/login", a.Login)
	mux.Handle("POST /api/auth/logout", protected(a.Logout))
	mux.Handle("GET /api/me", protected(a.Me))
	mux.Handle("POST /api/tasks", protected(t.Create))
	mux.Handle("GET /api/tasks", protected(t.List))
	mux.Handle("GET /api/tasks/{id}", protected(t.Get))
	mux.Handle("PATCH /api/tasks/{id}", protected(t.Update))
	mux.Handle("DELETE /api/tasks/{id}", protected(t.Delete))
	mux.Handle("GET /api/healthz", health)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		httpjson.Error(w, http.StatusNotFound, openapi.ErrorCodeNotFound, "не найдено")
	})
	return mux
}
