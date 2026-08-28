package order

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"strings"

	"github.com/google/uuid"
)

// Codes of the error envelope an order that cannot be placed is refused with.
const (
	CodeEmptyCart       = "empty_cart"
	CodePriceMismatch   = "price_mismatch"
	CodeVenueClosed     = "venue_closed"
	CodeItemUnavailable = "item_unavailable"
	CodeOutOfStock      = "out_of_stock"
	CodeBelowMinimum    = "below_minimum"
)

const (
	maxAddressLen = 500
	maxPhoneLen   = 32
	maxCommentLen = 500
)

// Request is what the customer asks for when placing an order.
// ExpectedTotal is the sum they saw on the screen.
type Request struct {
	Address       string
	Phone         string
	Comment       string
	ExpectedTotal int64
}

// Draft is an order ready to be stored:
// the cart with its prices already copied into it.
type Draft struct {
	UserID      uuid.UUID
	VenueID     uuid.UUID
	Items       []Item
	ItemsTotal  int64
	DeliveryFee int64
	Total       int64
	Address     string
	Phone       string
	Comment     string
}

// FromCart turns a cart into the order it would become, refusing everything
// that stops it: an empty cart, a closed venue, an item off the menu, a stock
// too small and a sum below the minimum of the venue.
//
// The order is priced at the current prices of the menu, and ExpectedTotal is
// what guards the customer against paying more than they saw: a price that
// moved while they were choosing does not block the order, a total that does
// not match does.
func FromCart(userID uuid.UUID, current cart.Cart, req Request) (Draft, error) {
	clean, err := req.normalize()
	if err != nil {
		return Draft{}, err
	}

	if current.IsEmpty() || current.Venue == nil {
		return Draft{}, domain.Unprocessablef(CodeEmptyCart, "cart is empty")
	}

	if err := refuse(current.Validate()); err != nil {
		return Draft{}, err
	}

	priced := current.Repriced()
	totals := priced.Totals()

	if clean.ExpectedTotal != totals.Total {
		return Draft{}, domain.Conflictf(CodePriceMismatch,
			"order costs %d, not %d", totals.Total, clean.ExpectedTotal)
	}

	items := make([]Item, 0, len(priced.Lines))
	for _, line := range priced.Lines {
		items = append(items, Item{
			MenuItemID: line.Item.ID,
			ExternalID: line.Item.ExternalID,
			Name:       line.Item.Name,
			Price:      line.PriceSnapshot,
			Qty:        line.Qty,
		})
	}

	return Draft{
		UserID:      userID,
		VenueID:     current.Venue.ID,
		Items:       items,
		ItemsTotal:  totals.ItemsTotal,
		DeliveryFee: totals.DeliveryFee,
		Total:       totals.Total,
		Address:     clean.Address,
		Phone:       clean.Phone,
		Comment:     clean.Comment,
	}, nil
}

// refuse turns the first problem of a cart check into the error the order is
// refused with. A price that has changed is not one of them: the customer
// confirms the new prices with ExpectedTotal.
func refuse(validation cart.Validation) error {
	for _, problem := range validation.Problems {
		switch problem.Type {
		case cart.ProblemVenueClosed:
			return domain.Conflictf(CodeVenueClosed, "%s", problem.Message)
		case cart.ProblemItemUnavailable:
			return domain.Conflictf(CodeItemUnavailable, "%s", problem.Message)
		case cart.ProblemOutOfStock:
			return domain.Conflictf(CodeOutOfStock, "%s", problem.Message)
		case cart.ProblemBelowMinimum:
			return domain.Unprocessablef(CodeBelowMinimum, "%s", problem.Message)
		case cart.ProblemPriceChanged:
		}
	}

	return nil
}

// normalize trims the request and checks the fields an order cannot go without.
func (r Request) normalize() (Request, error) {
	clean := Request{
		Address:       strings.TrimSpace(r.Address),
		Phone:         strings.TrimSpace(r.Phone),
		Comment:       strings.TrimSpace(r.Comment),
		ExpectedTotal: r.ExpectedTotal,
	}

	switch {
	case clean.Address == "":
		return Request{}, domain.InvalidArgumentf("address is required")
	case len(clean.Address) > maxAddressLen:
		return Request{}, domain.InvalidArgumentf("address must be at most %d bytes", maxAddressLen)
	case clean.Phone == "":
		return Request{}, domain.InvalidArgumentf("phone is required")
	case len(clean.Phone) > maxPhoneLen:
		return Request{}, domain.InvalidArgumentf("phone must be at most %d bytes", maxPhoneLen)
	case len(clean.Comment) > maxCommentLen:
		return Request{}, domain.InvalidArgumentf("comment must be at most %d bytes", maxCommentLen)
	case clean.ExpectedTotal < 0:
		return Request{}, domain.InvalidArgumentf("expected_total must not be negative")
	}

	return clean, nil
}
