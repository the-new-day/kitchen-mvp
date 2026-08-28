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

// SyncMenu applies a full menu snapshot of a venue in one transaction:
// categories and items are matched by their external identifiers, and the
// items missing from the snapshot are taken off sale. Nothing is deleted:
// past orders refer to the items of the menu.
func (r *MenuRepo) SyncMenu(
	ctx context.Context, venueID uuid.UUID, snapshot catalog.MenuSnapshot,
) (catalog.MenuSyncResult, error) {
	result := catalog.MenuSyncResult{CategoriesTotal: len(snapshot.Categories)}

	err := r.db.InTx(ctx, func(ctx context.Context) error {
		categories, err := r.upsertCategories(ctx, venueID, snapshot.Categories)
		if err != nil {
			return err
		}

		created, updated, err := r.upsertItems(ctx, venueID, snapshot.Items, categories)
		if err != nil {
			return err
		}

		deactivated, err := r.deactivateMissingItems(ctx, venueID, snapshot.Items)
		if err != nil {
			return err
		}

		result.ItemsCreated, result.ItemsUpdated, result.ItemsDeactivated = created, updated, deactivated

		return nil
	})
	if err != nil {
		return catalog.MenuSyncResult{}, err
	}

	return result, nil
}

// upsertCategories writes the categories of the snapshot and returns the
// identifier of every one of them, the ones that already existed included.
func (r *MenuRepo) upsertCategories(
	ctx context.Context, venueID uuid.UUID, categories []catalog.CategoryRow,
) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(categories))
	if len(categories) == 0 {
		return ids, nil
	}

	externalIDs := make([]string, 0, len(categories))
	names := make([]string, 0, len(categories))
	positions := make([]int64, 0, len(categories))

	for _, c := range categories {
		externalIDs = append(externalIDs, c.ExternalID)
		names = append(names, c.Name)
		positions = append(positions, int64(c.Position))
	}

	const query = `
		INSERT INTO menu_categories (venue_id, external_id, name, position)
		SELECT $1, u.external_id, u.name, u.position
		FROM unnest($2::text[], $3::text[], $4::bigint[])
		     AS u(external_id, name, position)
		ON CONFLICT (venue_id, external_id) DO UPDATE
		SET name = EXCLUDED.name, position = EXCLUDED.position, updated_at = now()
		RETURNING external_id, id`

	rows, err := r.db.conn(ctx).Query(ctx, query, venueID, externalIDs, names, positions)
	if err != nil {
		return nil, fmt.Errorf("upsert menu categories: %w", err)
	}

	type categoryID struct {
		externalID string
		id         uuid.UUID
	}

	collected, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (categoryID, error) {
		var c categoryID
		return c, row.Scan(&c.externalID, &c.id)
	})
	if err != nil {
		return nil, fmt.Errorf("scan menu categories: %w", err)
	}

	for _, c := range collected {
		ids[c.externalID] = c.id
	}

	return ids, nil
}

// upsertItems writes the items of the snapshot and counts how many of them
// were new. A row the statement inserted has xmax = 0; a row it updated
// carries the transaction that replaced the previous version.
func (r *MenuRepo) upsertItems(
	ctx context.Context, venueID uuid.UUID, items []catalog.ItemRow, categories map[string]uuid.UUID,
) (created, updated int, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	categoryIDs := make([]uuid.UUID, 0, len(items))
	externalIDs := make([]string, 0, len(items))
	names := make([]string, 0, len(items))
	descriptions := make([]string, 0, len(items))
	prices := make([]int64, 0, len(items))
	positions := make([]int64, 0, len(items))
	available := make([]bool, 0, len(items))
	stock := make([]*int, 0, len(items))

	for _, i := range items {
		categoryID, ok := categories[i.CategoryExternalID]
		if !ok {
			return 0, 0, fmt.Errorf("item %s refers to unknown category %s", i.ExternalID, i.CategoryExternalID)
		}

		categoryIDs = append(categoryIDs, categoryID)
		externalIDs = append(externalIDs, i.ExternalID)
		names = append(names, i.Name)
		descriptions = append(descriptions, i.Description)
		prices = append(prices, i.Price)
		positions = append(positions, int64(i.Position))
		available = append(available, i.IsAvailable)
		stock = append(stock, i.StockQty)
	}

	const query = `
		INSERT INTO menu_items (venue_id, category_id, external_id, name, description,
		                        price, position, is_available, stock_qty)
		SELECT $1, u.category_id, u.external_id, u.name, u.description,
		       u.price, u.position, u.is_available, u.stock_qty
		FROM unnest($2::uuid[], $3::text[], $4::text[], $5::text[],
		            $6::bigint[], $7::bigint[], $8::boolean[], $9::bigint[])
		     AS u(category_id, external_id, name, description,
		          price, position, is_available, stock_qty)
		ON CONFLICT (venue_id, external_id) DO UPDATE
		SET category_id  = EXCLUDED.category_id,
		    name         = EXCLUDED.name,
		    description  = EXCLUDED.description,
		    price        = EXCLUDED.price,
		    position     = EXCLUDED.position,
		    is_available = EXCLUDED.is_available,
		    stock_qty    = EXCLUDED.stock_qty,
		    updated_at   = now()
		RETURNING xmax = 0`

	rows, err := r.db.conn(ctx).Query(ctx, query, venueID, categoryIDs, externalIDs, names,
		descriptions, prices, positions, available, stock)
	if err != nil {
		return 0, 0, fmt.Errorf("upsert menu items: %w", err)
	}

	inserted, err := pgx.CollectRows(rows, pgx.RowTo[bool])
	if err != nil {
		return 0, 0, fmt.Errorf("scan menu items: %w", err)
	}

	for _, isNew := range inserted {
		if isNew {
			created++

			continue
		}

		updated++
	}

	return created, updated, nil
}

// deactivateMissingItems takes off sale the items of a venue that the upload
// does not mention, and reports how many of them there were.
func (r *MenuRepo) deactivateMissingItems(
	ctx context.Context, venueID uuid.UUID, items []catalog.ItemRow,
) (int, error) {
	externalIDs := make([]string, 0, len(items))
	for _, i := range items {
		externalIDs = append(externalIDs, i.ExternalID)
	}

	const query = `
		UPDATE menu_items SET is_available = false, updated_at = now()
		WHERE venue_id = $1 AND is_available AND NOT (external_id = ANY($2::text[]))
		RETURNING external_id`

	rows, err := r.db.conn(ctx).Query(ctx, query, venueID, externalIDs)
	if err != nil {
		return 0, fmt.Errorf("deactivate missing menu items: %w", err)
	}

	deactivated, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("scan deactivated menu items: %w", err)
	}

	return len(deactivated), nil
}

// PatchItem changes the fields the patch carries and returns the item as it is
// afterwards. An item of another venue is reported as domain.ErrNotFound: the
// venue comes from the API key, not from the request.
func (r *MenuRepo) PatchItem(
	ctx context.Context, venueID uuid.UUID, externalID string, patch catalog.MenuItemPatch,
) (catalog.CategorizedMenuItem, error) {
	const query = `
		UPDATE menu_items i
		SET price        = COALESCE($3::bigint, i.price),
		    is_available = COALESCE($4::boolean, i.is_available),
		    stock_qty    = COALESCE($5::bigint, i.stock_qty),
		    updated_at   = now()
		FROM menu_categories c
		WHERE c.id = i.category_id AND i.venue_id = $1 AND i.external_id = $2
		RETURNING i.id, i.external_id, c.external_id, i.name, i.description,
		          i.price, i.position, i.is_available, i.stock_qty`

	var item catalog.CategorizedMenuItem

	err := r.db.conn(ctx).
		QueryRow(ctx, query, venueID, externalID, patch.Price, patch.IsAvailable, patch.StockQty).
		Scan(&item.ID, &item.ExternalID, &item.CategoryExternalID, &item.Name, &item.Description,
			&item.Price, &item.Position, &item.IsAvailable, &item.StockQty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.CategorizedMenuItem{}, domain.ErrNotFound
		}

		return catalog.CategorizedMenuItem{}, fmt.Errorf("patch menu item: %w", err)
	}

	return item, nil
}
