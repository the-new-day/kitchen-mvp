package httpx

import (
	"avito-kitchen/internal/config"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Serve runs an HTTP server on cfg.Addr until ctx is cancelled, then waits up
// to cfg.ShutdownTimeout for the requests in flight to finish. Those requests
// keep running after cancellation: their context is not derived from ctx.
func Serve(ctx context.Context, cfg config.HTTP, handler http.Handler, log *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		// Handler contexts are independent of ctx, so cancelling it starts
		// the shutdown without cancelling the requests in flight.
		BaseContext: func(net.Listener) context.Context { return context.Background() },
		ErrorLog:    slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	return serve(ctx, listener, srv, cfg.ShutdownTimeout, log)
}

func serve(
	ctx context.Context,
	listener net.Listener,
	srv *http.Server,
	shutdownTimeout time.Duration,
	log *slog.Logger,
) error {
	serveErr := make(chan error, 1)
	go func() {
		err := srv.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	log.InfoContext(ctx, "http server listening", slog.String("addr", listener.Addr().String()))

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}

		return nil
	case <-ctx.Done():
	}

	log.Info("shutdown requested, draining in-flight requests",
		slog.Duration("timeout", shutdownTimeout))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if closeErr := srv.Close(); closeErr != nil {
			log.Error("force close after failed shutdown", slog.String("error", closeErr.Error()))
		}

		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Info("http server stopped gracefully")

	return <-serveErr
}
