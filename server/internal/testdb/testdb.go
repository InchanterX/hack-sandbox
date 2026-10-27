// Package testdb gives integration tests an isolated PostgreSQL schema on the
// database of compose (DATABASE_URL or DB_HOST/DB_NAME/DB_USER/POSTGRES_PASSWORD).
// Each call creates a fresh schema, points the pool search_path at it and drops
// it when the test ends, so tests never share rows or TRUNCATE common tables.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// URL returns the connection string of the test database from the
// environment, building it with URL escaping when DATABASE_URL is unset.
func URL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	host := envOr("DB_HOST", "db")
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "5432")
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(envOr("DB_USER", "calc"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     host,
		Path:     "/" + envOr("DB_NAME", "calc"),
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// NewSchema creates an empty schema with a random name and returns a pool
// whose connections use it as search_path, plus the schema name. The schema and
// the pool are removed with t.Cleanup. The test fails when the database is
// unreachable: integration tests run only in compose service dev.
func NewSchema(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, URL())
	if err != nil {
		t.Fatalf("testdb: connect to database: %v", err)
	}
	schema := "test_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = admin.Close(ctx) // The create error is the one worth reporting.
		t.Fatalf("testdb: create schema %s: %v", schema, err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("testdb: close admin connection: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(URL())
	if err != nil {
		t.Fatalf("testdb: parse database url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testdb: open pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, URL())
		if err != nil {
			t.Errorf("testdb: connect for cleanup: %v", err)
			return
		}
		defer func() { _ = conn.Close(ctx) }() // Cleanup result is reported by DROP.
		if _, err := conn.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("testdb: drop schema %s: %v", schema, err)
		}
	})
	return pool, schema
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomSuffix(t testing.TB) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("testdb: random schema name: %v", err)
	}
	return hex.EncodeToString(b)
}
