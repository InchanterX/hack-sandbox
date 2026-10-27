package tasks

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"calcqueue/internal/calc"

	"github.com/google/uuid"
)

var (
	errNotFound   = errors.New("task not found")
	errNotQueued  = errors.New("task not queued")
	errRunning    = errors.New("task running")
	errValidation = errors.New("invalid expression length")
	errSyntax     = errors.New("invalid expression syntax")
)

type task struct {
	id                      uuid.UUID
	expression, status      string
	result                  *float64
	errorCode, errorMessage *string
	createdAt               time.Time
	startedAt, finishedAt   *time.Time
}
type reader interface {
	create(context.Context, uuid.UUID, string) (task, error)
	list(context.Context, uuid.UUID) ([]task, error)
	get(context.Context, uuid.UUID, uuid.UUID) (task, error)
}
type mutator interface {
	update(context.Context, uuid.UUID, uuid.UUID, string, func(string) error) (task, error)
	delete(context.Context, uuid.UUID, uuid.UUID) error
}
type service struct {
	reader  reader
	mutator mutator
}

func validate(expression string) error {
	n := utf8.RuneCountInString(expression)
	if n < 1 || n > 200 {
		return errValidation
	}
	if _, err := calc.Parse(expression); err != nil {
		return errSyntax
	}
	return nil
}
func (s *service) create(ctx context.Context, user uuid.UUID, expression string) (task, error) {
	if err := validate(expression); err != nil {
		return task{}, err
	}
	return s.reader.create(ctx, user, expression)
}
func (s *service) update(ctx context.Context, user, id uuid.UUID, expression string) (task, error) {
	return s.mutator.update(ctx, user, id, expression, validate)
}
