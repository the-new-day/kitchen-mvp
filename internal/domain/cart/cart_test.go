package cart_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var (
	bakeryID    = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	pizzaID     = uuid.MustParse("0192f4c1-0000-7000-8000-000000000002")
	croissantID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000011")
	baguetteID  = uuid.MustParse("0192f4c1-0000-7000-8000-000000000012")

	bakery = cart.Venue{
		ID:             bakeryID,
		Name:           "Пекарня «Батон»",
		IsOpen:         true,
		MinOrderAmount: 50_000,
		DeliveryFee:    9_900,
	}
)

func croissant(price int64, stock *int, available bool) catalog.MenuItem {
	return catalog.MenuItem{
		ID:          croissantID,
		ExternalID:  "SKU-CROISSANT",
		Name:        "Круассан",
		Price:       price,
		IsAvailable: available,
		StockQty:    stock,
	}
}

func baguette(price int64) catalog.MenuItem {
	return catalog.MenuItem{
		ID:          baguetteID,
		ExternalID:  "SKU-BAGUETTE",
		Name:        "Багет",
		Price:       price,
		IsAvailable: true,
	}
}

func TestCartTotals(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cart cart.Cart
		want cart.Totals
	}{
		"empty cart costs nothing, delivery included": {
			cart: cart.Cart{Venue: &bakery},
			want: cart.Totals{},
		},
		"lines are summed at the price they were added at": {
			cart: cart.Cart{Venue: &bakery, Lines: []cart.Line{
				{Item: croissant(20_000, nil, true), Qty: 3, PriceSnapshot: 18_000},
				{Item: baguette(9_000), Qty: 1, PriceSnapshot: 9_000},
			}},
			want: cart.Totals{ItemsTotal: 63_000, DeliveryFee: 9_900, Total: 72_900},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.cart.Totals(); got != tc.want {
				t.Errorf("totals = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCartRepriced(t *testing.T) {
	t.Parallel()

	original := cart.Cart{Venue: &bakery, Lines: []cart.Line{
		{Item: croissant(20_000, nil, true), Qty: 2, PriceSnapshot: 18_000},
	}}

	repriced := original.Repriced()

	if got, want := repriced.Totals().ItemsTotal, int64(40_000); got != want {
		t.Errorf("repriced items total = %d, want %d", got, want)
	}

	if got, want := original.Totals().ItemsTotal, int64(36_000); got != want {
		t.Errorf("original items total = %d, want %d: repricing must not touch the cart", got, want)
	}
}

func TestCartCheckVenue(t *testing.T) {
	t.Parallel()

	filled := cart.Cart{Venue: &bakery, Lines: []cart.Line{
		{Item: croissant(20_000, nil, true), Qty: 1, PriceSnapshot: 20_000},
	}}

	cases := map[string]struct {
		cart    cart.Cart
		venueID uuid.UUID
		wantErr bool
	}{
		"empty cart takes any venue": {cart: cart.Cart{}, venueID: pizzaID},
		"same venue":                 {cart: filled, venueID: bakeryID},
		"another venue":              {cart: filled, venueID: pizzaID, wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.cart.CheckVenue(tc.venueID)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("check venue = %v, want nil", err)
				}

				return
			}

			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("check venue = %v, want a conflict", err)
			}

			var conflict domain.ConflictError
			if !errors.As(err, &conflict) || conflict.Code != cart.CodeVenueConflict {
				t.Errorf("conflict code = %q, want %q", conflict.Code, cart.CodeVenueConflict)
			}
		})
	}
}

func TestCartValidate(t *testing.T) {
	t.Parallel()

	closed := bakery
	closed.IsOpen = false

	cheap := bakery
	cheap.MinOrderAmount = 0

	one, three := 1, 3

	cases := map[string]struct {
		cart        cart.Cart
		wantValid   bool
		wantTypes   []cart.ProblemType
		wantProblem func(*testing.T, cart.Problem)
	}{
		"a cart that can be ordered": {
			cart: cart.Cart{Venue: &cheap, Lines: []cart.Line{
				{Item: croissant(20_000, &three, true), Qty: 3, PriceSnapshot: 20_000},
			}},
			wantValid: true,
			wantTypes: []cart.ProblemType{},
		},
		"an empty cart is not an order": {
			cart:      cart.Cart{},
			wantTypes: []cart.ProblemType{},
		},
		"price changed": {
			cart: cart.Cart{Venue: &cheap, Lines: []cart.Line{
				{Item: croissant(22_000, nil, true), Qty: 1, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemPriceChanged},
			wantProblem: func(t *testing.T, p cart.Problem) {
				if p.OldPrice == nil || *p.OldPrice != 20_000 || p.NewPrice == nil || *p.NewPrice != 22_000 {
					t.Errorf("prices = %v -> %v, want 20000 -> 22000", p.OldPrice, p.NewPrice)
				}
				if p.ItemID == nil || *p.ItemID != croissantID {
					t.Errorf("item id = %v, want %v", p.ItemID, croissantID)
				}
			},
		},
		"out of stock": {
			cart: cart.Cart{Venue: &cheap, Lines: []cart.Line{
				{Item: croissant(20_000, &one, true), Qty: 3, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemOutOfStock},
			wantProblem: func(t *testing.T, p cart.Problem) {
				if p.RequestedQty == nil || *p.RequestedQty != 3 || p.AvailableQty == nil || *p.AvailableQty != 1 {
					t.Errorf("quantities = %v of %v, want 3 of 1", p.RequestedQty, p.AvailableQty)
				}
			},
		},
		"item taken off the menu": {
			cart: cart.Cart{Venue: &cheap, Lines: []cart.Line{
				{Item: croissant(20_000, nil, false), Qty: 1, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemItemUnavailable},
		},
		"venue closed is reported once for the cart": {
			cart: cart.Cart{Venue: &closed, Lines: []cart.Line{
				{Item: croissant(20_000, nil, true), Qty: 1, PriceSnapshot: 20_000},
				{Item: baguette(30_000), Qty: 1, PriceSnapshot: 30_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemVenueClosed},
		},
		"below the minimum order amount": {
			cart: cart.Cart{Venue: &bakery, Lines: []cart.Line{
				{Item: croissant(20_000, nil, true), Qty: 1, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemBelowMinimum},
			wantProblem: func(t *testing.T, p cart.Problem) {
				if p.Shortfall == nil || *p.Shortfall != 30_000 {
					t.Errorf("shortfall = %v, want 30000", p.Shortfall)
				}
			},
		},
		"the minimum is counted at the current prices": {
			cart: cart.Cart{Venue: &bakery, Lines: []cart.Line{
				{Item: croissant(60_000, nil, true), Qty: 1, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{cart.ProblemPriceChanged},
		},
		"every problem of a cart is reported": {
			cart: cart.Cart{Venue: &closed, Lines: []cart.Line{
				{Item: croissant(22_000, &one, false), Qty: 2, PriceSnapshot: 20_000},
			}},
			wantTypes: []cart.ProblemType{
				cart.ProblemVenueClosed,
				cart.ProblemItemUnavailable,
				cart.ProblemOutOfStock,
				cart.ProblemPriceChanged,
				cart.ProblemBelowMinimum,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tc.cart.Validate()

			if got.IsValid != tc.wantValid {
				t.Errorf("is valid = %v, want %v", got.IsValid, tc.wantValid)
			}

			types := make([]cart.ProblemType, 0, len(got.Problems))
			for _, p := range got.Problems {
				types = append(types, p.Type)

				if p.Message == "" {
					t.Errorf("problem %s carries no message", p.Type)
				}
			}

			if len(types) != len(tc.wantTypes) {
				t.Fatalf("problems = %v, want %v", types, tc.wantTypes)
			}

			for i, want := range tc.wantTypes {
				if types[i] != want {
					t.Fatalf("problems = %v, want %v", types, tc.wantTypes)
				}
			}

			if tc.wantProblem != nil {
				tc.wantProblem(t, got.Problems[0])
			}
		})
	}
}
