package worker

import (
	"avito-kitchen/internal/config"
	"avito-kitchen/internal/domain/outbox"
	"avito-kitchen/internal/worker/mocks"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

const batchSize = 3

var errBroker = errors.New("broker unreachable")

// waiting is the message stored under id, with a key that puts the messages of
// one aggregate into one partition.
func waiting(id int64) outbox.Pending {
	return outbox.Pending{
		Message: outbox.Message{
			Topic:            "kitchen.orders.v1",
			Key:              "0192f4c1-0000-7000-8000-000000000001",
			EventType:        outbox.EventOrderCreated,
			AggregateVersion: 1,
			Payload:          []byte(`{}`),
		},
		ID:         id,
		EventID:    uuid.New(),
		OccurredAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
	}
}

// repositories is the set of mocks one case of the table is served by.
type repositories struct {
	messages  *mocks.MockMessages
	publisher *mocks.MockPublisher
}

func newOutbox(t *testing.T, setup func(repositories)) *Outbox {
	t.Helper()

	repos := repositories{
		messages:  mocks.NewMockMessages(t),
		publisher: mocks.NewMockPublisher(t),
	}

	setup(repos)

	tx := mocks.NewMockTransactor(t)
	tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Once()

	cfg := config.OutboxJob{BatchSize: batchSize, PollMin: time.Millisecond, PollMax: time.Millisecond}

	return NewOutbox(tx, repos.messages, repos.publisher, cfg, slog.New(slog.DiscardHandler))
}

// TestOutboxPublishBatch covers what one run of the job does to a batch: the
// messages go out in the order they were written, a message counts as
// published only after the broker has acknowledged it, and the first one that
// did not make it stops the batch and keeps its place in the queue.
func TestOutboxPublishBatch(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		batch     []outbox.Pending
		fetchErr  error
		failAt    int64
		wantSent  int
		wantOrder []int64
		wantErr   error
	}{
		"a whole batch is published in the order it was written": {
			batch:     []outbox.Pending{waiting(1), waiting(2), waiting(3)},
			wantSent:  3,
			wantOrder: []int64{1, 2, 3},
		},
		"an empty table publishes nothing": {
			batch: nil,
		},
		"the message that did not reach the broker stops the batch": {
			batch:     []outbox.Pending{waiting(1), waiting(2), waiting(3)},
			failAt:    2,
			wantSent:  1,
			wantOrder: []int64{1},
			wantErr:   errBroker,
		},
		"a broker that answers nothing publishes nothing": {
			batch:   []outbox.Pending{waiting(1), waiting(2)},
			failAt:  1,
			wantErr: errBroker,
		},
		"a database that cannot be read is reported": {
			fetchErr: errBroker,
			wantErr:  errBroker,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var published []int64

			job := newOutbox(t, func(r repositories) {
				r.messages.EXPECT().Fetch(mock.Anything, batchSize).
					Return(tc.batch, tc.fetchErr).Once()

				if tc.fetchErr != nil {
					return
				}

				var sent []int64

				r.publisher.EXPECT().Publish(mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, m outbox.Pending) error {
						if m.ID == tc.failAt {
							return errBroker
						}

						sent = append(sent, m.ID)

						return nil
					}).Maybe()

				if tc.failAt != 0 {
					r.messages.EXPECT().MarkFailed(mock.Anything, tc.failAt, mock.Anything).
						Return(nil).Once()
				}

				if tc.wantSent > 0 {
					r.messages.EXPECT().MarkPublished(mock.Anything, mock.Anything).
						RunAndReturn(func(_ context.Context, ids []int64) error {
							published = append(published, ids...)

							if len(sent) != len(ids) {
								t.Errorf("marked %d published, acknowledged %d", len(ids), len(sent))
							}

							return nil
						}).Once()
				}
			})

			sent, err := job.publishBatch(context.Background())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("publishBatch = %v, want %v", err, tc.wantErr)
			}

			if sent != tc.wantSent {
				t.Fatalf("sent = %d, want %d", sent, tc.wantSent)
			}

			if len(published) != len(tc.wantOrder) {
				t.Fatalf("published %v, want %v", published, tc.wantOrder)
			}

			for i, id := range tc.wantOrder {
				if published[i] != id {
					t.Fatalf("published %v, want %v", published, tc.wantOrder)
				}
			}
		})
	}
}
