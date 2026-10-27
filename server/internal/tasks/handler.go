// Package tasks implements CRUD of the authenticated user's calculation tasks.
package tasks

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/auth"
	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
)

// Handler serves authenticated task requests.
type Handler struct{ service *service }

// NewHandler wires the task service and store.
func NewHandler(pool *pgxpool.Pool) *Handler {
	s := &store{pool: pool}
	return &Handler{service: &service{reader: s, mutator: s}}
}
func userID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpjson.Error(w, http.StatusUnauthorized, openapi.ErrorCodeUnauthorized, "требуется вход")
		return uuid.Nil, false
	}
	return user.ID, true
}
func taskID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, errNotFound)
		return uuid.Nil, false
	}
	return id, true
}
func input(w http.ResponseWriter, r *http.Request) (string, bool) {
	var v openapi.ExpressionInput
	if err := httpjson.Decode(w, r, &v, "expression"); err != nil {
		httpjson.ValidationError(w)
		return "", false
	}
	return v.Expression, true
}

// Create enqueues a validated expression.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := userID(w, r)
	if !ok {
		return
	}
	expression, ok := input(w, r)
	if !ok {
		return
	}
	t, err := h.service.create(r.Context(), user, expression)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, wire(t))
}

// List returns the current user’s tasks.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := userID(w, r)
	if !ok {
		return
	}
	ts, err := h.service.reader.list(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]openapi.Task, 0, len(ts))
	for _, t := range ts {
		items = append(items, wire(t))
	}
	httpjson.Write(w, http.StatusOK, openapi.TaskList{Items: items})
}

// Get returns one owned task.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	t, err := h.service.reader.get(r.Context(), user, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, wire(t))
}

// Update replaces an owned queued expression.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	expression, ok := input(w, r)
	if !ok {
		return
	}
	t, err := h.service.update(r.Context(), user, id, expression)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, wire(t))
}

// Delete removes an owned non-running task.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	if err := h.service.mutator.delete(r.Context(), user, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func wire(t task) openapi.Task {
	result := openapi.Task{Id: t.id, Expression: t.expression, Status: openapi.TaskStatus(t.status), Result: t.result, CreatedAt: t.createdAt.UTC(), StartedAt: t.startedAt, FinishedAt: t.finishedAt}
	if t.errorCode != nil && t.errorMessage != nil {
		result.Error = &openapi.ComputationError{Code: openapi.ComputationErrorCode(*t.errorCode), Message: openapi.ComputationErrorMessage(*t.errorMessage)}
	}
	if result.StartedAt != nil {
		v := result.StartedAt.UTC()
		result.StartedAt = &v
	}
	if result.FinishedAt != nil {
		v := result.FinishedAt.UTC()
		result.FinishedAt = &v
	}
	return result
}
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		httpjson.Error(w, 404, openapi.ErrorCodeNotFound, "задача не найдена")
	case errors.Is(err, errNotQueued):
		httpjson.Error(w, 409, openapi.ErrorCodeNotQueued, "изменить можно только задачу в очереди")
	case errors.Is(err, errRunning):
		httpjson.Error(w, 409, openapi.ErrorCodeTaskRunning, "выполняющуюся задачу нельзя удалить")
	case errors.Is(err, errValidation):
		httpjson.Error(w, 422, openapi.ErrorCodeValidationError, "выражение должно содержать от 1 до 200 символов")
	case errors.Is(err, errSyntax):
		httpjson.Error(w, 422, openapi.ErrorCodeSyntaxError, "синтаксическая ошибка")
	default:
		slog.Error("Task operation failed")
		httpjson.Error(w, 500, openapi.ErrorCodeInternalError, "внутренняя ошибка сервера")
	}
}
