package worker

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/domain/outbox"
	"context"
	"fmt"
	"log/slog"
)

// Messages are the events waiting to be published.
type Messages interface {
	Fetch(ctx context.Context, limit int) ([]outbox.Pending, error)
	MarkPublished(ctx context.Context, ids []int64) error
	MarkFailed(ctx context.Context, id int64, cause error) error
}

// Publisher sends one message to the broker and
// returns once it has been acknowledged.
type Publisher interface {
	Publish(ctx context.Context, message outbox.Pending) error
}

// Outbox publishes what the domain has written, in the order it was written.
type Outbox struct {
	tx        Transactor
	messages  Messages
	publisher Publisher
	cfg       config.OutboxJob
	log       *slog.Logger
}

// NewOutbox returns the publishing job.
func NewOutbox(
	tx Transactor,
	messages Messages,
	publisher Publisher,
	cfg config.OutboxJob,
	log *slog.Logger,
) *Outbox {
	return &Outbox{
		tx:        tx,
		messages:  messages,
		publisher: publisher,
		cfg:       cfg,
		log:       log.With(slog.String("job", "outbox")),
	}
}

// Name returns the name of the job.
func (o *Outbox) Name() string { return "outbox" }

// Run publishes batch after batch until ctx is cancelled, waiting between them
// for as long as the pace allows.
func (o *Outbox) Run(ctx context.Context) {
	rate := newPace(o.cfg.PollMin, o.cfg.PollMax)

	for {
		sent, err := o.publishBatch(ctx)
		if err != nil && ctx.Err() == nil {
			o.log.ErrorContext(ctx, "publishing failed", slog.String("error", err.Error()))
		}

		if !pause(ctx, rate.next(sent, o.cfg.BatchSize, err != nil)) {
			return
		}
	}
}

// publishBatch takes the next batch, publishes it message by message and
// records how far it got. The batch is held by the transaction while it is
// being published, so no other worker takes the same messages; a message that
// did not reach the broker stops the batch, keeps its place in the queue and
// leaves the rest for the next run: the order of the events of one aggregate
// is what all of this protects.
func (o *Outbox) publishBatch(ctx context.Context) (int, error) {
	var (
		sent    int
		failure error
	)

	err := o.tx.InTx(ctx, func(ctx context.Context) error {
		batch, err := o.messages.Fetch(ctx, o.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("fetch outbox batch: %w", err)
		}

		published := make([]int64, 0, len(batch))

		for _, message := range batch {
			if err := o.publisher.Publish(ctx, message); err != nil {
				failure = err

				if err := o.messages.MarkFailed(ctx, message.ID, failure); err != nil {
					return err
				}

				break
			}

			published = append(published, message.ID)
		}

		if len(published) > 0 {
			if err := o.messages.MarkPublished(ctx, published); err != nil {
				return err
			}
		}

		sent = len(published)

		return nil
	})
	if err != nil {
		return 0, err
	}

	return sent, failure
}
