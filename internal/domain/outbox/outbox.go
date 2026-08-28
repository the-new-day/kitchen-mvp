// Package outbox holds the envelope of an event of the platform. Every domain
// change somebody else has to learn about is written as one of these in the
// same transaction as the change itself; publishing it to the broker happens
// later and elsewhere.
package outbox

import "github.com/google/uuid"

// EventOrderCreated is published when a customer has placed an order.
const EventOrderCreated = "order.created"

// Message is one event waiting to be published. Key is the partition key of
// the topic, AggregateVersion the version of the aggregate after the change,
// and Payload the already serialised body of the event.
type Message struct {
	Topic            string
	Key              string
	EventType        string
	AggregateID      uuid.UUID
	AggregateVersion int64
	Payload          []byte
}
