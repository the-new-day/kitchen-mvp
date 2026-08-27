// Command worker runs the background jobs of the platform: outbox publishing,
// the order reaper and idempotency-key garbage collection. It serves /healthz.
package main

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/logger"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const serviceName = "worker"

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serviceName, err)

		return 1
	}

	log := logger.New(os.Stdout, cfg.LogLevel, serviceName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "starting", slog.String("env", cfg.Env))

	if err := serve(ctx, cfg, log); err != nil {
		log.Error("stopped with error", slog.String("error", err.Error()))

		return 1
	}

	log.Info("stopped")

	return 0
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	router := httpx.NewRouter(log)

	return httpx.Serve(ctx, cfg.HTTP, router, log)
}
