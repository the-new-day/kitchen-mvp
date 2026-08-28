package order

import "avito-kitchen/internal/domain"

// Fulfillment is how an order reaches the customer.
type Fulfillment string

// FulfillmentDelivery is the only way an order is fulfilled today.
const FulfillmentDelivery Fulfillment = "DELIVERY"

// Event is what happens to an order.
type Event string

// Events the state machine accepts.
const (
	EventAccept   Event = "accept"
	EventReject   Event = "reject"
	EventStart    Event = "start"
	EventReady    Event = "ready"
	EventHandover Event = "handover"
	EventDeliver  Event = "deliver"
	EventCancel   Event = "cancel"
)

// Events returns every event the state machine accepts.
func Events() []Event {
	return []Event{
		EventAccept, EventReject, EventStart,
		EventReady, EventHandover, EventDeliver, EventCancel,
	}
}

// transitionKey is what an allowed transition is looked up by.
type transitionKey struct {
	Fulfillment Fulfillment
	From        Status
	Event       Event
}

// transitions is the state machine itself: the whole life of an order is this
// table and nothing else.
var transitions = map[transitionKey]Status{
	{FulfillmentDelivery, StatusCreated, EventAccept}:     StatusAccepted,
	{FulfillmentDelivery, StatusCreated, EventReject}:     StatusRejected,
	{FulfillmentDelivery, StatusCreated, EventCancel}:     StatusCancelled,
	{FulfillmentDelivery, StatusAccepted, EventStart}:     StatusCooking,
	{FulfillmentDelivery, StatusAccepted, EventReject}:    StatusRejected,
	{FulfillmentDelivery, StatusAccepted, EventCancel}:    StatusCancelled,
	{FulfillmentDelivery, StatusCooking, EventReady}:      StatusReady,
	{FulfillmentDelivery, StatusReady, EventHandover}:     StatusDelivering,
	{FulfillmentDelivery, StatusDelivering, EventDeliver}: StatusDelivered,
}

// Transition is the outcome of an event. Changed is false when the order is
// already in the status the event leads to: repeating an event that has
// already been applied is not an error and moves nothing.
type Transition struct {
	To      Status
	Changed bool
}

// Next reports what an event does to an order in the given status. An event
// the state machine does not allow from that status is reported as
// domain.ErrInvalidTransition.
func Next(fulfillment Fulfillment, from Status, event Event) (Transition, error) {
	if to, ok := transitions[transitionKey{fulfillment, from, event}]; ok {
		return Transition{To: to, Changed: true}, nil
	}

	if repeats(fulfillment, from, event) {
		return Transition{To: from, Changed: false}, nil
	}

	return Transition{}, domain.ErrInvalidTransition
}

// repeats reports whether an order in this status has already been brought
// here by this very event.
func repeats(fulfillment Fulfillment, from Status, event Event) bool {
	for key, to := range transitions {
		if key.Fulfillment == fulfillment && key.Event == event && to == from {
			return true
		}
	}

	return false
}
