package postgres

import (
	"avito-kitchen/venue/internal/kitchen"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// orderColumns is what an order is read by, qualified: every statement that
// selects an order joins something onto it.
const orderColumns = `
	o.main_order_id, o.payload, o.state, o.received_at, o.state_changed_at, o.decided_at`

// OrderRepo keeps the orders the venue has been given.
type OrderRepo struct {
	db *DB
}

// NewOrderRepo returns a repository over db.
func NewOrderRepo(db *DB) *OrderRepo {
	return &OrderRepo{db: db}
}

// Save writes an order the venue has just been told about and reports whether
// it is new. An order it already has is left untouched: a redelivered event
// must not put a working order back to the beginning.
func (r *OrderRepo) Save(ctx context.Context, placed kitchen.OrderCreated) (bool, error) {
	payload, err := json.Marshal(placed)
	if err != nil {
		return false, fmt.Errorf("marshal order %s: %w", placed.OrderID, err)
	}

	const query = `
		INSERT INTO incoming_orders (main_order_id, payload, state)
		VALUES ($1, $2, $3)
		ON CONFLICT (main_order_id) DO NOTHING
		RETURNING main_order_id`

	rows, err := r.db.conn(ctx).Query(ctx, query, placed.OrderID, payload, kitchen.StateNew)
	if err != nil {
		return false, fmt.Errorf("insert order: %w", err)
	}

	saved, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return false, fmt.Errorf("scan saved order: %w", err)
	}

	return len(saved) > 0, nil
}

// Get returns one order of the venue.
func (r *OrderRepo) Get(ctx context.Context, id uuid.UUID) (kitchen.Order, error) {
	query := `SELECT ` + orderColumns + ` FROM incoming_orders o WHERE o.main_order_id = $1`

	rows, err := r.db.conn(ctx).Query(ctx, query, id)
	if err != nil {
		return kitchen.Order{}, fmt.Errorf("select order: %w", err)
	}

	order, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return kitchen.Order{}, fmt.Errorf("order %s: %w", id, kitchen.ErrNotFound)
		}

		return kitchen.Order{}, fmt.Errorf("scan order: %w", err)
	}

	return order, nil
}

// Move puts an order in a new state and reports whether it was the one to do
// it: a move from a state the order has already left changes nothing. The
// moment the venue first decided the fate of an order is kept as it was.
func (r *OrderRepo) Move(ctx context.Context, id uuid.UUID, from, to kitchen.State) (bool, error) {
	const query = `
		UPDATE incoming_orders
		SET state = $3,
		    state_changed_at = now(),
		    decided_at = COALESCE(decided_at, now())
		WHERE main_order_id = $1 AND state = $2
		RETURNING main_order_id`

	rows, err := r.db.conn(ctx).Query(ctx, query, id, from, to)
	if err != nil {
		return false, fmt.Errorf("move order: %w", err)
	}

	moved, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return false, fmt.Errorf("scan moved order: %w", err)
	}

	return len(moved) > 0, nil
}

// Ripe returns the orders that have waited out their step, oldest first. The
// states and their deadlines come from the caller: the kitchen decides how long
// a step takes, the database only finds the rows.
func (r *OrderRepo) Ripe(ctx context.Context, due []kitchen.Due, limit int) ([]kitchen.Order, error) {
	if len(due) == 0 {
		return nil, nil
	}

	states := make([]string, 0, len(due))
	cutoffs := make([]time.Time, 0, len(due))

	for _, d := range due {
		states = append(states, string(d.State))
		cutoffs = append(cutoffs, d.Cutoff)
	}

	query := `
		SELECT ` + orderColumns + `
		FROM incoming_orders o
		JOIN unnest($1::text[]::kitchen_order_state[], $2::timestamptz[]) AS d(state, cutoff)
		  ON o.state = d.state AND o.state_changed_at < d.cutoff
		ORDER BY o.state_changed_at
		LIMIT $3`

	rows, err := r.db.conn(ctx).Query(ctx, query, states, cutoffs, limit)
	if err != nil {
		return nil, fmt.Errorf("select ripe orders: %w", err)
	}

	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, fmt.Errorf("scan ripe orders: %w", err)
	}

	return orders, nil
}

func scanOrder(row pgx.CollectableRow) (kitchen.Order, error) {
	var (
		order   kitchen.Order
		payload []byte
	)

	err := row.Scan(
		&order.ID, &payload, &order.State,
		&order.ReceivedAt, &order.StateChangedAt, &order.DecidedAt,
	)
	if err != nil {
		return kitchen.Order{}, err
	}

	if err := json.Unmarshal(payload, &order.Placed); err != nil {
		return kitchen.Order{}, fmt.Errorf("unmarshal order %s: %w", order.ID, err)
	}

	return order, nil
}
