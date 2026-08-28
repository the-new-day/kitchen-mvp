// Package catalog serves the read side of the venue catalogue: the cuisine
// reference book, the venue list with its filters and paging, a venue card
// and a venue menu.
package catalog

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	maxQueryLen  = 100
)

// VenueRepository reads venues and the cuisine reference book.
type VenueRepository interface {
	ListCuisines(ctx context.Context) ([]catalog.Cuisine, error)
	ListVenues(ctx context.Context, filter catalog.VenueFilter) ([]catalog.Venue, error)
	GetVenue(ctx context.Context, id uuid.UUID) (catalog.Venue, error)
}

// MenuRepository reads venue menus.
type MenuRepository interface {
	VenueMenu(ctx context.Context, venueID uuid.UUID) (catalog.Menu, error)
}

// Service is the catalogue use case.
type Service struct {
	venues VenueRepository
	menus  MenuRepository
}

// New returns a service reading through the given repositories.
func New(venues VenueRepository, menus MenuRepository) *Service {
	return &Service{venues: venues, menus: menus}
}

// VenuesQuery is a request for a page of the venue list as it arrived from
// the client: nothing is defaulted or checked yet.
type VenuesQuery struct {
	Q       string
	Cuisine string
	OpenNow bool
	Sort    string
	Cursor  string
	Limit   *int
}

// VenuePage is a page of the venue list. NextCursor is empty on the last page.
type VenuePage struct {
	Venues     []catalog.Venue
	NextCursor string
}

// Cuisines returns the cuisine reference book.
func (s *Service) Cuisines(ctx context.Context) ([]catalog.Cuisine, error) {
	cuisines, err := s.venues.ListCuisines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cuisines: %w", err)
	}

	return cuisines, nil
}

// Venues returns a page of the venue list. It reads one row more than asked
// for: that row decides whether there is a next page,
// and never leaves the use case.
func (s *Service) Venues(ctx context.Context, query VenuesQuery) (VenuePage, error) {
	filter, err := buildFilter(query)
	if err != nil {
		return VenuePage{}, err
	}

	limit := filter.Limit
	filter.Limit = limit + 1

	venues, err := s.venues.ListVenues(ctx, filter)
	if err != nil {
		return VenuePage{}, fmt.Errorf("list venues: %w", err)
	}

	page := VenuePage{Venues: venues}
	if len(venues) > limit {
		page.Venues = venues[:limit]
		page.NextCursor = encodeCursor(page.Venues[limit-1], filter.Sort)
	}

	return page, nil
}

// Venue returns one venue card.
func (s *Service) Venue(ctx context.Context, id uuid.UUID) (catalog.Venue, error) {
	venue, err := s.venues.GetVenue(ctx, id)
	if err != nil {
		return catalog.Venue{}, fmt.Errorf("get venue %s: %w", id, err)
	}

	return venue, nil
}

// Menu returns the menu of a venue together with the state of its shift.
func (s *Service) Menu(ctx context.Context, venueID uuid.UUID) (catalog.Menu, error) {
	menu, err := s.menus.VenueMenu(ctx, venueID)
	if err != nil {
		return catalog.Menu{}, fmt.Errorf("get menu of venue %s: %w", venueID, err)
	}

	return menu, nil
}

func buildFilter(query VenuesQuery) (catalog.VenueFilter, error) {
	limit := defaultLimit
	if query.Limit != nil {
		limit = *query.Limit
	}

	if limit < 1 || limit > maxLimit {
		return catalog.VenueFilter{}, domain.InvalidArgumentf(
			"limit must be between 1 and %d", maxLimit)
	}

	q := strings.TrimSpace(query.Q)
	if !utf8.ValidString(q) {
		return catalog.VenueFilter{}, domain.InvalidArgumentf("q must be valid UTF-8")
	}

	if utf8.RuneCountInString(q) > maxQueryLen {
		return catalog.VenueFilter{}, domain.InvalidArgumentf(
			"q must be at most %d characters", maxQueryLen)
	}

	sort := catalog.SortByName
	if query.Sort != "" {
		sort = catalog.VenueSort(query.Sort)
		if !sort.Valid() {
			return catalog.VenueFilter{}, domain.InvalidArgumentf(
				"unknown sort order %q", query.Sort)
		}
	}

	filter := catalog.VenueFilter{
		Q:       q,
		Cuisine: strings.TrimSpace(query.Cuisine),
		OpenNow: query.OpenNow,
		Sort:    sort,
		Limit:   limit,
	}

	if query.Cursor != "" {
		after, err := decodeCursor(query.Cursor, sort)
		if err != nil {
			return catalog.VenueFilter{}, err
		}

		filter.After = after
	}

	return filter, nil
}
