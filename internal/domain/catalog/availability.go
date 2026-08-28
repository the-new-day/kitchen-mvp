package catalog

// UnavailableReason explains why a menu item cannot be ordered.
type UnavailableReason string

// Reasons an item can be unavailable for.
const (
	ReasonNone        UnavailableReason = ""
	ReasonStopped     UnavailableReason = "stopped"
	ReasonOutOfStock  UnavailableReason = "out_of_stock"
	ReasonVenueClosed UnavailableReason = "venue_closed"
)

// Availability reports whether the item can be ordered right now and, when it
// cannot, why. The state of the item itself outweighs the state of the venue:
// an item taken off the menu stays "stopped" even while the shift is closed.
func (i MenuItem) Availability(venueOpen bool) (bool, UnavailableReason) {
	switch {
	case !i.IsAvailable:
		return false, ReasonStopped
	case i.StockQty != nil && *i.StockQty <= 0:
		return false, ReasonOutOfStock
	case !venueOpen:
		return false, ReasonVenueClosed
	default:
		return true, ReasonNone
	}
}
