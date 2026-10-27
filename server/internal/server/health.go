package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
)

// healthTimeout bounds the database ping of GET /api/healthz.
const healthTimeout = 2 * time.Second

// Pinger checks database connectivity; *pgxpool.Pool implements it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewHealthHandler serves GET /api/healthz: 200 {"status":"ok"} when the
// database answers a ping within timeout, otherwise 503 unavailable.
func NewHealthHandler(db Pinger, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			slog.WarnContext(r.Context(), "Health check failed", "error", err)
			httpjson.Error(w, http.StatusServiceUnavailable, openapi.ErrorCodeUnavailable, "база данных недоступна")
			return
		}
		httpjson.Write(w, http.StatusOK, openapi.Health{Status: openapi.Ok})
	})
}
