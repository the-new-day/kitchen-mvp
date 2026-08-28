package order_test

import (
	"avito-kitchen/internal/domain"
	domainorder "avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/usecase/order"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// TestCursorRoundTrip checks the only thing a cursor promises: the page after
// it starts exactly where the previous one ended. The value itself is opaque
// and is read back through the filter the repository is called with.
func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	first := placed
	second := placed
	second.ID = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f2")
	second.CreatedAt = placed.CreatedAt.Add(-time.Hour)

	page, err := newService(t, func(r repositories) {
		r.orders.EXPECT().List(mock.Anything, mock.Anything).
			Return([]domainorder.Order{first, second}, nil).Once()
	}).Orders(t.Context(), userID, order.OrdersQuery{Limit: intp(1)})
	if err != nil {
		t.Fatalf("Orders() error = %v", err)
	}

	var filter domainorder.Filter

	_, err = newService(t, func(r repositories) {
		r.orders.EXPECT().List(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, f domainorder.Filter) ([]domainorder.Order, error) {
				filter = f

				return nil, nil
			}).Once()
	}).Orders(t.Context(), userID, order.OrdersQuery{Cursor: page.NextCursor, Limit: intp(1)})
	if err != nil {
		t.Fatalf("Orders() error = %v", err)
	}

	if filter.After == nil {
		t.Fatal("cursor did not reach the filter")
	}

	switch {
	case filter.After.ID != first.ID:
		t.Fatalf("after id = %s, want %s", filter.After.ID, first.ID)
	case !filter.After.CreatedAt.Equal(first.CreatedAt):
		t.Fatalf("after time = %s, want %s", filter.After.CreatedAt, first.CreatedAt)
	}
}

func TestCursorIsRefusedWhenMalformed(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"not base64":               "!!!",
		"not json":                 "bm90IGpzb24",
		"identifier is not a uuid": "eyJ0IjoiMjAyNi0wOC0yNlQxMDowMDowMFoiLCJpZCI6IngifQ",
	}

	for name, cursor := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := newService(t, nil).
				Orders(t.Context(), userID, order.OrdersQuery{Cursor: cursor})

			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("error = %v, want %v", err, domain.ErrInvalidArgument)
			}
		})
	}
}
