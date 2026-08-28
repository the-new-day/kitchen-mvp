package order_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"avito-kitchen/internal/domain/order"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var (
	userID      = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")
	bakeryID    = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	croissantID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000011")

	bakery = cart.Venue{
		ID:             bakeryID,
		Name:           "Пекарня «Батон»",
		IsOpen:         true,
		MinOrderAmount: 30_000,
		DeliveryFee:    9_900,
	}

	croissant = catalog.MenuItem{
		ID:          croissantID,
		ExternalID:  "SKU-CROISSANT",
		Name:        "Круассан",
		Price:       20_000,
		IsAvailable: true,
	}

	request = order.Request{
		Address:       "  Москва, Лесная 7  ",
		Phone:         "+79990000000",
		Comment:       "позвонить",
		ExpectedTotal: 49_900,
	}
)

// filled is the cart of two croissants the happy path is built from: 40 000
// for the items and 9 900 for the delivery.
func filled() cart.Cart {
	venue := bakery

	return cart.Cart{
		Venue: &venue,
		Lines: []cart.Line{{Item: croissant, Qty: 2, PriceSnapshot: 20_000}},
	}
}

func TestFromCart(t *testing.T) {
	t.Parallel()

	stock := func(qty int) *int { return &qty }

	cases := map[string]struct {
		cart     func() cart.Cart
		request  order.Request
		wantErr  error
		wantCode string
	}{
		"an empty cart is not an order": {
			cart:     func() cart.Cart { return cart.Cart{} },
			request:  request,
			wantErr:  domain.ErrUnprocessable,
			wantCode: order.CodeEmptyCart,
		},
		"a closed venue takes no orders": {
			cart: func() cart.Cart {
				c := filled()
				c.Venue.IsOpen = false

				return c
			},
			request:  request,
			wantErr:  domain.ErrConflict,
			wantCode: order.CodeVenueClosed,
		},
		"an item off the menu stops the order": {
			cart: func() cart.Cart {
				c := filled()
				c.Lines[0].Item.IsAvailable = false

				return c
			},
			request:  request,
			wantErr:  domain.ErrConflict,
			wantCode: order.CodeItemUnavailable,
		},
		"a stock smaller than the quantity stops the order": {
			cart: func() cart.Cart {
				c := filled()
				c.Lines[0].Item.StockQty = stock(1)

				return c
			},
			request:  request,
			wantErr:  domain.ErrConflict,
			wantCode: order.CodeOutOfStock,
		},
		"a cart below the minimum of the venue stops the order": {
			cart: func() cart.Cart {
				c := filled()
				c.Lines[0].Qty = 1

				return c
			},
			request:  order.Request{Address: "Лесная 7", Phone: "+79990000000", ExpectedTotal: 29_900},
			wantErr:  domain.ErrUnprocessable,
			wantCode: order.CodeBelowMinimum,
		},
		"a total the customer did not see stops the order": {
			cart:     filled,
			request:  order.Request{Address: "Лесная 7", Phone: "+79990000000", ExpectedTotal: 40_000},
			wantErr:  domain.ErrConflict,
			wantCode: order.CodePriceMismatch,
		},
		"an order without an address is malformed": {
			cart:    filled,
			request: order.Request{Address: "   ", Phone: "+79990000000", ExpectedTotal: 49_900},
			wantErr: domain.ErrInvalidArgument,
		},
		"an order without a phone is malformed": {
			cart:    filled,
			request: order.Request{Address: "Лесная 7", ExpectedTotal: 49_900},
			wantErr: domain.ErrInvalidArgument,
		},
		"a price that moved is confirmed by the total, not refused": {
			cart: func() cart.Cart {
				c := filled()
				c.Lines[0].Item.Price = 25_000

				return c
			},
			request: order.Request{Address: "Лесная 7", Phone: "+79990000000", ExpectedTotal: 59_900},
		},
		"a full cart becomes an order": {
			cart:    filled,
			request: request,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			draft, err := order.FromCart(userID, tc.cart(), tc.request)

			if tc.wantErr != nil {
				assertRefused(t, err, tc.wantErr, tc.wantCode)

				return
			}

			if err != nil {
				t.Fatalf("FromCart() error = %v", err)
			}

			if draft.Total != tc.request.ExpectedTotal {
				t.Fatalf("total = %d, want %d", draft.Total, tc.request.ExpectedTotal)
			}
		})
	}
}

func TestFromCartCopiesTheCart(t *testing.T) {
	t.Parallel()

	draft, err := order.FromCart(userID, filled(), request)
	if err != nil {
		t.Fatalf("FromCart() error = %v", err)
	}

	switch {
	case draft.UserID != userID:
		t.Fatalf("user = %s, want %s", draft.UserID, userID)
	case draft.VenueID != bakeryID:
		t.Fatalf("venue = %s, want %s", draft.VenueID, bakeryID)
	case draft.ItemsTotal != 40_000:
		t.Fatalf("items total = %d, want 40000", draft.ItemsTotal)
	case draft.DeliveryFee != 9_900:
		t.Fatalf("delivery fee = %d, want 9900", draft.DeliveryFee)
	case draft.Address != "Москва, Лесная 7":
		t.Fatalf("address = %q, want it trimmed", draft.Address)
	case len(draft.Items) != 1:
		t.Fatalf("items = %d, want 1", len(draft.Items))
	}

	item := draft.Items[0]

	switch {
	case item.MenuItemID != croissantID:
		t.Fatalf("item id = %s, want %s", item.MenuItemID, croissantID)
	case item.ExternalID != "SKU-CROISSANT":
		t.Fatalf("external id = %q", item.ExternalID)
	case item.Name != croissant.Name:
		t.Fatalf("name = %q, want %q", item.Name, croissant.Name)
	case item.Price != 20_000 || item.Qty != 2 || item.LineTotal() != 40_000:
		t.Fatalf("item = %+v, want two croissants at 20000", item)
	}
}

// assertRefused checks that an order was refused with the expected kind of
// error and, where the envelope carries one, the expected code.
func assertRefused(t *testing.T, err, want error, code string) {
	t.Helper()

	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}

	if code == "" {
		return
	}

	var (
		conflict      domain.ConflictError
		unprocessable domain.UnprocessableError
	)

	switch {
	case errors.As(err, &conflict):
		if conflict.Code != code {
			t.Fatalf("code = %q, want %q", conflict.Code, code)
		}
	case errors.As(err, &unprocessable):
		if unprocessable.Code != code {
			t.Fatalf("code = %q, want %q", unprocessable.Code, code)
		}
	default:
		t.Fatalf("error %v carries no code", err)
	}
}
