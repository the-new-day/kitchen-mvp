// Command worker runs the background jobs of the platform: outbox publishing,
// the order reaper and idempotency-key garbage collection. It serves /healthz.
package main

import (
	kafkabroker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/logger"
	"avito-kitchen/internal/repo/postgres"
	orderusecase "avito-kitchen/internal/usecase/order"
	"avito-kitchen/internal/worker"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const serviceName = "worker"

// brokerRetry is how long the worker waits before asking the broker for its
// topics again.
const brokerRetry = 5 * time.Second

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

	producer := kafkabroker.NewProducer(cfg.Kafka)
	defer func() {
		if err := producer.Close(); err != nil {
			log.Error("closing producer failed", slog.String("error", err.Error()))
		}
	}()

	orders := orderusecase.New(
		db,
		postgres.NewOrderRepo(db),
		postgres.NewCartRepo(db),
		postgres.NewMenuRepo(db),
		postgres.NewOutboxRepo(db),
		orderusecase.Topics{Orders: cfg.Kafka.OrdersTopic, Status: cfg.Kafka.StatusTopic},
	)

	jobs := []worker.Job{
		worker.NewOutbox(db, postgres.NewOutboxRepo(db), producer, cfg.Worker.Outbox, log),
		worker.NewReaper(orders, cfg.Worker.Reaper, log),
		worker.NewIdempotencyGC(postgres.NewIdempotencyRepo(db), cfg.Worker.IdempotencyGC, log),
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		ensureTopics(ctx, cfg.Kafka, log)
	}()

	go func() {
		defer wg.Done()

		worker.Run(ctx, log, jobs...)
	}()

	defer wg.Wait()

	return httpx.Serve(ctx, cfg.HTTP, httpx.NewRouter(log), log)
}

// ensureTopics creates the topics of the platform, waiting for the broker as
// long as it takes. It runs beside the jobs rather than before them: a broker
// that is down delays publishing and nothing else, and the events wait in the
// outbox until it answers.
func ensureTopics(ctx context.Context, cfg config.Kafka, log *slog.Logger) {
	for {
		err := kafkabroker.EnsureTopics(ctx, cfg)
		if err == nil {
			log.InfoContext(ctx, "topics ready", slog.Any("topics", cfg.Topics()))

			return
		}

		if ctx.Err() != nil {
			return
		}

		log.WarnContext(ctx, "broker not ready, retrying",
			slog.String("error", err.Error()), slog.Duration("in", brokerRetry))

		select {
		case <-ctx.Done():
			return
		case <-time.After(brokerRetry):
		}
	}
}
