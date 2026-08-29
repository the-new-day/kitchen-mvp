// Package order holds the order of a customer, the state machine its status
// follows and the rules that turn a cart into an order.
package order

import (
	"time"

	"github.com/google/uuid"
)

// Status is the stage an order has reached.
type Status string

// Statuses an order goes through.
const (
	StatusCreated    Status = "CREATED"
	StatusAccepted   Status = "ACCEPTED"
	StatusCooking    Status = "COOKING"
	StatusReady      Status = "READY"
	StatusDelivering Status = "DELIVERING"
	StatusDelivered  Status = "DELIVERED"
	StatusRejected   Status = "REJECTED"
	StatusCancelled  Status = "CANCELLED"
)

// Statuses returns every status an order can be in.
func Statuses() []Status {
	return []Status{
		StatusCreated, StatusAccepted, StatusCooking, StatusReady,
		StatusDelivering, StatusDelivered, StatusRejected, StatusCancelled,
	}
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	for _, known := range Statuses() {
		if s == known {
			return true
		}
	}

	return false
}

// IsTerminal reports whether an order in this status has finished its life:
// no event takes it anywhere else.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusDelivered, StatusRejected, StatusCancelled:
		return true
	default:
		return false
	}
}

// Actor is who caused a change of status.
type Actor string

// Actors that appear in the status history.
const (
	ActorCustomer Actor = "customer"
	ActorVenue    Actor = "venue"
	ActorSystem   Actor = "system"
)

// Item is one position of an order. Name and Price are copies taken when the
// order was placed: an order is a financial document and is never recomputed.
type Item struct {
	MenuItemID uuid.UUID
	ExternalID string
	Name       string
	Price      int64
	Qty        int
}

// LineTotal is what the position costs at the price it was ordered at.
func (i Item) LineTotal() int64 {
	return int64(i.Qty) * i.Price
}

// Venue is the venue an order was placed at, as an order card shows it.
type Venue struct {
	ID   uuid.UUID
	Name string
}

// Order is a placed order. Money is in kopecks.
type Order struct {
	ID              uuid.UUID
	Number          string
	UserID          uuid.UUID
	Venue           Venue
	Status          Status
	Items           []Item
	ItemsTotal      int64
	DeliveryFee     int64
	Total           int64
	Address         string
	Phone           string
	Comment         string
	EtaMinutes      *int
	RejectionReason string
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Cursor is the position of the last row of a page of the order list: orders
// are listed newest first, and the identifier breaks ties between equal times.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Filter selects a page of the orders of one customer. An empty Statuses means
// the filter is not applied; Limit is the max number of rows the repository
// must return.
type Filter struct {
	UserID   uuid.UUID
	Statuses []Status
	After    *Cursor
	Limit    int
}

// StatusEntry is one entry of the status history of an order. Seq numbers the
// entries of one order and never repeats: it is what a client following the
// order continues from after a break.
type StatusEntry struct {
	Seq       int64
	From      Status
	To        Status
	Actor     Actor
	Reason    string
	ChangedAt time.Time
}
