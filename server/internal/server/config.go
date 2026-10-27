package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"
)

// Config is the runtime configuration of the server, read once from the
// environment in main and passed to constructors.
type Config struct {
	// Port is the HTTP listen port.
	Port int
	// DatabaseURL is the PostgreSQL connection string; it contains the password
	// and must never be logged.
	DatabaseURL string
	// Workers is the number of queue workers.
	Workers int
	// OpDelay is the delay before each binary operation.
	OpDelay time.Duration
	// PollInterval is the idle wait of a worker between empty claims.
	PollInterval time.Duration
	// SessionTTL is the lifetime of a new session.
	SessionTTL time.Duration
}

// LoadConfig reads the configuration through getenv (os.Getenv in main).
// DATABASE_URL wins when set; otherwise the URL is built from DB_HOST,
// DB_NAME, DB_USER and POSTGRES_PASSWORD with proper URL escaping. Every
// invalid variable is reported; error texts never contain the password.
func LoadConfig(getenv func(string) string) (Config, error) {
	var errs []error
	intVar := func(key string, def, minValue int) int {
		raw := getenv(key)
		if raw == "" {
			return def
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < minValue {
			errs = append(errs, fmt.Errorf("%s=%q: want an integer >= %d", key, raw, minValue))
			return def
		}
		return n
	}

	cfg := Config{
		Port:         intVar("PORT", 8080, 1),
		Workers:      intVar("WORKERS", 2, 1),
		OpDelay:      time.Duration(intVar("OP_DELAY_MS", 700, 0)) * time.Millisecond,
		PollInterval: time.Duration(intVar("POLL_INTERVAL_MS", 300, 1)) * time.Millisecond,
		SessionTTL:   time.Duration(intVar("SESSION_TTL_HOURS", 24, 1)) * time.Hour,
	}
	if cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("PORT=%d: want at most 65535", cfg.Port))
	}

	cfg.DatabaseURL = getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		password := getenv("POSTGRES_PASSWORD")
		if password == "" {
			errs = append(errs, errors.New("POSTGRES_PASSWORD or DATABASE_URL is required"))
		}
		cfg.DatabaseURL = DatabaseURL(
			valueOr(getenv("DB_HOST"), "db"),
			valueOr(getenv("DB_NAME"), "calc"),
			valueOr(getenv("DB_USER"), "calc"),
			password,
		)
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

// DatabaseURL builds a postgres:// connection string, escaping user and
// password. A host without a port gets the default 5432.
func DatabaseURL(host, name, user, password string) string {
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "5432")
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     host,
		Path:     "/" + name,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// Addr returns the HTTP listen address.
func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

// LogValue implements slog.LogValuer and omits the database URL.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("port", c.Port),
		slog.Int("workers", c.Workers),
		slog.Duration("op_delay", c.OpDelay),
		slog.Duration("poll_interval", c.PollInterval),
		slog.Duration("session_ttl", c.SessionTTL),
	)
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
