package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/order"
	"context"
)

// GetPartnerOrder serves GET /partner/orders/{order_id}.
func (s *Server) GetPartnerOrder(
	ctx context.Context, request partnerapi.GetPartnerOrderRequestObject,
) (partnerapi.GetPartnerOrderResponseObject, error) {
	found, err := s.orders.VenueOrder(ctx, venueID(ctx), request.OrderId)
	if err != nil {
		return nil, err
	}

	return partnerapi.GetPartnerOrder200JSONResponse(toPartnerOrder(found)), nil
}

// AcceptOrder serves POST /partner/orders/{order_id}/accept.
func (s *Server) AcceptOrder(
	ctx context.Context, request partnerapi.AcceptOrderRequestObject,
) (partnerapi.AcceptOrderResponseObject, error) {
	accepted, err := s.orders.Accept(ctx, venueID(ctx), request.OrderId, request.Body.EtaMinutes)
	if err != nil {
		return nil, err
	}

	return partnerapi.AcceptOrder200JSONResponse{
		OrderStateJSONResponse: partnerapi.OrderStateJSONResponse(toPartnerOrder(accepted)),
	}, nil
}

// RejectOrder serves POST /partner/orders/{order_id}/reject.
func (s *Server) RejectOrder(
	ctx context.Context, request partnerapi.RejectOrderRequestObject,
) (partnerapi.RejectOrderResponseObject, error) {
	rejected, err := s.orders.Reject(ctx, venueID(ctx), request.OrderId, request.Body.Reason)
	if err != nil {
		return nil, err
	}

	return partnerapi.RejectOrder200JSONResponse{
		OrderStateJSONResponse: partnerapi.OrderStateJSONResponse(toPartnerOrder(rejected)),
	}, nil
}

// StartCooking serves POST /partner/orders/{order_id}/cooking.
func (s *Server) StartCooking(
	ctx context.Context, request partnerapi.StartCookingRequestObject,
) (partnerapi.StartCookingResponseObject, error) {
	cooking, err := s.orders.StartCooking(ctx, venueID(ctx), request.OrderId)
	if err != nil {
		return nil, err
	}

	return partnerapi.StartCooking200JSONResponse{
		OrderStateJSONResponse: partnerapi.OrderStateJSONResponse(toPartnerOrder(cooking)),
	}, nil
}

// MarkReady serves POST /partner/orders/{order_id}/ready.
func (s *Server) MarkReady(
	ctx context.Context, request partnerapi.MarkReadyRequestObject,
) (partnerapi.MarkReadyResponseObject, error) {
	ready, err := s.orders.MarkReady(ctx, venueID(ctx), request.OrderId)
	if err != nil {
		return nil, err
	}

	return partnerapi.MarkReady200JSONResponse{
		OrderStateJSONResponse: partnerapi.OrderStateJSONResponse(toPartnerOrder(ready)),
	}, nil
}

// HandoverOrder serves POST /partner/orders/{order_id}/handover.
func (s *Server) HandoverOrder(
	ctx context.Context, request partnerapi.HandoverOrderRequestObject,
) (partnerapi.HandoverOrderResponseObject, error) {
	handed, err := s.orders.Handover(ctx, venueID(ctx), request.OrderId)
	if err != nil {
		return nil, err
	}

	return partnerapi.HandoverOrder200JSONResponse{
		OrderStateJSONResponse: partnerapi.OrderStateJSONResponse(toPartnerOrder(handed)),
	}, nil
}

// toPartnerOrder is the order as the venue sees it: its own identifiers of the
// positions and no customer of the platform behind them.
func toPartnerOrder(o order.Order) partnerapi.PartnerOrder {
	items := make([]partnerapi.PartnerOrderItem, 0, len(o.Items))
	for _, item := range o.Items {
		items = append(items, partnerapi.PartnerOrderItem{
			ExternalId: item.ExternalID,
			Name:       item.Name,
			Price:      item.Price,
			Qty:        item.Qty,
			LineTotal:  item.LineTotal(),
		})
	}

	response := partnerapi.PartnerOrder{
		Id:         o.ID,
		Number:     o.Number,
		Status:     partnerapi.OrderStatus(o.Status),
		Items:      items,
		ItemsTotal: o.ItemsTotal,
		Total:      o.Total,
		Phone:      &o.Phone,
		EtaMinutes: o.EtaMinutes,
		CreatedAt:  o.CreatedAt,
	}

	if o.Address != "" {
		response.Address = &o.Address
	}

	if o.Comment != "" {
		response.Comment = &o.Comment
	}

	if o.RejectionReason != "" {
		response.RejectionReason = &o.RejectionReason
	}

	return response
}
