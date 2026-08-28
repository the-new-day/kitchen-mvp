package kitchen

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/platform/httpx"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// errNotImplemented is returned by the operations of the specification that
// the service does not serve yet.
var errNotImplemented = errors.New("not implemented")

// errorHandler turns every error leaving the API into the one error envelope:
// domain errors become their status codes, anything else becomes 500 and a
// log record.
type errorHandler struct {
	log *slog.Logger
}

// request answers a request that could not be read: an unparsable parameter
// or a body that is not the expected JSON.
func (h errorHandler) request(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
}

// response answers a request whose handler failed.
func (h errorHandler) response(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrInvalidArgument):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, errNotImplemented):
		httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented",
			"operation is not implemented yet")
	default:
		h.log.ErrorContext(r.Context(), "request failed",
			slog.String("trace_id", middleware.GetReqID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error",
			"internal server error")
	}
}
