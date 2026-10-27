package server_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"calcqueue/internal/server"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Parallel()
	got, err := server.LoadConfig(env(map[string]string{"POSTGRES_PASSWORD": "secret"}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	want := server.Config{
		Port:         8080,
		DatabaseURL:  "postgres://calc:secret@db:5432/calc?sslmode=disable",
		Workers:      2,
		OpDelay:      700 * time.Millisecond,
		PollInterval: 300 * time.Millisecond,
		SessionTTL:   24 * time.Hour,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadConfig() mismatch (-want +got):\n%s", diff)
	}
	if got.Addr() != ":8080" {
		t.Errorf("Addr() = %q, want :8080", got.Addr())
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Parallel()
	got, err := server.LoadConfig(env(map[string]string{
		"PORT":              "9090",
		"WORKERS":           "4",
		"OP_DELAY_MS":       "0",
		"POLL_INTERVAL_MS":  "50",
		"SESSION_TTL_HOURS": "1",
		"DATABASE_URL":      "postgres://u:p@h:1/d",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	want := server.Config{
		Port:         9090,
		DatabaseURL:  "postgres://u:p@h:1/d",
		Workers:      4,
		PollInterval: 50 * time.Millisecond,
		SessionTTL:   time.Hour,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	t.Parallel()
	const password = "top-secret-value"
	tests := []struct {
		name    string
		env     map[string]string
		wantKey string
	}{
		{name: "non-numeric port", env: map[string]string{"PORT": "http"}, wantKey: "PORT"},
		{name: "port out of range", env: map[string]string{"PORT": "70000"}, wantKey: "PORT"},
		{name: "zero workers", env: map[string]string{"WORKERS": "0"}, wantKey: "WORKERS"},
		{name: "negative op delay", env: map[string]string{"OP_DELAY_MS": "-1"}, wantKey: "OP_DELAY_MS"},
		{name: "zero poll interval", env: map[string]string{"POLL_INTERVAL_MS": "0"}, wantKey: "POLL_INTERVAL_MS"},
		{name: "fractional session ttl", env: map[string]string{"SESSION_TTL_HOURS": "1.5"}, wantKey: "SESSION_TTL_HOURS"},
		{name: "no database credentials", env: map[string]string{}, wantKey: "POSTGRES_PASSWORD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := tc.env["POSTGRES_PASSWORD"]; !ok && tc.wantKey != "POSTGRES_PASSWORD" {
				tc.env["POSTGRES_PASSWORD"] = password
			}
			_, err := server.LoadConfig(env(tc.env))
			if err == nil {
				t.Fatalf("LoadConfig(%v) error = nil, want an error naming %s", tc.env, tc.wantKey)
			}
			if !strings.Contains(err.Error(), tc.wantKey) {
				t.Errorf("LoadConfig(%v) error = %q, want it to name %s", tc.env, err, tc.wantKey)
			}
			if strings.Contains(err.Error(), password) {
				t.Errorf("LoadConfig(%v) error leaks the password: %q", tc.env, err)
			}
		})
	}
}

func TestLoadConfigEscapesPassword(t *testing.T) {
	t.Parallel()
	const password = "p@ss w/rd:%?#&"
	cfg, err := server.LoadConfig(env(map[string]string{
		"DB_HOST":           "postgres.local",
		"DB_NAME":           "calc",
		"DB_USER":           "calc user",
		"POSTGRES_PASSWORD": password,
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("parse DatabaseURL: %v", err)
	}
	gotPassword, _ := u.User.Password()
	got := []string{u.Scheme, u.User.Username(), gotPassword, u.Host, u.Path}
	want := []string{"postgres", "calc user", password, "postgres.local:5432", "/calc"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DatabaseURL parts mismatch (-want +got):\n%s", diff)
	}
	if strings.Contains(cfg.LogValue().String(), password) {
		t.Errorf("Config.LogValue() leaks the password")
	}
}
