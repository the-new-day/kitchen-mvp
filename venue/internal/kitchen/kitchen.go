// Package kitchen is the domain of the demo venue: the dishes it sells, the
// orders it takes and the states an order passes through on its way out.
package kitchen

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a dish or an order the venue was asked about is
// not in its books.
var ErrNotFound = errors.New("not found")

// State is where an order stands in the kitchen.
type State string

// States an order can be in.
const (
	StateNew        State = "NEW"
	StateAccepted   State = "ACCEPTED"
	StateCooking    State = "COOKING"
	StateReady      State = "READY"
	StateHandedOver State = "HANDED_OVER"
	StateRejected   State = "REJECTED"
	StateCancelled  State = "CANCELLED"
)

// Dish is one position of the venue's own nomenclature. Stock is nil when the
// venue never runs out of it.
type Dish struct {
	SKU         string
	Name        string
	Description string
	Price       int64
	Stock       *int
	IsAvailable bool
	Category    Category
	Position    int
}

// Category groups the dishes the way the venue shows them.
type Category struct {
	ExternalID string
	Name       string
	Position   int
}

// Order is an order of the platform as the venue keeps it.
type Order struct {
	ID             uuid.UUID
	State          State
	Placed         OrderCreated
	ReceivedAt     time.Time
	StateChangedAt time.Time
	DecidedAt      *time.Time
}
