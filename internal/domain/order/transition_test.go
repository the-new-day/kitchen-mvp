package order_test

import (
	"avito-kitchen/internal/domain/order"
	"testing"
)

func TestStatusReturnsStock(t *testing.T) {
	t.Parallel()

	cases := map[order.Status]bool{
		order.StatusCreated:    false,
		order.StatusAccepted:   false,
		order.StatusCooking:    false,
		order.StatusReady:      false,
		order.StatusDelivering: false,
		order.StatusDelivered:  false,
		order.StatusRejected:   true,
		order.StatusCancelled:  true,
	}

	for status, want := range cases {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			if got := status.ReturnsStock(); got != want {
				t.Fatalf("%s returns stock = %v, want %v", status, got, want)
			}
		})
	}
}
