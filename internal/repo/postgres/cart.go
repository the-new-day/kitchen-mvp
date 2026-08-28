package postgres

import (
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CartRepo stores the carts of the customers.
type CartRepo struct {
	db *DB
}

// NewCartRepo returns a repository over db.
func NewCartRepo(db *DB) *CartRepo {
	return &CartRepo{db: db}
}

// cartRow is one row of the cart-venue-items join: everything to the right of
// the venue is null for a cart with no items in it.
type cartRow struct {
	venueID         *uuid.UUID
	venueName       *string
	venueIsOpen     *bool
	minOrderAmount  *int64
	deliveryFee     *int64
	itemID          *uuid.UUID
	itemExternalID  *string
	itemName        *string
	itemPrice       *int64
	itemIsAvailable *bool
	itemStockQty    *int
	qty             *int
	priceSnapshot   *int64
}

// Cart returns the cart of a customer together with the current state of the
// menu items in it. A customer who has never put anything into a cart has an
// empty one rather than none.
func (r *CartRepo) Cart(ctx context.Context, userID uuid.UUID) (cart.Cart, error) {
	const query = `
		SELECT v.id, v.name, v.is_open, v.min_order_amount, v.delivery_fee,
		       i.id, i.external_id, i.name, i.price, i.is_available, i.stock_qty,
		       ci.qty, ci.price_snapshot
		FROM carts c
		LEFT JOIN venues v ON v.id = c.venue_id
		LEFT JOIN cart_items ci ON ci.cart_id = c.id
		LEFT JOIN menu_items i ON i.id = ci.menu_item_id
		WHERE c.user_id = $1
		ORDER BY ci.created_at, i.name`

	rows, err := r.db.conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return cart.Cart{}, fmt.Errorf("query cart: %w", err)
	}

	collected, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cartRow, error) {
		var c cartRow

		return c, row.Scan(
			&c.venueID, &c.venueName, &c.venueIsOpen, &c.minOrderAmount, &c.deliveryFee,
			&c.itemID, &c.itemExternalID, &c.itemName, &c.itemPrice,
			&c.itemIsAvailable, &c.itemStockQty, &c.qty, &c.priceSnapshot,
		)
	})
	if err != nil {
		return cart.Cart{}, fmt.Errorf("scan cart: %w", err)
	}

	return assembleCart(collected), nil
}

func assembleCart(rows []cartRow) cart.Cart {
	var assembled cart.Cart

	for _, row := range rows {
		if assembled.Venue == nil && row.venueID != nil {
			assembled.Venue = &cart.Venue{
				ID:             *row.venueID,
				Name:           *row.venueName,
				IsOpen:         *row.venueIsOpen,
				MinOrderAmount: *row.minOrderAmount,
				DeliveryFee:    *row.deliveryFee,
			}
		}

		if row.itemID == nil {
			continue
		}

		assembled.Lines = append(assembled.Lines, cart.Line{
			Item: catalog.MenuItem{
				ID:          *row.itemID,
				ExternalID:  *row.itemExternalID,
				Name:        *row.itemName,
				Price:       *row.itemPrice,
				IsAvailable: *row.itemIsAvailable,
				StockQty:    row.itemStockQty,
			},
			Qty:           *row.qty,
			PriceSnapshot: *row.priceSnapshot,
		})
	}

	return assembled
}

// SetItem makes the cart of a customer hold qty of an item, creating the cart
// if there is none. The price of an item already in the cart is left as it
// was: it is the price the customer saw, and the cart check compares the
// current one with it.
func (r *CartRepo) SetItem(
	ctx context.Context, userID, venueID, itemID uuid.UUID, qty int, price int64,
) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		cartID, err := r.upsertCart(ctx, userID, &venueID)
		if err != nil {
			return err
		}

		const query = `
			INSERT INTO cart_items (cart_id, menu_item_id, qty, price_snapshot)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (cart_id, menu_item_id) DO UPDATE
			SET qty = EXCLUDED.qty, updated_at = now()`

		if _, err := r.db.conn(ctx).Exec(ctx, query, cartID, itemID, qty, price); err != nil {
			return fmt.Errorf("upsert cart item: %w", err)
		}

		return nil
	})
}

// RemoveItem takes an item out of the cart and reports whether there was one.
func (r *CartRepo) RemoveItem(ctx context.Context, userID, itemID uuid.UUID) (bool, error) {
	var removed bool

	err := r.db.InTx(ctx, func(ctx context.Context) error {
		const query = `
			DELETE FROM cart_items ci
			USING carts c
			WHERE c.id = ci.cart_id AND c.user_id = $1 AND ci.menu_item_id = $2`

		tag, err := r.db.conn(ctx).Exec(ctx, query, userID, itemID)
		if err != nil {
			return fmt.Errorf("delete cart item: %w", err)
		}

		removed = tag.RowsAffected() > 0
		if !removed {
			return nil
		}

		return r.forgetEmptyVenue(ctx, userID)
	})
	if err != nil {
		return false, err
	}

	return removed, nil
}

// Clear empties the cart of a customer.
// A customer without a cart keeps having none.
func (r *CartRepo) Clear(ctx context.Context, userID uuid.UUID) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		const query = `
			DELETE FROM cart_items ci
			USING carts c
			WHERE c.id = ci.cart_id AND c.user_id = $1`

		if _, err := r.db.conn(ctx).Exec(ctx, query, userID); err != nil {
			return fmt.Errorf("delete cart items: %w", err)
		}

		return r.forgetEmptyVenue(ctx, userID)
	})
}

// upsertCart returns the cart of a customer,
// creating it with the given venue if there is none.
func (r *CartRepo) upsertCart(ctx context.Context, userID uuid.UUID, venueID *uuid.UUID) (uuid.UUID, error) {
	const query = `
		INSERT INTO carts (user_id, venue_id) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET venue_id = COALESCE(carts.venue_id, EXCLUDED.venue_id), updated_at = now()
		RETURNING id`

	var cartID uuid.UUID
	if err := r.db.conn(ctx).QueryRow(ctx, query, userID, venueID).Scan(&cartID); err != nil {
		return uuid.Nil, fmt.Errorf("upsert cart: %w", err)
	}

	return cartID, nil
}

// forgetEmptyVenue releases the venue of a cart that has no items left, so
// that the next item may come from any venue.
func (r *CartRepo) forgetEmptyVenue(ctx context.Context, userID uuid.UUID) error {
	const query = `
		UPDATE carts c SET venue_id = NULL, updated_at = now()
		WHERE c.user_id = $1 AND c.venue_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM cart_items ci WHERE ci.cart_id = c.id)`

	if _, err := r.db.conn(ctx).Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("release cart venue: %w", err)
	}

	return nil
}
