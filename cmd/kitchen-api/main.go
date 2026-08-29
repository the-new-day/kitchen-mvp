// Command kitchen-api serves the customer and partner HTTP API of the platform.
package main

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/consumer"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/logger"
	"avito-kitchen/internal/repo/postgres"
	"avito-kitchen/internal/transport/http/kitchen"
	"avito-kitchen/internal/transport/http/partner"
	"avito-kitchen/internal/transport/sse"
	cartusecase "avito-kitchen/internal/usecase/cart"
	"avito-kitchen/internal/usecase/catalog"
	orderusecase "avito-kitchen/internal/usecase/order"
	partnerusecase "avito-kitchen/internal/usecase/partner"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/google/uuid"
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
		Order: orderusecase.New(db, orders, carts, menus, messages, orderusecase.Topics{
			Orders: cfg.Kafka.OrdersTopic,
			Status: cfg.Kafka.StatusTopic,
		}),
	}
	partnerService := partnerusecase.New(venues, menus)

	router := httpx.NewRouter(log)
	hub := sse.NewHub(cfg.SSE.Buffer, log)

	idempotent := kitchen.Idempotency{Store: keys, Tx: db, TTL: cfg.Orders.IdempotencyTTL}
	streams := kitchen.Streams{Hub: hub, Heartbeat: cfg.SSE.Heartbeat}

	if err := kitchen.Mount(router, services, idempotent, streams, log); err != nil {
		return err
	}

	partner.Mount(router, partnerService, services.Order, cfg.Kafka.OrdersTopic, log)

	var wg sync.WaitGroup

	wg.Go(func() {
		follow(ctx, cfg, hub, log)
	})

	defer wg.Wait()

	return httpx.Serve(ctx, cfg.HTTP, router, log)
}

// follow reads the statuses of the orders into the hub. Every instance reads
// the topic under a group of its own, so that each of them sees every
// transition and can find the customer watching it among its own subscribers.
func follow(ctx context.Context, cfg config.Config, hub *sse.Hub, log *slog.Logger) {
	broker.Serve(ctx, broker.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.StatusTopic,
		GroupID: streamGroup(),
		// What happened before this instance started belongs to nobody it is
		// serving: a customer connecting now is answered from the database.
		FromLatest: true,
	}, consumer.NewStatus(hub, log).Handle, log)
}

// streamGroup is the consumer group this instance reads the statuses under. It
// has to be unique: a group shared with another instance would turn the
// broadcast into a queue, and a transition would reach one instance of two.
func streamGroup() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = uuid.NewString()
	}

	return "sse-" + host
}
