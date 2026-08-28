// Package cart holds the cart of a customer and the rules that recompute and
// check it before an order is placed.
package cart

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"

	"github.com/google/uuid"
)

// CodeVenueConflict is the error code of an item of a venue other than the one
// the cart already holds.
const CodeVenueConflict = "cart_venue_conflict"

// Venue is the part of a venue card the cart depends on. Money is in kopecks.
type Venue struct {
	ID             uuid.UUID
	Name           string
	IsOpen         bool
	MinOrderAmount int64
	DeliveryFee    int64
}

// Line is one position of a cart: the menu item as it is now, the quantity
// asked for and the price the item was put into the cart at.
type Line struct {
	Item          catalog.MenuItem
	Qty           int
	PriceSnapshot int64
}

// LineTotal is what the line costs at the price it was added at.
func (l Line) LineTotal() int64 {
	return int64(l.Qty) * l.PriceSnapshot
}

// Cart is the cart of one customer. Venue is nil while the cart is empty.
type Cart struct {
	Venue *Venue
	Lines []Line
}

// Totals is the money of a cart.
type Totals struct {
	ItemsTotal  int64
	DeliveryFee int64
	Total       int64
}

// IsEmpty reports whether the cart holds no items.
func (c Cart) IsEmpty() bool {
	return len(c.Lines) == 0
}

// Totals sums the lines of the cart at the prices they were added at.
// An empty cart costs nothing, delivery included.
func (c Cart) Totals() Totals {
	if c.IsEmpty() {
		return Totals{}
	}

	var totals Totals
	for _, line := range c.Lines {
		totals.ItemsTotal += line.LineTotal()
	}

	if c.Venue != nil {
		totals.DeliveryFee = c.Venue.DeliveryFee
	}

	totals.Total = totals.ItemsTotal + totals.DeliveryFee

	return totals
}

// Repriced returns the cart as it would look at the current prices of the
// menu. The stored cart is not touched: the customer keeps seeing the prices
// they added the items at until they confirm the new ones.
func (c Cart) Repriced() Cart {
	repriced := Cart{Venue: c.Venue, Lines: make([]Line, len(c.Lines))}

	for i, line := range c.Lines {
		line.PriceSnapshot = line.Item.Price
		repriced.Lines[i] = line
	}

	return repriced
}

// CheckVenue reports whether an item of venueID may join the cart.
// A cart holds items of one venue only.
func (c Cart) CheckVenue(venueID uuid.UUID) error {
	if c.IsEmpty() || c.Venue == nil || c.Venue.ID == venueID {
		return nil
	}

	return domain.Conflictf(CodeVenueConflict,
		"cart already holds items of another venue; clear it first")
}
