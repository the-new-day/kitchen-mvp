package kafka

import (
	"avito-kitchen/internal/config"
	"context"
	"errors"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// EnsureTopics creates the topics of the platform if the broker does not have
// them yet. It is safe to call on every start: a topic that already exists is
// left as it is.
func EnsureTopics(ctx context.Context, cfg config.Kafka) error {
	topics := make([]kafka.TopicConfig, 0, len(cfg.Topics()))
	for _, topic := range cfg.Topics() {
		topics = append(topics, kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     cfg.Partitions,
			ReplicationFactor: 1,
		})
	}

	client := &kafka.Client{Addr: kafka.TCP(cfg.Brokers...), Timeout: cfg.Timeout}

	response, err := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: topics})
	if err != nil {
		return fmt.Errorf("create topics: %w", err)
	}

	for topic, topicErr := range response.Errors {
		if topicErr != nil && !errors.Is(topicErr, kafka.TopicAlreadyExists) {
			return fmt.Errorf("create topic %s: %w", topic, topicErr)
		}
	}

	return nil
}
