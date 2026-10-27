// Package tasks implements CRUD of the authenticated user's calculation tasks.
//
// Scaffold stub (T01): the handlers answer 501 until T04 implements them
// behind these stable signatures.
package tasks

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
)

// Handler serves the task endpoints; every route is behind auth.RequireAuth.
type Handler struct {
	pool *pgxpool.Pool
}

// NewHandler wires the task service and its SQL store.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// Create handles POST /api/tasks.
func (h *Handler) Create(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// List handles GET /api/tasks.
func (h *Handler) List(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Get handles GET /api/tasks/{id}.
func (h *Handler) Get(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Update handles PATCH /api/tasks/{id}.
func (h *Handler) Update(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Delete handles DELETE /api/tasks/{id}.
func (h *Handler) Delete(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

func notImplemented(w http.ResponseWriter) {
	httpjson.Error(w, http.StatusNotImplemented, openapi.ErrorCodeInternalError, "не реализовано")
}
