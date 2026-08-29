package kitchen

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event types the venue reads from its topic.
const (
	TypeOrderCreated   = "order.created"
	TypeOrderCancelled = "order.cancelled"
)

// OrderCreated is the body of an order.created event. The venue declares it
// after the published schema of the platform rather than sharing a package
// with it: a partner integrates against the contract and nothing else.
type OrderCreated struct {
	OrderID     uuid.UUID   `json:"order_id"`
	Number      string      `json:"number"`
	VenueID     uuid.UUID   `json:"venue_id"`
	Items       []OrderItem `json:"items"`
	ItemsTotal  int64       `json:"items_total"`
	DeliveryFee int64       `json:"delivery_fee"`
	Total       int64       `json:"total"`
	Address     string      `json:"address,omitempty"`
	Phone       string      `json:"phone"`
	Comment     string      `json:"comment,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// OrderItem is one position of an order, identified by the venue's own SKU.
type OrderItem struct {
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Price      int64  `json:"price"`
	Qty        int    `json:"qty"`
	LineTotal  int64  `json:"line_total"`
}

// Validate reports whether the event carries an order the venue can work on.
func (o OrderCreated) Validate() error {
	if o.OrderID == uuid.Nil {
		return fmt.Errorf("order without an identifier")
	}

	if o.VenueID == uuid.Nil {
		return fmt.Errorf("order %s without a venue", o.OrderID)
	}

	if len(o.Items) == 0 {
		return fmt.Errorf("order %s without items", o.OrderID)
	}

	for _, item := range o.Items {
		if item.ExternalID == "" {
			return fmt.Errorf("order %s carries a position without a sku", o.OrderID)
		}

		if item.Qty <= 0 {
			return fmt.Errorf("order %s carries %s in a quantity of %d", o.OrderID, item.ExternalID, item.Qty)
		}
	}

	return nil
}

// OrderCancelled is the body of an order.cancelled event: an order the venue
// was given is taken away by someone other than the venue itself.
type OrderCancelled struct {
	OrderID     uuid.UUID `json:"order_id"`
	Number      string    `json:"number"`
	VenueID     uuid.UUID `json:"venue_id"`
	Reason      string    `json:"reason,omitempty"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// Validate reports whether the event names an order the venue can look up.
func (o OrderCancelled) Validate() error {
	if o.OrderID == uuid.Nil {
		return fmt.Errorf("cancellation without an order")
	}

	if o.VenueID == uuid.Nil {
		return fmt.Errorf("cancellation of order %s without a venue", o.OrderID)
	}

	return nil
}
