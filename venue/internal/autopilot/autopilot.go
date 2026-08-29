// Package autopilot runs the kitchen without a cook: it takes every order that
// has waited out its step and moves it on, the same way a cook pressing the
// buttons of a cash register would.
package autopilot

import (
	"avito-kitchen/venue/internal/config"
	"avito-kitchen/venue/internal/kitchen"
	"context"
	"log/slog"
	"time"
)

// Kitchen is the part of the venue the autopilot drives.
type Kitchen interface {
	Ripe(ctx context.Context, due []kitchen.Due, limit int) ([]kitchen.Order, error)
	Advance(ctx context.Context, order kitchen.Order) error
	SyncStopList(ctx context.Context) error
}

// Autopilot moves the orders of one venue on a timer.
type Autopilot struct {
	kitchen Kitchen
	cfg     config.Autopilot
	log     *slog.Logger
}

// New returns an autopilot over the kitchen.
func New(kitchen Kitchen, cfg config.Autopilot, log *slog.Logger) *Autopilot {
	return &Autopilot{kitchen: kitchen, cfg: cfg, log: log}
}

// Run works the kitchen until ctx is cancelled. A run that fails is not fatal:
// the orders keep their state and the next tick is the retry.
func (a *Autopilot) Run(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err := a.tick(ctx); err != nil && ctx.Err() == nil {
			a.log.ErrorContext(ctx, "autopilot run failed", slog.String("error", err.Error()))
		}
	}
}

func (a *Autopilot) tick(ctx context.Context) error {
	orders, err := a.kitchen.Ripe(ctx, Deadlines(a.cfg, time.Now()), a.cfg.BatchSize)
	if err != nil {
		return err
	}

	for _, order := range orders {
		if err := a.kitchen.Advance(ctx, order); err != nil {
			return err
		}
	}

	return a.kitchen.SyncStopList(ctx)
}

// Deadlines turns the pace of the kitchen into the moment an order in each
// state has waited long enough. A state the configuration says nothing about
// is not hurried at all.
func Deadlines(cfg config.Autopilot, now time.Time) []kitchen.Due {
	waits := map[kitchen.State]time.Duration{
		kitchen.StateNew:      cfg.AcceptAfter,
		kitchen.StateAccepted: cfg.CookAfter,
		kitchen.StateCooking:  cfg.ReadyAfter,
		kitchen.StateReady:    cfg.HandAfter,
	}

	due := make([]kitchen.Due, 0, len(waits))

	for _, step := range kitchen.Pipeline() {
		wait, ok := waits[step.From]
		if !ok {
			continue
		}

		due = append(due, kitchen.Due{State: step.From, Cutoff: now.Add(-wait)})
	}

	return due
}
