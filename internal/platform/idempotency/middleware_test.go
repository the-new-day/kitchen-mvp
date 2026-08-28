package idempotency_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/platform/idempotency"
	"avito-kitchen/internal/platform/idempotency/mocks"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

const (
	route     = "/api/v1/orders"
	body      = `{"address":"Лесная 7","phone":"+79990000000","expected_total":49900}`
	reordered = `{"phone":"+79990000000","expected_total":49900,"address":"Лесная 7"}`
)

var (
	userID  = uuid.MustParse("0192f4c1-0000-7000-8000-0000000000a1")
	errRepo = errors.New("connection refused")
)

// handler is what the middleware guards: it answers with the status and body
// it was built with, and counts how often it ran.
type handler struct {
	status int
	body   string
	calls  int
}

func (h *handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.calls++
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(h.status)
	_, _ = w.Write([]byte(h.body))
}

// serve runs one POST /orders through the middleware and gives back the answer
// the client would see.
func serve(
	t *testing.T, store idempotency.Store, guarded http.Handler, key, requestBody string,
) *httptest.ResponseRecorder {
	t.Helper()

	tx := mocks.NewMockTransactor(t)
	tx.EXPECT().InTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}).Maybe()

	middleware := idempotency.Middleware(idempotency.Config{
		Store:      store,
		Tx:         tx,
		TTL:        time.Hour,
		Operations: map[string]struct{}{http.MethodPost + " " + route: {}},
		User: func(context.Context) (uuid.UUID, error) {
			return userID, nil
		},
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			var unprocessable domain.UnprocessableError

			switch {
			case errors.As(err, &unprocessable):
				httpx.WriteError(w, r, http.StatusUnprocessableEntity,
					unprocessable.Code, unprocessable.Message)
			case errors.Is(err, domain.ErrInvalidArgument):
				httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
			default:
				httpx.WriteError(w, r, http.StatusInternalServerError,
					"internal_error", "internal server error")
			}
		},
	})

	router := chi.NewRouter()
	router.Post(route, middleware(guarded).ServeHTTP)

	request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(requestBody))
	if key != "" {
		request.Header.Set(idempotency.Header, key)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	return response
}

// reserved answers a store that has never seen the key before.
func reserved(t *testing.T) *mocks.MockStore {
	t.Helper()

	store := mocks.NewMockStore(t)
	store.EXPECT().Reserve(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil).Once()

	return store
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	stored := func(record idempotency.Record) func(*testing.T) *mocks.MockStore {
		return func(t *testing.T) *mocks.MockStore {
			t.Helper()

			store := mocks.NewMockStore(t)
			store.EXPECT().Reserve(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(&record, nil).Once()

			return store
		}
	}

	firstAttempt := idempotency.Record{
		Endpoint:       http.MethodPost + " " + route,
		RequestHash:    idempotency.Fingerprint([]byte(canonical(t, body))),
		ResponseStatus: http.StatusCreated,
		ResponseBody:   []byte(`{"id":"the-first-order"}`),
	}

	cases := map[string]struct {
		store      func(*testing.T) *mocks.MockStore
		handler    *handler
		key        string
		body       string
		wantStatus int
		wantBody   string
		wantCalls  int
	}{
		"a first attempt is carried out and stored": {
			store: func(t *testing.T) *mocks.MockStore {
				t.Helper()

				store := reserved(t)
				store.EXPECT().Complete(mock.Anything, mock.Anything,
					http.StatusCreated, []byte(`{"id":"new"}`)).Return(nil).Once()

				return store
			},
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			key:        "01J8X000",
			body:       body,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":"new"}`,
			wantCalls:  1,
		},
		"a repeat gets the stored answer and creates nothing": {
			store:      stored(firstAttempt),
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			key:        "01J8X000",
			body:       body,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"the-first-order"}`,
		},
		"a body written in another order is still the same request": {
			store:      stored(firstAttempt),
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			key:        "01J8X000",
			body:       reordered,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"the-first-order"}`,
		},
		"the same key with another body is refused": {
			store:      stored(firstAttempt),
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			key:        "01J8X000",
			body:       `{"address":"Другая 1","phone":"+79990000000","expected_total":49900}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   idempotency.CodeKeyReuse,
		},
		"a request without a key is refused": {
			store:      func(t *testing.T) *mocks.MockStore { t.Helper(); return mocks.NewMockStore(t) },
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			body:       body,
			wantStatus: http.StatusBadRequest,
		},
		"a body that is not JSON is refused": {
			store:      func(t *testing.T) *mocks.MockStore { t.Helper(); return mocks.NewMockStore(t) },
			handler:    &handler{status: http.StatusCreated, body: `{"id":"new"}`},
			key:        "01J8X000",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		"a failed attempt answers the client and keeps no key": {
			store:      reserved,
			handler:    &handler{status: http.StatusConflict, body: `{"code":"out_of_stock"}`},
			key:        "01J8X000",
			body:       body,
			wantStatus: http.StatusConflict,
			wantBody:   `{"code":"out_of_stock"}`,
			wantCalls:  1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := serve(t, tc.store(t), tc.handler, tc.key, tc.body)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body)
			}

			if tc.wantBody != "" && !strings.Contains(response.Body.String(), tc.wantBody) {
				t.Fatalf("body = %s, want it to carry %s", response.Body, tc.wantBody)
			}

			if tc.handler.calls != tc.wantCalls {
				t.Fatalf("handler ran %d times, want %d", tc.handler.calls, tc.wantCalls)
			}
		})
	}
}

func TestMiddlewareLetsUnguardedOperationsThrough(t *testing.T) {
	t.Parallel()

	guarded := &handler{status: http.StatusOK, body: `{}`}

	tx := mocks.NewMockTransactor(t)
	middleware := idempotency.Middleware(idempotency.Config{
		Store:      mocks.NewMockStore(t),
		Tx:         tx,
		Operations: map[string]struct{}{},
		User: func(context.Context) (uuid.UUID, error) {
			return userID, nil
		},
	})

	router := chi.NewRouter()
	router.Post(route, middleware(guarded).ServeHTTP)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, route, strings.NewReader(body)))

	if response.Code != http.StatusOK || guarded.calls != 1 {
		t.Fatalf("status = %d after %d calls, want 200 after 1", response.Code, guarded.calls)
	}
}

func TestMiddlewarePassesTheBodyOn(t *testing.T) {
	t.Parallel()

	var seen string

	store := reserved(t)
	store.EXPECT().Complete(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	reader := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}

		seen = string(raw)
		w.WriteHeader(http.StatusCreated)
	})

	serve(t, store, reader, "01J8X000", body)

	if seen != body {
		t.Fatalf("handler read %q, want %q", seen, body)
	}
}

func TestMiddlewareReportsABrokenStore(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().Reserve(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errRepo).Once()

	guarded := &handler{status: http.StatusCreated, body: `{}`}

	response := serve(t, store, guarded, "01J8X000", body)

	if response.Code != http.StatusInternalServerError || guarded.calls != 0 {
		t.Fatalf("status = %d after %d calls, want 500 after none", response.Code, guarded.calls)
	}
}

// canonical is the body as the fingerprint sees it: parsed and written back
// with its keys in a fixed order.
func canonical(t *testing.T, raw string) string {
	t.Helper()

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	return string(out)
}
