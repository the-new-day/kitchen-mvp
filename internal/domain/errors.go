// Package domain holds the errors shared by every domain package. The
// transport layer maps them to HTTP status codes; nothing below it does.
package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound reports that the requested entity does not exist or is not
	// visible to the caller.
	ErrNotFound = errors.New("not found")

	// ErrInvalidArgument reports that the request itself is malformed.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrUnauthenticated reports that the caller did not prove who it is: the
	// credential is missing, unknown or revoked.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrConflict reports an action that contradicts the current state of the
	// entity it acts on.
	ErrConflict = errors.New("conflict")

	// ErrInvalidTransition reports an order event that the state machine does
	// not allow from the status the order is in.
	ErrInvalidTransition = errors.New("invalid transition")

	// ErrUnprocessable reports a request that is well formed and consistent
	// with the state of the entity, yet cannot be carried out.
	ErrUnprocessable = errors.New("unprocessable")
)

// InvalidArgumentf returns an ErrInvalidArgument whose message describes the
// offending argument and is safe to show to the client.
func InvalidArgumentf(format string, args ...any) error {
	return invalidArgument{message: fmt.Sprintf(format, args...)}
}

type invalidArgument struct {
	message string
}

func (e invalidArgument) Error() string { return e.message }

func (e invalidArgument) Unwrap() error { return ErrInvalidArgument }

// ConflictError is an ErrConflict carrying the machine-readable code of the
// error envelope: conflicts of different causes are told apart by it.
type ConflictError struct {
	Code    string
	Message string
}

// Conflictf returns a ConflictError whose message is safe to show to the
// client.
func Conflictf(code, format string, args ...any) error {
	return ConflictError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e ConflictError) Error() string { return e.Message }

func (e ConflictError) Unwrap() error { return ErrConflict }

// UnprocessableError is an ErrUnprocessable carrying the machine-readable code
// of the error envelope.
type UnprocessableError struct {
	Code    string
	Message string
}

// Unprocessablef returns an UnprocessableError whose message is safe to show
// to the client.
func Unprocessablef(code, format string, args ...any) error {
	return UnprocessableError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e UnprocessableError) Error() string { return e.Message }

func (e UnprocessableError) Unwrap() error { return ErrUnprocessable }

// TransitionError is an ErrInvalidTransition that reports the status the entity
// is actually in, so that the caller learns it without asking again.
type TransitionError struct {
	Current string
}

// InvalidTransition returns a TransitionError for an entity in the given
// status.
func InvalidTransition(current string) error {
	return TransitionError{Current: current}
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("invalid transition from %s", e.Current)
}

func (e TransitionError) Unwrap() error { return ErrInvalidTransition }
