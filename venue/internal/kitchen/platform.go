package kitchen

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnauthorized is the platform refusing the key of the venue.
var ErrUnauthorized = errors.New("api key is missing, unknown or revoked")

// RefusedError is the platform turning a move down because the order is no
// longer where the venue thinks it is. Current is the status it holds instead.
type RefusedError struct {
	Current string
}

// Error says what the platform refused.
func (e RefusedError) Error() string {
	if e.Current == "" {
		return "platform refused the transition"
	}

	return fmt.Sprintf("platform refused the transition: order is %s", e.Current)
}

// states maps the statuses of the platform onto the states of the kitchen: the
// venue and the platform describe an order in their own words, and this table
// is the whole of the translation.
var states = map[string]State{
	"CREATED":    StateNew,
	"ACCEPTED":   StateAccepted,
	"COOKING":    StateCooking,
	"READY":      StateReady,
	"DELIVERING": StateHandedOver,
	"DELIVERED":  StateHandedOver,
	"REJECTED":   StateRejected,
	"CANCELLED":  StateCancelled,
}

// StateOf returns the state an order is in for the kitchen, given the status
// the platform keeps for it.
func StateOf(status string) (State, bool) {
	state, ok := states[status]

	return state, ok
}

// ReturnsStock reports whether an order leaving into this state gives the
// dishes it holds back to the venue.
func ReturnsStock(state State) bool {
	return state == StateRejected || state == StateCancelled
}

// Due is one state and the moment an order sitting in it has waited long
// enough to be moved on.
type Due struct {
	State  State
	Cutoff time.Time
}
