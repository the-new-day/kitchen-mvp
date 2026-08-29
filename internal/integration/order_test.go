package integration_test

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/outbox"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// statusWait is how long a transition is given to travel from the venue to the
// stream of the customer: through the outbox, the broker and the consumer.
const statusWait = 30 * time.Second

// The whole life of an order after it has been placed. The last step is the
// platform closing the order itself: no venue and no customer reports that a
// delivery has arrived.
var travelled = []string{"ACCEPTED", "COOKING", "READY", "DELIVERING", "DELIVERED"}

// TestOrderReachesTheCustomer walks an order the whole way: out of the
// catalogue into a cart, out of the cart into an order, out of the outbox into
// the topic the venue reads, through the transitions the venue makes and into
// the stream the customer is watching.
func TestOrderReachesTheCustomer(t *testing.T) {
	s := platform(t)
	ctx := t.Context()

	customer, venue := customerOf(t, s), venueOf(t, s)

	orders := ordersReader(t, s)

	customer.fill(ctx, t, s, espresso, 2)

	total := customer.total(ctx, t)
	if want := int64(2*espressoPrice + deliveryFee); total != want {
		t.Fatalf("the cart costs %d, want %d", total, want)
	}

	placed := customer.place(ctx, t, uuid.New(), total, "")
	if placed.JSON201 == nil {
		t.Fatalf("placing the order answered %s: %s", placed.HTTPResponse.Status, placed.Body)
	}

	order := *placed.JSON201

	envelope, key := event(ctx, t, orders)

	if envelope.EventType != outbox.EventOrderCreated {
		t.Fatalf("the venue was told %q, want %q", envelope.EventType, outbox.EventOrderCreated)
	}

	if envelope.AggregateID != order.Id {
		t.Fatalf("the event is about order %s, want %s", envelope.AggregateID, order.Id)
	}

	if key != s.venueID.String() {
		t.Fatalf("the event went under key %s, want the venue %s", key, s.venueID)
	}

	updates := customer.statuses(ctx, t, s, order.Id)

	drive(ctx, t, venue, order.Id)
	awaits(t, updates, travelled, statusWait)

	if final := customer.order(ctx, t, order.Id); string(final.Status) != "DELIVERED" {
		t.Fatalf("the order ended in %s, want DELIVERED", final.Status)
	}

	if waiting := unpublished(ctx, t, s); waiting != 0 {
		t.Fatalf("%d events are still waiting in the outbox", waiting)
	}
}

// TestOrderIsRefused covers what stops a cart from becoming an order. Every
// case builds a cart of its own and asks for the very same thing.
func TestOrderIsRefused(t *testing.T) {
	s := platform(t)

	cases := map[string]struct {
		sku        string
		qty        int
		total      func(counted int64) int64
		wantStatus int
		wantCode   string
	}{
		"a cart with nothing in it": {
			total:      func(int64) int64 { return 0 },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "empty_cart",
		},
		"a cart below the minimum of the venue": {
			sku:        espresso,
			qty:        1,
			total:      func(counted int64) int64 { return counted },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "below_minimum",
		},
		"a sum the customer did not see": {
			sku:        espresso,
			qty:        2,
			total:      func(counted int64) int64 { return counted - 1 },
			wantStatus: http.StatusConflict,
			wantCode:   "price_mismatch",
		},
		"more portions than the venue has": {
			sku:        cheesecake,
			qty:        cheesecakeStock + 1,
			total:      func(counted int64) int64 { return counted },
			wantStatus: http.StatusConflict,
			wantCode:   "out_of_stock",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			customer := customerOf(t, s)

			var counted int64

			if tc.qty > 0 {
				customer.fill(ctx, t, s, tc.sku, tc.qty)
				counted = customer.total(ctx, t)
			}

			res := customer.place(ctx, t, uuid.New(), tc.total(counted), "")
			if res.JSON201 != nil {
				t.Fatalf("the order %s was placed", res.JSON201.Number)
			}

			body := res.JSON409
			if tc.wantStatus == http.StatusUnprocessableEntity {
				body = res.JSON422
			}

			status, code := refusalOf(t, res.StatusCode(), body)
			if status != tc.wantStatus || code != tc.wantCode {
				t.Fatalf("the platform answered %d %q, want %d %q",
					status, code, tc.wantStatus, tc.wantCode)
			}
		})
	}
}

// TestIdempotencyKeyAnswersForOneOrder checks that a repeated attempt of
// placing an order gives back the order that was placed, and that the key
// refuses to answer for a cart it never saw.
func TestIdempotencyKeyAnswersForOneOrder(t *testing.T) {
	s := platform(t)
	ctx := t.Context()

	customer := customerOf(t, s)
	customer.fill(ctx, t, s, espresso, 2)

	total := customer.total(ctx, t)
	key := uuid.New()

	placed := customer.place(ctx, t, key, total, "")
	if placed.JSON201 == nil {
		t.Fatalf("placing the order answered %s", placed.HTTPResponse.Status)
	}

	repeated := customer.place(ctx, t, key, total, "")
	if repeated.JSON200 == nil {
		t.Fatalf("the repeat answered %s, want 200", repeated.HTTPResponse.Status)
	}

	if repeated.JSON200.Id != placed.JSON201.Id {
		t.Fatalf("the repeat gave order %s, want %s", repeated.JSON200.Id, placed.JSON201.Id)
	}

	reused := customer.place(ctx, t, key, total, "без сахара")

	status, code := refusalOf(t, reused.StatusCode(), reused.JSON422)
	if status != http.StatusUnprocessableEntity || code != "idempotency_key_reuse" {
		t.Fatalf("the key with another cart answered %d %q, want 422 idempotency_key_reuse",
			status, code)
	}
}

// TestCancellationEndsWhereCookingBegins checks the two sides of a withdrawal:
// an order nobody has taken into work is cancelled and gives its stock back, an
// order already being cooked is no longer the business of the customer.
func TestCancellationEndsWhereCookingBegins(t *testing.T) {
	s := platform(t)
	ctx := t.Context()

	customer, venue := customerOf(t, s), venueOf(t, s)

	before := stockOf(ctx, t, s, cheesecake)

	customer.fill(ctx, t, s, cheesecake, 1)

	placed := customer.place(ctx, t, uuid.New(), customer.total(ctx, t), "")
	if placed.JSON201 == nil {
		t.Fatalf("placing the order answered %s", placed.HTTPResponse.Status)
	}

	order := *placed.JSON201

	if reserved := stockOf(ctx, t, s, cheesecake); reserved != before-1 {
		t.Fatalf("the stock after the order is %d, want %d", reserved, before-1)
	}

	cancelled, err := customer.api.CancelOrderWithResponse(ctx, order.Id, cancellation())
	if err != nil {
		t.Fatalf("cancel the order: %v", err)
	}

	if cancelled.JSON200 == nil {
		t.Fatalf("the cancellation answered %s", cancelled.HTTPResponse.Status)
	}

	if string(cancelled.JSON200.Status) != "CANCELLED" {
		t.Fatalf("the order is in %s, want CANCELLED", cancelled.JSON200.Status)
	}

	if returned := stockOf(ctx, t, s, cheesecake); returned != before {
		t.Fatalf("the stock after the cancellation is %d, want %d", returned, before)
	}

	cooking := cookingOrder(ctx, t, s, customer, venue)

	refused, err := customer.api.CancelOrderWithResponse(ctx, cooking, cancellation())
	if err != nil {
		t.Fatalf("cancel the order being cooked: %v", err)
	}

	status, code := refusalOf(t, refused.StatusCode(), refused.JSON409)
	if status != http.StatusConflict || code != "invalid_transition" {
		t.Fatalf("cancelling from COOKING answered %d %q, want 409 invalid_transition",
			status, code)
	}
}

// cookingOrder places another order of the same customer and has the venue
// take it into work, so that the withdrawal has something to be refused on.
func cookingOrder(
	ctx context.Context,
	t *testing.T,
	s *stand,
	customer *eater,
	venue *partnerapi.ClientWithResponses,
) uuid.UUID {
	t.Helper()

	customer.fill(ctx, t, s, espresso, 2)

	placed := customer.place(ctx, t, uuid.New(), customer.total(ctx, t), "")
	if placed.JSON201 == nil {
		t.Fatalf("placing the order answered %s", placed.HTTPResponse.Status)
	}

	accepted, err := venue.AcceptOrderWithResponse(ctx, placed.JSON201.Id,
		partnerapi.AcceptOrderRequest{EtaMinutes: 10})
	if err != nil {
		t.Fatalf("accept the order: %v", err)
	}

	if accepted.JSON200 == nil {
		t.Fatalf("accepting the order answered %s", accepted.HTTPResponse.Status)
	}

	started, err := venue.StartCookingWithResponse(ctx, placed.JSON201.Id)
	if err != nil {
		t.Fatalf("start cooking the order: %v", err)
	}

	if started.JSON200 == nil {
		t.Fatalf("starting to cook answered %s", started.HTTPResponse.Status)
	}

	return placed.JSON201.Id
}

// drive moves an order through the venue the way a cash register would: taken
// into work, cooked, ready and handed over to the courier.
func drive(
	ctx context.Context, t *testing.T, venue *partnerapi.ClientWithResponses, orderID uuid.UUID,
) {
	t.Helper()

	accepted, err := venue.AcceptOrderWithResponse(ctx, orderID,
		partnerapi.AcceptOrderRequest{EtaMinutes: 10})
	if err != nil || accepted.JSON200 == nil {
		t.Fatalf("accept the order: %v", err)
	}

	cooking, err := venue.StartCookingWithResponse(ctx, orderID)
	if err != nil || cooking.JSON200 == nil {
		t.Fatalf("start cooking the order: %v", err)
	}

	ready, err := venue.MarkReadyWithResponse(ctx, orderID)
	if err != nil || ready.JSON200 == nil {
		t.Fatalf("mark the order ready: %v", err)
	}

	handed, err := venue.HandoverOrderWithResponse(ctx, orderID)
	if err != nil || handed.JSON200 == nil {
		t.Fatalf("hand the order over: %v", err)
	}
}
