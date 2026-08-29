package consumer

import (
	broker "avito-kitchen/internal/broker/kafka"
	"context"
	"log/slog"
	"time"
)

// retryPause is how long the venue waits before joining the topic again after
// the broker has let it down.
const retryPause = 5 * time.Second

// Serve reads the topic until ctx is cancelled. A broker that goes away does
// not take the venue with it: the reader is opened again and the orders it
// missed are waiting for it at the last committed offset.
func Serve(ctx context.Context, cfg broker.ConsumerConfig, handle broker.Handler, log *slog.Logger) {
	for {
		if err := read(ctx, cfg, handle, log); err != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "reading orders failed, reconnecting",
				slog.String("error", err.Error()), slog.Duration("in", retryPause))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryPause):
		}
	}
}

func read(ctx context.Context, cfg broker.ConsumerConfig, handle broker.Handler, log *slog.Logger) error {
	consumer := broker.NewConsumer(cfg, log)

	defer func() {
		if err := consumer.Close(); err != nil {
			log.ErrorContext(ctx, "closing consumer failed", slog.String("error", err.Error()))
		}
	}()

	return consumer.Run(ctx, handle)
}
