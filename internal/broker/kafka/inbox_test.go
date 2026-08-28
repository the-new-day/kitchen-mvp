package kafka_test

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/broker/kafka/mocks"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

const consumer = "venue-service"

var errStore = errors.New("connection refused")

// TestDeduplicated covers what a consumer does with a redelivered event: the
// first delivery is handled and remembered in one transaction, every next one
// is skipped, and a handler that failed leaves nothing remembered.
func TestDeduplicated(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		fresh       bool
		rememberErr error
		handleErr   error
		wantHandled bool
		wantErr     error
	}{
		"the first delivery is handled": {
			fresh:       true,
			wantHandled: true,
		},
		"a redelivered event is skipped": {
			fresh: false,
		},
		"a failing handler asks for the event again": {
			fresh:       true,
			handleErr:   errStore,
			wantHandled: true,
			wantErr:     errStore,
		},
		"an inbox that cannot be read asks for the event again": {
			rememberErr: errStore,
			wantErr:     errStore,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			inbox := mocks.NewMockInbox(t)
			inbox.EXPECT().Remember(mock.Anything, consumer, eventID).
				Return(tc.fresh, tc.rememberErr).Once()

			tx := mocks.NewMockTransactor(t)
			tx.EXPECT().InTx(mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
					return fn(ctx)
				}).Once()

			handled := false
			handle := broker.Deduplicated(tx, inbox, consumer,
				func(context.Context, broker.Envelope) error {
					handled = true

					return tc.handleErr
				})

			err := handle(context.Background(), broker.Envelope{
				EventID:   eventID,
				EventType: "order.created",
			})

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("handle = %v, want %v", err, tc.wantErr)
			}

			if handled != tc.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, tc.wantHandled)
			}
		})
	}
}

// TestDeduplicatedIsNotFooledByAnotherConsumer checks that the mark belongs to
// the consumer that made it: two consumers of one topic each get the event.
func TestDeduplicatedIsNotFooledByAnotherConsumer(t *testing.T) {
	t.Parallel()

	seen := map[string]uuid.UUID{}

	inbox := mocks.NewMockInbox(t)
	inbox.EXPECT().Remember(mock.Anything, mock.Anything, eventID).
		RunAndReturn(func(_ context.Context, name string, id uuid.UUID) (bool, error) {
			if _, ok := seen[name]; ok {
				return false, nil
			}

			seen[name] = id

			return true, nil
		}).Twice()

	tx := mocks.NewMockTransactor(t)
	tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Twice()

	handled := 0
	count := func(context.Context, broker.Envelope) error {
		handled++

		return nil
	}

	envelope := broker.Envelope{EventID: eventID, EventType: "order.created"}

	for _, name := range []string{"venue-service", "sse-hub"} {
		if err := broker.Deduplicated(tx, inbox, name, count)(context.Background(), envelope); err != nil {
			t.Fatalf("handle for %s = %v", name, err)
		}
	}

	if handled != 2 {
		t.Fatalf("handled %d times, want once per consumer", handled)
	}
}
