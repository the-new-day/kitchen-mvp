package order_test

import (
	"avito-kitchen/internal/domain"
	domainorder "avito-kitchen/internal/domain/order"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestServiceFollow(t *testing.T) {
	t.Parallel()

	changedAt := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)

	history := []domainorder.StatusEntry{
		{Seq: 1, To: domainorder.StatusCreated, Actor: domainorder.ActorCustomer, ChangedAt: changedAt},
		{
			Seq:       2,
			From:      domainorder.StatusCreated,
			To:        domainorder.StatusAccepted,
			Actor:     domainorder.ActorVenue,
			ChangedAt: changedAt.Add(time.Minute),
		},
	}

	cases := map[string]struct {
		setup      func(repositories)
		after      int64
		wantMissed []domainorder.StatusEntry
		wantErr    error
	}{
		"a client that has seen nothing is given the order and its whole history": {
			setup: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, userID, orderID).Return(placed, nil).Once()
				r.orders.EXPECT().History(mock.Anything, orderID, int64(0)).Return(history, nil).Once()
			},
			wantMissed: history,
		},
		"a client that reconnects is given only what came after it left": {
			setup: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, userID, orderID).Return(placed, nil).Once()
				r.orders.EXPECT().History(mock.Anything, orderID, int64(1)).
					Return(history[1:], nil).Once()
			},
			after:      1,
			wantMissed: history[1:],
		},
		"an order of somebody else is not there to follow": {
			setup: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, userID, orderID).
					Return(domainorder.Order{}, domain.ErrNotFound).Once()
			},
			wantErr: domain.ErrNotFound,
		},
		"a history that cannot be read stops the stream before it starts": {
			setup: func(r repositories) {
				r.orders.EXPECT().Get(mock.Anything, userID, orderID).Return(placed, nil).Once()
				r.orders.EXPECT().History(mock.Anything, orderID, int64(0)).Return(nil, errRepo).Once()
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, tc.setup)

			timeline, err := service.Follow(t.Context(), userID, orderID, tc.after)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, placed.ID, timeline.Order.ID)
			assert.Equal(t, tc.wantMissed, timeline.Missed)
		})
	}
}

// TestServiceFollowOfAnOrderOfAnotherCustomer keeps the stream behind the same
// door as reading the order: the identifier of an order is not confirmed to
// those it does not belong to.
func TestServiceFollowOfAnOrderOfAnotherCustomer(t *testing.T) {
	t.Parallel()

	stranger := uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a2")

	service := newService(t, func(r repositories) {
		r.orders.EXPECT().Get(mock.Anything, stranger, orderID).
			Return(domainorder.Order{}, domain.ErrNotFound).Once()
	})

	_, err := service.Follow(t.Context(), stranger, orderID, 0)

	assert.ErrorIs(t, err, domain.ErrNotFound)
}
