package worker

import (
	"avito-kitchen/internal/config"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Deliveries closes the orders that have spent enough time on their way.
type Deliveries interface {
	Deliver(ctx context.Context, before time.Time, limit int) (int, error)
}

// Courier stands in for the delivery side of the platform: it closes an order
// once a delivery would have taken it home. A real courier pressing the button
// in an app of their own replaces it without touching the transition itself.
type Courier struct {
	orders Deliveries
	cfg    config.CourierJob
	log    *slog.Logger
}

// NewCourier returns the delivery job.
func NewCourier(orders Deliveries, cfg config.CourierJob, log *slog.Logger) *Courier {
	return &Courier{orders: orders, cfg: cfg, log: log.With(slog.String("job", "courier"))}
}

// Name returns the name of the job.
func (c *Courier) Name() string { return "courier" }

// Run closes the orders that have arrived until ctx is cancelled.
func (c *Courier) Run(ctx context.Context) {
	repeat(ctx, c.cfg.Interval, c.log, c.deliver)
}

func (c *Courier) deliver(ctx context.Context) error {
	before := time.Now().Add(-c.cfg.Delivery)

	delivered, err := c.orders.Deliver(ctx, before, c.cfg.BatchSize)
	if delivered > 0 {
		c.log.InfoContext(ctx, "orders delivered",
			slog.Int("count", delivered), slog.Duration("delivery", c.cfg.Delivery))
	}

	if err != nil {
		return fmt.Errorf("deliver orders on their way since %s: %w", before, err)
	}

	return nil
}
