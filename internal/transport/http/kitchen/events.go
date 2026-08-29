package kitchen

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/transport/sse"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Names of the events the stream of an order carries.
const (
	snapshotEvent      = "snapshot"
	statusChangedEvent = "status_changed"
)

// lastEventHeader is how a reconnecting client says what it has already seen.
const lastEventHeader = "Last-Event-ID"

// statusChanged is one transition of an order as the stream sends it.
type statusChanged struct {
	OrderID    uuid.UUID    `json:"order_id"`
	Seq        int64        `json:"seq"`
	FromStatus order.Status `json:"from_status,omitempty"`
	Status     order.Status `json:"status"`
	Actor      order.Actor  `json:"actor"`
	Reason     string       `json:"reason,omitempty"`
	EtaMinutes *int         `json:"eta_minutes,omitempty"`
	ChangedAt  time.Time    `json:"changed_at"`
}

// streamOrderEvents serves GET /orders/{order_id}/events. The operation is
// kept out of the generated server: it answers with a stream that lives as
// long as the order does, not with a body.
func (s *Server) streamOrderEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := requireUser(ctx)
	if err != nil {
		s.errs.Response(w, r, err)

		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "order_id"))
	if err != nil {
		s.errs.Response(w, r, domain.InvalidArgumentf("order_id must be a UUID"))

		return
	}

	// The subscription is opened before the order is read: a status changing
	// in between waits in the queue instead of being missed by both.
	subscriber := s.hub.Subscribe(orderID)
	defer s.hub.Unsubscribe(orderID, subscriber)

	seen := lastEventID(r)

	timeline, err := s.order.Follow(ctx, userID, orderID, seen)
	if err != nil {
		s.errs.Response(w, r, err)

		return
	}

	stream := sse.NewStream(w)

	if err := stream.Send(sse.Event{Name: snapshotEvent, Data: toOrder(timeline.Order)}); err != nil {
		return
	}

	for _, entry := range timeline.Missed {
		if err := stream.Send(statusEvent(toUpdate(orderID, entry))); err != nil {
			return
		}

		seen = entry.Seq
	}

	if timeline.Order.Status.IsTerminal() {
		return
	}

	s.follow(r, stream, subscriber, seen)
}

// follow writes the transitions of an order until it ends, the client leaves
// or the hub gives up on a stream nobody is reading.
func (s *Server) follow(
	r *http.Request, stream *sse.Stream, subscriber *sse.Subscriber, seen int64,
) {
	heartbeat := time.NewTicker(s.heartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-subscriber.Done():
			return
		case <-heartbeat.C:
			if err := stream.Heartbeat(); err != nil {
				return
			}
		case update := <-subscriber.Updates():
			if update.Seq <= seen {
				continue
			}

			if err := stream.Send(statusEvent(update)); err != nil {
				return
			}

			seen = update.Seq

			if update.To.IsTerminal() {
				return
			}
		}
	}
}

// lastEventID is the number of the last entry the client has seen. A header
// that is missing or is not a number means it has seen nothing.
func lastEventID(r *http.Request) int64 {
	seen, err := strconv.ParseInt(r.Header.Get(lastEventHeader), 10, 64)
	if err != nil || seen < 0 {
		return 0
	}

	return seen
}

// toUpdate reads an entry of the status history as the update it once was. The
// history keeps no estimate: the current one is in the snapshot of the order.
func toUpdate(orderID uuid.UUID, entry order.StatusEntry) sse.Update {
	return sse.Update{
		OrderID:   orderID,
		Seq:       entry.Seq,
		From:      entry.From,
		To:        entry.To,
		Actor:     entry.Actor,
		Reason:    entry.Reason,
		ChangedAt: entry.ChangedAt,
	}
}

// statusEvent turns an update into the frame the client reads. The number of
// the entry is the id of the event: it is what a reconnecting client asks to
// continue from.
func statusEvent(update sse.Update) sse.Event {
	return sse.Event{
		ID:   update.Seq,
		Name: statusChangedEvent,
		Data: statusChanged{
			OrderID:    update.OrderID,
			Seq:        update.Seq,
			FromStatus: update.From,
			Status:     update.To,
			Actor:      update.Actor,
			Reason:     update.Reason,
			EtaMinutes: update.EtaMinutes,
			ChangedAt:  update.ChangedAt,
		},
	}
}
