package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/order"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestToPartnerOrder(t *testing.T) {
	t.Parallel()

	orderID := uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")

	placed := order.Order{
		ID:     orderID,
		Number: "AK-100001",
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
	}

	accepted := placed
	accepted.Status = order.StatusAccepted
	accepted.EtaMinutes = new(15)
	accepted.Comment = "позвонить"

	rejected := placed
	rejected.Status = order.StatusRejected
	rejected.RejectionReason = "нет теста"

	cases := map[string]struct {
		order         order.Order
		wantStatus    partnerapi.OrderStatus
		wantEta       *int
		wantComment   *string
		wantRejection *string
	}{
		"a new order carries the identifiers of the venue itself": {
			order:      placed,
			wantStatus: partnerapi.OrderStatus(order.StatusCreated),
		},
		"an accepted order carries the estimate the venue promised": {
			order:       accepted,
			wantStatus:  partnerapi.OrderStatus(order.StatusAccepted),
			wantEta:     accepted.EtaMinutes,
			wantComment: &accepted.Comment,
		},
		"a refused order carries the reason": {
			order:         rejected,
			wantStatus:    partnerapi.OrderStatus(order.StatusRejected),
			wantRejection: &rejected.RejectionReason,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toPartnerOrder(tc.order)

			switch {
			case got.Id != orderID || got.Number != tc.order.Number:
				t.Fatalf("order = %+v", got)
			case got.Status != tc.wantStatus:
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			case len(got.Items) != 1:
				t.Fatalf("items = %d, want 1", len(got.Items))
			case got.Items[0].ExternalId != "SKU-CROISSANT":
				t.Fatalf("external id = %q", got.Items[0].ExternalId)
			case got.Items[0].LineTotal != 40_000:
				t.Fatalf("line total = %d, want 40000", got.Items[0].LineTotal)
			case got.ItemsTotal != 40_000 || got.Total != 49_900:
				t.Fatalf("totals = %d and %d", got.ItemsTotal, got.Total)
			case got.Address == nil || *got.Address != tc.order.Address:
				t.Fatalf("address = %v", got.Address)
			case got.Phone == nil || *got.Phone != tc.order.Phone:
				t.Fatalf("phone = %v", got.Phone)
			}

			if !samePtr(got.EtaMinutes, tc.wantEta) {
				t.Errorf("eta = %v, want %v", got.EtaMinutes, tc.wantEta)
			}

			if !samePtr(got.Comment, tc.wantComment) {
				t.Errorf("comment = %v, want %v", got.Comment, tc.wantComment)
			}

			if !samePtr(got.RejectionReason, tc.wantRejection) {
				t.Errorf("rejection = %v, want %v", got.RejectionReason, tc.wantRejection)
			}
		})
	}
}

func samePtr[T comparable](got, want *T) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}

	return *got == *want
}
