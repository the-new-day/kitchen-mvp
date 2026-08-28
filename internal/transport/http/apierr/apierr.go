// Package apierr answers a failed request of any HTTP API of the platform
// with the one error envelope: domain errors become their status codes,
// anything else becomes 500 and a log record.
package apierr

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/platform/httpx"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// ErrNotImplemented is returned by the operations of a specification that the
// service does not serve yet.
var ErrNotImplemented = errors.New("not implemented")

// Handler turns every error leaving an API into an error response.
type Handler struct {
	Log *slog.Logger
}

// Request answers a request that could not be read: an unparsable parameter
// or a body that is not the expected JSON.
func (h Handler) Request(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
}

// Response answers a request whose handler failed.
func (h Handler) Response(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrInvalidArgument):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, domain.ErrUnauthenticated):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized",
			"api key is missing, unknown or revoked")
	case errors.Is(err, ErrNotImplemented):
		httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented",
			"operation is not implemented yet")
	default:
		h.Log.ErrorContext(r.Context(), "request failed",
			slog.String("trace_id", middleware.GetReqID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error",
			"internal server error")
	}
}
