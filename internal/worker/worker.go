// Package worker holds the background jobs of the platform: publishing what
// the domain has written to the outbox, rejecting the orders no venue has
// taken in time, closing the orders a delivery has had time to bring home and
// collecting the expired idempotency keys.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Transactor runs a unit of work in one database transaction.
// The repositories called inside fn join it through the context.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Job is one background loop. Run returns when ctx is cancelled and not
// before: a job answers for its own errors and keeps going.
type Job interface {
	Name() string
	Run(ctx context.Context)
}

// Run starts every job and returns once all of them have stopped.
func Run(ctx context.Context, log *slog.Logger, jobs ...Job) {
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Go(func() {
			log.InfoContext(ctx, "job started", slog.String("job", job.Name()))
			job.Run(ctx)
			log.InfoContext(ctx, "job stopped", slog.String("job", job.Name()))
		})
	}

	wg.Wait()
}

// repeat calls fn every interval until ctx is cancelled. An error does not
// stop the loop: the next run is the retry.
func repeat(ctx context.Context, interval time.Duration, log *slog.Logger, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err := fn(ctx); err != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "job run failed", slog.String("error", err.Error()))
		}
	}
}

// pause waits for d and reports whether the caller may go on. A zero pause is
// not a yield: the loop continues at once.
func pause(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}

	if d == 0 {
		return true
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
