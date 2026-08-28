// Package kitchen serves the customer API of the platform on top of the
// generated server: it maps the request and response types of the
// specification to the domain by hand and answers with one error envelope.
package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/platform/idempotency"
	"avito-kitchen/internal/transport/http/apierr"
	cartusecase "avito-kitchen/internal/usecase/cart"
	catalogusecase "avito-kitchen/internal/usecase/catalog"
	orderusecase "avito-kitchen/internal/usecase/order"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
)

// basePath is the server prefix declared in the specification.
const basePath = "/api/v1"

// Server implements the operations of the customer API.
type Server struct {
	catalog *catalogusecase.Service
	cart    *cartusecase.Service
	order   *orderusecase.Service
}

var _ kitchenapi.StrictServerInterface = (*Server)(nil)

// Services are the use cases the customer API is served by.
type Services struct {
	Catalog *catalogusecase.Service
	Cart    *cartusecase.Service
	Order   *orderusecase.Service
}

// Idempotency is what the middleware guarding the repeatable operations needs.
type Idempotency struct {
	Store idempotency.Store
	Tx    idempotency.Transactor
	TTL   time.Duration
}

// Mount registers the customer API on r under /api/v1.
func Mount(r chi.Router, services Services, keys Idempotency, log *slog.Logger) error {
	server := &Server{catalog: services.Catalog, cart: services.Cart, order: services.Order}
	errs := apierr.Handler{Log: log}

	operations, err := idempotentOperations()
	if err != nil {
		return err
	}

	guard := idempotency.Middleware(idempotency.Config{
		Store:      keys.Store,
		Tx:         keys.Tx,
		TTL:        keys.TTL,
		Operations: operations,
		User:       requireUser,
		OnError:    errs.Response,
	})

	strict := kitchenapi.NewStrictHandlerWithOptions(server, nil, kitchenapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  errs.Request,
		ResponseErrorHandlerFunc: errs.Response,
	})

	// The generated server wraps the middleware in reverse, so withUser goes
	// last to end up outermost: the guard reads the customer from the context.
	kitchenapi.HandlerWithOptions(strict, kitchenapi.ChiServerOptions{
		BaseURL:          basePath,
		BaseRouter:       r,
		Middlewares:      []kitchenapi.MiddlewareFunc{guard, withUser},
		ErrorHandlerFunc: errs.Request,
	})

	return nil
}
