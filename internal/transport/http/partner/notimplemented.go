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
