package apierr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestConstructors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        *Error
		status     int
		code, text string
	}{
		{"bad request", BadRequest("Send JSON."), http.StatusBadRequest, "bad_request", "bad_request: Send JSON."},
		{"not found", NotFound("No page."), http.StatusNotFound, "not_found", "not_found: No page."},
		{"conflict", Conflict("Taken."), http.StatusConflict, "conflict", "conflict: Taken."},
		{"forbidden", Forbidden("pages:write"), http.StatusForbidden, "forbidden", "forbidden: Your role doesn't grant pages:write."},
		{"too large", TooLarge(10 << 20), http.StatusRequestEntityTooLarge, "too_large", "too_large: The request body is over 10 MB."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err.Status != tt.status || tt.err.Code != tt.code || tt.err.Error() != tt.text {
				t.Errorf("got %d %q %q; want %d %q %q", tt.err.Status, tt.err.Code, tt.err.Error(), tt.status, tt.code, tt.text)
			}
		})
	}
	if c := Forbidden("pages:write").Capability; c != "pages:write" {
		t.Errorf("Forbidden's capability = %q", c)
	}
}

func TestInvalid(t *testing.T) {
	t.Parallel()
	inv := Invalid{}
	if err := inv.Err(); err != nil {
		t.Fatalf("no problems is no error, got %v", err)
	}
	inv.Add("slug", "can't be blank")
	inv.Add("slug", "is invalid")
	inv.Add("title", "can't be blank")
	// Wrapped, as a handler's error often is, it's still found.
	err := fmt.Errorf("saving: %w", inv.Err())
	e, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("%v isn't an *Error", err)
	}
	if e.Status != http.StatusUnprocessableEntity || e.Code != "invalid" || len(e.Errors["slug"]) != 2 || e.Errors["title"][0] != "can't be blank" {
		t.Errorf("got %+v", e)
	}
}
