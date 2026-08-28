package worker

import (
	"avito-kitchen/internal/config"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Orders rejects the orders that have been waiting for their venue too long.
type Orders interface {
	Reap(ctx context.Context, before time.Time, limit int) (int, error)
}

// Reaper keeps orders from hanging in CREATED
// forever because the venue never answered.
type Reaper struct {
	orders Orders
	cfg    config.ReaperJob
	log    *slog.Logger
}

// NewReaper returns the automatic rejection job.
func NewReaper(orders Orders, cfg config.ReaperJob, log *slog.Logger) *Reaper {
	return &Reaper{orders: orders, cfg: cfg, log: log.With(slog.String("job", "order-reaper"))}
}

// Name returns the name of the job.
func (r *Reaper) Name() string { return "order-reaper" }

// Run rejects the orders that went unaccepted until ctx is cancelled.
func (r *Reaper) Run(ctx context.Context) {
	repeat(ctx, r.cfg.Interval, r.log, r.reap)
}

func (r *Reaper) reap(ctx context.Context) error {
	before := time.Now().Add(-r.cfg.AcceptTimeout)

	reaped, err := r.orders.Reap(ctx, before, r.cfg.BatchSize)
	if reaped > 0 {
		r.log.InfoContext(ctx, "orders rejected on timeout",
			slog.Int("count", reaped), slog.Duration("timeout", r.cfg.AcceptTimeout))
	}

	if err != nil {
		return fmt.Errorf("reap orders unaccepted since %s: %w", before, err)
	}

	return nil
}
