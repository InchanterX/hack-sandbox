package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"calcqueue/internal/server"
)

type fakePinger struct {
	err error
}

func (p fakePinger) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("ping without deadline")
	}
	return p.err
}

func TestHealthHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   map[string]any
	}{
		{
			name:       "reachable database",
			wantStatus: http.StatusOK,
			wantBody:   map[string]any{"status": "ok"},
		},
		{
			name:       "failed ping",
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody: map[string]any{"error": map[string]any{
				"code":    "unavailable",
				"message": "база данных недоступна",
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := server.NewHealthHandler(fakePinger{err: tc.pingErr}, time.Second)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %q: %v", w.Body.String(), err)
			}
			if diff := cmp.Diff(tc.wantBody, got); diff != "" {
				t.Errorf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
