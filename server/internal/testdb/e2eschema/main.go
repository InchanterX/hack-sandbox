// Command e2eschema gives the API e2e (tests/e2e) an isolated PostgreSQL
// schema on the compose database, because the dev image has no psql.
//
//	e2eschema create        creates a fresh schema and prints {"schema","database_url"}
//	e2eschema drop <schema> drops the schema with everything in it
//
// database_url is the testdb.URL connection string with search_path set to the
// new schema, so a server started with it migrates and queues only there.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"calcqueue/internal/testdb"
)

// schemaName is the only shape of schema this command creates or drops.
var schemaName = regexp.MustCompile(`^e2e_[0-9a-f]{16}$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "e2eschema:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch {
	case len(args) == 1 && args[0] == "create":
		return create(ctx)
	case len(args) == 2 && args[0] == "drop":
		return drop(ctx, args[1])
	default:
		return errors.New("usage: e2eschema create | e2eschema drop <schema>")
	}
}

func create(ctx context.Context) error {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("random schema name: %w", err)
	}
	schema := "e2e_" + hex.EncodeToString(b)
	dsn, err := withSearchPath(testdb.URL(), schema)
	if err != nil {
		return err
	}
	if err := exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	out := struct {
		Schema      string `json:"schema"`
		DatabaseURL string `json:"database_url"`
	}{schema, dsn}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return fmt.Errorf("print result: %w", err)
	}
	return nil
}

func drop(ctx context.Context, schema string) error {
	if !schemaName.MatchString(schema) {
		return fmt.Errorf("refusing to drop %q: not an e2e schema", schema)
	}
	return exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
}

func exec(ctx context.Context, sql string) error {
	conn, err := pgx.Connect(ctx, testdb.URL())
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }() // The statement result is the one worth reporting.
	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("%s: %w", sql, err)
	}
	return nil
}

// withSearchPath adds search_path to a URL or keyword/value connection string;
// pgx passes unknown settings to the server as runtime parameters.
func withSearchPath(dsn, schema string) (string, error) {
	if !strings.Contains(dsn, "://") {
		return dsn + " search_path=" + schema, nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.New("parse database url: invalid URL") // The URL may hold the password.
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
