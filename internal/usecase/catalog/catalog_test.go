package catalog_test

import (
	"avito-kitchen/internal/domain"
	domaincatalog "avito-kitchen/internal/domain/catalog"
	"avito-kitchen/internal/usecase/catalog"
	"avito-kitchen/internal/usecase/catalog/mocks"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

var (
	bakeryID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")
	pizzaID  = uuid.MustParse("0192f4c1-0000-7000-8000-000000000002")

	bakery = domaincatalog.Venue{ID: bakeryID, Slug: "baton", Name: "Пекарня «Батон»", DeliveryFee: 9_900}
	pizza  = domaincatalog.Venue{ID: pizzaID, Slug: "forno", Name: "Пиццерия «Форно»", DeliveryFee: 14_900}

	errRepo = errors.New("connection refused")
)

func TestServiceVenues(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		query      catalog.VenuesQuery
		setup      func(*mocks.MockVenueRepository)
		wantErr    error
		wantVenues []domaincatalog.Venue
		wantCursor bool
	}{
		"default limit and sort": {
			query: catalog.VenuesQuery{},
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().ListVenues(mock.Anything, domaincatalog.VenueFilter{
					Sort:  domaincatalog.SortByName,
					Limit: 21,
				}).Return([]domaincatalog.Venue{bakery, pizza}, nil).Once()
			},
			wantVenues: []domaincatalog.Venue{bakery, pizza},
		},
		"filters reach the repository": {
			query: catalog.VenuesQuery{
				Q:       "  пекар  ",
				Cuisine: "bakery",
				OpenNow: true,
				Sort:    "delivery_fee",
				Limit:   new(5),
			},
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().ListVenues(mock.Anything, domaincatalog.VenueFilter{
					Q:       "пекар",
					Cuisine: "bakery",
					OpenNow: true,
					Sort:    domaincatalog.SortByDeliveryFee,
					Limit:   6,
				}).Return([]domaincatalog.Venue{bakery}, nil).Once()
			},
			wantVenues: []domaincatalog.Venue{bakery},
		},
		"extra row becomes the next cursor": {
			query: catalog.VenuesQuery{Limit: new(1)},
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().ListVenues(mock.Anything, domaincatalog.VenueFilter{
					Sort:  domaincatalog.SortByName,
					Limit: 2,
				}).Return([]domaincatalog.Venue{bakery, pizza}, nil).Once()
			},
			wantVenues: []domaincatalog.Venue{bakery},
			wantCursor: true,
		},
		"last page has no cursor": {
			query: catalog.VenuesQuery{Limit: new(2)},
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().ListVenues(mock.Anything, mock.Anything).
					Return([]domaincatalog.Venue{bakery, pizza}, nil).Once()
			},
			wantVenues: []domaincatalog.Venue{bakery, pizza},
		},
		"limit below the range": {
			query:   catalog.VenuesQuery{Limit: new(0)},
			wantErr: domain.ErrInvalidArgument,
		},
		"limit above the range": {
			query:   catalog.VenuesQuery{Limit: new(101)},
			wantErr: domain.ErrInvalidArgument,
		},
		"search term too long": {
			query:   catalog.VenuesQuery{Q: strings.Repeat("я", 101)},
			wantErr: domain.ErrInvalidArgument,
		},
		"search term is not utf-8": {
			query:   catalog.VenuesQuery{Q: string([]byte{0xEF, 0xE5, 0xEA})},
			wantErr: domain.ErrInvalidArgument,
		},
		"unknown sort order": {
			query:   catalog.VenuesQuery{Sort: "rating"},
			wantErr: domain.ErrInvalidArgument,
		},
		"malformed cursor": {
			query:   catalog.VenuesQuery{Cursor: "не курсор"},
			wantErr: domain.ErrInvalidArgument,
		},
		"repository fails": {
			query: catalog.VenuesQuery{},
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().ListVenues(mock.Anything, mock.Anything).
					Return(nil, errRepo).Once()
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			venues := mocks.NewMockVenueRepository(t)
			if tc.setup != nil {
				tc.setup(venues)
			}

			page, err := catalog.New(venues, mocks.NewMockMenuRepository(t)).
				Venues(context.Background(), tc.query)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(page.Venues) != len(tc.wantVenues) {
				t.Fatalf("got %d venues, want %d", len(page.Venues), len(tc.wantVenues))
			}
			for i, want := range tc.wantVenues {
				if page.Venues[i].ID != want.ID {
					t.Errorf("venue %d = %s, want %s", i, page.Venues[i].ID, want.ID)
				}
			}

			if (page.NextCursor != "") != tc.wantCursor {
				t.Errorf("next cursor = %q, want present = %v", page.NextCursor, tc.wantCursor)
			}
		})
	}
}

func TestServiceCursorContinuesThePage(t *testing.T) {
	t.Parallel()

	venues := mocks.NewMockVenueRepository(t)
	venues.EXPECT().ListVenues(mock.Anything, mock.Anything).
		Return([]domaincatalog.Venue{bakery, pizza}, nil).Once()

	service := catalog.New(venues, mocks.NewMockMenuRepository(t))

	first, err := service.Venues(context.Background(), catalog.VenuesQuery{Limit: new(1)})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}

	venues.EXPECT().ListVenues(mock.Anything, domaincatalog.VenueFilter{
		Sort:  domaincatalog.SortByName,
		Limit: 2,
		After: &domaincatalog.VenueCursor{
			Sort: domaincatalog.SortByName,
			Key:  bakery.Name,
			ID:   bakery.ID,
		},
	}).Return([]domaincatalog.Venue{pizza}, nil).Once()

	second, err := service.Venues(context.Background(),
		catalog.VenuesQuery{Limit: new(1), Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}

	if second.NextCursor != "" {
		t.Errorf("next cursor = %q, want empty on the last page", second.NextCursor)
	}
}

func TestServiceVenue(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		repoErr error
		wantErr error
	}{
		"found":              {},
		"unknown venue":      {repoErr: domain.ErrNotFound, wantErr: domain.ErrNotFound},
		"repository failure": {repoErr: errRepo, wantErr: errRepo},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			venues := mocks.NewMockVenueRepository(t)
			venues.EXPECT().GetVenue(mock.Anything, bakeryID).Return(bakery, tc.repoErr).Once()

			venue, err := catalog.New(venues, mocks.NewMockMenuRepository(t)).
				Venue(context.Background(), bakeryID)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if venue.ID != bakeryID {
				t.Errorf("venue = %s, want %s", venue.ID, bakeryID)
			}
		})
	}
}

func TestServiceMenu(t *testing.T) {
	t.Parallel()

	menu := domaincatalog.Menu{VenueID: pizzaID, VenueIsOpen: true}

	cases := map[string]struct {
		repoErr error
		wantErr error
	}{
		"found":         {},
		"unknown venue": {repoErr: domain.ErrNotFound, wantErr: domain.ErrNotFound},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			menus := mocks.NewMockMenuRepository(t)
			menus.EXPECT().VenueMenu(mock.Anything, pizzaID).Return(menu, tc.repoErr).Once()

			got, err := catalog.New(mocks.NewMockVenueRepository(t), menus).
				Menu(context.Background(), pizzaID)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.VenueID != pizzaID || !got.VenueIsOpen {
				t.Errorf("menu = %+v, want the menu of the open venue %s", got, pizzaID)
			}
		})
	}
}
