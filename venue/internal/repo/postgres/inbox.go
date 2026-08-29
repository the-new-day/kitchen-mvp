package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// InboxRepo remembers the events the venue has already taken.
type InboxRepo struct {
	db *DB
}

// NewInboxRepo returns a repository over db.
func NewInboxRepo(db *DB) *InboxRepo {
	return &InboxRepo{db: db}
}

// Remember marks an event as taken and reports whether it is the first time.
// It is meant to be called in the transaction of the work the event causes, so
// that a redelivery finds the mark exactly when it finds the work already done.
func (r *InboxRepo) Remember(
	ctx context.Context, consumer string, eventID uuid.UUID,
) (bool, error) {
	const query = `
		INSERT INTO processed_events (consumer, event_id)
		VALUES ($1, $2)
		ON CONFLICT (consumer, event_id) DO NOTHING
		RETURNING event_id`

	rows, err := r.db.conn(ctx).Query(ctx, query, consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("remember event: %w", err)
	}

	remembered, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return false, fmt.Errorf("scan remembered event: %w", err)
	}

	return len(remembered) > 0, nil
}
