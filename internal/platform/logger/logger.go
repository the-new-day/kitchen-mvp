// Package logger builds the structured logger used across the services.
package logger

import (
	"io"
	"log/slog"
)

// New returns a JSON logger that tags every record with the service name.
func New(w io.Writer, level slog.Level, service string) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})

	return slog.New(handler).With(slog.String("service", service))
}
