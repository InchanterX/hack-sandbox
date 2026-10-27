// Package httpjson writes JSON responses and the OpenAPI error envelope, and
// strictly decodes size-bounded JSON request bodies.
package httpjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"calcqueue/internal/openapi"
)

// MaxBodyBytes is the largest accepted request body (16 KiB).
const MaxBodyBytes = 16 << 10

// ErrInvalidBody reports a request body that is not a single JSON object of
// the expected shape: malformed JSON, a missing required field, an unknown
// field, a wrong type, trailing data or a body over MaxBodyBytes. Handlers map
// it to 422 validation_error.
var ErrInvalidBody = errors.New("invalid request body")

// Write encodes v as JSON with the given status. An encoding failure is logged
// without the value; the status line may already be sent at that point.
func Write(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("Encode JSON response", "status", status, "error", err)
		status = http.StatusInternalServerError
		body = []byte(`{"error":{"code":"internal_error","message":"внутренняя ошибка сервера"}}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		slog.Debug("Write JSON response", "status", status, "error", err)
	}
}

// Error writes the error envelope {"error":{"code","message"}}; message is
// Russian text without secrets or SQL details.
func Error(w http.ResponseWriter, status int, code openapi.ErrorCode, message string) {
	Write(w, status, openapi.ErrorEnvelope{Error: openapi.ApiError{Code: code, Message: message}})
}

// ValidationError writes 422 validation_error with the generic Russian message
// for an invalid request body.
func ValidationError(w http.ResponseWriter) {
	Error(w, http.StatusUnprocessableEntity, openapi.ErrorCodeValidationError, "некорректное тело запроса")
}

// Decode reads at most MaxBodyBytes of the request body into dst. The body
// must be one JSON object containing every name in required with a non-null
// value and no fields unknown to dst. Every failure matches ErrInvalidBody.
func Decode(w http.ResponseWriter, r *http.Request, dst any, required ...string) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: read: %w", ErrInvalidBody, err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return fmt.Errorf("%w: not a JSON object", ErrInvalidBody)
	}
	for _, name := range required {
		raw, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%w: missing field %q", ErrInvalidBody, name)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBody, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data", ErrInvalidBody)
	}
	return nil
}
