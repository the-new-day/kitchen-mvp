// Package sse carries the status of an order to the customers watching it:
// a hub of subscribers inside the process and the writing of the event stream
// itself. Routing is not computed -- an update is offered to every instance,
// and an instance that holds nobody watching the order does nothing with it.
package sse

import (
	"avito-kitchen/internal/domain/order"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Update is one transition of an order as the hub carries it. Seq orders the
// transitions of one order and is what a reconnecting client continues from.
type Update struct {
	OrderID    uuid.UUID
	Seq        int64
	From       order.Status
	To         order.Status
	Actor      order.Actor
	Reason     string
	EtaMinutes *int
	ChangedAt  time.Time
}

// Subscriber is one open stream. Updates carries what the subscriber has to
// send; Done is closed when the hub gives up on it, and the handler owning the
// connection closes the stream itself.
type Subscriber struct {
	updates chan Update
	done    chan struct{}
	once    sync.Once
}

// Updates returns the channel the subscriber receives on.
func (s *Subscriber) Updates() <-chan Update {
	return s.updates
}

// Done is closed when the subscriber is to stop.
func (s *Subscriber) Done() <-chan struct{} {
	return s.done
}

// stop wakes the handler of the subscriber. It is safe to call twice: the hub
// may reach a subscriber again before its handler has woken up.
func (s *Subscriber) stop() {
	s.once.Do(func() { close(s.done) })
}

// Hub holds the subscribers of every order being watched in this process.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[*Subscriber]struct{}
	buffer      int
	lagged      atomic.Int64
	log         *slog.Logger
}

// NewHub returns an empty hub giving every subscriber a queue of buffer
// updates.
func NewHub(buffer int, log *slog.Logger) *Hub {
	return &Hub{
		subscribers: make(map[uuid.UUID]map[*Subscriber]struct{}),
		buffer:      buffer,
		log:         log,
	}
}

// Subscribe opens a stream of the updates of one order. The caller is expected
// to Unsubscribe it in a defer.
func (h *Hub) Subscribe(orderID uuid.UUID) *Subscriber {
	subscriber := &Subscriber{
		updates: make(chan Update, h.buffer),
		done:    make(chan struct{}),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	watchers, ok := h.subscribers[orderID]
	if !ok {
		watchers = make(map[*Subscriber]struct{})
		h.subscribers[orderID] = watchers
	}

	watchers[subscriber] = struct{}{}

	return subscriber
}

// Unsubscribe closes a stream and forgets the order it watched once nobody
// watches it any more.
func (h *Hub) Unsubscribe(orderID uuid.UUID, subscriber *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()

	watchers, ok := h.subscribers[orderID]
	if !ok {
		return
	}

	delete(watchers, subscriber)

	if len(watchers) == 0 {
		delete(h.subscribers, orderID)
	}

	subscriber.stop()
}

// Publish offers an update to everyone watching the order it belongs to.
// A subscriber whose queue is full is not waited for: it is asked to stop, and
// the client reconnects and asks for what it missed by its number.
func (h *Hub) Publish(update Update) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for subscriber := range h.subscribers[update.OrderID] {
		select {
		case subscriber.updates <- update:
		default:
			subscriber.stop()

			h.log.Warn("sse subscriber fell behind, closing its stream",
				slog.String("order_id", update.OrderID.String()),
				slog.Int64("sse_subscriber_lagged_total", h.lagged.Add(1)),
			)
		}
	}
}

// Lagged is how many streams the hub has closed because the client was not
// reading them. A growing number means the queue of a subscriber is too short.
func (h *Hub) Lagged() int64 {
	return h.lagged.Load()
}
