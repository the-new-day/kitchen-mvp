package kafka

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Transactor runs a unit of work in one database transaction.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Inbox remembers the events a consumer has already taken. Remember reports
// whether the event is new: an event seen before is not to be handled again.
type Inbox interface {
	Remember(ctx context.Context, consumer string, eventID uuid.UUID) (fresh bool, err error)
}

// Deduplicated wraps a handler so that a redelivered event is skipped. The
// mark and the handling share one transaction: an event is remembered exactly
// when the work it caused is committed, so delivery is at-least-once and the
// effect is once.
func Deduplicated(tx Transactor, inbox Inbox, consumer string, next Handler) Handler {
	return func(ctx context.Context, envelope Envelope) error {
		return tx.InTx(ctx, func(ctx context.Context) error {
			fresh, err := inbox.Remember(ctx, consumer, envelope.EventID)
			if err != nil {
				return fmt.Errorf("remember event %s: %w", envelope.EventID, err)
			}

			if !fresh {
				return nil
			}

			return next(ctx, envelope)
		})
	}
}
