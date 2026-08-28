// Package partner serves the write side of the platform used by venues: the
// profile behind an API key, menu synchronisation, stop lists and shifts.
package partner

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// VenueRepository reads venue profiles and their API keys, and switches shifts.
type VenueRepository interface {
	VenueByKeyHash(ctx context.Context, hash []byte) (catalog.VenueKey, error)
	GetVenue(ctx context.Context, id uuid.UUID) (catalog.Venue, error)
	SetShift(ctx context.Context, id uuid.UUID, open bool) (bool, error)
}

// MenuRepository writes the menu of a venue.
type MenuRepository interface {
	SyncMenu(ctx context.Context, venueID uuid.UUID, snapshot catalog.MenuSnapshot) (catalog.MenuSyncResult, error)
	PatchItem(ctx context.Context, venueID uuid.UUID, externalID string, patch catalog.MenuItemPatch) (catalog.CategorizedMenuItem, error)
}

// Service is the partner use case.
type Service struct {
	venues VenueRepository
	menus  MenuRepository
}

// New returns a service working through the given repositories.
func New(venues VenueRepository, menus MenuRepository) *Service {
	return &Service{venues: venues, menus: menus}
}

// Authenticate returns the venue an API key was issued to. A missing, unknown
// or revoked key is reported as domain.ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, key string) (uuid.UUID, error) {
	if key == "" {
		return uuid.Nil, domain.ErrUnauthenticated
	}

	sum := sha256.Sum256([]byte(key))

	stored, err := s.venues.VenueByKeyHash(ctx, sum[:])
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return uuid.Nil, domain.ErrUnauthenticated
		}

		return uuid.Nil, fmt.Errorf("read api key: %w", err)
	}

	if subtle.ConstantTimeCompare(stored.Hash, sum[:]) != 1 {
		return uuid.Nil, domain.ErrUnauthenticated
	}

	return stored.VenueID, nil
}

// Venue returns the profile of a venue.
func (s *Service) Venue(ctx context.Context, venueID uuid.UUID) (catalog.Venue, error) {
	venue, err := s.venues.GetVenue(ctx, venueID)
	if err != nil {
		return catalog.Venue{}, fmt.Errorf("get venue %s: %w", venueID, err)
	}

	return venue, nil
}

// SyncMenu applies a full menu upload of a venue and reports what it changed.
func (s *Service) SyncMenu(
	ctx context.Context, venueID uuid.UUID, upload catalog.MenuUpload,
) (catalog.MenuSyncResult, error) {
	snapshot, err := upload.Normalize()
	if err != nil {
		return catalog.MenuSyncResult{}, err
	}

	result, err := s.menus.SyncMenu(ctx, venueID, snapshot)
	if err != nil {
		return catalog.MenuSyncResult{}, fmt.Errorf("sync menu of venue %s: %w", venueID, err)
	}

	return result, nil
}

// PatchItem changes single fields of one item of the venue menu and returns
// the item as it is afterwards.
func (s *Service) PatchItem(
	ctx context.Context, venueID uuid.UUID, externalID string, patch catalog.MenuItemPatch,
) (catalog.CategorizedMenuItem, error) {
	if err := patch.Validate(); err != nil {
		return catalog.CategorizedMenuItem{}, err
	}

	item, err := s.menus.PatchItem(ctx, venueID, externalID, patch)
	if err != nil {
		return catalog.CategorizedMenuItem{}, fmt.Errorf("patch item %s of venue %s: %w", externalID, venueID, err)
	}

	return item, nil
}

// OpenShift lets a venue take orders and puts it into the open_now listing.
func (s *Service) OpenShift(ctx context.Context, venueID uuid.UUID) (bool, error) {
	return s.setShift(ctx, venueID, true)
}

// CloseShift stops new orders for a venue. Orders it already took stay as they
// are.
func (s *Service) CloseShift(ctx context.Context, venueID uuid.UUID) (bool, error) {
	return s.setShift(ctx, venueID, false)
}

func (s *Service) setShift(ctx context.Context, venueID uuid.UUID, open bool) (bool, error) {
	isOpen, err := s.venues.SetShift(ctx, venueID, open)
	if err != nil {
		return false, fmt.Errorf("set shift of venue %s: %w", venueID, err)
	}

	return isOpen, nil
}
