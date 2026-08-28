package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/domain/order"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	orderID      = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")
	orderVenueID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")

	placed = order.Order{
		ID:     orderID,
		Number: "AK-100001",
		Venue:  order.Venue{ID: orderVenueID, Name: "Пекарня «Батон»"},
		Status: order.StatusCreated,
		Items: []order.Item{
			{ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 20_000, Qty: 2},
		},
		ItemsTotal:  40_000,
		DeliveryFee: 9_900,
		Total:       49_900,
		Address:     "Москва, Лесная 7",
		Phone:       "+79990000000",
		CreatedAt:   time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
	}
)

func TestToOrder(t *testing.T) {
	t.Parallel()

	rejected := placed
	rejected.Status = order.StatusRejected
	rejected.RejectionReason = "нет теста"
	rejected.Comment = "позвонить"
	rejected.EtaMinutes = nil

	cases := map[string]struct {
		order            order.Order
		wantStatus       kitchenapi.OrderStatus
		wantLineTotal    int64
		wantComment      *string
		wantRejection    *string
		wantAddressGiven bool
	}{
		"a placed order carries its lines and its address": {
			order:            placed,
			wantStatus:       kitchenapi.OrderStatus(order.StatusCreated),
			wantLineTotal:    40_000,
			wantAddressGiven: true,
		},
		"the fields a venue fills in are absent until it does": {
			order:            rejected,
			wantStatus:       kitchenapi.OrderStatus(order.StatusRejected),
			wantLineTotal:    40_000,
			wantComment:      &rejected.Comment,
			wantRejection:    &rejected.RejectionReason,
			wantAddressGiven: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toOrder(tc.order)

			switch {
			case got.Id != orderID || got.Number != tc.order.Number:
				t.Fatalf("order = %+v, want %s", got, tc.order.Number)
			case got.Status != tc.wantStatus:
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			case got.Venue.Id != orderVenueID || got.Venue.Name != tc.order.Venue.Name:
				t.Fatalf("venue = %+v", got.Venue)
			case len(got.Items) != 1 || got.Items[0].LineTotal != tc.wantLineTotal:
				t.Fatalf("items = %+v, want one line of %d", got.Items, tc.wantLineTotal)
			case got.Total != tc.order.Total || got.ItemsTotal != tc.order.ItemsTotal:
				t.Fatalf("totals = %d and %d", got.ItemsTotal, got.Total)
			case tc.wantAddressGiven != (got.Address != nil):
				t.Fatalf("address = %v, want present = %v", got.Address, tc.wantAddressGiven)
			case got.EtaMinutes != nil:
				t.Fatalf("eta = %v, want none until the venue names one", *got.EtaMinutes)
			}

			assertOptional(t, "comment", got.Comment, tc.wantComment)
			assertOptional(t, "rejection_reason", got.RejectionReason, tc.wantRejection)
		})
	}
}

// TestIdempotentOperations checks that the middleware guards exactly what the
// specification marks, so that the two cannot drift apart unnoticed.
func TestIdempotentOperations(t *testing.T) {
	t.Parallel()

	operations, err := idempotentOperations()
	if err != nil {
		t.Fatalf("read idempotent operations: %v", err)
	}

	want := map[string]struct{}{
		http.MethodPost + " " + basePath + "/orders": {},
	}

	if len(operations) != len(want) {
		t.Fatalf("operations = %v, want %v", operations, want)
	}

	for operation := range want {
		if _, ok := operations[operation]; !ok {
			t.Fatalf("%s is not guarded", operation)
		}
	}
}

func assertOptional(t *testing.T, name string, got, want *string) {
	t.Helper()

	switch {
	case want == nil && got != nil:
		t.Fatalf("%s = %q, want none", name, *got)
	case want != nil && got == nil:
		t.Fatalf("%s is absent, want %q", name, *want)
	case want != nil && *got != *want:
		t.Fatalf("%s = %q, want %q", name, *got, *want)
	}
}
