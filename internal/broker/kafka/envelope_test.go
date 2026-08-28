package kafka_test

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/domain/outbox"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	eventID   = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000e1")
	orderID   = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")
	occurred  = time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	published = outbox.Pending{
		Message: outbox.Message{
			Topic:            "kitchen.order-status.v1",
			Key:              orderID.String(),
			EventType:        outbox.EventOrderStatusChanged,
			AggregateID:      orderID,
			AggregateVersion: 4,
			Payload:          []byte(`{"to_status":"ACCEPTED"}`),
		},
		ID:         12,
		EventID:    eventID,
		OccurredAt: occurred,
	}
)

// TestWrapKeepsWhatTheConsumerNeeds checks that the envelope carries the
// identity of the event rather than of the row it was stored in: the same
// event republished after a failure must look the same to a consumer that
// deduplicates by it.
func TestWrapKeepsWhatTheConsumerNeeds(t *testing.T) {
	t.Parallel()

	envelope := broker.Wrap(published)

	cases := map[string]struct {
		got, want any
	}{
		"event id":     {envelope.EventID, eventID},
		"event type":   {envelope.EventType, outbox.EventOrderStatusChanged},
		"occurred at":  {envelope.OccurredAt, occurred},
		"aggregate id": {envelope.AggregateID, orderID},
		"version":      {envelope.Version, int64(4)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.got != tc.want {
				t.Fatalf("%s = %v, want %v", name, tc.got, tc.want)
			}
		})
	}
}

// TestEncodeDecode checks that an envelope survives the wire unchanged.
func TestEncodeDecode(t *testing.T) {
	t.Parallel()

	body, err := broker.Encode(broker.Wrap(published))
	if err != nil {
		t.Fatalf("encode = %v", err)
	}

	envelope, err := broker.Decode(body)
	if err != nil {
		t.Fatalf("decode = %v", err)
	}

	if envelope.EventID != eventID || !envelope.OccurredAt.Equal(occurred) {
		t.Fatalf("decoded %s of %s, want %s of %s",
			envelope.EventID, envelope.OccurredAt, eventID, occurred)
	}

	var payload struct {
		ToStatus string `json:"to_status"`
	}

	if err := broker.UnmarshalPayload(envelope, &payload); err != nil {
		t.Fatalf("unmarshal payload = %v", err)
	}

	if payload.ToStatus != "ACCEPTED" {
		t.Fatalf("payload = %q, want ACCEPTED", payload.ToStatus)
	}
}

// TestDecodeRefusals covers the messages a consumer cannot do anything with:
// they are reported as malformed and dropped rather than retried forever.
func TestDecodeRefusals(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"not json at all":       "}{",
		"an event without id":   `{"event_type":"order.created","payload":{}}`,
		"an event without type": `{"event_id":"0192f4c1-0000-7000-8000-0000000000e1","payload":{}}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := broker.Decode([]byte(body))
			if !errors.Is(err, broker.ErrMalformed) {
				t.Fatalf("decode = %v, want a malformed event", err)
			}
		})
	}
}
