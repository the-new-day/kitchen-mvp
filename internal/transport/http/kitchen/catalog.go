package kitchen

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/domain/catalog"
	catalogusecase "avito-kitchen/internal/usecase/catalog"
	"context"
)

// ListCuisines serves GET /cuisines.
func (s *Server) ListCuisines(
	ctx context.Context, _ kitchenapi.ListCuisinesRequestObject,
) (kitchenapi.ListCuisinesResponseObject, error) {
	cuisines, err := s.catalog.Cuisines(ctx)
	if err != nil {
		return nil, err
	}

	return kitchenapi.ListCuisines200JSONResponse{Items: toCuisines(cuisines)}, nil
}

// ListVenues serves GET /venues.
func (s *Server) ListVenues(
	ctx context.Context, request kitchenapi.ListVenuesRequestObject,
) (kitchenapi.ListVenuesResponseObject, error) {
	page, err := s.catalog.Venues(ctx, toVenuesQuery(request.Params))
	if err != nil {
		return nil, err
	}

	items := make([]kitchenapi.Venue, 0, len(page.Venues))
	for _, v := range page.Venues {
		items = append(items, toVenue(v))
	}

	response := kitchenapi.ListVenues200JSONResponse{Items: items}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}

	return response, nil
}

// GetVenue serves GET /venues/{venue_id}.
func (s *Server) GetVenue(
	ctx context.Context, request kitchenapi.GetVenueRequestObject,
) (kitchenapi.GetVenueResponseObject, error) {
	venue, err := s.catalog.Venue(ctx, request.VenueId)
	if err != nil {
		return nil, err
	}

	return kitchenapi.GetVenue200JSONResponse(toVenue(venue)), nil
}

// GetVenueMenu serves GET /venues/{venue_id}/menu.
func (s *Server) GetVenueMenu(
	ctx context.Context, request kitchenapi.GetVenueMenuRequestObject,
) (kitchenapi.GetVenueMenuResponseObject, error) {
	menu, err := s.catalog.Menu(ctx, request.VenueId)
	if err != nil {
		return nil, err
	}

	return kitchenapi.GetVenueMenu200JSONResponse(toMenu(menu)), nil
}

func toVenuesQuery(params kitchenapi.ListVenuesParams) catalogusecase.VenuesQuery {
	query := catalogusecase.VenuesQuery{Limit: params.Limit}

	if params.Q != nil {
		query.Q = *params.Q
	}
	if params.Cuisine != nil {
		query.Cuisine = *params.Cuisine
	}
	if params.OpenNow != nil {
		query.OpenNow = *params.OpenNow
	}
	if params.Sort != nil {
		query.Sort = string(*params.Sort)
	}
	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}

	return query
}

func toCuisines(cuisines []catalog.Cuisine) []kitchenapi.Cuisine {
	out := make([]kitchenapi.Cuisine, 0, len(cuisines))
	for _, c := range cuisines {
		out = append(out, kitchenapi.Cuisine{Slug: c.Slug, Name: c.Name})
	}

	return out
}

func toVenue(v catalog.Venue) kitchenapi.Venue {
	venue := kitchenapi.Venue{
		Id:             v.ID,
		Slug:           v.Slug,
		Name:           v.Name,
		Address:        v.Address,
		Lat:            v.Lat,
		Lon:            v.Lon,
		Cuisines:       toCuisines(v.Cuisines),
		IsOpen:         v.IsOpen,
		MinOrderAmount: v.MinOrderAmount,
		DeliveryFee:    v.DeliveryFee,
		AvgCookMinutes: v.AvgCookMinutes,
	}

	if v.Description != "" {
		venue.Description = &v.Description
	}

	return venue
}

func toMenu(m catalog.Menu) kitchenapi.Menu {
	categories := make([]kitchenapi.MenuCategory, 0, len(m.Categories))
	for _, c := range m.Categories {
		items := make([]kitchenapi.MenuItem, 0, len(c.Items))
		for _, i := range c.Items {
			items = append(items, toMenuItem(i, m.VenueIsOpen))
		}

		categories = append(categories, kitchenapi.MenuCategory{
			Id:         c.ID,
			ExternalId: c.ExternalID,
			Name:       c.Name,
			Position:   c.Position,
			Items:      items,
		})
	}

	return kitchenapi.Menu{VenueId: m.VenueID, Categories: categories}
}

// toMenuItem keeps an item that cannot be ordered in the response and marks
// it, instead of hiding it from the menu.
func toMenuItem(i catalog.MenuItem, venueOpen bool) kitchenapi.MenuItem {
	available, reason := i.Availability(venueOpen)

	item := kitchenapi.MenuItem{
		Id:          i.ID,
		ExternalId:  i.ExternalID,
		Name:        i.Name,
		Price:       i.Price,
		IsAvailable: available,
		StockQty:    i.StockQty,
	}

	if i.Description != "" {
		item.Description = &i.Description
	}
	if reason != catalog.ReasonNone {
		unavailableReason := kitchenapi.UnavailableReason(reason)
		item.UnavailableReason = &unavailableReason
	}

	return item
}
