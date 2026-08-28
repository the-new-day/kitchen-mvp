// Package cart serves the cart of a customer: the items in it, their
// quantities and the check that decides whether it can become an order.
package cart

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// maxQty is the largest quantity of one item a cart accepts.
const maxQty = 100

// Transactor runs a unit of work in one database transaction.
// The repositories called inside fn join it through the context.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Repository stores the carts of the customers.
type Repository interface {
	Cart(ctx context.Context, userID uuid.UUID) (cart.Cart, error)
	SetItem(ctx context.Context, userID, venueID, itemID uuid.UUID, qty int, price int64) error
	RemoveItem(ctx context.Context, userID, itemID uuid.UUID) (bool, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// MenuRepository reads the menu items a cart refers to.
type MenuRepository interface {
	MenuItem(ctx context.Context, venueID, itemID uuid.UUID) (catalog.MenuItem, error)
}

// Service is the cart use case.
type Service struct {
	tx    Transactor
	carts Repository
	menus MenuRepository
}

// New returns a service working through the given repositories.
func New(tx Transactor, carts Repository, menus MenuRepository) *Service {
	return &Service{tx: tx, carts: carts, menus: menus}
}

// Cart returns the cart of a customer.
// A customer without a cart has an empty one.
func (s *Service) Cart(ctx context.Context, userID uuid.UUID) (cart.Cart, error) {
	current, err := s.carts.Cart(ctx, userID)
	if err != nil {
		return cart.Cart{}, fmt.Errorf("get cart of user %s: %w", userID, err)
	}

	return current, nil
}

// SetItem makes the cart hold exactly qty of an item; qty of zero takes the
// item out. The item is looked up in the menu of venueID, so an item of
// another venue is unknown rather than forbidden.
// The price the item is stored at is the one it has now.
func (s *Service) SetItem(
	ctx context.Context, userID, venueID, itemID uuid.UUID, qty int,
) (cart.Cart, error) {
	if qty < 0 || qty > maxQty {
		return cart.Cart{}, domain.InvalidArgumentf("qty must be between 0 and %d", maxQty)
	}

	var updated cart.Cart

	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		item, err := s.menus.MenuItem(ctx, venueID, itemID)
		if err != nil {
			return fmt.Errorf("get item %s of venue %s: %w", itemID, venueID, err)
		}

		if err = s.applyItem(ctx, userID, venueID, item, qty); err != nil {
			return err
		}

		updated, err = s.carts.Cart(ctx, userID)
		if err != nil {
			return fmt.Errorf("get cart of user %s: %w", userID, err)
		}

		return nil
	})
	if err != nil {
		return cart.Cart{}, err
	}

	return updated, nil
}

// applyItem writes the wanted quantity of one item. Taking an item out never
// conflicts with the venue of the cart: it leaves the cart with fewer items,
// not with items of two venues.
func (s *Service) applyItem(
	ctx context.Context, userID, venueID uuid.UUID, item catalog.MenuItem, qty int,
) error {
	if qty == 0 {
		if _, err := s.carts.RemoveItem(ctx, userID, item.ID); err != nil {
			return fmt.Errorf("remove item %s from cart of user %s: %w", item.ID, userID, err)
		}

		return nil
	}

	current, err := s.carts.Cart(ctx, userID)
	if err != nil {
		return fmt.Errorf("get cart of user %s: %w", userID, err)
	}

	if err := current.CheckVenue(venueID); err != nil {
		return err
	}

	if err := s.carts.SetItem(ctx, userID, venueID, item.ID, qty, item.Price); err != nil {
		return fmt.Errorf("set item %s in cart of user %s: %w", item.ID, userID, err)
	}

	return nil
}

// RemoveItem takes an item out of the cart.
// An item the cart does not hold is reported as domain.ErrNotFound.
func (s *Service) RemoveItem(ctx context.Context, userID, itemID uuid.UUID) (cart.Cart, error) {
	var updated cart.Cart

	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		removed, err := s.carts.RemoveItem(ctx, userID, itemID)
		if err != nil {
			return fmt.Errorf("remove item %s from cart of user %s: %w", itemID, userID, err)
		}

		if !removed {
			return domain.ErrNotFound
		}

		updated, err = s.carts.Cart(ctx, userID)
		if err != nil {
			return fmt.Errorf("get cart of user %s: %w", userID, err)
		}

		return nil
	})
	if err != nil {
		return cart.Cart{}, err
	}

	return updated, nil
}

// Clear empties the cart. Clearing an empty cart changes nothing.
func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	if err := s.carts.Clear(ctx, userID); err != nil {
		return fmt.Errorf("clear cart of user %s: %w", userID, err)
	}

	return nil
}

// Validate compares the cart with the current state of the menu and the venue
// and reports everything that stops it from becoming an order.
func (s *Service) Validate(ctx context.Context, userID uuid.UUID) (cart.Validation, error) {
	current, err := s.carts.Cart(ctx, userID)
	if err != nil {
		return cart.Validation{}, fmt.Errorf("get cart of user %s: %w", userID, err)
	}

	return current.Validate(), nil
}
