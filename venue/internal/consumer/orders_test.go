package consumer_test

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/venue/internal/consumer"
	"avito-kitchen/venue/internal/consumer/mocks"
	"avito-kitchen/venue/internal/kitchen"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	ourVenue   = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	anotherOne = uuid.MustParse("0192f4c1-0000-7000-8000-000000000002")
	orderID    = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")

	errBusy = errors.New("connection refused")

	placedOrder = kitchen.OrderCreated{
		OrderID: orderID,
		Number:  "AK-100001",
		VenueID: ourVenue,
		Items: []kitchen.OrderItem{
			{ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 19_000, Qty: 2, LineTotal: 38_000},
		},
		Total: 47_900,
	}

	cancelledOrder = kitchen.OrderCancelled{
		OrderID: orderID,
		Number:  "AK-100001",
		VenueID: ourVenue,
		Reason:  "клиент отменил",
	}
)

// envelope wraps a payload the way the platform publishes it.
func envelope(t *testing.T, eventType string, payload any) broker.Envelope {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	return broker.Envelope{EventID: uuid.New(), EventType: eventType, Payload: body}
}

func TestOrdersHandle(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event         func(*testing.T) broker.Envelope
		arrange       func(*mocks.MockKitchen)
		wantMalformed bool
		wantErr       error
	}{
		"an order of this venue reaches the kitchen": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, kitchen.TypeOrderCreated, placedOrder)
			},
			arrange: func(k *mocks.MockKitchen) {
				k.EXPECT().Receive(mock.Anything, placedOrder).Return(nil).Once()
			},
		},
		"an order of another venue is passed over": {
			event: func(t *testing.T) broker.Envelope {
				foreign := placedOrder
				foreign.VenueID = anotherOne

				return envelope(t, kitchen.TypeOrderCreated, foreign)
			},
			arrange: func(*mocks.MockKitchen) {},
		},
		"a cancellation takes the order away": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, kitchen.TypeOrderCancelled, cancelledOrder)
			},
			arrange: func(k *mocks.MockKitchen) {
				k.EXPECT().Cancel(mock.Anything, cancelledOrder).Return(nil).Once()
			},
		},
		"a cancellation for another venue is passed over": {
			event: func(t *testing.T) broker.Envelope {
				foreign := cancelledOrder
				foreign.VenueID = anotherOne

				return envelope(t, kitchen.TypeOrderCancelled, foreign)
			},
			arrange: func(*mocks.MockKitchen) {},
		},
		"an event of another kind is none of the kitchen's business": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, "order.status_changed", map[string]string{"to_status": "DELIVERED"})
			},
			arrange: func(*mocks.MockKitchen) {},
		},
		"a payload that is not an order is dropped": {
			event: func(*testing.T) broker.Envelope {
				return broker.Envelope{
					EventID:   uuid.New(),
					EventType: kitchen.TypeOrderCreated,
					Payload:   json.RawMessage(`"это не заказ"`),
				}
			},
			arrange:       func(*mocks.MockKitchen) {},
			wantMalformed: true,
		},
		"an order without items is dropped": {
			event: func(t *testing.T) broker.Envelope {
				empty := placedOrder
				empty.Items = nil

				return envelope(t, kitchen.TypeOrderCreated, empty)
			},
			arrange:       func(*mocks.MockKitchen) {},
			wantMalformed: true,
		},
		"a cancellation without an order is dropped": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, kitchen.TypeOrderCancelled, kitchen.OrderCancelled{VenueID: ourVenue})
			},
			arrange:       func(*mocks.MockKitchen) {},
			wantMalformed: true,
		},
		"a kitchen that could not take the order asks for the event again": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, kitchen.TypeOrderCreated, placedOrder)
			},
			arrange: func(k *mocks.MockKitchen) {
				k.EXPECT().Receive(mock.Anything, placedOrder).Return(errBusy).Once()
			},
			wantErr: errBusy,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kitchenMock := mocks.NewMockKitchen(t)
			tc.arrange(kitchenMock)

			log := slog.New(slog.NewTextHandler(io.Discard, nil))

			err := consumer.NewOrders(kitchenMock, ourVenue, log).Handle(t.Context(), tc.event(t))

			if tc.wantMalformed {
				assert.ErrorIs(t, err, broker.ErrMalformed)

				return
			}

			assert.ErrorIs(t, err, tc.wantErr)
			assert.NotErrorIs(t, err, broker.ErrMalformed)
		})
	}
}
