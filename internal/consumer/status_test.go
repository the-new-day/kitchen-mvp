package consumer_test

import (
	broker "avito-kitchen/internal/broker/kafka"
	"avito-kitchen/internal/consumer"
	"avito-kitchen/internal/consumer/mocks"
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/transport/sse"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	orderID   = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")
	changedAt = time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	eta       = 15

	accepted = map[string]any{
		"order_id":    orderID,
		"user_id":     uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1"),
		"venue_id":    uuid.MustParse("0192f4c1-0000-7000-8000-000000000001"),
		"from_status": order.StatusCreated,
		"to_status":   order.StatusAccepted,
		"actor":       order.ActorVenue,
		"eta_minutes": eta,
		"seq":         int64(2),
		"changed_at":  changedAt,
	}
)

// envelope wraps a payload the way the platform publishes it.
func envelope(t *testing.T, eventType string, payload any) broker.Envelope {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	return broker.Envelope{EventID: uuid.New(), EventType: eventType, Payload: body}
}

func TestStatusHandle(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event         func(*testing.T) broker.Envelope
		wantPublished *sse.Update
		wantMalformed bool
	}{
		"a transition is offered to whoever watches the order": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, "order.status_changed", accepted)
			},
			wantPublished: &sse.Update{
				OrderID:    orderID,
				Seq:        2,
				From:       order.StatusCreated,
				To:         order.StatusAccepted,
				Actor:      order.ActorVenue,
				EtaMinutes: &eta,
				ChangedAt:  changedAt,
			},
		},
		"an event of another kind is none of the stream's business": {
			event: func(t *testing.T) broker.Envelope {
				return envelope(t, "order.created", map[string]any{"order_id": orderID})
			},
		},
		"a payload that is not a transition is dropped": {
			event: func(*testing.T) broker.Envelope {
				return broker.Envelope{
					EventID:   uuid.New(),
					EventType: "order.status_changed",
					Payload:   json.RawMessage(`"это не переход"`),
				}
			},
			wantMalformed: true,
		},
		"a transition to an unknown status is dropped": {
			event: func(t *testing.T) broker.Envelope {
				unknown := map[string]any{"order_id": orderID, "to_status": "COOLING", "seq": 2}

				return envelope(t, "order.status_changed", unknown)
			},
			wantMalformed: true,
		},
		"a transition without a number cannot be followed and is dropped": {
			event: func(t *testing.T) broker.Envelope {
				unnumbered := map[string]any{
					"order_id": orderID, "to_status": order.StatusReady,
				}

				return envelope(t, "order.status_changed", unnumbered)
			},
			wantMalformed: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			hub := mocks.NewMockHub(t)
			if tc.wantPublished != nil {
				hub.EXPECT().Publish(*tc.wantPublished).Once()
			}

			log := slog.New(slog.NewTextHandler(io.Discard, nil))

			err := consumer.NewStatus(hub, log).Handle(t.Context(), tc.event(t))

			if tc.wantMalformed {
				assert.ErrorIs(t, err, broker.ErrMalformed)

				return
			}

			assert.NoError(t, err)
		})
	}
}
