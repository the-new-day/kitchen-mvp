package order_test

import (
	"avito-kitchen/internal/domain"
	domainorder "avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/domain/outbox"
	"avito-kitchen/internal/usecase/order"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

var venueID = bakeryID

// inStatus is the order the transition under test is applied to.
func inStatus(status domainorder.Status) domainorder.Order {
	current := placed
	current.Status = status

	return current
}

// applied is what the repository answers with after a transition: the order in
// its new status, one version further, and the number of the history entry.
func applied(to domainorder.Status, seq int64) domainorder.Applied {
	after := placed
	after.Status = to
	after.Version = placed.Version + 1

	return domainorder.Applied{Order: after, Seq: seq}
}

// collect records every event a transition writes to the outbox.
func collect(r repositories, into *[]outbox.Message) {
	r.messages.EXPECT().Append(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, m outbox.Message) error {
			*into = append(*into, m)

			return nil
		})
}

func TestServiceTransitions(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		from       domainorder.Status
		byVenue    bool
		move       func(*order.Service, context.Context, uuid.UUID) (domainorder.Order, error)
		wantTo     domainorder.Status
		wantEvents []string
		wantStock  bool
	}{
		"a venue takes an order into work": {
			from:    domainorder.StatusCreated,
			byVenue: true,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.Accept(ctx, id, orderID, 15)
			},
			wantTo:     domainorder.StatusAccepted,
			wantEvents: []string{outbox.EventOrderStatusChanged},
		},
		"a venue starts cooking": {
			from:    domainorder.StatusAccepted,
			byVenue: true,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.StartCooking(ctx, id, orderID)
			},
			wantTo:     domainorder.StatusCooking,
			wantEvents: []string{outbox.EventOrderStatusChanged},
		},
		"a venue reports the order cooked": {
			from:    domainorder.StatusCooking,
			byVenue: true,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.MarkReady(ctx, id, orderID)
			},
			wantTo:     domainorder.StatusReady,
			wantEvents: []string{outbox.EventOrderStatusChanged},
		},
		"a venue hands the order to the courier": {
			from:    domainorder.StatusReady,
			byVenue: true,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.Handover(ctx, id, orderID)
			},
			wantTo:     domainorder.StatusDelivering,
			wantEvents: []string{outbox.EventOrderStatusChanged},
		},
		"a venue refusing an order gives the stock back and is not told twice": {
			from:    domainorder.StatusCreated,
			byVenue: true,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.Reject(ctx, id, orderID, "нет теста")
			},
			wantTo:     domainorder.StatusRejected,
			wantEvents: []string{outbox.EventOrderStatusChanged},
			wantStock:  true,
		},
		"a customer cancelling gives the stock back and takes the order from the venue": {
			from: domainorder.StatusAccepted,
			move: func(s *order.Service, ctx context.Context, id uuid.UUID) (domainorder.Order, error) {
				return s.Cancel(ctx, id, orderID, "передумал")
			},
			wantTo: domainorder.StatusCancelled,
			wantEvents: []string{
				outbox.EventOrderStatusChanged, outbox.EventOrderCancelled,
			},
			wantStock: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var written []outbox.Message

			owner := userID
			if tc.byVenue {
				owner = venueID
			}

			service := newService(t, func(r repositories) {
				if tc.byVenue {
					r.orders.EXPECT().LockForVenue(mock.Anything, venueID, orderID).
						Return(inStatus(tc.from), nil).Once()
				} else {
					r.orders.EXPECT().LockForCustomer(mock.Anything, userID, orderID).
						Return(inStatus(tc.from), nil).Once()
				}

				r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
					Return(applied(tc.wantTo, 7), nil).Once()

				if tc.wantStock {
					r.menus.EXPECT().ReleaseStock(mock.Anything, bakeryID, placed.Items).
						Return(nil).Once()
				}

				collect(r, &written)
			})

			got, err := tc.move(service, context.Background(), owner)
			if err != nil {
				t.Fatalf("move = %v", err)
			}

			if got.Status != tc.wantTo {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantTo)
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

// TestServiceCancelRoutesBothEvents pins down why a cancellation writes two
// rows rather than one: the venue reads its orders keyed by itself, the
// customer follows one order keyed by the order, and a row of the outbox
// carries a single topic and a single key.
func TestServiceCancelRoutesBothEvents(t *testing.T) {
	t.Parallel()

	var written []outbox.Message

	service := newService(t, func(r repositories) {
		r.orders.EXPECT().LockForCustomer(mock.Anything, userID, orderID).
			Return(inStatus(domainorder.StatusCreated), nil).Once()
		r.orders.EXPECT().ApplyStatus(mock.Anything, orderID, mock.Anything).
			Return(applied(domainorder.StatusCancelled, 2), nil).Once()
		r.menus.EXPECT().ReleaseStock(mock.Anything, bakeryID, placed.Items).Return(nil).Once()
		collect(r, &written)
	})

	if _, err := service.Cancel(context.Background(), userID, orderID, "передумал"); err != nil {
		t.Fatalf("cancel = %v", err)
	}

	want := map[string]struct{ topic, key string }{
		outbox.EventOrderStatusChanged: {statusTopic, orderID.String()},
		outbox.EventOrderCancelled:     {ordersTopic, bakeryID.String()},
	}

	for _, message := range written {
		expected, ok := want[message.EventType]
		if !ok {
			t.Fatalf("unexpected event %q", message.EventType)
		}

		if message.Topic != expected.topic || message.Key != expected.key {
			t.Fatalf("%s went to %s keyed by %s, want %s keyed by %s",
				message.EventType, message.Topic, message.Key, expected.topic, expected.key)
		}

		delete(want, message.EventType)
	}

	if len(want) != 0 {
		t.Fatalf("events missing: %v", want)
	}
}

func TestServiceTransitionRefusals(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup       func(repositories)
		move        func(*order.Service, context.Context) (domainorder.Order, error)
		wantErr     error
		wantCurrent string
	}{
		"repeating an applied event changes nothing and is not an error": {
			setup: func(r repositories) {
				r.orders.EXPECT().LockForVenue(mock.Anything, venueID, orderID).
					Return(inStatus(domainorder.StatusAccepted), nil).Once()
			},
			move: func(s *order.Service, ctx context.Context) (domainorder.Order, error) {
				return s.Accept(ctx, venueID, orderID, 15)
			},
		},
		"cancelling an order already cancelled changes nothing": {
			setup: func(r repositories) {
				r.orders.EXPECT().LockForCustomer(mock.Anything, userID, orderID).
					Return(inStatus(domainorder.StatusCancelled), nil).Once()
			},
			move: func(s *order.Service, ctx context.Context) (domainorder.Order, error) {
				return s.Cancel(ctx, userID, orderID, "")
			},
		},
		"an event the order cannot take reports the status it is in": {
			setup: func(r repositories) {
				r.orders.EXPECT().LockForVenue(mock.Anything, venueID, orderID).
					Return(inStatus(domainorder.StatusAccepted), nil).Once()
			},
			move: func(s *order.Service, ctx context.Context) (domainorder.Order, error) {
				return s.Handover(ctx, venueID, orderID)
			},
			wantErr:     domain.ErrInvalidTransition,
			wantCurrent: string(domainorder.StatusAccepted),
		},
		"a customer cannot cancel an order already being cooked": {
			setup: func(r repositories) {
				r.orders.EXPECT().LockForCustomer(mock.Anything, userID, orderID).
					Return(inStatus(domainorder.StatusCooking), nil).Once()
			},
			move: func(s *order.Service, ctx context.Context) (domainorder.Order, error) {
				return s.Cancel(ctx, userID, orderID, "")
			},
			wantErr:     domain.ErrInvalidTransition,
			wantCurrent: string(domainorder.StatusCooking),
		},
		"the order of another venue does not exist": {
			setup: func(r repositories) {
				r.orders.EXPECT().LockForVenue(mock.Anything, venueID, orderID).
					Return(domainorder.Order{}, domain.ErrNotFound).Once()
			},
			move: func(s *order.Service, ctx context.Context) (domainorder.Order, error) {
				return s.Accept(ctx, venueID, orderID, 15)
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Nothing is expected of ApplyStatus, ReleaseStock or Append: the
			// mocks fail the case if a refused or repeated event writes.
			service := newService(t, tc.setup)

			_, err := tc.move(service, context.Background())

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			if tc.wantCurrent == "" {
				return
			}

			var transition domain.TransitionError
			if !errors.As(err, &transition) {
				t.Fatalf("error = %v, want a transition error", err)
			}

			if transition.Current != tc.wantCurrent {
				t.Fatalf("current = %q, want %q", transition.Current, tc.wantCurrent)
			}
		})
	}
}
