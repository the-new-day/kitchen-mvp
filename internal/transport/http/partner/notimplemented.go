package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/transport/http/apierr"
	"context"
)

// notImplemented answers the operations of the specification that the service
// does not serve yet. Every one of them reports apierr.ErrNotImplemented,
// which the error handler turns into 501. An operation leaves this type as
// soon as it gets a real handler.
type notImplemented struct{}

func (notImplemented) ListPartnerOrders(
	context.Context, partnerapi.ListPartnerOrdersRequestObject,
) (partnerapi.ListPartnerOrdersResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) GetPartnerOrder(
	context.Context, partnerapi.GetPartnerOrderRequestObject,
) (partnerapi.GetPartnerOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) AcceptOrder(
	context.Context, partnerapi.AcceptOrderRequestObject,
) (partnerapi.AcceptOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) RejectOrder(
	context.Context, partnerapi.RejectOrderRequestObject,
) (partnerapi.RejectOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) StartCooking(
	context.Context, partnerapi.StartCookingRequestObject,
) (partnerapi.StartCookingResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) MarkReady(
	context.Context, partnerapi.MarkReadyRequestObject,
) (partnerapi.MarkReadyResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}

func (notImplemented) HandoverOrder(
	context.Context, partnerapi.HandoverOrderRequestObject,
) (partnerapi.HandoverOrderResponseObject, error) {
	return nil, apierr.ErrNotImplemented
}
