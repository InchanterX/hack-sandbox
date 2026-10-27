// Package migrations embeds the goose SQL migrations of the database schema
// and applies them at startup.
package migrations

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// Apply runs every pending embedded migration against pool. It is idempotent:
// already applied versions are skipped. A Postgres advisory lock serializes
// concurrent callers, so two starting processes never migrate at once.
// Migrations run in the first schema of the connection search_path.
func Apply(ctx context.Context, pool *pgxpool.Pool) (err error) {
	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close migration db: %w", closeErr)
		}
	}()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
