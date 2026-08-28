package postgres

import (
	"avito-kitchen/internal/domain/outbox"
	"context"
	"fmt"
)

// OutboxRepo stores the events waiting to be published to the broker.
type OutboxRepo struct {
	db *DB
}

// NewOutboxRepo returns a repository over db.
func NewOutboxRepo(db *DB) *OutboxRepo {
	return &OutboxRepo{db: db}
}

// Append stores one event. It is meant to be called inside the transaction of
// the domain change it describes: the row and the change are committed
// together or not at all. The identifier of the event is assigned here and
// survives every retry of publishing it, so that a consumer can deduplicate.
func (r *OutboxRepo) Append(ctx context.Context, message outbox.Message) error {
	const query = `
		INSERT INTO outbox_messages (
			topic, key, event_type, aggregate_id, aggregate_version, payload
		)
		VALUES ($1, $2, $3, $4, $5, $6)`

	if _, err := r.db.conn(ctx).Exec(ctx, query,
		message.Topic, message.Key, message.EventType,
		message.AggregateID, message.AggregateVersion, message.Payload,
	); err != nil {
		return fmt.Errorf("insert outbox message: %w", err)
	}

	return nil
}
