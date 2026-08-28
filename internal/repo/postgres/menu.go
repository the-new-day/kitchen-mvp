package postgres

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MenuRepo reads the menus of the catalogue.
type MenuRepo struct {
	db *DB
}

// NewMenuRepo returns a repository over db.
func NewMenuRepo(db *DB) *MenuRepo {
	return &MenuRepo{db: db}
}

// menuRow is one row of the venue-categories-items join: everything to the
// right of the venue is null for a venue whose menu is still empty.
type menuRow struct {
	venueIsOpen        bool
	categoryID         *uuid.UUID
	categoryExternalID *string
	categoryName       *string
	categoryPosition   *int
	itemID             *uuid.UUID
	itemExternalID     *string
	itemName           *string
	itemDescription    *string
	itemPrice          *int64
	itemPosition       *int
	itemIsAvailable    *bool
	itemStockQty       *int
}

// VenueMenu returns the whole menu of a venue in one query, together with the
// state of its shift. A venue that does not exist or is not active is
// reported as domain.ErrNotFound; a venue without a menu yields no categories.
func (r *MenuRepo) VenueMenu(ctx context.Context, venueID uuid.UUID) (catalog.Menu, error) {
	const query = `
		SELECT v.is_open,
		       c.id, c.external_id, c.name, c.position,
		       i.id, i.external_id, i.name, i.description, i.price, i.position,
		       i.is_available, i.stock_qty
		FROM venues v
		LEFT JOIN menu_categories c ON c.venue_id = v.id
		LEFT JOIN menu_items i ON i.category_id = c.id
		WHERE v.id = $1 AND v.is_active
		ORDER BY c.position, c.name, c.id, i.position, i.name`

	rows, err := r.db.conn(ctx).Query(ctx, query, venueID)
	if err != nil {
		return catalog.Menu{}, fmt.Errorf("query venue menu: %w", err)
	}

	collected, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (menuRow, error) {
		var m menuRow

		return m, row.Scan(
			&m.venueIsOpen,
			&m.categoryID, &m.categoryExternalID, &m.categoryName, &m.categoryPosition,
			&m.itemID, &m.itemExternalID, &m.itemName, &m.itemDescription,
			&m.itemPrice, &m.itemPosition, &m.itemIsAvailable, &m.itemStockQty,
		)
	})
	if err != nil {
		return catalog.Menu{}, fmt.Errorf("scan venue menu: %w", err)
	}

	if len(collected) == 0 {
		return catalog.Menu{}, domain.ErrNotFound
	}

	return assembleMenu(venueID, collected), nil
}

func assembleMenu(venueID uuid.UUID, rows []menuRow) catalog.Menu {
	menu := catalog.Menu{VenueID: venueID, VenueIsOpen: rows[0].venueIsOpen}

	for _, row := range rows {
		if row.categoryID == nil {
			continue
		}

		if len(menu.Categories) == 0 || menu.Categories[len(menu.Categories)-1].ID != *row.categoryID {
			menu.Categories = append(menu.Categories, catalog.MenuCategory{
				ID:         *row.categoryID,
				ExternalID: *row.categoryExternalID,
				Name:       *row.categoryName,
				Position:   *row.categoryPosition,
			})
		}

		if row.itemID == nil {
			continue
		}

		category := &menu.Categories[len(menu.Categories)-1]
		category.Items = append(category.Items, catalog.MenuItem{
			ID:          *row.itemID,
			ExternalID:  *row.itemExternalID,
			Name:        *row.itemName,
			Description: *row.itemDescription,
			Price:       *row.itemPrice,
			Position:    *row.itemPosition,
			IsAvailable: *row.itemIsAvailable,
			StockQty:    row.itemStockQty,
		})
	}

	return menu
}

// MenuItem returns one item of the menu of a venue. An item that does not
// exist, belongs to another venue or to a venue that is not active is reported
// as domain.ErrNotFound.
func (r *MenuRepo) MenuItem(ctx context.Context, venueID, itemID uuid.UUID) (catalog.MenuItem, error) {
	const query = `
		SELECT i.id, i.external_id, i.name, i.description, i.price, i.position,
		       i.is_available, i.stock_qty
		FROM menu_items i
		JOIN venues v ON v.id = i.venue_id
		WHERE i.id = $1 AND i.venue_id = $2 AND v.is_active`

	rows, err := r.db.conn(ctx).Query(ctx, query, itemID, venueID)
	if err != nil {
		return catalog.MenuItem{}, fmt.Errorf("query menu item: %w", err)
	}

	item, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (catalog.MenuItem, error) {
		var i catalog.MenuItem

		return i, row.Scan(&i.ID, &i.ExternalID, &i.Name, &i.Description,
			&i.Price, &i.Position, &i.IsAvailable, &i.StockQty)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.MenuItem{}, domain.ErrNotFound
		}

		return catalog.MenuItem{}, fmt.Errorf("scan menu item: %w", err)
	}

	return item, nil
}
