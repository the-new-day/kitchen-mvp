package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger writes one structured record per completed request through
// log, and exposes a LogEntry that Recoverer uses to report panics.
func RequestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return middleware.RequestLogger(&slogFormatter{log: log})
}

type slogFormatter struct {
	log *slog.Logger
}

func (f *slogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &slogEntry{
		log:    f.log.With(slog.String("trace_id", middleware.GetReqID(r.Context()))),
		method: r.Method,
		path:   r.URL.Path,
	}
}

// slogEntry carries the request attributes that chi does
// not pass back to Write and Panic.
type slogEntry struct {
	log    *slog.Logger
	method string
	path   string
}

func (e *slogEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ any) {
	if status == 0 {
		// The handler returned without touching the writer, so net/http
		// sends a bare 200.
		status = http.StatusOK
	}

	e.log.LogAttrs(context.Background(), slog.LevelInfo, "request completed",
		slog.String("method", e.method),
		slog.String("path", e.path),
		slog.Int("status", status),
		slog.Int("bytes", bytes),
		slog.Duration("duration", elapsed),
	)
}

func (e *slogEntry) Panic(v any, stack []byte) {
	e.log.LogAttrs(context.Background(), slog.LevelError, "panic recovered",
		slog.String("method", e.method),
		slog.String("path", e.path),
		slog.Any("panic", v),
		slog.String("stack", string(stack)),
	)
}
