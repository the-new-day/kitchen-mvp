package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
)

// Recoverer catches a panic, reports it through the LogEntry that
// RequestLogger put in the context and answers 500 with the error envelope.
// It must be mounted inside RequestLogger, otherwise the panic is not logged.
// http.ErrAbortHandler is re-panicked so that net/http drops the connection.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rvr := recover()
			if rvr == nil {
				return
			}
			if errors.Is(toError(rvr), http.ErrAbortHandler) {
				panic(rvr)
			}

			if entry := middleware.GetLogEntry(r); entry != nil {
				entry.Panic(rvr, debug.Stack())
			}
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
		}()

		next.ServeHTTP(w, r)
	})
}

func toError(rvr any) error {
	if err, ok := rvr.(error); ok {
		return err
	}

	return fmt.Errorf("%v", rvr)
}
