package kitchen

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"avito-kitchen/internal/transport/http/apierr"
	"avito-kitchen/internal/transport/sse"
	orderusecase "avito-kitchen/internal/usecase/order"
	"avito-kitchen/internal/usecase/order/mocks"
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	changedAt  = time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	streamUser = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")
)

func TestLastEventID(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		header string
		want   int64
	}{
		"a client that has seen nothing sends no header":          {want: 0},
		"a reconnecting client continues from the entry it names": {header: "7", want: 7},
		"a header that is not a number is read as nothing seen":   {header: "seven", want: 0},
		"a negative number is read as nothing seen":               {header: "-3", want: 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/orders/1/events", nil)
			if tc.header != "" {
				r.Header.Set(lastEventHeader, tc.header)
			}

			assert.Equal(t, tc.want, lastEventID(r))
		})
	}
}

// streaming is a service serving the events of one order, and the hub the
// transitions of it arrive in.
func streaming(t *testing.T, setup func(*mocks.MockRepository)) (http.Handler, *sse.Hub) {
	t.Helper()

	orders := mocks.NewMockRepository(t)
	setup(orders)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := sse.NewHub(4, log)

	service := orderusecase.New(
		mocks.NewMockTransactor(t), orders, mocks.NewMockCartRepository(t),
		mocks.NewMockMenuRepository(t), mocks.NewMockOutboxRepository(t),
		orderusecase.Topics{},
	)

	server := &Server{
		order:     service,
		hub:       hub,
		heartbeat: time.Minute,
		errs:      apierr.Handler{Log: log},
	}

	router := chi.NewRouter()
	router.Use(withUser)
	router.Get("/orders/{order_id}/events", server.streamOrderEvents)

	return router, hub
}

// follow opens the stream of the order as its customer.
func follow(t *testing.T, url string) *http.Response {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	request.Header.Set(userHeader, streamUser.String())

	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)

	return response
}

// frame reads one event of the stream: its lines up to the empty one that ends
// it. An empty result means the server has closed the stream.
func frame(t *testing.T, r *bufio.Reader) string {
	t.Helper()

	var b strings.Builder

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return b.String()
		}

		if line == "\n" {
			return b.String()
		}

		b.WriteString(line)
	}
}

// TestStreamOrderEvents follows an order the way a customer does: the snapshot
// first, then what the customer has not seen, then the transitions as they
// happen, and the stream closes when the order is over.
func TestStreamOrderEvents(t *testing.T) {
	t.Parallel()

	created := order.StatusEntry{
		Seq: 1, To: order.StatusCreated, Actor: order.ActorCustomer, ChangedAt: changedAt,
	}

	handler, hub := streaming(t, func(orders *mocks.MockRepository) {
		orders.EXPECT().Get(mock.Anything, streamUser, orderID).Return(placed, nil).Once()
		orders.EXPECT().History(mock.Anything, orderID, int64(0)).
			Return([]order.StatusEntry{created}, nil).Once()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	response := follow(t, server.URL+"/orders/"+orderID.String()+"/events")
	defer func() { _ = response.Body.Close() }()

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "text/event-stream; charset=utf-8", response.Header.Get("Content-Type"))

	reader := bufio.NewReader(response.Body)

	snapshot := frame(t, reader)
	assert.Contains(t, snapshot, "event: snapshot")
	assert.Contains(t, snapshot, `"number":"AK-100001"`)
	assert.NotContains(t, snapshot, "id:", "a snapshot must not move the client forward")

	replayed := frame(t, reader)
	assert.Contains(t, replayed, "id: 1")
	assert.Contains(t, replayed, `"status":"CREATED"`)

	// An entry the snapshot already covered is not sent twice.
	hub.Publish(sse.Update{OrderID: orderID, Seq: 1, To: order.StatusCreated, ChangedAt: changedAt})
	hub.Publish(sse.Update{
		OrderID: orderID, Seq: 2, From: order.StatusCreated, To: order.StatusAccepted,
		Actor: order.ActorVenue, ChangedAt: changedAt,
	})

	accepted := frame(t, reader)
	assert.Contains(t, accepted, "id: 2")
	assert.Contains(t, accepted, `"status":"ACCEPTED"`)

	hub.Publish(sse.Update{
		OrderID: orderID, Seq: 3, From: order.StatusDelivering, To: order.StatusDelivered,
		Actor: order.ActorSystem, ChangedAt: changedAt,
	})

	delivered := frame(t, reader)
	assert.Contains(t, delivered, `"status":"DELIVERED"`)

	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Empty(t, string(rest), "the stream stayed open after the order was over")
}

// TestStreamOrderEventsOfAnOrderOfSomebodyElse answers before the stream is
// opened at all, with the error envelope every other operation answers with.
func TestStreamOrderEventsOfAnOrderOfSomebodyElse(t *testing.T) {
	t.Parallel()

	handler, _ := streaming(t, func(orders *mocks.MockRepository) {
		orders.EXPECT().Get(mock.Anything, streamUser, orderID).
			Return(order.Order{}, domain.ErrNotFound).Once()
	})

	request := httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String()+"/events", nil)
	request.Header.Set(userHeader, streamUser.String())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "not_found")
}

// TestStreamOrderEventsEndsWithTheClient checks that a client walking away
// takes its handler with it: the hub would otherwise fill up with subscribers
// nobody reads.
func TestStreamOrderEventsEndsWithTheClient(t *testing.T) {
	t.Parallel()

	handler, _ := streaming(t, func(orders *mocks.MockRepository) {
		orders.EXPECT().Get(mock.Anything, streamUser, orderID).Return(placed, nil).Once()
		orders.EXPECT().History(mock.Anything, orderID, int64(0)).Return(nil, nil).Once()
	})

	server := httptest.NewServer(handler)

	ctx, cancel := context.WithCancel(t.Context())

	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server.URL+"/orders/"+orderID.String()+"/events", nil)
	require.NoError(t, err)

	request.Header.Set(userHeader, streamUser.String())

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	require.NotEmpty(t, frame(t, bufio.NewReader(response.Body)))

	cancel()
	_ = response.Body.Close()

	// Close waits for the handlers still running, so a stream that outlived
	// its client would hang here.
	closed := make(chan struct{})

	go func() {
		defer close(closed)

		server.Close()
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler is still streaming to a client that is gone")
	}
}
