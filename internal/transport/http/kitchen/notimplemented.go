package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/transport/http/apierr"
	"context"
)

// notImplemented answers the operations of the specification that the service
// does not serve yet. Every one of them reports
// errNotImplemented, which the error handler turns into 501. An operation
// leaves this type as soon as it gets a real handler.
type notImplemented struct{}

func (notImplemented) ListOrders(
	context.Context, kitchenapi.ListOrdersRequestObject,
) (kitchenapi.ListOrdersResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) CreateOrder(
	context.Context, kitchenapi.CreateOrderRequestObject,
) (kitchenapi.CreateOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) GetOrder(
	context.Context, kitchenapi.GetOrderRequestObject,
) (kitchenapi.GetOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) CancelOrder(
	context.Context, kitchenapi.CancelOrderRequestObject,
) (kitchenapi.CancelOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}
