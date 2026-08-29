package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// retryPause is how long a consumer waits before handling a message again
// after the handler has failed.
const retryPause = time.Second

// Handler processes one event. Returning an error asks for the event to be
// delivered again, so a handler must be safe to run twice.
type Handler func(ctx context.Context, envelope Envelope) error

// ConsumerConfig is what a consumer needs to join a topic. GroupID is the
// consumer group it reads under: a fixed one continues from the last committed
// offset after a restart, a unique one makes every instance see every message.
// FromLatest starts a group that has committed nothing at the end of the topic
// instead of its beginning: what happened before it joined is of no use to it.
type ConsumerConfig struct {
	Brokers    []string
	Topic      string
	GroupID    string
	FromLatest bool
}

// Consumer reads the events of one topic and hands them over one at a time.
type Consumer struct {
	reader *kafka.Reader
	log    *slog.Logger
}

// NewConsumer returns a consumer of the topic of cfg.
func NewConsumer(cfg ConsumerConfig, log *slog.Logger) *Consumer {
	startOffset := kafka.FirstOffset
	if cfg.FromLatest {
		startOffset = kafka.LastOffset
	}

	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:     cfg.Brokers,
			Topic:       cfg.Topic,
			GroupID:     cfg.GroupID,
			StartOffset: startOffset,
		}),
		log: log.With(slog.String("topic", cfg.Topic), slog.String("group", cfg.GroupID)),
	}
}

// Run reads the topic until ctx is cancelled. The offset of a message is
// committed only once the handler has accepted it: a consumer that dies in the
// middle of an event sees it again.
func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("fetch message: %w", err)
		}

		if err := c.deliver(ctx, message, handle); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		if err := c.reader.CommitMessages(ctx, message); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("commit offset: %w", err)
		}
	}
}

// deliver hands one message to the handler, repeating until it is accepted. A
// message that is not an envelope of the platform is dropped instead: another
// delivery of it would fail exactly the same way.
func (c *Consumer) deliver(ctx context.Context, message kafka.Message, handle Handler) error {
	envelope, err := Decode(message.Value)
	if err != nil {
		c.log.ErrorContext(ctx, "dropping unreadable message",
			slog.Int64("offset", message.Offset), slog.String("error", err.Error()))

		return nil
	}

	log := c.log.With(
		slog.String("event_id", envelope.EventID.String()),
		slog.String("event_type", envelope.EventType),
	)

	for {
		err := handle(ctx, envelope)
		if err == nil {
			return nil
		}

		if errors.Is(err, ErrMalformed) {
			log.ErrorContext(ctx, "dropping unhandleable event", slog.String("error", err.Error()))

			return nil
		}

		log.ErrorContext(ctx, "event handling failed, retrying",
			slog.String("error", err.Error()), slog.Duration("in", retryPause))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryPause):
		}
	}
}

// Close stops the consumer and releases its connections.
func (c *Consumer) Close() error {
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("close consumer: %w", err)
	}

	return nil
}
