package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/domain/order"
	orderusecase "avito-kitchen/internal/usecase/order"
	"context"
)

// CreateOrder serves POST /orders. A repeat of an attempt never reaches this
// handler: the idempotency middleware answers it with the stored response.
func (s *Server) CreateOrder(
	ctx context.Context, request kitchenapi.CreateOrderRequestObject,
) (kitchenapi.CreateOrderResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	placed, err := s.order.Create(ctx, userID, toOrderRequest(*request.Body))
	if err != nil {
		return nil, err
	}

	return kitchenapi.CreateOrder201JSONResponse(toOrder(placed)), nil
}

// ListOrders serves GET /orders.
func (s *Server) ListOrders(
	ctx context.Context, request kitchenapi.ListOrdersRequestObject,
) (kitchenapi.ListOrdersResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	page, err := s.order.Orders(ctx, userID, toOrdersQuery(request.Params))
	if err != nil {
		return nil, err
	}

	items := make([]kitchenapi.Order, 0, len(page.Orders))
	for _, o := range page.Orders {
		items = append(items, toOrder(o))
	}

	response := kitchenapi.ListOrders200JSONResponse{Items: items}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}

	return response, nil
}

// GetOrder serves GET /orders/{order_id}.
func (s *Server) GetOrder(
	ctx context.Context, request kitchenapi.GetOrderRequestObject,
) (kitchenapi.GetOrderResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	found, err := s.order.Order(ctx, userID, request.OrderId)
	if err != nil {
		return nil, err
	}

	return kitchenapi.GetOrder200JSONResponse(toOrder(found)), nil
}

func toOrderRequest(body kitchenapi.CreateOrderRequest) order.Request {
	request := order.Request{
		Address:       body.Address,
		Phone:         body.Phone,
		ExpectedTotal: body.ExpectedTotal,
	}

	if body.Comment != nil {
		request.Comment = *body.Comment
	}

	return request
}

func toOrdersQuery(params kitchenapi.ListOrdersParams) orderusecase.OrdersQuery {
	query := orderusecase.OrdersQuery{Limit: params.Limit}

	if params.Status != nil {
		for _, status := range *params.Status {
			query.Statuses = append(query.Statuses, string(status))
		}
	}

	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}

	return query
}

func toOrder(o order.Order) kitchenapi.Order {
	items := make([]kitchenapi.OrderItem, 0, len(o.Items))
	for _, item := range o.Items {
		items = append(items, kitchenapi.OrderItem{
			ExternalId: item.ExternalID,
			Name:       item.Name,
			Price:      item.Price,
			Qty:        item.Qty,
			LineTotal:  item.LineTotal(),
		})
	}

	response := kitchenapi.Order{
		Id:          o.ID,
		Number:      o.Number,
		Venue:       kitchenapi.OrderVenue{Id: o.Venue.ID, Name: o.Venue.Name},
		Status:      kitchenapi.OrderStatus(o.Status),
		Items:       items,
		ItemsTotal:  o.ItemsTotal,
		DeliveryFee: o.DeliveryFee,
		Total:       o.Total,
		Phone:       o.Phone,
		EtaMinutes:  o.EtaMinutes,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
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
