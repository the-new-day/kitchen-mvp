// Package httpx provides the HTTP scaffolding shared by the services of
// the repository: a router with common middleware, the error envelope and
// a server with graceful shutdown.
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter returns a router with the common middleware chain and /healthz.
// Further routes are mounted by the caller.
//
// The middleware order is significant: RequestID assigns the trace id the
// other two report, and Recoverer runs inside RequestLogger.
func NewRouter(log *slog.Logger) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(RequestLogger(log))
	r.Use(Recoverer)

	r.Get("/healthz", Healthz())

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return r
}
