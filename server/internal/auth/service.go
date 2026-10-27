package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// ErrValidation reports credentials outside the contract's syntax or byte limits.
var ErrValidation = errors.New("invalid credentials format")

// ErrLoginTaken reports an exact login already registered by another user.
var ErrLoginTaken = errors.New("login taken")

// ErrInvalidCredentials reports a login and password that do not match.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrUnauthorized reports a missing, invalid, expired or revoked session.
var ErrUnauthorized = errors.New("unauthorized")

var loginPattern = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

type userStore interface {
	createUserSession(context.Context, string, string, session) (User, error)
	credentials(context.Context, string) (User, string, error)
}

type sessionStore interface {
	createSession(context.Context, User, session) error
	sessionUser(context.Context, [sha256.Size]byte, time.Time) (User, error)
	deleteSession(context.Context, [sha256.Size]byte) error
}

type session struct {
	key       [sha256.Size]byte
	expiresAt time.Time
}

type issuedSession struct {
	token     string
	expiresAt time.Time
	user      User
}

type service struct {
	users      userStore
	sessions   sessionStore
	ttl        time.Duration
	bcryptCost int
	now        func() time.Time
}

func newService(users userStore, sessions sessionStore, ttl time.Duration) *service {
	return &service{users: users, sessions: sessions, ttl: ttl, bcryptCost: bcrypt.DefaultCost, now: time.Now}
}

func (s *service) register(ctx context.Context, login, password string) (issuedSession, error) {
	if err := validateCredentials(login, password); err != nil {
		return issuedSession{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return issuedSession{}, fmt.Errorf("hash password: %w", err)
	}
	token, stored, err := s.issue()
	if err != nil {
		return issuedSession{}, err
	}
	user, err := s.users.createUserSession(ctx, login, string(hash), stored)
	if err != nil {
		return issuedSession{}, fmt.Errorf("register user: %w", err)
	}
	return issuedSession{token: token, expiresAt: stored.expiresAt, user: user}, nil
}

func (s *service) login(ctx context.Context, login, password string) (issuedSession, error) {
	if err := validateCredentials(login, password); err != nil {
		return issuedSession{}, err
	}
	user, hash, err := s.users.credentials(ctx, login)
	if err != nil {
		return issuedSession{}, fmt.Errorf("look up credentials: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return issuedSession{}, ErrInvalidCredentials
		}
		return issuedSession{}, fmt.Errorf("compare password hash: %w", err)
	}
	token, stored, err := s.issue()
	if err != nil {
		return issuedSession{}, err
	}
	if err := s.sessions.createSession(ctx, user, stored); err != nil {
		return issuedSession{}, fmt.Errorf("create login session: %w", err)
	}
	return issuedSession{token: token, expiresAt: stored.expiresAt, user: user}, nil
}

func (s *service) authenticate(ctx context.Context, token string) (User, [sha256.Size]byte, error) {
	var key [sha256.Size]byte
	if len(token) != 43 {
		return User{}, key, ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return User{}, key, ErrUnauthorized
	}
	key = sha256.Sum256([]byte(token))
	user, err := s.sessions.sessionUser(ctx, key, s.now().UTC())
	if err != nil {
		return User{}, key, fmt.Errorf("authenticate session: %w", err)
	}
	return user, key, nil
}

func (s *service) logout(ctx context.Context, key [sha256.Size]byte) error {
	if err := s.sessions.deleteSession(ctx, key); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *service) issue() (string, session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", session{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, session{key: sha256.Sum256([]byte(token)), expiresAt: s.now().UTC().Truncate(time.Microsecond).Add(s.ttl)}, nil
}

func validateCredentials(login, password string) error {
	if !loginPattern.MatchString(login) || !utf8.ValidString(password) || len(password) < 8 || len(password) > 72 {
		return ErrValidation
	}
	return nil
}
