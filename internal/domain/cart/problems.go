package cart

import (
	"fmt"

	"github.com/google/uuid"
)

// ProblemType names what stops a cart from becoming an order.
type ProblemType string

// Problems the check of a cart reports.
const (
	ProblemPriceChanged    ProblemType = "price_changed"
	ProblemOutOfStock      ProblemType = "out_of_stock"
	ProblemItemUnavailable ProblemType = "item_unavailable"
	ProblemVenueClosed     ProblemType = "venue_closed"
	ProblemBelowMinimum    ProblemType = "below_minimum"
)

// Problem is one finding of a cart check. The fields beyond Type and Message
// are filled in only where the type gives them a meaning.
type Problem struct {
	Type         ProblemType
	Message      string
	ItemID       *uuid.UUID
	ItemName     string
	OldPrice     *int64
	NewPrice     *int64
	RequestedQty *int
	AvailableQty *int
	Shortfall    *int64
}

// Validation is the answer of a cart check: everything that is wrong with the
// cart and the cart itself recomputed at the current prices of the menu.
type Validation struct {
	IsValid  bool
	Problems []Problem
	Cart     Cart
}

// Validate compares the cart with the current state of the menu and the venue.
// The venue is reported once for the whole cart, the rest item by item in the
// order of the cart.
func (c Cart) Validate() Validation {
	problems := make([]Problem, 0)

	if !c.IsEmpty() && c.Venue != nil && !c.Venue.IsOpen {
		problems = append(problems, Problem{
			Type:    ProblemVenueClosed,
			Message: fmt.Sprintf("%s is closed and takes no orders right now", c.Venue.Name),
		})
	}

	for _, line := range c.Lines {
		problems = append(problems, line.problems()...)
	}

	repriced := c.Repriced()

	if problem, ok := c.belowMinimum(repriced.Totals().ItemsTotal); ok {
		problems = append(problems, problem)
	}

	return Validation{
		IsValid:  len(problems) == 0 && !c.IsEmpty(),
		Problems: problems,
		Cart:     repriced,
	}
}

// problems reports what is wrong with one line: an item taken off the menu, a
// stock too small for the quantity asked for and a price that has moved since
// the item was added.
func (l Line) problems() []Problem {
	var problems []Problem

	itemID := l.Item.ID

	if !l.Item.IsAvailable {
		problems = append(problems, Problem{
			Type:     ProblemItemUnavailable,
			Message:  fmt.Sprintf("%s is not on sale right now", l.Item.Name),
			ItemID:   &itemID,
			ItemName: l.Item.Name,
		})
	}

	if l.Item.StockQty != nil && *l.Item.StockQty < l.Qty {
		available, requested := *l.Item.StockQty, l.Qty
		problems = append(problems, Problem{
			Type:         ProblemOutOfStock,
			Message:      fmt.Sprintf("only %d of %s left", available, l.Item.Name),
			ItemID:       &itemID,
			ItemName:     l.Item.Name,
			RequestedQty: &requested,
			AvailableQty: &available,
		})
	}

	if l.Item.Price != l.PriceSnapshot {
		oldPrice, newPrice := l.PriceSnapshot, l.Item.Price
		problems = append(problems, Problem{
			Type:     ProblemPriceChanged,
			Message:  fmt.Sprintf("price of %s has changed", l.Item.Name),
			ItemID:   &itemID,
			ItemName: l.Item.Name,
			OldPrice: &oldPrice,
			NewPrice: &newPrice,
		})
	}

	return problems
}

// belowMinimum reports how much the cart is short of the minimum order amount
// of its venue, counted at the current prices.
func (c Cart) belowMinimum(itemsTotal int64) (Problem, bool) {
	if c.IsEmpty() || c.Venue == nil || itemsTotal >= c.Venue.MinOrderAmount {
		return Problem{}, false
	}

	shortfall := c.Venue.MinOrderAmount - itemsTotal

	return Problem{
		Type:      ProblemBelowMinimum,
		Message:   fmt.Sprintf("order is below the minimum of %s", c.Venue.Name),
		Shortfall: &shortfall,
	}, true
}
