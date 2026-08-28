package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/domain/cart"
	"context"
)

// GetCart serves GET /cart.
func (s *Server) GetCart(
	ctx context.Context, _ kitchenapi.GetCartRequestObject,
) (kitchenapi.GetCartResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	current, err := s.cart.Cart(ctx, userID)
	if err != nil {
		return nil, err
	}

	return kitchenapi.GetCart200JSONResponse(toCart(current)), nil
}

// PutCartItem serves PUT /cart/items.
func (s *Server) PutCartItem(
	ctx context.Context, request kitchenapi.PutCartItemRequestObject,
) (kitchenapi.PutCartItemResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := s.cart.SetItem(ctx, userID, request.Body.VenueId, request.Body.ItemId, request.Body.Qty)
	if err != nil {
		return nil, err
	}

	return kitchenapi.PutCartItem200JSONResponse(toCart(updated)), nil
}

// DeleteCartItem serves DELETE /cart/items/{item_id}.
func (s *Server) DeleteCartItem(
	ctx context.Context, request kitchenapi.DeleteCartItemRequestObject,
) (kitchenapi.DeleteCartItemResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := s.cart.RemoveItem(ctx, userID, request.ItemId)
	if err != nil {
		return nil, err
	}

	return kitchenapi.DeleteCartItem200JSONResponse(toCart(updated)), nil
}

// ClearCart serves DELETE /cart.
func (s *Server) ClearCart(
	ctx context.Context, _ kitchenapi.ClearCartRequestObject,
) (kitchenapi.ClearCartResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.cart.Clear(ctx, userID); err != nil {
		return nil, err
	}

	return kitchenapi.ClearCart204Response{}, nil
}

// ValidateCart serves POST /cart/validate.
func (s *Server) ValidateCart(
	ctx context.Context, _ kitchenapi.ValidateCartRequestObject,
) (kitchenapi.ValidateCartResponseObject, error) {
	userID, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	validation, err := s.cart.Validate(ctx, userID)
	if err != nil {
		return nil, err
	}

	problems := make([]kitchenapi.CartProblem, 0, len(validation.Problems))
	for _, p := range validation.Problems {
		problems = append(problems, toCartProblem(p))
	}

	return kitchenapi.ValidateCart200JSONResponse{
		IsValid:  validation.IsValid,
		Problems: problems,
		Cart:     toCart(validation.Cart),
	}, nil
}

func toCart(c cart.Cart) kitchenapi.Cart {
	totals := c.Totals()

	items := make([]kitchenapi.CartItem, 0, len(c.Lines))
	for _, line := range c.Lines {
		items = append(items, toCartItem(line, c.Venue))
	}

	response := kitchenapi.Cart{
		Items:       items,
		ItemsTotal:  totals.ItemsTotal,
		DeliveryFee: totals.DeliveryFee,
		Total:       totals.Total,
	}

	if c.Venue != nil {
		response.VenueId = &c.Venue.ID
		response.VenueName = &c.Venue.Name
		response.MinOrderAmount = &c.Venue.MinOrderAmount
	}

	return response
}

func toCartItem(line cart.Line, venue *cart.Venue) kitchenapi.CartItem {
	available, _ := line.Item.Availability(venue != nil && venue.IsOpen)

	return kitchenapi.CartItem{
		ItemId:      line.Item.ID,
		ExternalId:  line.Item.ExternalID,
		Name:        line.Item.Name,
		Price:       line.PriceSnapshot,
		Qty:         line.Qty,
		LineTotal:   line.LineTotal(),
		IsAvailable: available,
	}
}

func toCartProblem(p cart.Problem) kitchenapi.CartProblem {
	problem := kitchenapi.CartProblem{
		Type:         kitchenapi.CartProblemType(p.Type),
		Message:      p.Message,
		ItemId:       p.ItemID,
		OldPrice:     p.OldPrice,
		NewPrice:     p.NewPrice,
		RequestedQty: p.RequestedQty,
		AvailableQty: p.AvailableQty,
		Shortfall:    p.Shortfall,
	}

	if p.ItemName != "" {
		problem.ItemName = &p.ItemName
	}

	return problem
}
