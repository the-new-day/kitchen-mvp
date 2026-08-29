package sse

import (
	"avito-kitchen/internal/domain/order"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	watchedOrder = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")
	anotherOrder = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f2")
)

func update(orderID uuid.UUID, seq int64) Update {
	return Update{
		OrderID:   orderID,
		Seq:       seq,
		From:      order.StatusCreated,
		To:        order.StatusAccepted,
		Actor:     order.ActorVenue,
		ChangedAt: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
	}
}

func newHub(t *testing.T, buffer int) *Hub {
	t.Helper()

	return NewHub(buffer, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// drain reads what a subscriber has been offered without waiting for more.
func drain(s *Subscriber) []Update {
	var got []Update

	for {
		select {
		case u := <-s.Updates():
			got = append(got, u)
		default:
			return got
		}
	}
}

func TestHubPublish(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		buffer     int
		published  []Update
		wantSeqs   []int64
		wantClosed bool
		wantLagged int64
	}{
		"an update reaches whoever watches the order": {
			buffer:    2,
			published: []Update{update(watchedOrder, 1), update(watchedOrder, 2)},
			wantSeqs:  []int64{1, 2},
		},
		"an update about another order is not offered": {
			buffer:    2,
			published: []Update{update(anotherOrder, 1)},
		},
		"a subscriber that is not reading is closed instead of waited for": {
			buffer: 1,
			published: []Update{
				update(watchedOrder, 1), update(watchedOrder, 2), update(watchedOrder, 3),
			},
			wantSeqs:   []int64{1},
			wantClosed: true,
			wantLagged: 2,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			hub := newHub(t, tc.buffer)
			subscriber := hub.Subscribe(watchedOrder)

			for _, u := range tc.published {
				hub.Publish(u)
			}

			var seqs []int64
			for _, u := range drain(subscriber) {
				seqs = append(seqs, u.Seq)
			}

			assert.Equal(t, tc.wantSeqs, seqs)
			assert.Equal(t, tc.wantLagged, hub.Lagged())

			select {
			case <-subscriber.Done():
				assert.True(t, tc.wantClosed, "the stream was closed, want it kept")
			default:
				assert.False(t, tc.wantClosed, "the stream is open, want it closed")
			}
		})
	}
}

// TestHubForgetsAnOrderNobodyWatches keeps the hub from growing a map entry per
// order ever streamed.
func TestHubForgetsAnOrderNobodyWatches(t *testing.T) {
	t.Parallel()

	hub := newHub(t, 1)

	first, second := hub.Subscribe(watchedOrder), hub.Subscribe(watchedOrder)
	require.Len(t, hub.subscribers[watchedOrder], 2)

	hub.Unsubscribe(watchedOrder, first)
	assert.Len(t, hub.subscribers[watchedOrder], 1)

	hub.Unsubscribe(watchedOrder, second)
	assert.Empty(t, hub.subscribers)

	// Unsubscribing wakes the handler that was reading the stream.
	select {
	case <-second.Done():
	default:
		t.Fatal("an unsubscribed stream is still open")
	}
}

// TestHubUnderConcurrentUse runs the hub the way the service does: many streams
// coming and going while the consumer publishes into it.
func TestHubUnderConcurrentUse(t *testing.T) {
	t.Parallel()

	hub := newHub(t, 4)

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			subscriber := hub.Subscribe(watchedOrder)
			defer hub.Unsubscribe(watchedOrder, subscriber)

			drain(subscriber)
		}()

		go func() {
			defer wg.Done()

			hub.Publish(update(watchedOrder, 1))
		}()
	}

	wg.Wait()

	assert.Empty(t, hub.subscribers)
}
