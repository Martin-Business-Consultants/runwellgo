// Package apierr is the one shape every /api error answers with:
// {"error": code, "message": …}, plus per-field problems for a 422.
package apierr

import (
	"fmt"
	"net/http"
)

// Error is every /api error, one shape: {"error": code, "message": …}.
type Error struct {
	Status     int
	Code       string
	Message    string
	Errors     map[string][]string
	Capability string
	Extra      map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// BadRequest is a 400.
func BadRequest(msg string) *Error { return &Error{Status: 400, Code: "bad_request", Message: msg} }

// NotFound is a 404.
func NotFound(msg string) *Error { return &Error{Status: 404, Code: "not_found", Message: msg} }

// Conflict is a 409.
func Conflict(msg string) *Error { return &Error{Status: 409, Code: "conflict", Message: msg} }

// Forbidden is a 403 naming the capability the caller's role lacks.
func Forbidden(capability string) *Error {
	return &Error{Status: 403, Code: "forbidden", Message: "Your role doesn't grant " + capability + ".", Capability: capability}
}

// Invalid collects per-field problems into a 422.
type Invalid map[string][]string

// Add notes one problem with field.
func (v Invalid) Add(field, msg string) { v[field] = append(v[field], msg) }

// Err is the 422 for the problems noted, nil when there are none.
func (v Invalid) Err() error {
	if len(v) == 0 {
		return nil
	}
	return &Error{Status: 422, Code: "invalid", Message: "Validation failed", Errors: v}
}

// TooLarge is a 413 for a body over limit bytes.
func TooLarge(limit int64) *Error {
	return &Error{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Message: fmt.Sprintf("The request body is over %d MB.", limit>>20)}
}
