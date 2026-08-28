package worker

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/worker/mocks"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

// tick is short enough for a test to wait for a run and long enough not to
// flood the mock while it does.
const tick = 5 * time.Millisecond

// awaited waits for a job to report its first run and stops it.
func awaited[T any](t *testing.T, ran <-chan T, stop context.CancelFunc, done <-chan struct{}) T {
	t.Helper()

	var first T

	select {
	case first = <-ran:
	case <-time.After(time.Second):
		t.Fatal("the job never ran")
	}

	stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the job did not stop with its context")
	}

	return first
}

// TestReaperRunsUntilStopped checks that the reaper asks for the orders that
// went unaccepted for longer than the timeout, and only for those.
func TestReaperRunsUntilStopped(t *testing.T) {
	t.Parallel()

	const timeout = 30 * time.Second

	ran := make(chan time.Time, 1)

	orders := mocks.NewMockOrders(t)
	orders.EXPECT().Reap(mock.Anything, mock.Anything, 50).
		RunAndReturn(func(_ context.Context, before time.Time, _ int) (int, error) {
			select {
			case ran <- before:
			default:
			}

			return 1, nil
		})

	job := NewReaper(orders, config.ReaperJob{
		AcceptTimeout: timeout,
		Interval:      tick,
		BatchSize:     50,
	}, slog.New(slog.DiscardHandler))

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	done := make(chan struct{})

	go func() {
		defer close(done)

		job.Run(ctx)
	}()

	asked := awaited(t, ran, stop, done)

	if waited := time.Since(asked); waited < timeout || waited > timeout+time.Second {
		t.Fatalf("asked for orders older than %s, want about %s", waited, timeout)
	}
}

// TestIdempotencyGCRunsUntilStopped checks that the collector keeps running on
// its own schedule and stops with its context.
func TestIdempotencyGCRunsUntilStopped(t *testing.T) {
	t.Parallel()

	ran := make(chan struct{}, 1)

	keys := mocks.NewMockKeys(t)
	keys.EXPECT().DeleteExpired(mock.Anything, 1000).
		RunAndReturn(func(context.Context, int) (int64, error) {
			select {
			case ran <- struct{}{}:
			default:
			}

			return 2, nil
		})

	job := NewIdempotencyGC(keys, config.IdempotencyGCJob{
		Interval:  tick,
		BatchSize: 1000,
	}, slog.New(slog.DiscardHandler))

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	done := make(chan struct{})

	go func() {
		defer close(done)

		job.Run(ctx)
	}()

	awaited(t, ran, stop, done)
}

// TestRunWaitsForEveryJob checks that the worker starts all of its jobs and
// returns only once every one of them has stopped.
func TestRunWaitsForEveryJob(t *testing.T) {
	t.Parallel()

	started := make(chan string, 2)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	done := make(chan struct{})

	go func() {
		defer close(done)

		Run(ctx, slog.New(slog.DiscardHandler),
			&countingJob{name: "first", started: started},
			&countingJob{name: "second", started: started},
		)
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("not every job was started")
		}
	}

	stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the worker did not wait for its jobs")
	}
}

// countingJob reports that it has been started and runs until it is stopped.
type countingJob struct {
	name    string
	started chan<- string
}

func (j *countingJob) Name() string { return j.name }

func (j *countingJob) Run(ctx context.Context) {
	j.started <- j.name

	<-ctx.Done()
}
