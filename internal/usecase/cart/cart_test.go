package cart_test

import (
	"avito-kitchen/internal/domain"
	domaincart "avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"avito-kitchen/internal/usecase/cart"
	"avito-kitchen/internal/usecase/cart/mocks"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

var (
	userID      = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")
	bakeryID    = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	pizzaID     = uuid.MustParse("0192f4c1-0000-7000-8000-000000000002")
	croissantID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000011")

	croissant = catalog.MenuItem{
		ID:          croissantID,
		ExternalID:  "SKU-CROISSANT",
		Name:        "Круассан",
		Price:       20_000,
		IsAvailable: true,
	}

	bakeryCart = domaincart.Cart{
		Venue: &domaincart.Venue{ID: bakeryID, Name: "Пекарня «Батон»", IsOpen: true},
		Lines: []domaincart.Line{{Item: croissant, Qty: 1, PriceSnapshot: 20_000}},
	}

	errRepo = errors.New("connection refused")
)

// repositories is the set of mocks one case of the table is served by.
type repositories struct {
	carts *mocks.MockRepository
	menus *mocks.MockMenuRepository
}

func newService(t *testing.T, setup func(repositories)) *cart.Service {
	t.Helper()

	repos := repositories{carts: mocks.NewMockRepository(t), menus: mocks.NewMockMenuRepository(t)}
	if setup != nil {
		setup(repos)
	}

	tx := mocks.NewMockTransactor(t)
	tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Maybe()

	return cart.New(tx, repos.carts, repos.menus)
}

func TestServiceSetItem(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		qty     int
		venueID uuid.UUID
		setup   func(repositories)
		wantErr error
	}{
		"a new item is stored at the price of the menu": {
			qty:     2,
			venueID: bakeryID,
			setup: func(r repositories) {
				r.menus.EXPECT().MenuItem(mock.Anything, bakeryID, croissantID).Return(croissant, nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(domaincart.Cart{}, nil).Once()
				r.carts.EXPECT().SetItem(mock.Anything, userID, bakeryID, croissantID, 2, int64(20_000)).
					Return(nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(bakeryCart, nil).Once()
			},
		},
		"zero quantity takes the item out": {
			qty:     0,
			venueID: bakeryID,
			setup: func(r repositories) {
				r.menus.EXPECT().MenuItem(mock.Anything, bakeryID, croissantID).Return(croissant, nil).Once()
				r.carts.EXPECT().RemoveItem(mock.Anything, userID, croissantID).Return(true, nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(domaincart.Cart{}, nil).Once()
			},
		},
		"an item of another venue conflicts with the cart": {
			qty:     1,
			venueID: pizzaID,
			setup: func(r repositories) {
				r.menus.EXPECT().MenuItem(mock.Anything, pizzaID, croissantID).Return(croissant, nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(bakeryCart, nil).Once()
			},
			wantErr: domain.ErrConflict,
		},
		"an unknown item is not found": {
			qty:     1,
			venueID: bakeryID,
			setup: func(r repositories) {
				r.menus.EXPECT().MenuItem(mock.Anything, bakeryID, croissantID).
					Return(catalog.MenuItem{}, domain.ErrNotFound).Once()
			},
			wantErr: domain.ErrNotFound,
		},
		"a quantity beyond the limit is refused before the repositories": {
			qty:     101,
			venueID: bakeryID,
			wantErr: domain.ErrInvalidArgument,
		},
		"a negative quantity is refused": {
			qty:     -1,
			venueID: bakeryID,
			wantErr: domain.ErrInvalidArgument,
		},
		"a failing repository is reported": {
			qty:     1,
			venueID: bakeryID,
			setup: func(r repositories) {
				r.menus.EXPECT().MenuItem(mock.Anything, bakeryID, croissantID).Return(croissant, nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(domaincart.Cart{}, errRepo).Once()
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, tc.setup)

			_, err := service.SetItem(context.Background(), userID, tc.venueID, croissantID, tc.qty)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("set item = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestServiceRemoveItem(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup   func(repositories)
		wantErr error
	}{
		"an item of the cart is removed": {
			setup: func(r repositories) {
				r.carts.EXPECT().RemoveItem(mock.Anything, userID, croissantID).Return(true, nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(domaincart.Cart{}, nil).Once()
			},
		},
		"an item the cart does not hold is not found": {
			setup: func(r repositories) {
				r.carts.EXPECT().RemoveItem(mock.Anything, userID, croissantID).Return(false, nil).Once()
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, tc.setup)

			_, err := service.RemoveItem(context.Background(), userID, croissantID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("remove item = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestServiceValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		stored    domaincart.Cart
		wantValid bool
		wantErr   error
	}{
		"a cart of an open venue with no minimum": {
			stored:    bakeryCart,
			wantValid: true,
		},
		"an empty cart cannot be ordered": {
			stored: domaincart.Cart{},
		},
		"a failing repository is reported": {
			stored:  domaincart.Cart{},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, func(r repositories) {
				r.carts.EXPECT().Cart(mock.Anything, userID).Return(tc.stored, tc.wantErr).Once()
			})

			validation, err := service.Validate(context.Background(), userID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validate = %v, want %v", err, tc.wantErr)
			}

			if err == nil && validation.IsValid != tc.wantValid {
				t.Errorf("is valid = %v, want %v", validation.IsValid, tc.wantValid)
			}
		})
	}
}

func TestServiceClear(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		repoErr error
		wantErr error
	}{
		"an emptied cart":                  {},
		"a failing repository is reported": {repoErr: errRepo, wantErr: errRepo},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, func(r repositories) {
				r.carts.EXPECT().Clear(mock.Anything, userID).Return(tc.repoErr).Once()
			})

			if err := service.Clear(context.Background(), userID); !errors.Is(err, tc.wantErr) {
				t.Fatalf("clear = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
