package domain

import (
	"errors"
	"fmt"
)

// ValidationError is a bad-request input problem. Handlers map it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

// NotFoundError is returned when a resource does not exist (or is out of the
// caller's tenant scope). Handlers map it to HTTP 404.
type NotFoundError struct {
	Resource string
}

func (e NotFoundError) Error() string {
	if e.Resource == "" {
		return "not found"
	}
	return e.Resource + " not found"
}

// ConflictError signals a state/uniqueness conflict. Handlers map it to HTTP 409.
type ConflictError struct {
	Message string
}

func (e ConflictError) Error() string { return e.Message }

// Sentinel errors for auth outcomes.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

// NotFound is a small constructor helper.
func NotFound(resource string) error { return NotFoundError{Resource: resource} }

// Invalid is a small constructor helper.
func Invalid(field, message string) error { return ValidationError{Field: field, Message: message} }
