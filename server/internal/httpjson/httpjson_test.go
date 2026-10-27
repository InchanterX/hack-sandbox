package httpjson_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"calcqueue/internal/httpjson"
	"calcqueue/internal/openapi"
)

func TestDecode(t *testing.T) {
	t.Parallel()
	type input struct {
		Expression string `json:"expression"`
	}
	tests := []struct {
		name    string
		body    string
		want    input
		wantErr bool
	}{
		{name: "valid object", body: `{"expression":"1+2"}`, want: input{Expression: "1+2"}},
		{name: "malformed JSON", body: `{"expression":`, wantErr: true},
		{name: "array instead of object", body: `["1+2"]`, wantErr: true},
		{name: "null body", body: `null`, wantErr: true},
		{name: "missing required field", body: `{}`, wantErr: true},
		{name: "required field is null", body: `{"expression":null}`, wantErr: true},
		{name: "unknown field", body: `{"expression":"1","extra":1}`, wantErr: true},
		{name: "wrong type", body: `{"expression":1}`, wantErr: true},
		{name: "trailing object", body: `{"expression":"1"}{}`, wantErr: true},
		{name: "body over 16 KiB", body: `{"expression":"` + strings.Repeat("1", httpjson.MaxBodyBytes) + `"}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var got input
			err := httpjson.Decode(httptest.NewRecorder(), r, &got, "expression")
			if tc.wantErr {
				if !errors.Is(err, httpjson.ErrInvalidBody) {
					t.Errorf("Decode(%q) error = %v, want ErrInvalidBody", tc.body, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode(%q) error = %v", tc.body, err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Decode(%q) mismatch (-want +got):\n%s", tc.body, diff)
			}
		})
	}
}

func TestError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	httpjson.Error(w, http.StatusNotFound, openapi.ErrorCodeNotFound, "задача не найдена")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var got openapi.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	want := openapi.ErrorEnvelope{Error: openapi.ApiError{Code: openapi.ErrorCodeNotFound, Message: "задача не найдена"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("envelope mismatch (-want +got):\n%s", diff)
	}
}
