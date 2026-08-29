package kafka

import (
	"context"
	"log/slog"
	"time"
)

// reconnectPause is how long a consumer waits before joining its topic again
// after the broker has let it down.
const reconnectPause = 5 * time.Second

// Serve reads a topic until ctx is cancelled. A broker that goes away does not
// take the service with it: the reader is opened again, and a consumer reading
// under a group finds what it missed waiting at the last committed offset.
func Serve(ctx context.Context, cfg ConsumerConfig, handle Handler, log *slog.Logger) {
	for {
		if err := read(ctx, cfg, handle, log); err != nil && ctx.Err() == nil {
			log.ErrorContext(ctx, "reading the topic failed, reconnecting",
				slog.String("topic", cfg.Topic),
				slog.String("error", err.Error()),
				slog.Duration("in", reconnectPause))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectPause):
		}
	}
}

func read(ctx context.Context, cfg ConsumerConfig, handle Handler, log *slog.Logger) error {
	consumer := NewConsumer(cfg, log)

	defer func() {
		if err := consumer.Close(); err != nil {
			log.ErrorContext(ctx, "closing consumer failed", slog.String("error", err.Error()))
		}
	}()

	return consumer.Run(ctx, handle)
}
