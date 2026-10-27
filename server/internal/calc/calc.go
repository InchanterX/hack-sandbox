// Package calc parses and evaluates arithmetic expressions without I/O.
package calc

import (
	"context"
	"errors"
	"math"
	"strconv"
)

var (
	// ErrSyntax reports an illegal character or invalid grammar.
	ErrSyntax = errors.New("syntax error")
	// ErrDivisionByZero reports division by zero during evaluation.
	ErrDivisionByZero = errors.New("division by zero")
	// ErrOverflow reports a non-finite intermediate or rounded result.
	ErrOverflow = errors.New("overflow")
)

// Expr is an immutable parsed expression.
type Expr struct{ root *node }

type node struct {
	op          byte
	literal     string
	left, right *node
}

type parser struct {
	input string
	pos   int
}

// Parse parses the whole input into an expression; errors match ErrSyntax.
func Parse(input string) (Expr, error) {
	p := parser{input: input}
	root, err := p.expression()
	if err != nil {
		return Expr{}, err
	}
	p.spaces()
	if p.pos != len(input) {
		return Expr{}, ErrSyntax
	}
	return Expr{root: root}, nil
}

func (p *parser) spaces() {
	for p.pos < len(p.input) && p.input[p.pos] == ' ' {
		p.pos++
	}
}
func (p *parser) take(c byte) bool {
	p.spaces()
	if p.pos < len(p.input) && p.input[p.pos] == c {
		p.pos++
		return true
	}
	return false
}
func (p *parser) expression() (*node, error) { return p.binary(p.term, "+-") }
func (p *parser) term() (*node, error)       { return p.binary(p.unary, "*/") }
func (p *parser) binary(next func() (*node, error), ops string) (*node, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for {
		p.spaces()
		if p.pos >= len(p.input) || (p.input[p.pos] != ops[0] && p.input[p.pos] != ops[1]) {
			return left, nil
		}
		op := p.input[p.pos]
		p.pos++
		right, err := next()
		if err != nil {
			return nil, err
		}
		left = &node{op: op, left: left, right: right}
	}
}
func (p *parser) unary() (*node, error) {
	if p.take('-') {
		child, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &node{op: '~', left: child}, nil
	}
	if p.take('(') {
		child, err := p.expression()
		if err != nil {
			return nil, err
		}
		if !p.take(')') {
			return nil, ErrSyntax
		}
		return child, nil
	}
	p.spaces()
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return nil, ErrSyntax
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		decimal := p.pos
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == decimal {
			return nil, ErrSyntax
		}
	}
	return &node{literal: p.input[start:p.pos]}, nil
}

// Eval evaluates the expression, calling beforeBinary once before each binary
// operation. The result is rounded to ten fractional digits.
func (e Expr) Eval(ctx context.Context, beforeBinary func(context.Context) error) (float64, error) {
	if e.root == nil {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return 0, ErrSyntax
	}
	value, err := e.root.eval(ctx, beforeBinary)
	if err != nil {
		return 0, err
	}
	scaled := value * 1e10
	if !finite(scaled) {
		return 0, ErrOverflow
	}
	value = math.Round(scaled) / 1e10
	if !finite(value) {
		return 0, ErrOverflow
	}
	if value == 0 {
		value = 0
	}
	return value, nil
}
func finite(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }
func (n *node) eval(ctx context.Context, callback func(context.Context) error) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if n.op == 0 {
		value, err := strconv.ParseFloat(n.literal, 64)
		if err != nil || !finite(value) {
			return 0, ErrOverflow
		}
		return value, nil
	}
	left, err := n.left.eval(ctx, callback)
	if err != nil {
		return 0, err
	}
	if n.op == '~' {
		return -left, nil
	}
	right, err := n.right.eval(ctx, callback)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if callback != nil {
		if err := callback(ctx); err != nil {
			return 0, err
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var value float64
	switch n.op {
	case '+':
		value = left + right
	case '-':
		value = left - right
	case '*':
		value = left * right
	case '/':
		if right == 0 {
			return 0, ErrDivisionByZero
		}
		value = left / right
	}
	if !finite(value) {
		return 0, ErrOverflow
	}
	return value, nil
}
