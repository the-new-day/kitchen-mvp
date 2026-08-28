package order

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// locate reads the order a transition is applied to and holds it locked. It is
// what tells the customer side of a transition from the venue side: each looks
// the order up by its own owner and hides the orders of everybody else.
type locate func(ctx context.Context) (order.Order, error)

// Cancel withdraws an order at the request of the customer who placed it.
func (s *Service) Cancel(
	ctx context.Context, userID, orderID uuid.UUID, reason string,
) (order.Order, error) {
	return s.move(ctx, s.byCustomer(userID, orderID), order.Command{
		Event:  order.EventCancel,
		Actor:  order.ActorCustomer,
		Reason: reason,
	})
}

// Accept takes an order into work and promises it in etaMinutes.
func (s *Service) Accept(
	ctx context.Context, venueID, orderID uuid.UUID, etaMinutes int,
) (order.Order, error) {
	return s.move(ctx, s.byVenue(venueID, orderID), order.Command{
		Event:      order.EventAccept,
		Actor:      order.ActorVenue,
		EtaMinutes: &etaMinutes,
	})
}

// Reject refuses an order at the venue it was placed at.
func (s *Service) Reject(
	ctx context.Context, venueID, orderID uuid.UUID, reason string,
) (order.Order, error) {
	return s.move(ctx, s.byVenue(venueID, orderID), order.Command{
		Event:  order.EventReject,
		Actor:  order.ActorVenue,
		Reason: reason,
	})
}

// StartCooking reports that the venue has started cooking an order. From here
// on the customer can no longer cancel it.
func (s *Service) StartCooking(
	ctx context.Context, venueID, orderID uuid.UUID,
) (order.Order, error) {
	return s.move(ctx, s.byVenue(venueID, orderID), order.Command{
		Event: order.EventStart,
		Actor: order.ActorVenue,
	})
}

// MarkReady reports that an order is cooked and waits to be picked up.
func (s *Service) MarkReady(
	ctx context.Context, venueID, orderID uuid.UUID,
) (order.Order, error) {
	return s.move(ctx, s.byVenue(venueID, orderID), order.Command{
		Event: order.EventReady,
		Actor: order.ActorVenue,
	})
}

// Handover reports that an order has left the venue with the courier.
func (s *Service) Handover(
	ctx context.Context, venueID, orderID uuid.UUID,
) (order.Order, error) {
	return s.move(ctx, s.byVenue(venueID, orderID), order.Command{
		Event: order.EventHandover,
		Actor: order.ActorVenue,
	})
}

func (s *Service) byCustomer(userID, orderID uuid.UUID) locate {
	return func(ctx context.Context) (order.Order, error) {
		return s.orders.LockForCustomer(ctx, userID, orderID)
	}
}

func (s *Service) byVenue(venueID, orderID uuid.UUID) locate {
	return func(ctx context.Context) (order.Order, error) {
		return s.orders.LockForVenue(ctx, venueID, orderID)
	}
}

// move applies one event to an order. The status, the history entry, the stock
// an ended order gives back and the events about all of it are one transaction:
// nobody learns of a transition that did not happen.
//
// An event that has already been applied changes nothing and is not an error.
func (s *Service) move(ctx context.Context, lock locate, cmd order.Command) (order.Order, error) {
	var result order.Order

	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		current, err := lock(ctx)
		if err != nil {
			return err
		}

		transition, err := order.Next(order.FulfillmentDelivery, current.Status, cmd.Event)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidTransition) {
				return domain.InvalidTransition(string(current.Status))
			}

			return fmt.Errorf("move order %s: %w", current.ID, err)
		}

		if !transition.Changed {
			result = current

			return nil
		}

		result, err = s.apply(ctx, current, order.StatusChange{
			From:       current.Status,
			To:         transition.To,
			Actor:      cmd.Actor,
			Reason:     cmd.Reason,
			EtaMinutes: cmd.EtaMinutes,
		})

		return err
	})
	if err != nil {
		return order.Order{}, err
	}

	return result, nil
}

// apply writes a transition that really moves the order and everything that
// follows from it.
func (s *Service) apply(
	ctx context.Context, current order.Order, change order.StatusChange,
) (order.Order, error) {
	applied, err := s.orders.ApplyStatus(ctx, current.ID, change)
	if err != nil {
		return order.Order{}, fmt.Errorf("apply status to order %s: %w", current.ID, err)
	}

	if change.To.ReturnsStock() {
		if err = s.menus.ReleaseStock(ctx, current.Venue.ID, current.Items); err != nil {
			return order.Order{}, fmt.Errorf("release stock of order %s: %w", current.ID, err)
		}
	}

	message, err := statusChanged(s.topics.Status, applied, change)
	if err != nil {
		return order.Order{}, err
	}

	if err = s.outbox.Append(ctx, message); err != nil {
		return order.Order{}, fmt.Errorf("append transition of order %s to outbox: %w", current.ID, err)
	}

	if !tellsVenue(change) {
		return applied.Order, nil
	}

	if message, err = orderCancelled(s.topics.Orders, applied.Order, change.Reason); err != nil {
		return order.Order{}, err
	}

	if err = s.outbox.Append(ctx, message); err != nil {
		return order.Order{}, fmt.Errorf("append cancellation of order %s to outbox: %w", current.ID, err)
	}

	return applied.Order, nil
}
