package integration_test

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/api/partnerapi"
	broker "avito-kitchen/internal/broker/kafka"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
)

// Headers the test introduces itself with, the same two the platform knows its
// callers by.
const (
	userHeader = "X-User-Id"
	keyHeader  = "X-Api-Key" //nolint:gosec // a header name, not a credential
)

// Where the orders of the test are taken and who to call about them.
const (
	statusCreated = "CREATED"

	address = "Москва, Тестовая ул., 2, кв. 3"
	phone   = "+79990000000"
)

// eater is the platform as one customer sees it. Every test takes a customer
// of its own: a cart belongs to a customer, and tests must not share one.
type eater struct {
	id  uuid.UUID
	api *kitchenapi.ClientWithResponses
}

// customerOf returns a customer nobody else in the run is using.
func customerOf(t *testing.T, s *stand) *eater {
	t.Helper()

	id := uuid.New()

	api, err := kitchenapi.NewClientWithResponses(s.baseURL,
		kitchenapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set(userHeader, id.String())

			return nil
		}),
	)
	if err != nil {
		t.Fatalf("create the customer client: %v", err)
	}

	return &eater{id: id, api: api}
}

// venueOf returns the venue of the fixture as it reaches the partner API.
func venueOf(t *testing.T, s *stand) *partnerapi.ClientWithResponses {
	t.Helper()

	api, err := partnerapi.NewClientWithResponses(s.baseURL,
		partnerapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set(keyHeader, venueKey)

			return nil
		}),
	)
	if err != nil {
		t.Fatalf("create the partner client: %v", err)
	}

	return api
}

// fill puts qty of a dish into the cart of a customer.
func (e *eater) fill(ctx context.Context, t *testing.T, s *stand, sku string, qty int) {
	t.Helper()

	res, err := e.api.PutCartItemWithResponse(ctx, kitchenapi.PutCartItemRequest{
		VenueId: s.venueID,
		ItemId:  s.items[sku],
		Qty:     qty,
	})
	if err != nil {
		t.Fatalf("put %s into the cart: %v", sku, err)
	}

	if res.JSON200 == nil {
		t.Fatalf("the cart answered %s on %s", res.HTTPResponse.Status, sku)
	}
}

// total is what the platform counts the cart of a customer at.
func (e *eater) total(ctx context.Context, t *testing.T) int64 {
	t.Helper()

	res, err := e.api.ValidateCartWithResponse(ctx)
	if err != nil {
		t.Fatalf("validate the cart: %v", err)
	}

	if res.JSON200 == nil {
		t.Fatalf("the check of the cart answered %s", res.HTTPResponse.Status)
	}

	return res.JSON200.Cart.Total
}

// place asks the platform to turn the cart of a customer into an order.
func (e *eater) place(
	ctx context.Context, t *testing.T, key uuid.UUID, total int64, comment string,
) *kitchenapi.CreateOrderResponse {
	t.Helper()

	request := kitchenapi.CreateOrderRequest{Address: address, Phone: phone, ExpectedTotal: total}
	if comment != "" {
		request.Comment = new(comment)
	}

	res, err := e.api.CreateOrderWithResponse(ctx,
		&kitchenapi.CreateOrderParams{IdempotencyKey: key}, request)
	if err != nil {
		t.Fatalf("place the order: %v", err)
	}

	return res
}

// order reads an order the way its customer sees it.
func (e *eater) order(ctx context.Context, t *testing.T, orderID uuid.UUID) kitchenapi.Order {
	t.Helper()

	res, err := e.api.GetOrderWithResponse(ctx, orderID)
	if err != nil {
		t.Fatalf("read the order: %v", err)
	}

	if res.JSON200 == nil {
		t.Fatalf("the order answered %s", res.HTTPResponse.Status)
	}

	return *res.JSON200
}

// refusalOf reads the code the platform refused a request with.
func refusalOf(t *testing.T, status int, body *kitchenapi.Error) (int, string) {
	t.Helper()

	if body == nil {
		return status, ""
	}

	return status, body.Code
}

// statuses opens the event stream of an order and reports every transition it
// carries. The stream is followed from the very first entry of the history, so
// nothing that happened before the subscription is lost.
func (e *eater) statuses(ctx context.Context, t *testing.T, s *stand, orderID uuid.UUID) <-chan string {
	t.Helper()

	url := fmt.Sprintf("%s/orders/%s/events", s.baseURL, orderID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build the stream request: %v", err)
	}

	req.Header.Set(userHeader, e.id.String())
	req.Header.Set("Last-Event-ID", "0")

	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // the reader closes it
	if err != nil {
		t.Fatalf("open the event stream: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("the event stream answered %s", res.Status)
	}

	updates := make(chan string, 16)

	go func() {
		defer res.Body.Close()
		defer close(updates)

		scanner := bufio.NewScanner(res.Body)
		if scanner.Err() != nil {
			log.Fatalf("bufio scanner error: %v", scanner.Err())
		}

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			var frame struct {
				Status string `json:"status"`
			}

			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &frame); err != nil {
				continue
			}

			if frame.Status == "" {
				continue
			}

			select {
			case updates <- frame.Status:
			case <-ctx.Done():
				return
			}
		}
	}()

	return updates
}

// awaits reads the statuses of an order out of the stream and checks that they
// arrive whole and in this very order. The status the order was placed in is
// skipped: it comes both in the snapshot and in the replayed history.
func awaits(t *testing.T, updates <-chan string, want []string, wait time.Duration) {
	t.Helper()

	deadline := time.NewTimer(wait)
	defer deadline.Stop()

	for seen := 0; seen < len(want); {
		select {
		case status, ok := <-updates:
			if !ok {
				t.Fatalf("the stream closed after %d statuses of %d", seen, len(want))
			}

			if status == statusCreated {
				continue
			}

			if status != want[seen] {
				t.Fatalf("status %s arrived, want %s", status, want[seen])
			}

			seen++
		case <-deadline.C:
			t.Fatalf("in %s the order reached only %s", wait, want[seen])
		}
	}
}

// ordersReader reads the topic the venues are given their orders in, from the
// moment it is opened.
func ordersReader(t *testing.T, s *stand) *kafka.Reader {
	t.Helper()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     s.topics.Brokers,
		Topic:       s.topics.OrdersTopic,
		Partition:   0,
		StartOffset: kafka.LastOffset,
		MaxWait:     100 * time.Millisecond,
	})

	t.Cleanup(func() {
		_ = reader.Close()
	})

	return reader
}

// event reads the next message of a topic and unwraps it.
func event(ctx context.Context, t *testing.T, reader *kafka.Reader) (broker.Envelope, string) {
	t.Helper()

	message, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read the event: %v", err)
	}

	envelope, err := broker.Decode(message.Value)
	if err != nil {
		t.Fatalf("decode the event: %v", err)
	}

	return envelope, string(message.Key)
}

// unpublished counts the events still waiting in the outbox.
func unpublished(ctx context.Context, t *testing.T, s *stand) int {
	t.Helper()

	conn, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatalf("connect to the database: %v", err)
	}
	defer conn.Close(ctx)

	var waiting int

	const query = `SELECT count(*) FROM outbox_messages WHERE published_at IS NULL`

	if err := conn.QueryRow(ctx, query).Scan(&waiting); err != nil {
		t.Fatalf("count the unpublished events: %v", err)
	}

	return waiting
}

// stockOf is what the platform believes the venue still has of a dish.
func stockOf(ctx context.Context, t *testing.T, s *stand, sku string) int {
	t.Helper()

	conn, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatalf("connect to the database: %v", err)
	}
	defer conn.Close(ctx)

	var stock int

	const query = `SELECT stock_qty FROM menu_items WHERE venue_id = $1 AND external_id = $2`

	if err := conn.QueryRow(ctx, query, s.venueID, sku).Scan(&stock); err != nil {
		t.Fatalf("read the stock of %s: %v", sku, err)
	}

	return stock
}

// cancellation is what a customer withdrawing an order says about it.
func cancellation() kitchenapi.CancelOrderRequest {
	return kitchenapi.CancelOrderRequest{Reason: new("передумал")}
}
