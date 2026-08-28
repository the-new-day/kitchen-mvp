package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/domain/catalog"
	"testing"

	"github.com/google/uuid"
)

func TestToMenuItem(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		item        catalog.MenuItem
		venueOpen   bool
		available   bool
		reason      *kitchenapi.UnavailableReason
		description *string
	}{
		"on sale": {
			item:      catalog.MenuItem{IsAvailable: true, StockQty: new(3)},
			venueOpen: true,
			available: true,
		},
		"stopped item stays in the menu": {
			item:      catalog.MenuItem{IsAvailable: false},
			venueOpen: true,
			reason:    new(kitchenapi.UnavailableReasonStopped),
		},
		"closed venue": {
			item:      catalog.MenuItem{IsAvailable: true},
			venueOpen: false,
			reason:    new(kitchenapi.UnavailableReasonVenueClosed),
		},
		"description is sent only when there is one": {
			item:        catalog.MenuItem{IsAvailable: true, Description: "0,33 л."},
			venueOpen:   true,
			available:   true,
			description: new("0,33 л."),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toMenuItem(tc.item, tc.venueOpen)

			if got.IsAvailable != tc.available {
				t.Errorf("is_available = %v, want %v", got.IsAvailable, tc.available)
			}

			switch {
			case tc.reason == nil && got.UnavailableReason != nil:
				t.Errorf("unavailable_reason = %q, want none", *got.UnavailableReason)
			case tc.reason != nil && got.UnavailableReason == nil:
				t.Errorf("unavailable_reason is missing, want %q", *tc.reason)
			case tc.reason != nil && *got.UnavailableReason != *tc.reason:
				t.Errorf("unavailable_reason = %q, want %q", *got.UnavailableReason, *tc.reason)
			}

			switch {
			case tc.description == nil && got.Description != nil:
				t.Errorf("description = %q, want none", *got.Description)
			case tc.description != nil && (got.Description == nil || *got.Description != *tc.description):
				t.Errorf("description = %v, want %q", got.Description, *tc.description)
			}

			if got.StockQty != tc.item.StockQty {
				t.Errorf("stock_qty = %v, want %v", got.StockQty, tc.item.StockQty)
			}
		})
	}
}

func TestToMenuKeepsCategoriesAndItems(t *testing.T) {
	t.Parallel()

	venueID := uuid.MustParse("0192f4c1-0000-7000-8000-000000000002")
	menu := catalog.Menu{
		VenueID:     venueID,
		VenueIsOpen: false,
		Categories: []catalog.MenuCategory{{
			ExternalID: "CAT-PIZZA",
			Name:       "Пицца",
			Position:   10,
			Items: []catalog.MenuItem{
				{ExternalID: "SKU-MARGHERITA", IsAvailable: true, Position: 10},
				{ExternalID: "SKU-PEPPERONI", IsAvailable: false, Position: 20},
			},
		}},
	}

	got := toMenu(menu)

	if got.VenueId != venueID {
		t.Errorf("venue_id = %s, want %s", got.VenueId, venueID)
	}
	if len(got.Categories) != 1 || len(got.Categories[0].Items) != 2 {
		t.Fatalf("got %d categories, want 1 with 2 items", len(got.Categories))
	}
	for _, item := range got.Categories[0].Items {
		if item.IsAvailable {
			t.Errorf("%s is available while the venue is closed", item.ExternalId)
		}
	}

	for i, want := range []int{10, 20} {
		if got.Categories[0].Items[i].Position != want {
			t.Errorf("item %d position = %d, want %d", i, got.Categories[0].Items[i].Position, want)
		}
	}
}

func TestToMenuOfAnEmptyMenuSendsAnArray(t *testing.T) {
	t.Parallel()

	if got := toMenu(catalog.Menu{}); got.Categories == nil {
		t.Fatal("categories are nil, want an empty array")
	}
}
