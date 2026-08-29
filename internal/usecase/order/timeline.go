package order

import (
	"avito-kitchen/internal/domain/order"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Timeline is what a customer needs to start following an order: the order as
// it stands and the transitions it has made since the one the customer last
// saw. Missed is the whole history when nothing was seen yet.
type Timeline struct {
	Order  order.Order
	Missed []order.StatusEntry
}

// Follow returns the timeline of an order of a customer, starting after the
// entry number after. An order of somebody else is reported as
// domain.ErrNotFound, exactly as reading it is.
func (s *Service) Follow(
	ctx context.Context, userID, orderID uuid.UUID, after int64,
) (Timeline, error) {
	found, err := s.Order(ctx, userID, orderID)
	if err != nil {
		return Timeline{}, err
	}

	missed, err := s.orders.History(ctx, orderID, after)
	if err != nil {
		return Timeline{}, fmt.Errorf("get history of order %s: %w", orderID, err)
	}

	return Timeline{Order: found, Missed: missed}, nil
}
