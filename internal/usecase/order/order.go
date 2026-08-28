// Package order serves the orders of a customer: placing one out of a cart in
// a single transaction and reading the ones already placed.
package order

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/domain/outbox"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Transactor runs a unit of work in one database transaction.
// The repositories called inside fn join it through the context.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Repository stores the orders of the customers. The Lock methods read an
// order and hold it for the rest of the transaction: a transition is applied to
// what nobody else can be changing at the same time.
type Repository interface {
	Create(ctx context.Context, draft order.Draft) (order.Order, error)
	Get(ctx context.Context, userID, orderID uuid.UUID) (order.Order, error)
	GetForVenue(ctx context.Context, venueID, orderID uuid.UUID) (order.Order, error)
	List(ctx context.Context, filter order.Filter) ([]order.Order, error)
	LockForCustomer(ctx context.Context, userID, orderID uuid.UUID) (order.Order, error)
	LockForVenue(ctx context.Context, venueID, orderID uuid.UUID) (order.Order, error)
	LockUnaccepted(ctx context.Context, orderID uuid.UUID) (order.Order, error)
	StaleUnaccepted(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error)
	ApplyStatus(ctx context.Context, orderID uuid.UUID, change order.StatusChange) (order.Applied, error)
}

// CartRepository reads and empties the cart an order is placed out of.
type CartRepository interface {
	Lock(ctx context.Context, userID uuid.UUID) error
	Cart(ctx context.Context, userID uuid.UUID) (cart.Cart, error)
	Clear(ctx context.Context, userID uuid.UUID) error
}

// MenuRepository holds the stock the ordered items are taken from.
type MenuRepository interface {
	ReserveStock(ctx context.Context, venueID uuid.UUID, items []order.Item) ([]uuid.UUID, error)
	ReleaseStock(ctx context.Context, venueID uuid.UUID, items []order.Item) error
}

// OutboxRepository stores the events waiting to be published.
type OutboxRepository interface {
	Append(ctx context.Context, message outbox.Message) error
}

// Topics are the topics the events of an order go to: Orders is read by the
// venue the order was placed at, Status by whoever follows the order.
type Topics struct {
	Orders string
	Status string
}

// Service is the order use case.
type Service struct {
	tx     Transactor
	orders Repository
	carts  CartRepository
	menus  MenuRepository
	outbox OutboxRepository
	topics Topics
}

// New returns a service working through the given repositories and publishing
// its events to the given topics.
func New(
	tx Transactor,
	orders Repository,
	carts CartRepository,
	menus MenuRepository,
	messages OutboxRepository,
	topics Topics,
) *Service {
	return &Service{
		tx:     tx,
		orders: orders,
		carts:  carts,
		menus:  menus,
		outbox: messages,
		topics: topics,
	}
}

// Create turns the cart of a customer into an order and empties it. Reserving
// the stock, storing the order with its prices copied into it, its first entry
// in the status history and the event about it all happen in one transaction:
// an order either exists whole or does not exist at all.
func (s *Service) Create(
	ctx context.Context, userID uuid.UUID, request order.Request,
) (order.Order, error) {
	var created order.Order

	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		draft, err := s.draft(ctx, userID, request)
		if err != nil {
			return err
		}

		if err = s.reserve(ctx, draft); err != nil {
			return err
		}

		created, err = s.orders.Create(ctx, draft)
		if err != nil {
			return fmt.Errorf("create order of user %s: %w", userID, err)
		}

		message, err := orderCreated(s.topics.Orders, created)
		if err != nil {
			return err
		}

		if err = s.outbox.Append(ctx, message); err != nil {
			return fmt.Errorf("append order %s to outbox: %w", created.ID, err)
		}

		if err = s.carts.Clear(ctx, userID); err != nil {
			return fmt.Errorf("clear cart of user %s: %w", userID, err)
		}

		return nil
	})
	if err != nil {
		return order.Order{}, err
	}

	return created, nil
}

// draft reads the cart of a customer under a lock and checks that it may
// become an order. The lock makes two orders placed at once out of one cart
// take turns instead of both spending it.
func (s *Service) draft(
	ctx context.Context, userID uuid.UUID, request order.Request,
) (order.Draft, error) {
	if err := s.carts.Lock(ctx, userID); err != nil {
		return order.Draft{}, fmt.Errorf("lock cart of user %s: %w", userID, err)
	}

	current, err := s.carts.Cart(ctx, userID)
	if err != nil {
		return order.Draft{}, fmt.Errorf("get cart of user %s: %w", userID, err)
	}

	return order.FromCart(userID, current, request)
}

// reserve takes the ordered quantities out of the stock of the venue. An item
// somebody else has taken in the meantime is reported as a conflict: the check
// of the cart and the reservation are two moments, and only the second one
// counts.
func (s *Service) reserve(ctx context.Context, draft order.Draft) error {
	missed, err := s.menus.ReserveStock(ctx, draft.VenueID, draft.Items)
	if err != nil {
		return fmt.Errorf("reserve stock of venue %s: %w", draft.VenueID, err)
	}

	if len(missed) == 0 {
		return nil
	}

	names := make([]string, 0, len(missed))

	for _, id := range missed {
		for _, item := range draft.Items {
			if item.MenuItemID == id {
				names = append(names, item.Name)
			}
		}
	}

	return domain.Conflictf(order.CodeOutOfStock,
		"%s ran out while the order was being placed", strings.Join(names, ", "))
}

// Order returns one order of a customer. An order of somebody else is reported
// as domain.ErrNotFound rather than as a refusal: the identifier of an order
// is not confirmed to those it does not belong to.
func (s *Service) Order(ctx context.Context, userID, orderID uuid.UUID) (order.Order, error) {
	found, err := s.orders.Get(ctx, userID, orderID)
	if err != nil {
		return order.Order{}, fmt.Errorf("get order %s of user %s: %w", orderID, userID, err)
	}

	return found, nil
}

// VenueOrder returns one order placed at a venue. An order of another venue is
// reported as domain.ErrNotFound.
func (s *Service) VenueOrder(ctx context.Context, venueID, orderID uuid.UUID) (order.Order, error) {
	found, err := s.orders.GetForVenue(ctx, venueID, orderID)
	if err != nil {
		return order.Order{}, fmt.Errorf("get order %s of venue %s: %w", orderID, venueID, err)
	}

	return found, nil
}

// OrdersQuery is a request for a page of the order list as it arrived from the
// client: nothing is defaulted or checked yet.
type OrdersQuery struct {
	Statuses []string
	Cursor   string
	Limit    *int
}

// Page is a page of the order list, newest first.
// NextCursor is empty on the last page.
type Page struct {
	Orders     []order.Order
	NextCursor string
}

// Orders returns a page of the orders of a customer. It reads one row more
// than asked for: that row decides whether there is a next page, and never
// leaves the use case.
func (s *Service) Orders(
	ctx context.Context, userID uuid.UUID, query OrdersQuery,
) (Page, error) {
	filter, err := buildFilter(userID, query)
	if err != nil {
		return Page{}, err
	}

	limit := filter.Limit
	filter.Limit = limit + 1

	orders, err := s.orders.List(ctx, filter)
	if err != nil {
		return Page{}, fmt.Errorf("list orders of user %s: %w", userID, err)
	}

	page := Page{Orders: orders}
	if len(orders) > limit {
		page.Orders = orders[:limit]
		page.NextCursor = encodeCursor(page.Orders[limit-1])
	}

	return page, nil
}

func buildFilter(userID uuid.UUID, query OrdersQuery) (order.Filter, error) {
	limit := defaultLimit
	if query.Limit != nil {
		limit = *query.Limit
	}

	if limit < 1 || limit > maxLimit {
		return order.Filter{}, domain.InvalidArgumentf("limit must be between 1 and %d", maxLimit)
	}

	statuses := make([]order.Status, 0, len(query.Statuses))

	for _, raw := range query.Statuses {
		status := order.Status(raw)
		if !status.Valid() {
			return order.Filter{}, domain.InvalidArgumentf("unknown status %q", raw)
		}

		statuses = append(statuses, status)
	}

	filter := order.Filter{UserID: userID, Statuses: statuses, Limit: limit}

	if query.Cursor != "" {
		after, err := decodeCursor(query.Cursor)
		if err != nil {
			return order.Filter{}, err
		}

		filter.After = after
	}

	return filter, nil
}
