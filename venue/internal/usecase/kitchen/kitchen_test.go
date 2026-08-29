package kitchen_test

import (
	domain "avito-kitchen/venue/internal/kitchen"
	"avito-kitchen/venue/internal/usecase/kitchen"
	"avito-kitchen/venue/internal/usecase/kitchen/mocks"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const etaMinutes = 15

var (
	orderID = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")
	venueID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")

	errRepo = errors.New("connection refused")

	placed = domain.OrderCreated{
		OrderID: orderID,
		Number:  "AK-100001",
		VenueID: venueID,
		Items: []domain.OrderItem{
			{ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 19_000, Qty: 2, LineTotal: 38_000},
			{ExternalID: "SKU-AMERICANO", Name: "Американо", Price: 18_000, Qty: 1, LineTotal: 18_000},
		},
		ItemsTotal: 56_000,
		Total:      65_900,
	}
)

type repositories struct {
	tx       *mocks.MockTransactor
	dishes   *mocks.MockDishes
	orders   *mocks.MockOrders
	platform *mocks.MockPartnerClient
}

// service returns a kitchen whose every dependency is a mock, with the
// transaction passing the work straight through.
func service(t *testing.T) (*kitchen.Service, repositories) {
	t.Helper()

	repos := repositories{
		tx:       mocks.NewMockTransactor(t),
		dishes:   mocks.NewMockDishes(t),
		orders:   mocks.NewMockOrders(t),
		platform: mocks.NewMockPartnerClient(t),
	}

	repos.tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Maybe()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return kitchen.New(repos.tx, repos.dishes, repos.orders, repos.platform, etaMinutes, log), repos
}

// inState is an order of the venue sitting in a state.
func inState(state domain.State) domain.Order {
	return domain.Order{
		ID:             orderID,
		State:          state,
		Placed:         placed,
		ReceivedAt:     time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
		StateChangedAt: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
	}
}

func TestServiceReceive(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		arrange func(repositories)
		wantErr error
	}{
		"a new order is written down and spends the stock": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Save(mock.Anything, placed).Return(true, nil)
				r.dishes.EXPECT().Take(mock.Anything, "SKU-CROISSANT", 2).Return(domain.Dish{}, nil)
				r.dishes.EXPECT().Take(mock.Anything, "SKU-AMERICANO", 1).Return(domain.Dish{}, nil)
			},
		},
		"an order delivered twice spends the stock once": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Save(mock.Anything, placed).Return(false, nil)
			},
		},
		"a dish the venue does not sell is passed over": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Save(mock.Anything, placed).Return(true, nil)
				r.dishes.EXPECT().Take(mock.Anything, "SKU-CROISSANT", 2).
					Return(domain.Dish{}, domain.ErrNotFound)
				r.dishes.EXPECT().Take(mock.Anything, "SKU-AMERICANO", 1).Return(domain.Dish{}, nil)
			},
		},
		"a database that fails asks for the event again": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Save(mock.Anything, placed).Return(false, errRepo)
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, repos := service(t)
			tc.arrange(repos)

			err := svc.Receive(t.Context(), placed)

			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestServiceCancel(t *testing.T) {
	t.Parallel()

	cancelled := domain.OrderCancelled{OrderID: orderID, VenueID: venueID, Reason: "клиент отменил"}

	cases := map[string]struct {
		arrange func(repositories)
		wantErr error
	}{
		"an order in the making is dropped and gives the stock back": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, orderID).Return(inState(domain.StateCooking), nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateCooking, domain.StateCancelled).
					Return(true, nil)
				r.dishes.EXPECT().Give(mock.Anything, "SKU-CROISSANT", 2).Return(domain.Dish{}, nil)
				r.dishes.EXPECT().Give(mock.Anything, "SKU-AMERICANO", 1).Return(domain.Dish{}, nil)
			},
		},
		"an order already out of the kitchen keeps its stock": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, orderID).Return(inState(domain.StateHandedOver), nil)
			},
		},
		"a cancellation of an order the venue never had changes nothing": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, orderID).
					Return(domain.Order{}, domain.ErrNotFound)
			},
		},
		"a cancellation that lost the race writes nothing back": {
			arrange: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, orderID).Return(inState(domain.StateNew), nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateNew, domain.StateCancelled).
					Return(false, nil)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, repos := service(t)
			tc.arrange(repos)

			err := svc.Cancel(t.Context(), cancelled)

			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestServiceAdvance(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		from    domain.State
		arrange func(repositories)
		wantErr error
	}{
		"a new order is accepted": {
			from: domain.StateNew,
			arrange: func(r repositories) {
				r.platform.EXPECT().Accept(mock.Anything, orderID, etaMinutes).Return(nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateNew, domain.StateAccepted).
					Return(true, nil)
			},
		},
		"an accepted order goes on the stove": {
			from: domain.StateAccepted,
			arrange: func(r repositories) {
				r.platform.EXPECT().StartCooking(mock.Anything, orderID).Return(nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateAccepted, domain.StateCooking).
					Return(true, nil)
			},
		},
		"a cooking order becomes ready": {
			from: domain.StateCooking,
			arrange: func(r repositories) {
				r.platform.EXPECT().MarkReady(mock.Anything, orderID).Return(nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateCooking, domain.StateReady).
					Return(true, nil)
			},
		},
		"a ready order is handed to the courier": {
			from: domain.StateReady,
			arrange: func(r repositories) {
				r.platform.EXPECT().Handover(mock.Anything, orderID).Return(nil)
				r.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateReady, domain.StateHandedOver).
					Return(true, nil)
			},
		},
		"an order out of the kitchen is left alone": {
			from:    domain.StateHandedOver,
			arrange: func(repositories) {},
		},
		"a cancelled order is not pushed further": {
			from:    domain.StateCancelled,
			arrange: func(repositories) {},
		},
		"a platform that fails leaves the order where it is": {
			from: domain.StateNew,
			arrange: func(r repositories) {
				r.platform.EXPECT().Accept(mock.Anything, orderID, etaMinutes).Return(errRepo)
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, repos := service(t)
			tc.arrange(repos)

			err := svc.Advance(t.Context(), inState(tc.from))

			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// TestServiceAdvanceCatchesUpWithThePlatform covers the order the platform has
// moved without the venue: the reaper rejected it while it waited for a cook.
func TestServiceAdvanceCatchesUpWithThePlatform(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status    string
		wantState domain.State
		wantStock bool
		wantErr   bool
	}{
		"rejected by the platform": {
			status:    "REJECTED",
			wantState: domain.StateRejected,
			wantStock: true,
		},
		"cancelled by the customer": {
			status:    "CANCELLED",
			wantState: domain.StateCancelled,
			wantStock: true,
		},
		"already accepted, nothing to give back": {
			status:    "ACCEPTED",
			wantState: domain.StateAccepted,
		},
		"a status the venue does not know": {
			status:  "PLATING",
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, repos := service(t)

			repos.platform.EXPECT().Accept(mock.Anything, orderID, etaMinutes).
				Return(domain.RefusedError{Current: tc.status})

			if !tc.wantErr {
				repos.orders.EXPECT().
					Move(mock.Anything, orderID, domain.StateNew, tc.wantState).
					Return(true, nil)
			}

			if tc.wantStock {
				repos.dishes.EXPECT().Give(mock.Anything, "SKU-CROISSANT", 2).Return(domain.Dish{}, nil)
				repos.dishes.EXPECT().Give(mock.Anything, "SKU-AMERICANO", 1).Return(domain.Dish{}, nil)
			}

			err := svc.Advance(t.Context(), inState(domain.StateNew))

			if tc.wantErr {
				require.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestServiceSyncStopList(t *testing.T) {
	t.Parallel()

	dish := func(sku string, stock *int, available bool) domain.Dish {
		return domain.Dish{SKU: sku, Name: sku, Price: 19_000, Stock: stock, IsAvailable: available}
	}

	out, left := 0, 3

	cases := map[string]struct {
		dishes  []domain.Dish
		arrange func(repositories)
		wantErr error
	}{
		"a dish that ran out goes off sale": {
			dishes: []domain.Dish{dish("SKU-CROISSANT", &out, true)},
			arrange: func(r repositories) {
				r.platform.EXPECT().SetItemAvailability(mock.Anything, "SKU-CROISSANT", false).Return(nil)
				r.dishes.EXPECT().SetAvailable(mock.Anything, "SKU-CROISSANT", false).
					Return(domain.Dish{}, nil)
			},
		},
		"a dish baked again comes back on sale": {
			dishes: []domain.Dish{dish("SKU-CROISSANT", &left, false)},
			arrange: func(r repositories) {
				r.platform.EXPECT().SetItemAvailability(mock.Anything, "SKU-CROISSANT", true).Return(nil)
				r.dishes.EXPECT().SetAvailable(mock.Anything, "SKU-CROISSANT", true).
					Return(domain.Dish{}, nil)
			},
		},
		"a dish the platform already knows about is left alone": {
			dishes:  []domain.Dish{dish("SKU-CROISSANT", &left, true), dish("SKU-AMERICANO", nil, true)},
			arrange: func(repositories) {},
		},
		"a dish sold without a limit never runs out": {
			dishes:  []domain.Dish{dish("SKU-AMERICANO", nil, true)},
			arrange: func(repositories) {},
		},
		"a platform that fails keeps the venue's own books untouched": {
			dishes: []domain.Dish{dish("SKU-CROISSANT", &out, true)},
			arrange: func(r repositories) {
				r.platform.EXPECT().SetItemAvailability(mock.Anything, "SKU-CROISSANT", false).
					Return(errRepo)
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, repos := service(t)
			repos.dishes.EXPECT().List(mock.Anything).Return(tc.dishes, nil)
			tc.arrange(repos)

			err := svc.SyncStopList(t.Context())

			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}
