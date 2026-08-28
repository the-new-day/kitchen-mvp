package worker

import (
	"avito-kitchen/internal/config"
	"context"
	"fmt"
	"log/slog"
)

// Keys are the stored answers to the requests that must not happen twice.
type Keys interface {
	DeleteExpired(ctx context.Context, limit int) (int64, error)
}

// IdempotencyGC removes the idempotency keys whose answers are no longer kept.
type IdempotencyGC struct {
	keys Keys
	cfg  config.IdempotencyGCJob
	log  *slog.Logger
}

// NewIdempotencyGC returns the collection job.
func NewIdempotencyGC(keys Keys, cfg config.IdempotencyGCJob, log *slog.Logger) *IdempotencyGC {
	return &IdempotencyGC{
		keys: keys,
		cfg:  cfg,
		log:  log.With(slog.String("job", "idempotency-gc")),
	}
}

// Name returns the name of the job.
func (g *IdempotencyGC) Name() string { return "idempotency-gc" }

// Run collects the expired keys until ctx is cancelled.
func (g *IdempotencyGC) Run(ctx context.Context) {
	repeat(ctx, g.cfg.Interval, g.log, g.collect)
}

func (g *IdempotencyGC) collect(ctx context.Context) error {
	deleted, err := g.keys.DeleteExpired(ctx, g.cfg.BatchSize)
	if err != nil {
		return fmt.Errorf("collect expired idempotency keys: %w", err)
	}

	if deleted > 0 {
		g.log.InfoContext(ctx, "expired idempotency keys collected", slog.Int64("count", deleted))
	}

	return nil
}
