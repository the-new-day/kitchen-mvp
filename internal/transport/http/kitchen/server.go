// Package kitchen serves the customer API of the platform on top of the
// generated server: it maps the request and response types of the
// specification to the domain by hand and answers with one error envelope.
package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/transport/http/apierr"
	cartusecase "avito-kitchen/internal/usecase/cart"
	catalogusecase "avito-kitchen/internal/usecase/catalog"
	"log/slog"

	"github.com/go-chi/chi/v5"
)

// basePath is the server prefix declared in the specification.
const basePath = "/api/v1"

// Server implements the operations of the customer API. The operations that
// belong to a part of the service not built yet come from notImplemented.
type Server struct {
	notImplemented

	catalog *catalogusecase.Service
	cart    *cartusecase.Service
}

var _ kitchenapi.StrictServerInterface = (*Server)(nil)

// Mount registers the customer API on r under /api/v1.
func Mount(
	r chi.Router,
	catalogService *catalogusecase.Service,
	cartService *cartusecase.Service,
	log *slog.Logger,
) {
	server := &Server{catalog: catalogService, cart: cartService}
	errs := apierr.Handler{Log: log}

	strict := kitchenapi.NewStrictHandlerWithOptions(server, nil, kitchenapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  errs.Request,
		ResponseErrorHandlerFunc: errs.Response,
	})

	kitchenapi.HandlerWithOptions(strict, kitchenapi.ChiServerOptions{
		BaseURL:          basePath,
		BaseRouter:       r,
		Middlewares:      []kitchenapi.MiddlewareFunc{withUser},
		ErrorHandlerFunc: errs.Request,
	})
}
