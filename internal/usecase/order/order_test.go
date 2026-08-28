package order_test

import (
	"avito-kitchen/internal/domain"
	domaincart "avito-kitchen/internal/domain/cart"
	"avito-kitchen/internal/domain/catalog"
	domainorder "avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/domain/outbox"
	"avito-kitchen/internal/usecase/order"
	"avito-kitchen/internal/usecase/order/mocks"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

const ordersTopic = "kitchen.orders.v1"

var (
	userID      = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")
	bakeryID    = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	croissantID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000011")
	orderID     = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f1")

	croissant = catalog.MenuItem{
		ID:          croissantID,
		ExternalID:  "SKU-CROISSANT",
		Name:        "Круассан",
		Price:       20_000,
		IsAvailable: true,
	}

	request = domainorder.Request{
		Address:       "Москва, Лесная 7",
		Phone:         "+79990000000",
		ExpectedTotal: 49_900,
	}

	placed = domainorder.Order{
		ID:          orderID,
		Number:      "AK-100001",
		UserID:      userID,
		Venue:       domainorder.Venue{ID: bakeryID, Name: "Пекарня «Батон»"},
		Status:      domainorder.StatusCreated,
		Items:       []domainorder.Item{{MenuItemID: croissantID, ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 20_000, Qty: 2}},
		ItemsTotal:  40_000,
		DeliveryFee: 9_900,
		Total:       49_900,
		Version:     1,
		CreatedAt:   time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
	}

	errRepo = errors.New("connection refused")
)

// filled is the cart of two croissants every order in the table is placed out
// of: 40 000 for the items and 9 900 for the delivery.
func filled() domaincart.Cart {
	return domaincart.Cart{
		Venue: &domaincart.Venue{
			ID:             bakeryID,
			Name:           "Пекарня «Батон»",
			IsOpen:         true,
			MinOrderAmount: 30_000,
			DeliveryFee:    9_900,
		},
		Lines: []domaincart.Line{{Item: croissant, Qty: 2, PriceSnapshot: 20_000}},
	}
}

// repositories is the set of mocks one case of the table is served by.
type repositories struct {
	orders   *mocks.MockRepository
	carts    *mocks.MockCartRepository
	menus    *mocks.MockMenuRepository
	messages *mocks.MockOutboxRepository
}

func newService(t *testing.T, setup func(repositories)) *order.Service {
	t.Helper()

	repos := repositories{
		orders:   mocks.NewMockRepository(t),
		carts:    mocks.NewMockCartRepository(t),
		menus:    mocks.NewMockMenuRepository(t),
		messages: mocks.NewMockOutboxRepository(t),
	}

	if setup != nil {
		setup(repos)
	}

	tx := mocks.NewMockTransactor(t)
	tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Maybe()

	return order.New(tx, repos.orders, repos.carts, repos.menus, repos.messages, ordersTopic)
}

// readCart is the part every attempt at placing an order begins with.
func readCart(r repositories, current domaincart.Cart) {
	r.carts.EXPECT().Lock(mock.Anything, userID).Return(nil).Once()
	r.carts.EXPECT().Cart(mock.Anything, userID).Return(current, nil).Once()
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup    func(repositories)
		request  domainorder.Request
		wantErr  error
		wantCode string
	}{
		"a cart becomes an order, an event and an empty cart": {
			setup: func(r repositories) {
				readCart(r, filled())
				r.menus.EXPECT().ReserveStock(mock.Anything, bakeryID, mock.Anything).
					Return(nil, nil).Once()
				r.orders.EXPECT().Create(mock.Anything, mock.Anything).Return(placed, nil).Once()
				r.messages.EXPECT().Append(mock.Anything, mock.Anything).Return(nil).Once()
				r.carts.EXPECT().Clear(mock.Anything, userID).Return(nil).Once()
			},
			request: request,
		},
		"an item taken by somebody else stops the order": {
			setup: func(r repositories) {
				readCart(r, filled())
				r.menus.EXPECT().ReserveStock(mock.Anything, bakeryID, mock.Anything).
					Return([]uuid.UUID{croissantID}, nil).Once()
			},
			request:  request,
			wantErr:  domain.ErrConflict,
			wantCode: domainorder.CodeOutOfStock,
		},
		"an empty cart is not an order": {
			setup: func(r repositories) {
				readCart(r, domaincart.Cart{})
			},
			request:  request,
			wantErr:  domain.ErrUnprocessable,
			wantCode: domainorder.CodeEmptyCart,
		},
		"a total the customer did not see stops the order": {
			setup: func(r repositories) {
				readCart(r, filled())
			},
			request: domainorder.Request{
				Address: "Лесная 7", Phone: "+79990000000", ExpectedTotal: 40_000,
			},
			wantErr:  domain.ErrConflict,
			wantCode: domainorder.CodePriceMismatch,
		},
		"a broken cart is not swallowed": {
			setup: func(r repositories) {
				r.carts.EXPECT().Lock(mock.Anything, userID).Return(nil).Once()
				r.carts.EXPECT().Cart(mock.Anything, userID).
					Return(domaincart.Cart{}, errRepo).Once()
			},
			request: request,
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, tc.setup)

			created, err := service.Create(t.Context(), userID, tc.request)

			if tc.wantErr != nil {
				assertRefused(t, err, tc.wantErr, tc.wantCode)

				return
			}

			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			if created.ID != placed.ID {
				t.Fatalf("order = %s, want %s", created.ID, placed.ID)
			}
		})
	}
}

func TestServiceCreateWritesTheEventOfTheOrder(t *testing.T) {
	t.Parallel()

	var written outbox.Message

	service := newService(t, func(r repositories) {
		readCart(r, filled())
		r.menus.EXPECT().ReserveStock(mock.Anything, bakeryID, mock.Anything).
			Return(nil, nil).Once()
		r.orders.EXPECT().Create(mock.Anything, mock.Anything).Return(placed, nil).Once()
		r.messages.EXPECT().Append(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, message outbox.Message) error {
				written = message

				return nil
			}).Once()
		r.carts.EXPECT().Clear(mock.Anything, userID).Return(nil).Once()
	})

	if _, err := service.Create(t.Context(), userID, request); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	switch {
	case written.Topic != ordersTopic:
		t.Fatalf("topic = %q, want %q", written.Topic, ordersTopic)
	case written.Key != bakeryID.String():
		t.Fatalf("key = %q, want the venue %q", written.Key, bakeryID)
	case written.EventType != outbox.EventOrderCreated:
		t.Fatalf("event type = %q", written.EventType)
	case written.AggregateID != orderID:
		t.Fatalf("aggregate = %s, want %s", written.AggregateID, orderID)
	case written.AggregateVersion != placed.Version:
		t.Fatalf("version = %d, want %d", written.AggregateVersion, placed.Version)
	}

	var payload struct {
		OrderID uuid.UUID `json:"order_id"`
		Number  string    `json:"number"`
		Total   int64     `json:"total"`
		Items   []struct {
			ExternalID string `json:"external_id"`
			LineTotal  int64  `json:"line_total"`
		} `json:"items"`
	}

	if err := json.Unmarshal(written.Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}

	switch {
	case payload.OrderID != orderID || payload.Number != placed.Number:
		t.Fatalf("payload = %+v, want the placed order", payload)
	case payload.Total != placed.Total:
		t.Fatalf("total = %d, want %d", payload.Total, placed.Total)
	case len(payload.Items) != 1 || payload.Items[0].LineTotal != 40_000:
		t.Fatalf("items = %+v, want one line of 40000", payload.Items)
	}
}

func TestServiceOrders(t *testing.T) {
	t.Parallel()

	older := placed
	older.ID = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000f2")
	older.CreatedAt = placed.CreatedAt.Add(-time.Hour)

	cases := map[string]struct {
		query      order.OrdersQuery
		stored     []domainorder.Order
		wantLen    int
		wantCursor bool
		wantErr    error
	}{
		"a page shorter than the limit is the last one": {
			query:   order.OrdersQuery{Limit: intp(2)},
			stored:  []domainorder.Order{placed},
			wantLen: 1,
		},
		"a full page carries the cursor of its last order": {
			query:      order.OrdersQuery{Limit: intp(1)},
			stored:     []domainorder.Order{placed, older},
			wantLen:    1,
			wantCursor: true,
		},
		"an unknown status is refused": {
			query:   order.OrdersQuery{Statuses: []string{"COOKED"}},
			wantErr: domain.ErrInvalidArgument,
		},
		"a limit out of range is refused": {
			query:   order.OrdersQuery{Limit: intp(1000)},
			wantErr: domain.ErrInvalidArgument,
		},
		"a malformed cursor is refused": {
			query:   order.OrdersQuery{Cursor: "not-a-cursor"},
			wantErr: domain.ErrInvalidArgument,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := newService(t, func(r repositories) {
				if tc.wantErr != nil {
					return
				}

				r.orders.EXPECT().List(mock.Anything, mock.Anything).Return(tc.stored, nil).Once()
			})

			page, err := service.Orders(t.Context(), userID, tc.query)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Orders() error = %v", err)
			}

			if len(page.Orders) != tc.wantLen {
				t.Fatalf("orders = %d, want %d", len(page.Orders), tc.wantLen)
			}

			if (page.NextCursor != "") != tc.wantCursor {
				t.Fatalf("next cursor = %q, want present = %v", page.NextCursor, tc.wantCursor)
			}
		})
	}
}

func TestServiceOrder(t *testing.T) {
	t.Parallel()

	service := newService(t, func(r repositories) {
		r.orders.EXPECT().Get(mock.Anything, userID, orderID).
			Return(domainorder.Order{}, domain.ErrNotFound).Once()
	})

	if _, err := service.Order(t.Context(), userID, orderID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want %v", err, domain.ErrNotFound)
	}
}

func intp(v int) *int { return &v }

// assertRefused checks that an order was refused with the expected kind of
// error and, where the envelope carries one, the expected code.
func assertRefused(t *testing.T, err, want error, code string) {
	t.Helper()

	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}

	if code == "" {
		return
	}

	var (
		conflict      domain.ConflictError
		unprocessable domain.UnprocessableError
	)

	switch {
	case errors.As(err, &conflict):
		if conflict.Code != code {
			t.Fatalf("code = %q, want %q", conflict.Code, code)
		}
	case errors.As(err, &unprocessable):
		if unprocessable.Code != code {
			t.Fatalf("code = %q, want %q", unprocessable.Code, code)
		}
	default:
		t.Fatalf("error %v carries no code", err)
	}
}
