// Command seed fills the platform database with the venues of the closed
// pilot: their profiles, API keys and the menus of the venues that have no
// service of their own. Running it twice changes nothing.
package main

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/platform/logger"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"
)

const serviceName = "seed"

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serviceName, err)

		return 1
	}

	log := logger.New(os.Stdout, cfg.LogLevel, serviceName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "starting", slog.String("env", cfg.Env))

	if err := seed(ctx, cfg.Postgres.DSN, log); err != nil {
		log.Error("stopped with error", slog.String("error", err.Error()))

		return 1
	}

	log.Info("stopped")

	return 0
}

func seed(ctx context.Context, dsn string, log *slog.Logger) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	for _, v := range venues {
		if err := seedVenue(ctx, tx, v); err != nil {
			return fmt.Errorf("seed venue %s: %w", v.slug, err)
		}

		log.InfoContext(ctx, "venue seeded",
			slog.String("slug", v.slug),
			slog.Int("categories", len(v.menu)),
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func seedVenue(ctx context.Context, tx pgx.Tx, v venue) error {
	const insertVenue = `
		INSERT INTO venues (id, slug, name, description, address, lat, lon,
		                    is_open, min_order_amount, delivery_fee, avg_cook_minutes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO NOTHING`

	if _, err := tx.Exec(ctx, insertVenue,
		v.id, v.slug, v.name, v.description, v.address, v.lat, v.lon,
		v.isOpen, v.minOrderAmount, v.deliveryFee, v.avgCookMinutes,
	); err != nil {
		return fmt.Errorf("insert venue: %w", err)
	}

	const linkCuisines = `
		INSERT INTO venue_cuisines (venue_id, cuisine_id)
		SELECT $1, id FROM cuisines WHERE slug = ANY($2)
		ON CONFLICT DO NOTHING`

	if _, err := tx.Exec(ctx, linkCuisines, v.id, v.cuisines); err != nil {
		return fmt.Errorf("link cuisines: %w", err)
	}

	const insertKey = `
		INSERT INTO venue_api_keys (venue_id, key_hash)
		VALUES ($1, $2)
		ON CONFLICT (key_hash) DO NOTHING`

	keyHash := sha256.Sum256([]byte(v.apiKey))
	if _, err := tx.Exec(ctx, insertKey, v.id, keyHash[:]); err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}

	for _, c := range v.menu {
		if err := seedCategory(ctx, tx, v.id, c); err != nil {
			return fmt.Errorf("seed category %s: %w", c.externalID, err)
		}
	}

	return nil
}

func seedCategory(ctx context.Context, tx pgx.Tx, venueID string, c category) error {
	const insertCategory = `
		INSERT INTO menu_categories (venue_id, external_id, name, position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (venue_id, external_id) DO NOTHING`

	if _, err := tx.Exec(ctx, insertCategory, venueID, c.externalID, c.name, c.position); err != nil {
		return fmt.Errorf("insert category: %w", err)
	}

	const selectCategoryID = `
		SELECT id FROM menu_categories WHERE venue_id = $1 AND external_id = $2`

	var categoryID string
	if err := tx.QueryRow(ctx, selectCategoryID, venueID, c.externalID).Scan(&categoryID); err != nil {
		return fmt.Errorf("read category id: %w", err)
	}

	const insertItem = `
		INSERT INTO menu_items (venue_id, category_id, external_id, name, description, price, stock_qty)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (venue_id, external_id) DO NOTHING`

	for _, i := range c.items {
		if _, err := tx.Exec(ctx, insertItem,
			venueID, categoryID, i.externalID, i.name, i.description, i.price, i.stockQty,
		); err != nil {
			return fmt.Errorf("insert item %s: %w", i.externalID, err)
		}
	}

	return nil
}
