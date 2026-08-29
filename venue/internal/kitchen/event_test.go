package kitchen_test

import (
	"avito-kitchen/venue/internal/kitchen"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderCreatedValidate(t *testing.T) {
	t.Parallel()

	order := func(mutate func(*kitchen.OrderCreated)) kitchen.OrderCreated {
		placed := kitchen.OrderCreated{
			OrderID: uuid.New(),
			Number:  "A-1042",
			VenueID: uuid.New(),
			Items: []kitchen.OrderItem{
				{ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 19_000, Qty: 2, LineTotal: 38_000},
			},
			Total: 47_000,
		}

		if mutate != nil {
			mutate(&placed)
		}

		return placed
	}

	cases := map[string]struct {
		order kitchen.OrderCreated
		valid bool
	}{
		"a whole order": {order: order(nil), valid: true},
		"without an identifier": {
			order: order(func(o *kitchen.OrderCreated) { o.OrderID = uuid.Nil }),
		},
		"without a venue": {
			order: order(func(o *kitchen.OrderCreated) { o.VenueID = uuid.Nil }),
		},
		"without items": {
			order: order(func(o *kitchen.OrderCreated) { o.Items = nil }),
		},
		"with a position of no sku": {
			order: order(func(o *kitchen.OrderCreated) { o.Items[0].ExternalID = "" }),
		},
		"with a position of no quantity": {
			order: order(func(o *kitchen.OrderCreated) { o.Items[0].Qty = 0 }),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.order.Validate()

			if tc.valid {
				assert.NoError(t, err)

				return
			}

			assert.Error(t, err)
		})
	}
}

func TestOrderCancelledValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event kitchen.OrderCancelled
		valid bool
	}{
		"a whole cancellation": {
			event: kitchen.OrderCancelled{OrderID: uuid.New(), VenueID: uuid.New(), Reason: "клиент передумал"},
			valid: true,
		},
		"without an order": {event: kitchen.OrderCancelled{VenueID: uuid.New()}},
		"without a venue":  {event: kitchen.OrderCancelled{OrderID: uuid.New()}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.event.Validate()

			if tc.valid {
				assert.NoError(t, err)

				return
			}

			assert.Error(t, err)
		})
	}
}

// TestOrderCreatedReadsThePlatformPayload keeps the venue's own view of the
// event on the wire: the fields are those the platform publishes.
func TestOrderCreatedReadsThePlatformPayload(t *testing.T) {
	t.Parallel()

	const payload = `{
		"order_id": "0192f4c1-0000-7000-8000-0000000000aa",
		"number": "A-1042",
		"venue_id": "0192f4c1-0000-7000-8000-000000000001",
		"status": "CREATED",
		"items": [{"external_id": "SKU-CROISSANT", "name": "Круассан",
		           "price": 19000, "qty": 2, "line_total": 38000}],
		"items_total": 38000,
		"delivery_fee": 9900,
		"total": 47900,
		"address": "Москва, Лесная ул., 5",
		"phone": "+79990000000",
		"created_at": "2026-08-29T10:00:00Z"
	}`

	var placed kitchen.OrderCreated
	require.NoError(t, json.Unmarshal([]byte(payload), &placed))
	require.NoError(t, placed.Validate())

	assert.Equal(t, "A-1042", placed.Number)
	assert.Equal(t, int64(47_900), placed.Total)
	require.Len(t, placed.Items, 1)
	assert.Equal(t, "SKU-CROISSANT", placed.Items[0].ExternalID)
	assert.Equal(t, 2, placed.Items[0].Qty)
}
