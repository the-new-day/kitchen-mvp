// Package kafka carries the events of the platform to and from the broker:
// the envelope every message is wrapped in, a synchronous producer, a consumer
// bound to a group and the deduplication a consumer needs to survive redelivery.
package kafka

import (
	"avito-kitchen/internal/domain/outbox"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Envelope is what every message on every topic looks like. EventID is the
// deduplication key of the consumer, OccurredAt the moment of the domain
// change, Version the version of the aggregate after it.
type Envelope struct {
	EventID     uuid.UUID       `json:"event_id"`
	EventType   string          `json:"event_type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	Version     int64           `json:"version"`
	Payload     json.RawMessage `json:"payload"`
}

// Wrap puts a stored message into its envelope.
func Wrap(message outbox.Pending) Envelope {
	return Envelope{
		EventID:     message.EventID,
		EventType:   message.EventType,
		OccurredAt:  message.OccurredAt,
		AggregateID: message.AggregateID,
		Version:     message.AggregateVersion,
		Payload:     message.Payload,
	}
}

// Encode serialises an envelope for the wire.
func Encode(envelope Envelope) ([]byte, error) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal event %s: %w", envelope.EventID, err)
	}

	return body, nil
}

// Decode reads an envelope off the wire.
func Decode(body []byte) (Envelope, error) {
	var envelope Envelope

	if err := json.Unmarshal(body, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("%w: unmarshal event: %w", ErrMalformed, err)
	}

	if envelope.EventID == uuid.Nil {
		return Envelope{}, fmt.Errorf("%w: event without an identifier", ErrMalformed)
	}

	if envelope.EventType == "" {
		return Envelope{}, fmt.Errorf("%w: event %s without a type", ErrMalformed, envelope.EventID)
	}

	return envelope, nil
}

// UnmarshalPayload reads the body of an event into a value of its type.
func UnmarshalPayload(envelope Envelope, into any) error {
	if err := json.Unmarshal(envelope.Payload, into); err != nil {
		return fmt.Errorf("unmarshal payload of event %s: %w", envelope.EventID, err)
	}

	return nil
}
