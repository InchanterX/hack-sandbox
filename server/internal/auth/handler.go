// Package auth registers users, issues opaque Bearer sessions and protects
// routes with the RequireAuth middleware.
package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
)

// User is the authenticated user stored in the request context. It never
// carries a password, password hash or session token.
type User struct {
	ID        uuid.UUID
	Login     string
	CreatedAt time.Time
}

// Handler serves the authentication endpoints and the Bearer middleware.
type Handler struct{ service *service }

// NewHandler wires the auth service and its SQL store; ttl is the session lifetime.
func NewHandler(pool *pgxpool.Pool, ttl time.Duration) *Handler {
	store := &store{pool: pool}
	return &Handler{service: newService(store, store, ttl)}
}

// Register handles POST /api/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var input openapi.Credentials
	if err := httpjson.Decode(w, r, &input, "login", "password"); err != nil || input.Password == nil {
		httpjson.ValidationError(w)
		return
	}
	result, err := h.service.register(r.Context(), input.Login, *input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, wireSession(result))
}

// Login handles POST /api/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var input openapi.Credentials
	if err := httpjson.Decode(w, r, &input, "login", "password"); err != nil || input.Password == nil {
		httpjson.ValidationError(w)
		return
	}
	result, err := h.service.login(r.Context(), input.Login, *input.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, wireSession(result))
}

// Logout handles POST /api/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	key, ok := r.Context().Value(sessionContextKey{}).([sha256.Size]byte)
	if !ok {
		writeError(w, ErrUnauthorized)
		return
	}
	if err := h.service.logout(r.Context(), key); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeError(w, ErrUnauthorized)
		return
	}
	httpjson.Write(w, http.StatusOK, wireUser(user))
}

// RequireAuth authenticates the Bearer token and puts the User into the
// request context before calling next; otherwise it answers 401 unauthorized.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 {
			writeError(w, ErrUnauthorized)
			return
		}
		scheme, token, ok := strings.Cut(headers[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			writeError(w, ErrUnauthorized)
			return
		}
		user, key, err := h.service.authenticate(r.Context(), token)
		if err != nil {
			writeError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, user)
		ctx = context.WithValue(ctx, sessionContextKey{}, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey struct{}
type sessionContextKey struct{}

// UserFromContext returns the user stored by RequireAuth.
func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(contextKey{}).(User)
	return user, ok
}

func wireUser(user User) openapi.User {
	return openapi.User{Id: user.ID, Login: user.Login, CreatedAt: user.CreatedAt.UTC()}
}

func wireSession(sess issuedSession) openapi.AuthSession {
	return openapi.AuthSession{Token: sess.token, ExpiresAt: sess.expiresAt, User: wireUser(sess.user)}
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpjson.Error(w, http.StatusUnprocessableEntity, openapi.ErrorCodeValidationError, "проверьте поля формы")
	case errors.Is(err, ErrLoginTaken):
		httpjson.Error(w, http.StatusConflict, openapi.ErrorCodeLoginTaken, "логин занят")
	case errors.Is(err, ErrInvalidCredentials):
		httpjson.Error(w, http.StatusUnauthorized, openapi.ErrorCodeInvalidCredentials, "неверный логин или пароль")
	case errors.Is(err, ErrUnauthorized):
		httpjson.Error(w, http.StatusUnauthorized, openapi.ErrorCodeUnauthorized, "требуется вход")
	default:
		// Driver errors may contain credentials or SQL values; never log their text.
		slog.Error("Authentication operation failed")
		httpjson.Error(w, http.StatusInternalServerError, openapi.ErrorCodeInternalError, "внутренняя ошибка сервера")
	}
}
