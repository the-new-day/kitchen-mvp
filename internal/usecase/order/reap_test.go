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

// TestServiceReap covers the automatic rejection of the orders no venue has
// taken into work: it is the very transition a venue would have caused, only
// by another actor, and an order somebody else is deciding is left alone.
func TestServiceReap(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		stale      int
		lock       func(repositories)
		wantReaped int
		wantEvents []string
		wantErr    error
	}{
		"an order nobody accepted is rejected by the platform": {
			stale: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockUnaccepted(mock.Anything, orderID).
					Return(inStatus(domainorder.StatusCreated), nil).Once()
				r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
					Return(applied(domainorder.StatusRejected, 2), nil).Once()
				r.menus.EXPECT().ReleaseStock(mock.Anything, bakeryID, placed.Items).
					Return(nil).Once()
			},
			wantReaped: 1,
			wantEvents: []string{outbox.EventOrderStatusChanged, outbox.EventOrderCancelled},
		},
		"an order the venue is deciding right now is left to it": {
			stale: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockUnaccepted(mock.Anything, orderID).
					Return(domainorder.Order{}, domain.ErrNotFound).Once()
			},
		},
		"a failing order stops the run and keeps what was done": {
			stale: 1,
			lock: func(r repositories) {
				r.orders.EXPECT().LockUnaccepted(mock.Anything, orderID).
					Return(domainorder.Order{}, errRepo).Once()
			},
			wantErr: errRepo,
		},
		"nothing waits for too long": {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var written []outbox.Message

			before := time.Date(2026, 8, 26, 10, 5, 0, 0, time.UTC)

			service := newService(t, func(r repositories) {
				stale := make([]uuid.UUID, tc.stale)
				for i := range stale {
					stale[i] = orderID
				}

				r.orders.EXPECT().StaleUnaccepted(mock.Anything, before, 50).
					Return(stale, nil).Once()

				if tc.lock != nil {
					tc.lock(r)
				}

				if len(tc.wantEvents) > 0 {
					collect(r, &written)
				}
			})

			reaped, err := service.Reap(context.Background(), before, 50)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("reap = %v, want %v", err, tc.wantErr)
			}

			if reaped != tc.wantReaped {
				t.Fatalf("reaped = %d, want %d", reaped, tc.wantReaped)
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

// TestServiceReapRejectsAsTheSystem pins down who the history records as the
// author of an automatic rejection and what the customer is told about it.
func TestServiceReapRejectsAsTheSystem(t *testing.T) {
	t.Parallel()

	var change domainorder.StatusChange

	service := newService(t, func(r repositories) {
		r.orders.EXPECT().StaleUnaccepted(mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{orderID}, nil).Once()
		r.orders.EXPECT().LockUnaccepted(mock.Anything, orderID).
			Return(inStatus(domainorder.StatusCreated), nil).Once()
		r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
			RunAndReturn(func(_ context.Context, _ uuid.UUID, c domainorder.StatusChange) (domainorder.Applied, error) {
				change = c

				return applied(domainorder.StatusRejected, 2), nil
			}).Once()
		r.menus.EXPECT().ReleaseStock(mock.Anything, bakeryID, placed.Items).Return(nil).Once()
		r.messages.EXPECT().Append(mock.Anything, mock.Anything).Return(nil).Twice()
	})

	if _, err := service.Reap(context.Background(), time.Now(), 50); err != nil {
		t.Fatalf("reap = %v", err)
	}

	if change.Actor != domainorder.ActorSystem {
		t.Fatalf("actor = %q, want %q", change.Actor, domainorder.ActorSystem)
	}

	if change.From != domainorder.StatusCreated || change.To != domainorder.StatusRejected {
		t.Fatalf("moved %q -> %q, want CREATED -> REJECTED", change.From, change.To)
	}

	if change.Reason == "" {
		t.Fatal("the customer is told nothing about the rejection")
	}
}
