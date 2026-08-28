// Command kitchen-api serves the customer and partner HTTP API of the platform.
package main

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/logger"
	"avito-kitchen/internal/repo/postgres"
	"avito-kitchen/internal/transport/http/kitchen"
	"avito-kitchen/internal/transport/http/partner"
	cartusecase "avito-kitchen/internal/usecase/cart"
	"avito-kitchen/internal/usecase/catalog"
	orderusecase "avito-kitchen/internal/usecase/order"
	partnerusecase "avito-kitchen/internal/usecase/partner"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const serviceName = "kitchen-api"

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

	if err := serve(ctx, cfg, log); err != nil {
		log.Error("stopped with error", slog.String("error", err.Error()))

		return 1
	}

	log.Info("stopped")

	return 0
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	db, err := postgres.New(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer db.Close()

	venues, menus := postgres.NewVenueRepo(db), postgres.NewMenuRepo(db)
	carts, orders := postgres.NewCartRepo(db), postgres.NewOrderRepo(db)
	messages, keys := postgres.NewOutboxRepo(db), postgres.NewIdempotencyRepo(db)

	services := kitchen.Services{
		Catalog: catalog.New(venues, menus),
		Cart:    cartusecase.New(db, carts, menus),
		Order:   orderusecase.New(db, orders, carts, menus, messages, cfg.Kafka.OrdersTopic),
	}
	partnerService := partnerusecase.New(venues, menus)

	router := httpx.NewRouter(log)

	idempotent := kitchen.Idempotency{Store: keys, Tx: db, TTL: cfg.Orders.IdempotencyTTL}
	if err := kitchen.Mount(router, services, idempotent, log); err != nil {
		return err
	}

	partner.Mount(router, partnerService, cfg.Kafka.OrdersTopic, log)

	return httpx.Serve(ctx, cfg.HTTP, router, log)
}
