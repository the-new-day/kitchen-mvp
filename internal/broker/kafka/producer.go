package kafka

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/domain/outbox"
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// Producer publishes the events of the platform. It is synchronous on
// purpose: the caller marks a message as published only once the broker has
// acknowledged it, and messages of one aggregate keep the order they were
// written in.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer returns a producer writing to the brokers of cfg.
func NewProducer(cfg config.Kafka) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireAll,
			Async:        false,
			// One message per write: a batch would hold the first message
			// until BatchTimeout and answer for the whole batch at once.
			BatchSize: 1,
			// One attempt per write: retrying is the job of whoever stores the
			// message, and a writer that keeps trying holds the batch it was
			// given for as long as the broker stays unreachable.
			MaxAttempts:  1,
			WriteTimeout: cfg.Timeout,
		},
	}
}

// Publish sends one message to the topic it names and returns once the brokers
// have acknowledged it. The key decides the partition, and with it the order
// the consumer sees.
func (p *Producer) Publish(ctx context.Context, message outbox.Pending) error {
	body, err := Encode(Wrap(message))
	if err != nil {
		return err
	}

	err = p.writer.WriteMessages(ctx, kafka.Message{
		Topic: message.Topic,
		Key:   []byte(message.Key),
		Value: body,
	})
	if err != nil {
		return fmt.Errorf("publish event %s to %s: %w", message.EventID, message.Topic, err)
	}

	return nil
}

// Close flushes what is left and releases the connections.
func (p *Producer) Close() error {
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("close producer: %w", err)
	}

	return nil
}
