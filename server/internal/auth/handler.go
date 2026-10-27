// Package auth registers users, issues opaque Bearer sessions and protects
// routes with the RequireAuth middleware.
//
// Scaffold stub (T01): the handlers answer 501 and RequireAuth rejects every
// request until T03 implements the package behind these stable signatures.
package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

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
type Handler struct {
	pool       *pgxpool.Pool
	ttl        time.Duration
	bcryptCost int
}

// NewHandler wires the auth service and its SQL store; ttl is the session lifetime.
func NewHandler(pool *pgxpool.Pool, ttl time.Duration) *Handler {
	return &Handler{pool: pool, ttl: ttl, bcryptCost: bcrypt.DefaultCost}
}

// Register handles POST /api/auth/register.
func (h *Handler) Register(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Login handles POST /api/auth/login.
func (h *Handler) Login(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Logout handles POST /api/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// Me handles GET /api/me.
func (h *Handler) Me(w http.ResponseWriter, _ *http.Request) { notImplemented(w) }

// RequireAuth authenticates the Bearer token and puts the User into the
// request context before calling next; otherwise it answers 401 unauthorized.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	_ = next // The stub fails closed: nothing is authenticated yet.
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpjson.Error(w, http.StatusUnauthorized, openapi.ErrorCodeUnauthorized, "требуется вход")
	})
}

type contextKey struct{}

// UserFromContext returns the user stored by RequireAuth.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}

func notImplemented(w http.ResponseWriter) {
	httpjson.Error(w, http.StatusNotImplemented, openapi.ErrorCodeInternalError, "не реализовано")
}
