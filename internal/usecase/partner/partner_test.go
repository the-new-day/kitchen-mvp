package partner_test

import (
	"avito-kitchen/internal/domain"
	domaincatalog "avito-kitchen/internal/domain/catalog"
	"avito-kitchen/internal/usecase/partner"
	"avito-kitchen/internal/usecase/partner/mocks"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

var (
	bakeryID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")

	apiKey     = "vk_demo_bakery_dev"
	apiKeyHash = sha256.Sum256([]byte(apiKey))

	errRepo = errors.New("connection refused")
)

func TestServiceAuthenticate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		key     string
		setup   func(*mocks.MockVenueRepository)
		wantErr error
	}{
		"the key of a venue": {
			key: apiKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, apiKeyHash[:]).
					Return(domaincatalog.VenueKey{VenueID: bakeryID, Hash: apiKeyHash[:]}, nil).Once()
			},
		},
		"no key at all": {
			wantErr: domain.ErrUnauthenticated,
		},
		"an unknown or revoked key": {
			key: apiKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, mock.Anything).
					Return(domaincatalog.VenueKey{}, domain.ErrNotFound).Once()
			},
			wantErr: domain.ErrUnauthenticated,
		},
		"a stored hash that does not match": {
			key: apiKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, mock.Anything).
					Return(domaincatalog.VenueKey{VenueID: bakeryID, Hash: []byte("another hash")}, nil).Once()
			},
			wantErr: domain.ErrUnauthenticated,
		},
		"the database is down": {
			key: apiKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, mock.Anything).
					Return(domaincatalog.VenueKey{}, errRepo).Once()
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

			id, err := partner.New(venues, mocks.NewMockMenuRepository(t)).
				Authenticate(context.Background(), tc.key)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if id != uuid.Nil {
					t.Errorf("venue = %s, want none", id)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != bakeryID {
				t.Errorf("venue = %s, want %s", id, bakeryID)
			}
		})
	}
}

func TestServiceSyncMenu(t *testing.T) {
	t.Parallel()

	upload := domaincatalog.MenuUpload{Categories: []domaincatalog.UploadCategory{{
		ExternalID: "CAT-BREAD",
		Name:       "Хлеб",
		Items: []domaincatalog.UploadItem{
			{ExternalID: "SKU-SOURDOUGH", Name: "На закваске", Price: 25_000, IsAvailable: true},
		},
	}}}

	snapshot := domaincatalog.MenuSnapshot{
		Categories: []domaincatalog.CategoryRow{{ExternalID: "CAT-BREAD", Name: "Хлеб", Position: 10}},
		Items: []domaincatalog.ItemRow{{
			CategoryExternalID: "CAT-BREAD", ExternalID: "SKU-SOURDOUGH", Name: "На закваске",
			Price: 25_000, Position: 10, IsAvailable: true,
		}},
	}

	duplicates := domaincatalog.MenuUpload{Categories: []domaincatalog.UploadCategory{
		{ExternalID: "CAT-BREAD", Name: "Хлеб"},
		{ExternalID: "CAT-BREAD", Name: "Хлеб"},
	}}

	cases := map[string]struct {
		upload  domaincatalog.MenuUpload
		setup   func(*mocks.MockMenuRepository)
		want    domaincatalog.MenuSyncResult
		wantErr error
	}{
		"the normalized snapshot reaches the repository": {
			upload: upload,
			setup: func(m *mocks.MockMenuRepository) {
				m.EXPECT().SyncMenu(mock.Anything, bakeryID, snapshot).
					Return(domaincatalog.MenuSyncResult{CategoriesTotal: 1, ItemsCreated: 1}, nil).Once()
			},
			want: domaincatalog.MenuSyncResult{CategoriesTotal: 1, ItemsCreated: 1},
		},
		"an invalid upload does not reach the repository": {
			upload:  duplicates,
			wantErr: domain.ErrInvalidArgument,
		},
		"the database is down": {
			upload: upload,
			setup: func(m *mocks.MockMenuRepository) {
				m.EXPECT().SyncMenu(mock.Anything, mock.Anything, mock.Anything).
					Return(domaincatalog.MenuSyncResult{}, errRepo).Once()
			},
			wantErr: errRepo,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			menus := mocks.NewMockMenuRepository(t)
			if tc.setup != nil {
				tc.setup(menus)
			}

			result, err := partner.New(mocks.NewMockVenueRepository(t), menus).
				SyncMenu(context.Background(), bakeryID, tc.upload)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.want {
				t.Errorf("result = %+v, want %+v", result, tc.want)
			}
		})
	}
}

func TestServicePatchItem(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		patch   domaincatalog.MenuItemPatch
		setup   func(*mocks.MockMenuRepository)
		wantErr error
	}{
		"a stop list entry": {
			patch: domaincatalog.MenuItemPatch{IsAvailable: new(false)},
			setup: func(m *mocks.MockMenuRepository) {
				m.EXPECT().PatchItem(mock.Anything, bakeryID, "SKU-CROISSANT",
					domaincatalog.MenuItemPatch{IsAvailable: new(false)}).
					Return(domaincatalog.CategorizedMenuItem{
						MenuItem:           domaincatalog.MenuItem{ExternalID: "SKU-CROISSANT"},
						CategoryExternalID: "CAT-BREAD",
					}, nil).Once()
			},
		},
		"a negative price does not reach the repository": {
			patch:   domaincatalog.MenuItemPatch{Price: new(int64(-1))},
			wantErr: domain.ErrInvalidArgument,
		},
		"an item of another venue": {
			patch: domaincatalog.MenuItemPatch{},
			setup: func(m *mocks.MockMenuRepository) {
				m.EXPECT().PatchItem(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(domaincatalog.CategorizedMenuItem{}, domain.ErrNotFound).Once()
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			menus := mocks.NewMockMenuRepository(t)
			if tc.setup != nil {
				tc.setup(menus)
			}

			item, err := partner.New(mocks.NewMockVenueRepository(t), menus).
				PatchItem(context.Background(), bakeryID, "SKU-CROISSANT", tc.patch)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if item.ExternalID != "SKU-CROISSANT" || item.CategoryExternalID != "CAT-BREAD" {
				t.Errorf("item = %+v, want SKU-CROISSANT of CAT-BREAD", item)
			}
		})
	}
}

func TestServiceShift(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		open    bool
		repoErr error
		wantErr error
	}{
		"the shift opens":      {open: true},
		"the shift closes":     {open: false},
		"an unknown venue":     {open: true, repoErr: domain.ErrNotFound, wantErr: domain.ErrNotFound},
		"the database is down": {open: false, repoErr: errRepo, wantErr: errRepo},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			venues := mocks.NewMockVenueRepository(t)
			venues.EXPECT().SetShift(mock.Anything, bakeryID, tc.open).Return(tc.open, tc.repoErr).Once()

			service := partner.New(venues, mocks.NewMockMenuRepository(t))

			shift := service.CloseShift
			if tc.open {
				shift = service.OpenShift
			}

			isOpen, err := shift(context.Background(), bakeryID)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if isOpen != tc.open {
				t.Errorf("is_open = %v, want %v", isOpen, tc.open)
			}
		})
	}
}

func TestServiceVenue(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		repoErr error
		wantErr error
	}{
		"the venue of the key": {},
		"an unknown venue":     {repoErr: domain.ErrNotFound, wantErr: domain.ErrNotFound},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			venues := mocks.NewMockVenueRepository(t)
			venues.EXPECT().GetVenue(mock.Anything, bakeryID).
				Return(domaincatalog.Venue{ID: bakeryID, Slug: "baton"}, tc.repoErr).Once()

			venue, err := partner.New(venues, mocks.NewMockMenuRepository(t)).
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
			if venue.Slug != "baton" {
				t.Errorf("venue = %+v, want the bakery", venue)
			}
		})
	}
}
