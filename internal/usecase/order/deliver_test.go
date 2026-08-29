package order_test

import (
	"avito-kitchen/internal/domain"
	domainorder "avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/domain/outbox"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// TestServiceDeliver covers the closing of the orders a delivery has had time
// to bring home: it is the same domain transition every other move goes
// through, and an order that has already left the road is left alone.
func TestServiceDeliver(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		onTheRoad     int
		lock          func(repositories)
		wantDelivered int
		wantEvents    []string
		wantErr       error
	}{
		"an order that has arrived is closed by the platform": {
			onTheRoad: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockDelivering(mock.Anything, orderID).
					Return(inStatus(domainorder.StatusDelivering), nil).Once()
				r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
					Return(applied(domainorder.StatusDelivered, 6), nil).Once()
			},
			wantDelivered: 1,
			wantEvents:    []string{outbox.EventOrderStatusChanged},
		},
		"an order that has left the road is skipped": {
			onTheRoad: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockDelivering(mock.Anything, orderID).
					Return(domainorder.Order{}, domain.ErrNotFound).Once()
			},
		},
		"a failing order stops the run and keeps what was done": {
			onTheRoad: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockDelivering(mock.Anything, orderID).
					Return(domainorder.Order{}, errRepo).Once()
			},
			wantErr: errRepo,
		},
		"nothing is on its way": {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var written []outbox.Message

			before := time.Date(2026, 8, 26, 10, 5, 0, 0, time.UTC)

			service := newService(t, func(r repositories) {
				onTheRoad := make([]uuid.UUID, tc.onTheRoad)
				for i := range onTheRoad {
					onTheRoad[i] = orderID
				}

				r.orders.EXPECT().StaleDelivering(mock.Anything, before, 50).
					Return(onTheRoad, nil).Once()

				if tc.lock != nil {
					tc.lock(r)
				}

				if len(tc.wantEvents) > 0 {
					collect(r, &written)
				}
			})

			delivered, err := service.Deliver(context.Background(), before, 50)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("deliver = %v, want %v", err, tc.wantErr)
			}

			if delivered != tc.wantDelivered {
				t.Fatalf("delivered = %d, want %d", delivered, tc.wantDelivered)
			}

			if len(written) != len(tc.wantEvents) {
				t.Fatalf("events = %d, want %d", len(written), len(tc.wantEvents))
			}

			for i, want := range tc.wantEvents {
				if written[i].EventType != want {
					t.Fatalf("event %d = %q, want %q", i, written[i].EventType, want)
				}
			}
		})
	}
}

// TestServiceDeliverClosesAsTheSystem pins down who the history records as the
// author of an arrival nobody reported.
func TestServiceDeliverClosesAsTheSystem(t *testing.T) {
	t.Parallel()

	var change domainorder.StatusChange

	service := newService(t, func(r repositories) {
		r.orders.EXPECT().StaleDelivering(mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{orderID}, nil).Once()
		r.orders.EXPECT().LockDelivering(mock.Anything, orderID).
			Return(inStatus(domainorder.StatusDelivering), nil).Once()
		r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
			RunAndReturn(func(_ context.Context, _ uuid.UUID, c domainorder.StatusChange) (domainorder.Applied, error) {
				change = c

				return applied(domainorder.StatusDelivered, 6), nil
			}).Once()
		r.messages.EXPECT().Append(mock.Anything, mock.Anything).Return(nil).Once()
	})

	if _, err := service.Deliver(context.Background(), time.Now(), 50); err != nil {
		t.Fatalf("deliver = %v", err)
	}

	if change.Actor != domainorder.ActorSystem {
		t.Fatalf("actor = %q, want %q", change.Actor, domainorder.ActorSystem)
	}

	if change.From != domainorder.StatusDelivering || change.To != domainorder.StatusDelivered {
		t.Fatalf("moved %q -> %q, want DELIVERING -> DELIVERED", change.From, change.To)
	}
}
