// Package consumer reads the events of the platform back from the broker. The
// status of an order is changed by one process and published by another, so
// the process holding the stream of a customer open learns of it here.
package consumer

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/transport/sse"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// typeOrderStatusChanged is the event a change of status is announced with.
const typeOrderStatusChanged = "order.status_changed"

// Hub is where an update is offered to the customers watching the order.
type Hub interface {
	Publish(update sse.Update)
}

// statusChanged is the body of an order.status_changed event.
type statusChanged struct {
	OrderID    uuid.UUID    `json:"order_id"`
	FromStatus order.Status `json:"from_status"`
	ToStatus   order.Status `json:"to_status"`
	Actor      order.Actor  `json:"actor"`
	Reason     string       `json:"reason"`
	EtaMinutes *int         `json:"eta_minutes"`
	Seq        int64        `json:"seq"`
	ChangedAt  time.Time    `json:"changed_at"`
}

// Validate reports whether the event names a transition that can be followed.
func (s statusChanged) Validate() error {
	if s.OrderID == uuid.Nil {
		return fmt.Errorf("transition without an order")
	}

	if !s.ToStatus.Valid() {
		return fmt.Errorf("order %s moved to an unknown status %q", s.OrderID, s.ToStatus)
	}

	if s.Seq <= 0 {
		return fmt.Errorf("transition of order %s without a number", s.OrderID)
	}

	return nil
}

// Status hands the transitions of the orders over to the hub.
type Status struct {
	hub Hub
	log *slog.Logger
}

// NewStatus returns a handler publishing into hub.
func NewStatus(hub Hub, log *slog.Logger) *Status {
	return &Status{hub: hub, log: log}
}

// Handle offers one transition to whoever is watching the order it belongs to.
// Nobody watching is the normal case and not a failure: the truth about an
// order is in the database, and the stream only saves the customer a request.
func (s *Status) Handle(ctx context.Context, envelope broker.Envelope) error {
	if envelope.EventType != typeOrderStatusChanged {
		return nil
	}

	var event statusChanged

	if err := broker.UnmarshalPayload(envelope, &event); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	if err := event.Validate(); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	s.log.DebugContext(ctx, "order status changed",
		slog.String("order_id", event.OrderID.String()),
		slog.String("status", string(event.ToStatus)),
		slog.Int64("seq", event.Seq),
	)

	s.hub.Publish(sse.Update{
		OrderID:    event.OrderID,
		Seq:        event.Seq,
		From:       event.FromStatus,
		To:         event.ToStatus,
		Actor:      event.Actor,
		Reason:     event.Reason,
		EtaMinutes: event.EtaMinutes,
		ChangedAt:  event.ChangedAt,
	})

	return nil
}
