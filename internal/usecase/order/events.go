package order

import (
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/domain/outbox"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// orderCreatedPayload is the body of the order.created event: everything the
// venue needs to start cooking, so that it never has to call back for it.
type orderCreatedPayload struct {
	OrderID     uuid.UUID        `json:"order_id"`
	Number      string           `json:"number"`
	VenueID     uuid.UUID        `json:"venue_id"`
	Status      order.Status     `json:"status"`
	Items       []orderItemEvent `json:"items"`
	ItemsTotal  int64            `json:"items_total"`
	DeliveryFee int64            `json:"delivery_fee"`
	Total       int64            `json:"total"`
	Address     string           `json:"address,omitempty"`
	Phone       string           `json:"phone"`
	Comment     string           `json:"comment,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
}

// orderItemEvent is one position of an order as the event carries it.
type orderItemEvent struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Price      int64  `json:"price"`
	Qty        int    `json:"qty"`
	LineTotal  int64  `json:"line_total"`
}

// orderCreated builds the event about a placed order. The partition key is the
// venue, so that the orders of one venue keep their order.
func orderCreated(topic string, placed order.Order) (outbox.Message, error) {
	items := make([]orderItemEvent, 0, len(placed.Items))
	for _, item := range placed.Items {
		items = append(items, orderItemEvent{
			ExternalID: item.ExternalID,
			Name:       item.Name,
			Price:      item.Price,
			Qty:        item.Qty,
			LineTotal:  item.LineTotal(),
		})
	}

	payload, err := json.Marshal(orderCreatedPayload{
		OrderID:     placed.ID,
		Number:      placed.Number,
		VenueID:     placed.Venue.ID,
		Status:      placed.Status,
		Items:       items,
		ItemsTotal:  placed.ItemsTotal,
		DeliveryFee: placed.DeliveryFee,
		Total:       placed.Total,
		Address:     placed.Address,
		Phone:       placed.Phone,
		Comment:     placed.Comment,
		CreatedAt:   placed.CreatedAt,
	})
	if err != nil {
		return outbox.Message{}, fmt.Errorf("marshal order %s: %w", placed.ID, err)
	}

	return outbox.Message{
		Topic:            topic,
		Key:              placed.Venue.ID.String(),
		EventType:        outbox.EventOrderCreated,
		AggregateID:      placed.ID,
		AggregateVersion: placed.Version,
		Payload:          payload,
	}, nil
}

// orderCancelledPayload is the body of the order.cancelled event: the venue is
// told to stop working on an order it did not stop itself.
type orderCancelledPayload struct {
	OrderID     uuid.UUID `json:"order_id"`
	Number      string    `json:"number"`
	VenueID     uuid.UUID `json:"venue_id"`
	Reason      string    `json:"reason,omitempty"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// orderCancelled builds the event that takes an order away from a venue. It
// goes to the topic of the orders and is keyed by the venue, so that a venue
// never sees an order taken away before it was given.
func orderCancelled(topic string, cancelled order.Order, reason string) (outbox.Message, error) {
	payload, err := json.Marshal(orderCancelledPayload{
		OrderID:     cancelled.ID,
		Number:      cancelled.Number,
		VenueID:     cancelled.Venue.ID,
		Reason:      reason,
		CancelledAt: cancelled.UpdatedAt,
	})
	if err != nil {
		return outbox.Message{}, fmt.Errorf("marshal cancellation of order %s: %w", cancelled.ID, err)
	}

	return outbox.Message{
		Topic:            topic,
		Key:              cancelled.Venue.ID.String(),
		EventType:        outbox.EventOrderCancelled,
		AggregateID:      cancelled.ID,
		AggregateVersion: cancelled.Version,
		Payload:          payload,
	}, nil
}

// orderStatusChangedPayload is the body of the order.status_changed event: one
// transition, with the number that orders it among the others of the order.
type orderStatusChangedPayload struct {
	OrderID    uuid.UUID    `json:"order_id"`
	UserID     uuid.UUID    `json:"user_id"`
	VenueID    uuid.UUID    `json:"venue_id"`
	FromStatus order.Status `json:"from_status,omitempty"`
	ToStatus   order.Status `json:"to_status"`
	Actor      order.Actor  `json:"actor"`
	Reason     string       `json:"reason,omitempty"`
	EtaMinutes *int         `json:"eta_minutes,omitempty"`
	Seq        int64        `json:"seq"`
	ChangedAt  time.Time    `json:"changed_at"`
}

// statusChanged builds the event about a transition of an order. It goes to the
// status topic and is keyed by the order, so that the statuses of one order
// keep their order.
func statusChanged(
	topic string, applied order.Applied, change order.StatusChange,
) (outbox.Message, error) {
	moved := applied.Order

	payload, err := json.Marshal(orderStatusChangedPayload{
		OrderID:    moved.ID,
		UserID:     moved.UserID,
		VenueID:    moved.Venue.ID,
		FromStatus: change.From,
		ToStatus:   change.To,
		Actor:      change.Actor,
		Reason:     change.Reason,
		EtaMinutes: moved.EtaMinutes,
		Seq:        applied.Seq,
		ChangedAt:  moved.UpdatedAt,
	})
	if err != nil {
		return outbox.Message{}, fmt.Errorf("marshal transition of order %s: %w", moved.ID, err)
	}

	return outbox.Message{
		Topic:            topic,
		Key:              moved.ID.String(),
		EventType:        outbox.EventOrderStatusChanged,
		AggregateID:      moved.ID,
		AggregateVersion: moved.Version,
		Payload:          payload,
	}, nil
}

// tellsVenue reports whether a venue has to be told that an order it was given
// is over. A venue that ended the order itself already knows: it was answered
// synchronously. One that did not -- the customer cancelled, or the platform
// gave up waiting -- learns it through the topic it reads its orders from.
func tellsVenue(change order.StatusChange) bool {
	return change.To.ReturnsStock() && change.Actor != order.ActorVenue
}
