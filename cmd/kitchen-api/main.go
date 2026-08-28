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
	carts := postgres.NewCartRepo(db)

	catalogService := catalog.New(venues, menus)
	cartService := cartusecase.New(db, carts, menus)
	partnerService := partnerusecase.New(venues, menus)

	router := httpx.NewRouter(log)
	kitchen.Mount(router, catalogService, cartService, log)
	partner.Mount(router, partnerService, cfg.Kafka.OrdersTopic, log)

	return httpx.Serve(ctx, cfg.HTTP, router, log)
}
