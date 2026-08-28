// Package partner serves the partner API of the platform on top of the
// generated server: every request is authenticated by its API key, and the
// venue it acts on comes from that key rather than from the request.
package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/transport/http/apierr"
	orderusecase "avito-kitchen/internal/usecase/order"
	partnerusecase "avito-kitchen/internal/usecase/partner"
	"log/slog"

	"github.com/go-chi/chi/v5"
)

// basePath is the server prefix declared in the specification.
const basePath = "/api/v1"

// Server implements the operations of the partner API. The operations that
// belong to a part of the service not built yet come from notImplemented.
type Server struct {
	notImplemented

	partner     *partnerusecase.Service
	orders      *orderusecase.Service
	ordersTopic string
}

var _ partnerapi.StrictServerInterface = (*Server)(nil)

// Mount registers the partner API on r under /api/v1.
// Authentication is mounted with the operations of the specification,
// so it guards them and only them.
func Mount(
	r chi.Router,
	service *partnerusecase.Service,
	orders *orderusecase.Service,
	ordersTopic string,
	log *slog.Logger,
) {
	server := &Server{partner: service, orders: orders, ordersTopic: ordersTopic}
	errs := apierr.Handler{Log: log}

	strict := partnerapi.NewStrictHandlerWithOptions(server, nil, partnerapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  errs.Request,
		ResponseErrorHandlerFunc: errs.Response,
	})

	partnerapi.HandlerWithOptions(strict, partnerapi.ChiServerOptions{
		BaseURL:          basePath,
		BaseRouter:       r,
		Middlewares:      []partnerapi.MiddlewareFunc{authenticate(service, errs)},
		ErrorHandlerFunc: errs.Request,
	})
}
