// Package calc parses and evaluates arithmetic expressions with + - * /,
// unary minus, parentheses and decimal numbers, without I/O.
//
// Scaffold stub (T01): Parse rejects every input until T02 implements the
// recursive-descent parser behind these stable signatures.
package calc

import (
	"context"
	"errors"
)

// Sentinel errors of parsing and evaluation.
var (
	// ErrSyntax reports an illegal character or invalid grammar.
	ErrSyntax = errors.New("syntax error")
	// ErrDivisionByZero reports division by zero during evaluation.
	ErrDivisionByZero = errors.New("division by zero")
	// ErrOverflow reports a non-finite intermediate or rounded result.
	ErrOverflow = errors.New("overflow")
)

// Expr is an immutable parsed expression.
type Expr struct {
	root node
}

type node interface{}

// Parse parses the whole input into an expression; errors match ErrSyntax.
func Parse(_ string) (Expr, error) {
	return Expr{}, ErrSyntax
}

// Eval evaluates the expression, calling beforeBinary once before each binary
// operation (nil means no callback). The result is rounded to ten fractional
// digits; errors match ErrDivisionByZero, ErrOverflow or a context error.
func (e Expr) Eval(ctx context.Context, beforeBinary func(context.Context) error) (float64, error) {
	_ = e.root
	_ = beforeBinary
	if err := ctx.Err(); err != nil {
		return 0, err //nolint:wrapcheck // Context errors are returned as such by contract.
	}
	return 0, ErrSyntax
}
