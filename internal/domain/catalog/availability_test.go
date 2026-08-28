package catalog_test

import (
	"avito-kitchen/internal/domain/catalog"
	"testing"
)

func TestMenuItemAvailability(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		item      catalog.MenuItem
		venueOpen bool
		available bool
		reason    catalog.UnavailableReason
	}{
		"on sale, open venue, unlimited stock": {
			item:      catalog.MenuItem{IsAvailable: true},
			venueOpen: true,
			available: true,
			reason:    catalog.ReasonNone,
		},
		"on sale, open venue, stock left": {
			item:      catalog.MenuItem{IsAvailable: true, StockQty: new(3)},
			venueOpen: true,
			available: true,
			reason:    catalog.ReasonNone,
		},
		"stopped by the venue": {
			item:      catalog.MenuItem{IsAvailable: false, StockQty: new(3)},
			venueOpen: true,
			reason:    catalog.ReasonStopped,
		},
		"stopped wins over a closed shift": {
			item:      catalog.MenuItem{IsAvailable: false},
			venueOpen: false,
			reason:    catalog.ReasonStopped,
		},
		"stock ran out": {
			item:      catalog.MenuItem{IsAvailable: true, StockQty: new(0)},
			venueOpen: true,
			reason:    catalog.ReasonOutOfStock,
		},
		"out of stock wins over a closed shift": {
			item:      catalog.MenuItem{IsAvailable: true, StockQty: new(0)},
			venueOpen: false,
			reason:    catalog.ReasonOutOfStock,
		},
		"shift is closed": {
			item:      catalog.MenuItem{IsAvailable: true, StockQty: new(5)},
			venueOpen: false,
			reason:    catalog.ReasonVenueClosed,
		},
		"unlimited stock and a closed shift": {
			item:      catalog.MenuItem{IsAvailable: true},
			venueOpen: false,
			reason:    catalog.ReasonVenueClosed,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			available, reason := tc.item.Availability(tc.venueOpen)
			if available != tc.available {
				t.Errorf("available = %v, want %v", available, tc.available)
			}
			if reason != tc.reason {
				t.Errorf("reason = %q, want %q", reason, tc.reason)
			}
		})
	}
}
