package calc

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
)

func TestExamples(t *testing.T) {
	data, err := os.ReadFile("../../../contracts/examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Success []struct {
			Expression string  `json:"expression"`
			Result     float64 `json:"result"`
		} `json:"success"`
		Syntax []string `json:"syntax_error"`
		Failed []struct {
			Expression string `json:"expression"`
		} `json:"failed"`
		Overflow []struct {
			Expression string `json:"expression"`
		} `json:"calculator_only_overflow"`
	}
	if err := json.Unmarshal(data, &examples); err != nil {
		t.Fatal(err)
	}
	for _, example := range examples.Success {
		t.Run(example.Expression, func(t *testing.T) {
			expr, err := Parse(example.Expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := expr.Eval(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != example.Result || (got == 0 && math.Signbit(got)) {
				t.Fatalf("got %v, want %v with positive zero", got, example.Result)
			}
		})
	}
	for _, input := range examples.Syntax {
		t.Run("syntax/"+input, func(t *testing.T) {
			if _, err := Parse(input); !errors.Is(err, ErrSyntax) {
				t.Fatalf("got %v, want syntax error", err)
			}
		})
	}
	for _, group := range []struct {
		name        string
		expressions []string
		want        error
	}{
		{name: "division", expressions: collectFailed(examples.Failed), want: ErrDivisionByZero},
		{name: "overflow", expressions: collectOverflow(examples.Overflow), want: ErrOverflow},
	} {
		for _, input := range group.expressions {
			t.Run(group.name+"/"+input, func(t *testing.T) {
				expr, err := Parse(input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := expr.Eval(context.Background(), nil)
				if !errors.Is(err, group.want) || math.IsNaN(got) || math.IsInf(got, 0) {
					t.Fatalf("got %v, %v; want %v", got, err, group.want)
				}
			})
		}
	}
}

func collectFailed(items []struct {
	Expression string `json:"expression"`
}) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Expression)
	}
	return result
}
func collectOverflow(items []struct {
	Expression string `json:"expression"`
}) []string { return collectFailed(items) }

func TestCallbacksAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		input string
		count int
		want  error
	}{
		{input: "-((1))", count: 0},
		{input: "1 + 2 * 3 - 4", count: 3},
		{input: "1 + 2 / 0", count: 1, want: ErrDivisionByZero},
	} {
		t.Run(tc.input, func(t *testing.T) {
			expr, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			_, err = expr.Eval(context.Background(), func(context.Context) error { count++; return nil })
			if !errors.Is(err, tc.want) || count != tc.count {
				t.Fatalf("got %d callbacks and %v; want %d and %v", count, err, tc.count, tc.want)
			}
		})
	}
	expr, err := Parse("1 + 2 * 3 + 4")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	_, err = expr.Eval(ctx, func(context.Context) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatalf("got %d callbacks and %v", count, err)
	}
}
