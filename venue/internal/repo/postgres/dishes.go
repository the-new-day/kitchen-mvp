package postgres

import (
	"avito-kitchen/venue/internal/kitchen"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// dishColumns is what a dish is read by.
const dishColumns = `
	sku, name, description, price, stock, is_available,
	category_external_id, category_name, category_position, position`

// DishRepo keeps the nomenclature of the venue.
type DishRepo struct {
	db *DB
}

// NewDishRepo returns a repository over db.
func NewDishRepo(db *DB) *DishRepo {
	return &DishRepo{db: db}
}

// Ensure writes the dishes the venue sells, leaving the ones it already has as
// they are: a restart must not undo what the shift has done to them.
func (r *DishRepo) Ensure(ctx context.Context, dishes []kitchen.Dish) error {
	if len(dishes) == 0 {
		return nil
	}

	skus := make([]string, 0, len(dishes))
	names := make([]string, 0, len(dishes))
	descriptions := make([]string, 0, len(dishes))
	prices := make([]int64, 0, len(dishes))
	stock := make([]*int, 0, len(dishes))
	categoryIDs := make([]string, 0, len(dishes))
	categoryNames := make([]string, 0, len(dishes))
	categoryPositions := make([]int64, 0, len(dishes))
	positions := make([]int64, 0, len(dishes))

	for _, dish := range dishes {
		skus = append(skus, dish.SKU)
		names = append(names, dish.Name)
		descriptions = append(descriptions, dish.Description)
		prices = append(prices, dish.Price)
		stock = append(stock, dish.Stock)
		categoryIDs = append(categoryIDs, dish.Category.ExternalID)
		categoryNames = append(categoryNames, dish.Category.Name)
		categoryPositions = append(categoryPositions, int64(dish.Category.Position))
		positions = append(positions, int64(dish.Position))
	}

	const query = `
		INSERT INTO dishes (sku, name, description, price, stock,
		                    category_external_id, category_name, category_position, position)
		SELECT u.sku, u.name, u.description, u.price, u.stock,
		       u.category_external_id, u.category_name, u.category_position, u.position
		FROM unnest($1::text[], $2::text[], $3::text[], $4::bigint[], $5::bigint[],
		            $6::text[], $7::text[], $8::bigint[], $9::bigint[])
		     AS u(sku, name, description, price, stock,
		          category_external_id, category_name, category_position, position)
		ON CONFLICT (sku) DO NOTHING`

	_, err := r.db.conn(ctx).Exec(ctx, query,
		skus, names, descriptions, prices, stock,
		categoryIDs, categoryNames, categoryPositions, positions,
	)
	if err != nil {
		return fmt.Errorf("insert dishes: %w", err)
	}

	return nil
}

// List returns the whole nomenclature in the order the venue shows it.
func (r *DishRepo) List(ctx context.Context) ([]kitchen.Dish, error) {
	query := `SELECT ` + dishColumns + `
		FROM dishes
		ORDER BY category_position, position, sku`

	rows, err := r.db.conn(ctx).Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select dishes: %w", err)
	}

	dishes, err := pgx.CollectRows(rows, scanDish)
	if err != nil {
		return nil, fmt.Errorf("scan dishes: %w", err)
	}

	return dishes, nil
}

// Take writes off what an order spends. A dish without a stock is unlimited
// and is left alone; one that runs short is taken down to zero rather than
// below it.
func (r *DishRepo) Take(ctx context.Context, sku string, qty int) (kitchen.Dish, error) {
	query := `
		UPDATE dishes
		SET stock = GREATEST(stock - $2, 0), updated_at = now()
		WHERE sku = $1
		RETURNING ` + dishColumns

	return r.one(ctx, query, sku, qty)
}

// Give returns to the stock what an order no longer spends.
func (r *DishRepo) Give(ctx context.Context, sku string, qty int) (kitchen.Dish, error) {
	query := `
		UPDATE dishes
		SET stock = stock + $2, updated_at = now()
		WHERE sku = $1
		RETURNING ` + dishColumns

	return r.one(ctx, query, sku, qty)
}

// SetAvailable puts a dish on or off the menu.
func (r *DishRepo) SetAvailable(ctx context.Context, sku string, available bool) (kitchen.Dish, error) {
	query := `
		UPDATE dishes
		SET is_available = $2, updated_at = now()
		WHERE sku = $1
		RETURNING ` + dishColumns

	return r.one(ctx, query, sku, available)
}

func (r *DishRepo) one(ctx context.Context, query string, args ...any) (kitchen.Dish, error) {
	rows, err := r.db.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return kitchen.Dish{}, fmt.Errorf("update dish: %w", err)
	}

	dish, err := pgx.CollectExactlyOneRow(rows, scanDish)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return kitchen.Dish{}, fmt.Errorf("dish %v: %w", args[0], kitchen.ErrNotFound)
		}

		return kitchen.Dish{}, fmt.Errorf("scan dish: %w", err)
	}

	return dish, nil
}

func scanDish(row pgx.CollectableRow) (kitchen.Dish, error) {
	var dish kitchen.Dish

	err := row.Scan(
		&dish.SKU, &dish.Name, &dish.Description, &dish.Price, &dish.Stock, &dish.IsAvailable,
		&dish.Category.ExternalID, &dish.Category.Name, &dish.Category.Position, &dish.Position,
	)

	return dish, err
}
