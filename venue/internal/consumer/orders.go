// Package consumer reads the orders the platform sends to the venue. The
// venue connects to the broker itself and takes every event once: delivery is
// at-least-once, so an event already seen is passed over.
package consumer

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/venue/internal/kitchen"
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Kitchen is what the events are handed to.
type Kitchen interface {
	Receive(ctx context.Context, placed kitchen.OrderCreated) error
	Cancel(ctx context.Context, cancelled kitchen.OrderCancelled) error
}

// Orders turns the events of the topic into the work of the kitchen. VenueID
// is the venue this service runs: the topic carries the orders of every venue,
// keyed by venue, and the ones of the others are none of its business.
type Orders struct {
	kitchen Kitchen
	venueID uuid.UUID
	log     *slog.Logger
}

// NewOrders returns a handler of the order topic for one venue.
func NewOrders(kitchen Kitchen, venueID uuid.UUID, log *slog.Logger) *Orders {
	return &Orders{kitchen: kitchen, venueID: venueID, log: log}
}

// Handle takes one event. An event that cannot be read is reported as
// malformed: another delivery of it would fail in exactly the same way.
func (o *Orders) Handle(ctx context.Context, envelope broker.Envelope) error {
	switch envelope.EventType {
	case kitchen.TypeOrderCreated:
		return o.created(ctx, envelope)
	case kitchen.TypeOrderCancelled:
		return o.cancelled(ctx, envelope)
	default:
		o.log.DebugContext(ctx, "event is not for the kitchen",
			slog.String("event_type", envelope.EventType))

		return nil
	}
}

func (o *Orders) created(ctx context.Context, envelope broker.Envelope) error {
	var placed kitchen.OrderCreated

	if err := broker.UnmarshalPayload(envelope, &placed); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	if err := placed.Validate(); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	if !o.ours(ctx, placed.VenueID) {
		return nil
	}

	return o.kitchen.Receive(ctx, placed)
}

func (o *Orders) cancelled(ctx context.Context, envelope broker.Envelope) error {
	var cancelled kitchen.OrderCancelled

	if err := broker.UnmarshalPayload(envelope, &cancelled); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	if err := cancelled.Validate(); err != nil {
		return fmt.Errorf("%w: %w", broker.ErrMalformed, err)
	}

	if !o.ours(ctx, cancelled.VenueID) {
		return nil
	}

	return o.kitchen.Cancel(ctx, cancelled)
}

// ours reports whether an event is about the venue this service runs.
func (o *Orders) ours(ctx context.Context, venueID uuid.UUID) bool {
	if venueID == o.venueID {
		return true
	}

	o.log.DebugContext(ctx, "event belongs to another venue",
		slog.String("venue_id", venueID.String()))

	return false
}
