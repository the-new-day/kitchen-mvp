package postgres

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OrderRepo stores the orders of the customers.
type OrderRepo struct {
	db *DB
}

// NewOrderRepo returns a repository over db.
func NewOrderRepo(db *DB) *OrderRepo {
	return &OrderRepo{db: db}
}

const orderColumns = `
	o.id, o.number, o.user_id, o.venue_id, v.name, o.status,
	o.items_total, o.delivery_fee, o.total, o.address, o.phone, o.comment,
	o.eta_minutes, o.rejection_reason, o.version, o.created_at, o.updated_at`

// Create stores an order with the prices of its items copied into it and opens
// its status history. The stock the order spends is expected to have been
// reserved in the same transaction.
func (r *OrderRepo) Create(ctx context.Context, draft order.Draft) (order.Order, error) {
	created, err := r.insertOrder(ctx, draft)
	if err != nil {
		return order.Order{}, err
	}

	if err := r.insertItems(ctx, created.ID, draft.Items); err != nil {
		return order.Order{}, err
	}

	if err := r.appendHistory(ctx, created.ID, created.Status); err != nil {
		return order.Order{}, err
	}

	created.Items = draft.Items

	return created, nil
}

// insertOrder writes the order itself and reads it back together with the name
// of its venue, so that the card is complete without a second round trip.
func (r *OrderRepo) insertOrder(ctx context.Context, draft order.Draft) (order.Order, error) {
	query := fmt.Sprintf(`
		WITH inserted AS (
			INSERT INTO orders (
				user_id, venue_id, items_total, delivery_fee, total,
				address, phone, comment
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING *
		)
		SELECT %s FROM inserted o JOIN venues v ON v.id = o.venue_id`, orderColumns)

	rows, err := r.db.conn(ctx).Query(ctx, query,
		draft.UserID, draft.VenueID, draft.ItemsTotal, draft.DeliveryFee, draft.Total,
		draft.Address, draft.Phone, draft.Comment,
	)
	if err != nil {
		return order.Order{}, fmt.Errorf("insert order: %w", err)
	}

	created, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		return order.Order{}, fmt.Errorf("scan order: %w", err)
	}

	return created, nil
}

// insertItems writes the ordered positions as a snapshot: the name and the
// price are copies, so that the order reads the same years later.
func (r *OrderRepo) insertItems(ctx context.Context, orderID uuid.UUID, items []order.Item) error {
	const query = `
		INSERT INTO order_items (
			order_id, menu_item_id, external_id, name_snapshot, price_snapshot, qty
		)
		SELECT $1, i.menu_item_id, i.external_id, i.name, i.price, i.qty
		FROM unnest($2::uuid[], $3::text[], $4::text[], $5::bigint[], $6::integer[])
			AS i(menu_item_id, external_id, name, price, qty)`

	ids := make([]uuid.UUID, len(items))
	externalIDs := make([]string, len(items))
	names := make([]string, len(items))
	prices := make([]int64, len(items))
	quantities := make([]int, len(items))

	for i, item := range items {
		ids[i] = item.MenuItemID
		externalIDs[i] = item.ExternalID
		names[i] = item.Name
		prices[i] = item.Price
		quantities[i] = item.Qty
	}

	if _, err := r.db.conn(ctx).Exec(ctx, query,
		orderID, ids, externalIDs, names, prices, quantities,
	); err != nil {
		return fmt.Errorf("insert order items: %w", err)
	}

	return nil
}

// appendHistory opens the status history of a new order: it comes from no
// status at all, and the customer is who put it there.
func (r *OrderRepo) appendHistory(
	ctx context.Context, orderID uuid.UUID, status order.Status,
) error {
	const query = `
		INSERT INTO order_status_history (order_id, from_status, to_status, actor)
		VALUES ($1, NULL, $2, $3)`

	if _, err := r.db.conn(ctx).Exec(ctx, query, orderID, status, order.ActorCustomer); err != nil {
		return fmt.Errorf("insert order status history: %w", err)
	}

	return nil
}

// Get returns one order of a customer. An order that does not exist and an
// order of somebody else are both reported as domain.ErrNotFound.
func (r *OrderRepo) Get(ctx context.Context, userID, orderID uuid.UUID) (order.Order, error) {
	return r.get(ctx, "o.user_id", userID, orderID)
}

// GetForVenue returns one order placed at a venue. An order of another venue is
// reported as domain.ErrNotFound.
func (r *OrderRepo) GetForVenue(ctx context.Context, venueID, orderID uuid.UUID) (order.Order, error) {
	return r.get(ctx, "o.venue_id", venueID, orderID)
}

// get returns one order visible to the owner it is looked up by.
func (r *OrderRepo) get(
	ctx context.Context, ownerColumn string, ownerID, orderID uuid.UUID,
) (order.Order, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM orders o JOIN venues v ON v.id = o.venue_id
		WHERE o.id = $1 AND %s = $2`, orderColumns, ownerColumn)

	rows, err := r.db.conn(ctx).Query(ctx, query, orderID, ownerID)
	if err != nil {
		return order.Order{}, fmt.Errorf("query order: %w", err)
	}

	found, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, domain.ErrNotFound
		}

		return order.Order{}, fmt.Errorf("scan order: %w", err)
	}

	page := []order.Order{found}
	if err := r.attachItems(ctx, page); err != nil {
		return order.Order{}, err
	}

	return page[0], nil
}

// List returns at most f.Limit orders of a customer, newest first, starting
// after f.After. The positions of the page are read by a second query, not one
// per order.
func (r *OrderRepo) List(ctx context.Context, f order.Filter) ([]order.Order, error) {
	conds := []string{"o.user_id = $1"}
	args := []any{f.UserID}

	if len(f.Statuses) > 0 {
		statuses := make([]string, 0, len(f.Statuses))
		for _, status := range f.Statuses {
			statuses = append(statuses, string(status))
		}

		args = append(args, statuses)
		conds = append(conds, fmt.Sprintf("o.status = ANY($%d::text[]::order_status[])", len(args)))
	}

	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		conds = append(conds, fmt.Sprintf("(o.created_at, o.id) < ($%d::timestamptz, $%d::uuid)",
			len(args)-1, len(args)))
	}

	args = append(args, f.Limit)
	query := fmt.Sprintf(`
		SELECT %s FROM orders o JOIN venues v ON v.id = o.venue_id
		WHERE %s ORDER BY o.created_at DESC, o.id DESC LIMIT $%d`,
		orderColumns, strings.Join(conds, " AND "), len(args))

	rows, err := r.db.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}

	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, fmt.Errorf("scan orders: %w", err)
	}

	if err := r.attachItems(ctx, orders); err != nil {
		return nil, err
	}

	return orders, nil
}

// orderItem is one row of the positions of a page of orders.
type orderItem struct {
	orderID uuid.UUID
	item    order.Item
}

// attachItems fills the positions of every order of the page with one query.
func (r *OrderRepo) attachItems(ctx context.Context, orders []order.Order) error {
	if len(orders) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.ID)
	}

	const query = `
		SELECT order_id, menu_item_id, external_id, name_snapshot, price_snapshot, qty
		FROM order_items
		WHERE order_id = ANY($1)
		ORDER BY order_id, name_snapshot`

	rows, err := r.db.conn(ctx).Query(ctx, query, ids)
	if err != nil {
		return fmt.Errorf("query order items: %w", err)
	}

	pairs, err := pgx.CollectRows(rows, scanOrderItem)
	if err != nil {
		return fmt.Errorf("scan order items: %w", err)
	}

	byOrder := make(map[uuid.UUID][]order.Item, len(orders))
	for _, p := range pairs {
		byOrder[p.orderID] = append(byOrder[p.orderID], p.item)
	}

	for i := range orders {
		orders[i].Items = byOrder[orders[i].ID]
	}

	return nil
}

func scanOrder(row pgx.CollectableRow) (order.Order, error) {
	var (
		o               order.Order
		address         *string
		comment         *string
		rejectionReason *string
	)

	err := row.Scan(
		&o.ID, &o.Number, &o.UserID, &o.Venue.ID, &o.Venue.Name, &o.Status,
		&o.ItemsTotal, &o.DeliveryFee, &o.Total, &address, &o.Phone, &comment,
		&o.EtaMinutes, &rejectionReason, &o.Version, &o.CreatedAt, &o.UpdatedAt,
	)

	if address != nil {
		o.Address = *address
	}

	if comment != nil {
		o.Comment = *comment
	}

	if rejectionReason != nil {
		o.RejectionReason = *rejectionReason
	}

	return o, err
}

// scanOrderItem reads one position. The menu item it was taken from may be
// gone: the snapshot in the order does not depend on it.
func scanOrderItem(row pgx.CollectableRow) (orderItem, error) {
	var (
		p        orderItem
		menuItem *uuid.UUID
	)

	err := row.Scan(&p.orderID, &menuItem, &p.item.ExternalID,
		&p.item.Name, &p.item.Price, &p.item.Qty)

	if menuItem != nil {
		p.item.MenuItemID = *menuItem
	}

	return p, err
}

// LockForCustomer reads an order of a customer and holds it until the end of
// the transaction, so that only one transition is applied to it at a time. An
// order of somebody else is reported as domain.ErrNotFound.
func (r *OrderRepo) LockForCustomer(
	ctx context.Context, userID, orderID uuid.UUID,
) (order.Order, error) {
	return r.lock(ctx, "o.user_id", userID, orderID)
}

// LockForVenue reads an order placed at a venue and holds it until the end of
// the transaction. An order of another venue is reported as domain.ErrNotFound.
func (r *OrderRepo) LockForVenue(
	ctx context.Context, venueID, orderID uuid.UUID,
) (order.Order, error) {
	return r.lock(ctx, "o.venue_id", venueID, orderID)
}

// lock reads one order under a row lock. Only the order itself is locked: the
// venue is joined to complete the card and must stay free.
func (r *OrderRepo) lock(
	ctx context.Context, ownerColumn string, ownerID, orderID uuid.UUID,
) (order.Order, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM orders o JOIN venues v ON v.id = o.venue_id
		WHERE o.id = $1 AND %s = $2
		FOR UPDATE OF o`, orderColumns, ownerColumn)

	rows, err := r.db.conn(ctx).Query(ctx, query, orderID, ownerID)
	if err != nil {
		return order.Order{}, fmt.Errorf("query order for update: %w", err)
	}

	locked, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, domain.ErrNotFound
		}

		return order.Order{}, fmt.Errorf("scan order for update: %w", err)
	}

	page := []order.Order{locked}
	if err := r.attachItems(ctx, page); err != nil {
		return order.Order{}, err
	}

	return page[0], nil
}

// ApplyStatus moves an order to the status of the change and records the move
// in its status history. The caller is expected to hold the order locked.
func (r *OrderRepo) ApplyStatus(
	ctx context.Context, orderID uuid.UUID, change order.StatusChange,
) (order.Applied, error) {
	moved, err := r.updateStatus(ctx, orderID, change)
	if err != nil {
		return order.Applied{}, err
	}

	seq, err := r.recordChange(ctx, orderID, change)
	if err != nil {
		return order.Applied{}, err
	}

	return order.Applied{Order: moved, Seq: seq}, nil
}

// updateStatus writes the new status and bumps the version. The estimate is
// only written when the transition brings one, and the reason only when the
// venue refused the order: what a customer wrote when cancelling is theirs and
// stays in the status history.
func (r *OrderRepo) updateStatus(
	ctx context.Context, orderID uuid.UUID, change order.StatusChange,
) (order.Order, error) {
	query := fmt.Sprintf(`
		WITH updated AS (
			UPDATE orders SET
				status = $2,
				eta_minutes = COALESCE($3, eta_minutes),
				rejection_reason = COALESCE($4, rejection_reason),
				version = version + 1,
				updated_at = now()
			WHERE id = $1
			RETURNING *
		)
		SELECT %s FROM updated o JOIN venues v ON v.id = o.venue_id`, orderColumns)

	var reason *string
	if change.Reason != "" && change.To == order.StatusRejected {
		reason = &change.Reason
	}

	rows, err := r.db.conn(ctx).Query(ctx, query,
		orderID, change.To, change.EtaMinutes, reason)
	if err != nil {
		return order.Order{}, fmt.Errorf("update order status: %w", err)
	}

	moved, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		return order.Order{}, fmt.Errorf("scan updated order: %w", err)
	}

	page := []order.Order{moved}
	if err := r.attachItems(ctx, page); err != nil {
		return order.Order{}, err
	}

	return page[0], nil
}

// recordChange appends the transition to the status history and returns the
// number of the entry.
func (r *OrderRepo) recordChange(
	ctx context.Context, orderID uuid.UUID, change order.StatusChange,
) (int64, error) {
	const query = `
		INSERT INTO order_status_history (order_id, from_status, to_status, actor, reason)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	var reason *string
	if change.Reason != "" {
		reason = &change.Reason
	}

	var seq int64

	err := r.db.conn(ctx).QueryRow(ctx, query,
		orderID, change.From, change.To, change.Actor, reason).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("insert order status history: %w", err)
	}

	return seq, nil
}

// StaleUnaccepted returns the orders no venue has taken into work since
// before, oldest first. Only the identifiers are read: each of them is then
// moved on its own, under its own lock.
func (r *OrderRepo) StaleUnaccepted(
	ctx context.Context, before time.Time, limit int,
) ([]uuid.UUID, error) {
	const query = `
		SELECT id FROM orders
		WHERE status = 'CREATED' AND created_at < $1
		ORDER BY created_at
		LIMIT $2`

	rows, err := r.db.conn(ctx).Query(ctx, query, before, limit)
	if err != nil {
		return nil, fmt.Errorf("query stale orders: %w", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("scan stale orders: %w", err)
	}

	return ids, nil
}

// LockUnaccepted reads an order that is still waiting to be accepted and holds
// it until the end of the transaction. An order the venue has meanwhile taken,
// or is holding right now, is reported as domain.ErrNotFound instead of being
// waited for: its fate is being decided already and there is nothing to reap.
func (r *OrderRepo) LockUnaccepted(ctx context.Context, orderID uuid.UUID) (order.Order, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM orders o JOIN venues v ON v.id = o.venue_id
		WHERE o.id = $1 AND o.status = 'CREATED'
		FOR UPDATE OF o SKIP LOCKED`, orderColumns)

	rows, err := r.db.conn(ctx).Query(ctx, query, orderID)
	if err != nil {
		return order.Order{}, fmt.Errorf("query unaccepted order for update: %w", err)
	}

	locked, err := pgx.CollectExactlyOneRow(rows, scanOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, domain.ErrNotFound
		}

		return order.Order{}, fmt.Errorf("scan unaccepted order for update: %w", err)
	}

	page := []order.Order{locked}
	if err := r.attachItems(ctx, page); err != nil {
		return order.Order{}, err
	}

	return page[0], nil
}

// History returns the status entries of an order made after the entry number
// after, oldest first. Zero returns the history whole.
func (r *OrderRepo) History(
	ctx context.Context, orderID uuid.UUID, after int64,
) ([]order.StatusEntry, error) {
	const query = `
		SELECT id, from_status, to_status, actor, reason, created_at
		FROM order_status_history
		WHERE order_id = $1 AND id > $2
		ORDER BY id`

	rows, err := r.db.conn(ctx).Query(ctx, query, orderID, after)
	if err != nil {
		return nil, fmt.Errorf("query order status history: %w", err)
	}

	entries, err := pgx.CollectRows(rows, scanStatusEntry)
	if err != nil {
		return nil, fmt.Errorf("scan order status history: %w", err)
	}

	return entries, nil
}

// scanStatusEntry reads one entry. The very first one comes from no status at
// all, and a transition nobody explained carries no reason.
func scanStatusEntry(row pgx.CollectableRow) (order.StatusEntry, error) {
	var (
		entry  order.StatusEntry
		from   *order.Status
		reason *string
	)

	err := row.Scan(&entry.Seq, &from, &entry.To, &entry.Actor, &reason, &entry.ChangedAt)

	if from != nil {
		entry.From = *from
	}

	if reason != nil {
		entry.Reason = *reason
	}

	return entry, err
}
