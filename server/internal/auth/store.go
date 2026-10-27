package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct{ pool *pgxpool.Pool }

var (
	_ userStore    = (*store)(nil)
	_ sessionStore = (*store)(nil)
)

func (s *store) createUserSession(ctx context.Context, login, hash string, sess session) (User, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin registration: %w", err)
	}
	// Rollback after Commit is harmless; all write/commit errors are returned below.
	defer func() { _ = tx.Rollback(ctx) }()
	var user User
	err = tx.QueryRow(ctx, `INSERT INTO users (login, password_hash) VALUES ($1, $2)
 RETURNING id, login, created_at`, login, hash).Scan(&user.ID, &user.Login, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_login_key" {
			return User{}, ErrLoginTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (token_sha256, user_id, expires_at) VALUES ($1, $2, $3)`, sess.key[:], user.ID, sess.expiresAt); err != nil {
		return User{}, fmt.Errorf("insert registration session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit registration: %w", err)
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return user, nil
}

func (s *store) credentials(ctx context.Context, login string) (User, string, error) {
	var user User
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT id, login, created_at, password_hash FROM users WHERE login = $1`, login).Scan(&user.ID, &user.Login, &user.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", fmt.Errorf("select credentials: %w", err)
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return user, hash, nil
}

func (s *store) createSession(ctx context.Context, user User, sess session) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_sha256, user_id, expires_at) VALUES ($1, $2, $3)`, sess.key[:], user.ID, sess.expiresAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (s *store) sessionUser(ctx context.Context, key [sha256.Size]byte, now time.Time) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `SELECT u.id, u.login, u.created_at FROM sessions s
 JOIN users u ON u.id = s.user_id WHERE s.token_sha256 = $1 AND s.expires_at > $2`, key[:], now).Scan(&user.ID, &user.Login, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, fmt.Errorf("select session user: %w", err)
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return user, nil
}

func (s *store) deleteSession(ctx context.Context, key [sha256.Size]byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_sha256 = $1`, key[:]); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
