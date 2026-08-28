package postgres

import (
	"avito-kitchen/internal/platform/idempotency"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// IdempotencyRepo stores the attempts of the requests that must not happen
// twice.
type IdempotencyRepo struct {
	db *DB
}

// NewIdempotencyRepo returns a repository over db.
func NewIdempotencyRepo(db *DB) *IdempotencyRepo {
	return &IdempotencyRepo{db: db}
}

// Reserve claims a key for a new attempt, or returns the attempt already
// stored under it. Postgres serialises concurrent claims on the primary key,
// so two retries racing each other cannot both be the first one.
func (r *IdempotencyRepo) Reserve(
	ctx context.Context, key idempotency.Key, record idempotency.Record, expires time.Time,
) (*idempotency.Record, error) {
	const claim = `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, key) DO NOTHING
		RETURNING key`

	rows, err := r.db.conn(ctx).Query(ctx, claim,
		key.UserID, key.Value, record.Endpoint, record.RequestHash, expires)
	if err != nil {
		return nil, fmt.Errorf("claim idempotency key: %w", err)
	}

	claimed, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan idempotency key: %w", err)
	}

	if len(claimed) > 0 {
		return nil, nil
	}

	return r.get(ctx, key)
}

// get reads the attempt stored under a key.
func (r *IdempotencyRepo) get(
	ctx context.Context, key idempotency.Key,
) (*idempotency.Record, error) {
	const query = `
		SELECT endpoint, request_hash, response_status, response_body
		FROM idempotency_keys
		WHERE user_id = $1 AND key = $2`

	rows, err := r.db.conn(ctx).Query(ctx, query, key.UserID, key.Value)
	if err != nil {
		return nil, fmt.Errorf("query idempotency key: %w", err)
	}

	stored, err := pgx.CollectExactlyOneRow(rows, scanIdempotencyRecord)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The claim conflicted with a row that has since been collected.
			return nil, nil
		}

		return nil, fmt.Errorf("scan idempotency key: %w", err)
	}

	return &stored, nil
}

// Complete writes the answer of an attempt into the row that claimed the key.
func (r *IdempotencyRepo) Complete(
	ctx context.Context, key idempotency.Key, status int, body []byte,
) error {
	const query = `
		UPDATE idempotency_keys
		SET response_status = $3, response_body = $4
		WHERE user_id = $1 AND key = $2`

	if _, err := r.db.conn(ctx).Exec(ctx, query, key.UserID, key.Value, status, body); err != nil {
		return fmt.Errorf("store idempotent response: %w", err)
	}

	return nil
}

func scanIdempotencyRecord(row pgx.CollectableRow) (idempotency.Record, error) {
	var (
		stored idempotency.Record
		status *int
	)

	err := row.Scan(&stored.Endpoint, &stored.RequestHash, &status, &stored.ResponseBody)
	if status != nil {
		stored.ResponseStatus = *status
	}

	return stored, err
}
