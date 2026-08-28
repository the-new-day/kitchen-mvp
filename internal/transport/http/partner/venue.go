package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/catalog"
	"context"
)

// GetMe serves GET /partner/me.
func (s *Server) GetMe(
	ctx context.Context, _ partnerapi.GetMeRequestObject,
) (partnerapi.GetMeResponseObject, error) {
	venue, err := s.partner.Venue(ctx, venueID(ctx))
	if err != nil {
		return nil, err
	}

	return partnerapi.GetMe200JSONResponse(s.toPartnerVenue(venue)), nil
}

// OpenShift serves POST /partner/shift/open.
func (s *Server) OpenShift(
	ctx context.Context, _ partnerapi.OpenShiftRequestObject,
) (partnerapi.OpenShiftResponseObject, error) {
	id := venueID(ctx)

	isOpen, err := s.partner.OpenShift(ctx, id)
	if err != nil {
		return nil, err
	}

	return partnerapi.OpenShift200JSONResponse{VenueId: id, IsOpen: isOpen}, nil
}

// CloseShift serves POST /partner/shift/close.
func (s *Server) CloseShift(
	ctx context.Context, _ partnerapi.CloseShiftRequestObject,
) (partnerapi.CloseShiftResponseObject, error) {
	id := venueID(ctx)

	isOpen, err := s.partner.CloseShift(ctx, id)
	if err != nil {
		return nil, err
	}

	return partnerapi.CloseShift200JSONResponse{VenueId: id, IsOpen: isOpen}, nil
}

func (s *Server) toPartnerVenue(v catalog.Venue) partnerapi.PartnerVenue {
	venue := partnerapi.PartnerVenue{
		VenueId:        v.ID,
		Slug:           v.Slug,
		Name:           v.Name,
		IsOpen:         v.IsOpen,
		MinOrderAmount: v.MinOrderAmount,
		DeliveryFee:    v.DeliveryFee,
		AvgCookMinutes: v.AvgCookMinutes,
	}

	if v.Address != "" {
		venue.Address = &v.Address
	}
	if s.ordersTopic != "" {
		venue.OrdersTopic = &s.ordersTopic
	}

	return venue
}
