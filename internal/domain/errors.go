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
