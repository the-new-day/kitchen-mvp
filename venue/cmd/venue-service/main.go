// Command venue-service is the demo venue that ships with the platform. It
// owns its own data and reaches the platform the way any restaurant would:
// through the generated partner client and its topic in Kafka. It serves no
// API of its own -- only /healthz, so that compose can see it is alive.
package main

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/logger"
	"avito-kitchen/venue/internal/autopilot"
	"avito-kitchen/venue/internal/bootstrap"
	"avito-kitchen/venue/internal/config"
	"avito-kitchen/venue/internal/consumer"
	"avito-kitchen/venue/internal/partner"
	"avito-kitchen/venue/internal/repo/postgres"
	kitchenusecase "avito-kitchen/venue/internal/usecase/kitchen"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const serviceName = "venue-service"

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

	platform, err := partner.New(cfg.Partner)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		work(ctx, cfg, db, platform, log)
	}()

	defer wg.Wait()

	return httpx.Serve(ctx, cfg.HTTP, httpx.NewRouter(log), log)
}

// work brings the venue online and then keeps it running: reading its orders
// out of the topic and moving them through the kitchen. It starts with the
// bootstrap because everything after it needs the profile of the venue: the
// identity it consumes under and the time it promises its orders in.
func work(
	ctx context.Context,
	cfg config.Config,
	db *postgres.DB,
	platform *partner.Client,
	log *slog.Logger,
) {
	dishes, orders := postgres.NewDishRepo(db), postgres.NewOrderRepo(db)

	venue, err := bootstrap.Open(ctx, dishes, platform, cfg.Bootstrap.Retry, log)
	if err != nil {
		return
	}

	kitchen := kitchenusecase.New(db, dishes, orders, platform, venue.AvgCookMinutes, log)
	identity := "venue-" + venue.ID.String()

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		consumer.Serve(ctx, broker.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers,
			Topic:   topic(cfg, venue.OrdersTopic),
			GroupID: identity,
		}, broker.Deduplicated(
			db,
			postgres.NewInboxRepo(db),
			identity,
			consumer.NewOrders(kitchen, venue.ID, log).Handle,
		), log)
	}()

	defer wg.Wait()

	if !cfg.Autopilot.Enabled {
		log.InfoContext(ctx, "autopilot is off, orders wait for a cook")

		return
	}

	autopilot.New(kitchen, cfg.Autopilot, log).Run(ctx)
}

// topic returns the topic the venue reads its orders from. The platform names
// it in the profile of the venue; the configured one is what the venue falls
// back to when it does not.
func topic(cfg config.Config, named string) string {
	if named != "" {
		return named
	}

	return cfg.Kafka.OrdersTopic
}
