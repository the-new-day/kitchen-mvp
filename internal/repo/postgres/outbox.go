package postgres

import (
	"avito-kitchen/internal/domain/outbox"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// Fetch takes the next batch of messages waiting to be published and holds
// them until the end of the transaction. The batch is ordered by id: the
// events of one aggregate are published in the order they were written, and
// created_at cannot order the two events of a single transaction. Rows another
// worker is already holding are skipped rather than waited for.
func (r *OutboxRepo) Fetch(ctx context.Context, limit int) ([]outbox.Pending, error) {
	const query = `
		SELECT id, event_id, topic, key, event_type,
			aggregate_id, aggregate_version, payload, created_at
		FROM outbox_messages
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := r.db.conn(ctx).Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query outbox messages: %w", err)
	}

	pending, err := pgx.CollectRows(rows, scanPending)
	if err != nil {
		return nil, fmt.Errorf("scan outbox messages: %w", err)
	}

	return pending, nil
}

// MarkPublished records that the broker has acknowledged the messages. It is
// called after the acknowledgement, never before: a message lost on the way
// would otherwise look delivered.
func (r *OutboxRepo) MarkPublished(ctx context.Context, ids []int64) error {
	const query = `
		UPDATE outbox_messages
		SET published_at = now(), attempts = attempts + 1, last_error = NULL
		WHERE id = ANY($1)`

	if _, err := r.db.conn(ctx).Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("mark outbox messages published: %w", err)
	}

	return nil
}

// MarkFailed counts an attempt that did not reach the broker and keeps its
// cause. The message stays unpublished and is taken again next time.
func (r *OutboxRepo) MarkFailed(ctx context.Context, id int64, cause error) error {
	const query = `
		UPDATE outbox_messages
		SET attempts = attempts + 1, last_error = $2
		WHERE id = $1`

	if _, err := r.db.conn(ctx).Exec(ctx, query, id, cause.Error()); err != nil {
		return fmt.Errorf("mark outbox message failed: %w", err)
	}

	return nil
}

func scanPending(row pgx.CollectableRow) (outbox.Pending, error) {
	var message outbox.Pending

	err := row.Scan(
		&message.ID, &message.EventID, &message.Topic, &message.Key, &message.EventType,
		&message.AggregateID, &message.AggregateVersion, &message.Payload, &message.OccurredAt,
	)

	return message, err
}
