package order_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"errors"
	"fmt"
	"testing"
)

// allowed is every transition the state machine is meant to have.
// The test walks the whole product of statuses and events against it,
// so a row added to the table without a case here fails
// as a forbidden transition that suddenly works.
var allowed = map[order.Status]map[order.Event]order.Status{
	order.StatusCreated: {
		order.EventAccept: order.StatusAccepted,
		order.EventReject: order.StatusRejected,
		order.EventCancel: order.StatusCancelled,
	},
	order.StatusAccepted: {
		order.EventStart:  order.StatusCooking,
		order.EventReject: order.StatusRejected,
		order.EventCancel: order.StatusCancelled,
	},
	order.StatusCooking:    {order.EventReady: order.StatusReady},
	order.StatusReady:      {order.EventHandover: order.StatusDelivering},
	order.StatusDelivering: {order.EventDeliver: order.StatusDelivered},
}

// repeated is the event that has already brought an order into a status:
// applying it again changes nothing instead of failing.
var repeated = map[order.Status]order.Event{
	order.StatusAccepted:   order.EventAccept,
	order.StatusCooking:    order.EventStart,
	order.StatusReady:      order.EventReady,
	order.StatusDelivering: order.EventHandover,
	order.StatusDelivered:  order.EventDeliver,
	order.StatusRejected:   order.EventReject,
	order.StatusCancelled:  order.EventCancel,
}

// transitionCase is one cell of the status-by-event table.
type transitionCase struct {
	from    order.Status
	event   order.Event
	to      order.Status
	changed bool
	err     error
}

func TestNext(t *testing.T) {
	t.Parallel()

	cases := map[string]transitionCase{}

	for _, from := range order.Statuses() {
		for _, event := range order.Events() {
			tc := transitionCase{from: from, event: event, err: domain.ErrInvalidTransition}

			switch {
			case allowed[from][event] != "":
				tc = transitionCase{from: from, event: event, to: allowed[from][event], changed: true}
			case repeated[from] == event:
				tc = transitionCase{from: from, event: event, to: from}
			}

			cases[fmt.Sprintf("%s on %s", event, from)] = tc
		}
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := order.Next(order.FulfillmentDelivery, tc.from, tc.event)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}

			if tc.err != nil {
				return
			}

			if got.To != tc.to || got.Changed != tc.changed {
				t.Fatalf("got %+v, want to=%s changed=%v", got, tc.to, tc.changed)
			}
		})
	}
}

func TestNextUnknownFulfillment(t *testing.T) {
	t.Parallel()

	_, err := order.Next("PICKUP", order.StatusCreated, order.EventAccept)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("error = %v, want %v", err, domain.ErrInvalidTransition)
	}
}

func TestStatusIsTerminal(t *testing.T) {
	t.Parallel()

	cases := map[order.Status]bool{
		order.StatusCreated:    false,
		order.StatusAccepted:   false,
		order.StatusCooking:    false,
		order.StatusReady:      false,
		order.StatusDelivering: false,
		order.StatusDelivered:  true,
		order.StatusRejected:   true,
		order.StatusCancelled:  true,
	}

	for status, want := range cases {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			if got := status.IsTerminal(); got != want {
				t.Fatalf("IsTerminal() = %v, want %v", got, want)
			}
		})
	}
}
