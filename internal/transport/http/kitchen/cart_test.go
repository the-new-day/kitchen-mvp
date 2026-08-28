package kitchen

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

var (
	cartVenueID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	cartItemID  = uuid.MustParse("0192f4c1-0000-7000-8000-000000000011")
	cartUserID  = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")

	openBakery = cart.Venue{
		ID:             cartVenueID,
		Name:           "Пекарня «Батон»",
		IsOpen:         true,
		MinOrderAmount: 50_000,
		DeliveryFee:    9_900,
	}
)

func TestToCart(t *testing.T) {
	t.Parallel()

	closedBakery := openBakery
	closedBakery.IsOpen = false

	filled := cart.Cart{Venue: &openBakery, Lines: []cart.Line{
		{
			Item: catalog.MenuItem{
				ID:          cartItemID,
				ExternalID:  "SKU-CROISSANT",
				Name:        "Круассан",
				Price:       22_000,
				IsAvailable: true,
			},
			Qty:           3,
			PriceSnapshot: 20_000,
		},
	}}

	cases := map[string]struct {
		cart           cart.Cart
		wantVenue      bool
		wantItemsTotal int64
		wantTotal      int64
		wantItemPrice  int64
		wantAvailable  bool
	}{
		"an empty cart carries no venue": {
			cart: cart.Cart{},
		},
		"a filled cart is priced at the prices the items were added at": {
			cart:           filled,
			wantVenue:      true,
			wantItemsTotal: 60_000,
			wantTotal:      69_900,
			wantItemPrice:  20_000,
			wantAvailable:  true,
		},
		"an item of a closed venue cannot be ordered": {
			cart:           cart.Cart{Venue: &closedBakery, Lines: filled.Lines},
			wantVenue:      true,
			wantItemsTotal: 60_000,
			wantTotal:      69_900,
			wantItemPrice:  20_000,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toCart(tc.cart)

			if (got.VenueId != nil) != tc.wantVenue {
				t.Errorf("venue id = %v, want present = %v", got.VenueId, tc.wantVenue)
			}

			if (got.MinOrderAmount != nil) != tc.wantVenue {
				t.Errorf("min order amount = %v, want present = %v", got.MinOrderAmount, tc.wantVenue)
			}

			if got.ItemsTotal != tc.wantItemsTotal || got.Total != tc.wantTotal {
				t.Errorf("totals = %d + delivery = %d, want %d and %d",
					got.ItemsTotal, got.Total, tc.wantItemsTotal, tc.wantTotal)
			}

			if len(tc.cart.Lines) == 0 {
				if len(got.Items) != 0 {
					t.Errorf("items = %d, want none", len(got.Items))
				}

				return
			}

			item := got.Items[0]
			if item.Price != tc.wantItemPrice || item.LineTotal != tc.wantItemPrice*3 {
				t.Errorf("item price = %d, line total = %d, want %d and %d",
					item.Price, item.LineTotal, tc.wantItemPrice, tc.wantItemPrice*3)
			}

			if item.IsAvailable != tc.wantAvailable {
				t.Errorf("is available = %v, want %v", item.IsAvailable, tc.wantAvailable)
			}
		})
	}
}

func TestToCartProblem(t *testing.T) {
	t.Parallel()

	shortfall := int64(30_000)

	cases := map[string]struct {
		problem      cart.Problem
		wantItemName bool
	}{
		"a problem of the whole cart names no item": {
			problem: cart.Problem{
				Type:      cart.ProblemBelowMinimum,
				Message:   "order is below the minimum",
				Shortfall: &shortfall,
			},
		},
		"a problem of one item names it": {
			problem: cart.Problem{
				Type:     cart.ProblemItemUnavailable,
				Message:  "Круассан is not on sale right now",
				ItemID:   &cartItemID,
				ItemName: "Круассан",
			},
			wantItemName: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toCartProblem(tc.problem)

			if string(got.Type) != string(tc.problem.Type) || got.Message != tc.problem.Message {
				t.Errorf("problem = %s %q, want %s %q",
					got.Type, got.Message, tc.problem.Type, tc.problem.Message)
			}

			if (got.ItemName != nil) != tc.wantItemName {
				t.Errorf("item name = %v, want present = %v", got.ItemName, tc.wantItemName)
			}

			if (got.Shortfall != nil) != (tc.problem.Shortfall != nil) {
				t.Errorf("shortfall = %v, want %v", got.Shortfall, tc.problem.Shortfall)
			}
		})
	}
}

func TestRequireUser(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		header  string
		wantErr bool
	}{
		"a customer identifier": {header: cartUserID.String()},
		"no header at all":      {wantErr: true},
		"not an identifier":     {header: "not-a-uuid", wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				got    uuid.UUID
				gotErr error
			)

			handler := withUser(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got, gotErr = requireUser(r.Context())
			}))

			request := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
			if tc.header != "" {
				request.Header.Set(userHeader, tc.header)
			}

			handler.ServeHTTP(httptest.NewRecorder(), request)

			if tc.wantErr {
				if !errors.Is(gotErr, domain.ErrInvalidArgument) {
					t.Fatalf("require user = %v, want an invalid argument", gotErr)
				}

				return
			}

			if gotErr != nil {
				t.Fatalf("require user = %v, want nil", gotErr)
			}

			if got != cartUserID {
				t.Errorf("user = %s, want %s", got, cartUserID)
			}
		})
	}
}
