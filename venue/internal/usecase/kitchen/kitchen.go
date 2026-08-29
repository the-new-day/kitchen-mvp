// Package kitchen runs the venue: it takes the orders the platform sends,
// writes off what they spend, moves them through the kitchen and tells the
// platform about every move.
package kitchen

import (
	"avito-kitchen/venue/internal/kitchen"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Transactor runs a unit of work in one database transaction.
// The repositories called inside fn join it through the context.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Dishes is the nomenclature of the venue.
type Dishes interface {
	List(ctx context.Context) ([]kitchen.Dish, error)
	Take(ctx context.Context, sku string, qty int) (kitchen.Dish, error)
	Give(ctx context.Context, sku string, qty int) (kitchen.Dish, error)
	SetAvailable(ctx context.Context, sku string, available bool) (kitchen.Dish, error)
}

// Orders are the orders the venue has been given.
type Orders interface {
	Save(ctx context.Context, placed kitchen.OrderCreated) (bool, error)
	Get(ctx context.Context, id uuid.UUID) (kitchen.Order, error)
	Move(ctx context.Context, id uuid.UUID, from, to kitchen.State) (bool, error)
	Ripe(ctx context.Context, due []kitchen.Due, limit int) ([]kitchen.Order, error)
}

// PartnerClient is the platform as the venue reaches it: the calls a cash
// register makes and nothing else.
type PartnerClient interface {
	Accept(ctx context.Context, orderID uuid.UUID, eta int) error
	StartCooking(ctx context.Context, orderID uuid.UUID) error
	MarkReady(ctx context.Context, orderID uuid.UUID) error
	Handover(ctx context.Context, orderID uuid.UUID) error
	OrderStatus(ctx context.Context, orderID uuid.UUID) (string, error)
	SetItemAvailability(ctx context.Context, sku string, available bool) error
}

// Service is the kitchen of the venue.
type Service struct {
	tx       Transactor
	dishes   Dishes
	orders   Orders
	platform PartnerClient
	eta      int
	log      *slog.Logger
}

// New returns a kitchen that promises its orders in eta minutes.
func New(
	tx Transactor,
	dishes Dishes,
	orders Orders,
	platform PartnerClient,
	eta int,
	log *slog.Logger,
) *Service {
	return &Service{tx: tx, dishes: dishes, orders: orders, platform: platform, eta: eta, log: log}
}

// Receive takes an order the platform has given the venue: it is written down
// and the dishes it holds are written off the stock. An order the venue
// already has is left alone, so a redelivered event spends nothing twice.
func (s *Service) Receive(ctx context.Context, placed kitchen.OrderCreated) error {
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		fresh, err := s.orders.Save(ctx, placed)
		if err != nil {
			return err
		}

		if !fresh {
			s.log.InfoContext(ctx, "order is already on the kitchen",
				slog.String("order_id", placed.OrderID.String()))

			return nil
		}

		if err := s.take(ctx, placed); err != nil {
			return err
		}

		s.log.InfoContext(ctx, "order taken",
			slog.String("order_id", placed.OrderID.String()),
			slog.String("number", placed.Number),
			slog.Int("items", len(placed.Items)),
		)

		return nil
	})
}

// Cancel takes an order away from the kitchen: the platform, not the venue,
// has ended it. What the order held goes back to the stock.
func (s *Service) Cancel(ctx context.Context, cancelled kitchen.OrderCancelled) error {
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		order, err := s.orders.Get(ctx, cancelled.OrderID)
		if err != nil {
			if errors.Is(err, kitchen.ErrNotFound) {
				return nil
			}

			return err
		}

		if !kitchen.Moving(order.State) {
			return nil
		}

		moved, err := s.settle(ctx, order, kitchen.StateCancelled)
		if err != nil || !moved {
			return err
		}

		s.log.InfoContext(ctx, "order taken away",
			slog.String("order_id", order.ID.String()),
			slog.String("reason", cancelled.Reason),
		)

		return nil
	})
}

// Advance moves one order one step further and tells the platform about it.
// An order the platform refuses to move is not fought over: the venue asks
// where it stands and writes that down instead.
func (s *Service) Advance(ctx context.Context, order kitchen.Order) error {
	step, ok := kitchen.Next(order.State)
	if !ok {
		return nil
	}

	err := s.tell(ctx, step.Event, order.ID)

	var refused kitchen.RefusedError
	if errors.As(err, &refused) {
		return s.resync(ctx, order, refused.Current)
	}

	if err != nil {
		return err
	}

	moved, err := s.orders.Move(ctx, order.ID, step.From, step.To)
	if err != nil {
		return err
	}

	if moved {
		s.log.InfoContext(ctx, "order moved",
			slog.String("order_id", order.ID.String()),
			slog.String("from", string(step.From)),
			slog.String("to", string(step.To)),
		)
	}

	return nil
}

// Ripe returns the orders that have waited out their step.
func (s *Service) Ripe(ctx context.Context, due []kitchen.Due, limit int) ([]kitchen.Order, error) {
	return s.orders.Ripe(ctx, due, limit)
}

// SyncStopList puts the dishes the venue has run out of off sale on the
// platform, and the ones it has again back on. What the platform was last told
// is what the venue keeps for them, so a run that changes nothing calls nothing.
func (s *Service) SyncStopList(ctx context.Context) error {
	dishes, err := s.dishes.List(ctx)
	if err != nil {
		return err
	}

	for _, dish := range dishes {
		inStock := dish.Stock == nil || *dish.Stock > 0
		if inStock == dish.IsAvailable {
			continue
		}

		if err := s.platform.SetItemAvailability(ctx, dish.SKU, inStock); err != nil {
			return err
		}

		if _, err := s.dishes.SetAvailable(ctx, dish.SKU, inStock); err != nil {
			return err
		}

		s.log.InfoContext(ctx, "dish availability changed",
			slog.String("sku", dish.SKU), slog.Bool("is_available", inStock))
	}

	return nil
}

// tell makes the call the step of the pipeline stands for.
func (s *Service) tell(ctx context.Context, event kitchen.Event, orderID uuid.UUID) error {
	switch event {
	case kitchen.EventAccept:
		return s.platform.Accept(ctx, orderID, s.eta)
	case kitchen.EventCooking:
		return s.platform.StartCooking(ctx, orderID)
	case kitchen.EventReady:
		return s.platform.MarkReady(ctx, orderID)
	case kitchen.EventHandover:
		return s.platform.Handover(ctx, orderID)
	default:
		return fmt.Errorf("order %s: kitchen has no call for %q", orderID, event)
	}
}

// resync writes down the status the platform turned out to hold. A status the
// venue does not know is left to the next attempt.
func (s *Service) resync(ctx context.Context, order kitchen.Order, status string) error {
	state, ok := kitchen.StateOf(status)
	if !ok {
		return fmt.Errorf("order %s: platform reports unknown status %q", order.ID, status)
	}

	if state == order.State {
		return nil
	}

	return s.tx.InTx(ctx, func(ctx context.Context) error {
		moved, err := s.settle(ctx, order, state)
		if err != nil || !moved {
			return err
		}

		s.log.WarnContext(ctx, "order state caught up with the platform",
			slog.String("order_id", order.ID.String()),
			slog.String("from", string(order.State)),
			slog.String("to", string(state)),
		)

		return nil
	})
}

// settle moves an order into a state and gives the stock back when the order
// ends without leaving the kitchen.
func (s *Service) settle(ctx context.Context, order kitchen.Order, to kitchen.State) (bool, error) {
	moved, err := s.orders.Move(ctx, order.ID, order.State, to)
	if err != nil || !moved {
		return false, err
	}

	if !kitchen.ReturnsStock(to) {
		return true, nil
	}

	return true, s.give(ctx, order.Placed)
}

// take writes the dishes of an order off the stock.
func (s *Service) take(ctx context.Context, placed kitchen.OrderCreated) error {
	for _, item := range placed.Items {
		if _, err := s.dishes.Take(ctx, item.ExternalID, item.Qty); err != nil {
			if errors.Is(err, kitchen.ErrNotFound) {
				s.log.WarnContext(ctx, "order holds a dish the venue does not sell",
					slog.String("order_id", placed.OrderID.String()),
					slog.String("sku", item.ExternalID))

				continue
			}

			return err
		}
	}

	return nil
}

// give returns the dishes of an order to the stock.
func (s *Service) give(ctx context.Context, placed kitchen.OrderCreated) error {
	for _, item := range placed.Items {
		if _, err := s.dishes.Give(ctx, item.ExternalID, item.Qty); err != nil {
			if errors.Is(err, kitchen.ErrNotFound) {
				continue
			}

			return err
		}
	}

	return nil
}
