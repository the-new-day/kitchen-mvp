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
