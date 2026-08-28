package partner

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/transport/http/apierr"
	partnerusecase "avito-kitchen/internal/usecase/partner"
	"avito-kitchen/internal/usecase/partner/mocks"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

const demoKey = "vk_demo_bakery_dev"

var demoVenueID = uuid.MustParse("0192f4c1-0000-7000-8000-000000000001")

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	hash := sha256.Sum256([]byte(demoKey))

	cases := map[string]struct {
		key        string
		setup      func(*mocks.MockVenueRepository)
		wantStatus int
		wantCode   string
		wantVenue  uuid.UUID
	}{
		"the key of a venue": {
			key: demoKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, hash[:]).
					Return(catalog.VenueKey{VenueID: demoVenueID, Hash: hash[:]}, nil).Once()
			},
			wantStatus: http.StatusOK,
			wantVenue:  demoVenueID,
		},
		"no key at all": {
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		"an unknown key": {
			key: demoKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, mock.Anything).
					Return(catalog.VenueKey{}, domain.ErrNotFound).Once()
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		"the database is down": {
			key: demoKey,
			setup: func(m *mocks.MockVenueRepository) {
				m.EXPECT().VenueByKeyHash(mock.Anything, mock.Anything).
					Return(catalog.VenueKey{}, errors.New("connection refused")).Once()
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			venues := mocks.NewMockVenueRepository(t)
			if tc.setup != nil {
				tc.setup(venues)
			}

			var logs bytes.Buffer

			service := partnerusecase.New(venues, mocks.NewMockMenuRepository(t))
			errs := apierr.Handler{Log: slog.New(slog.NewJSONHandler(&logs, nil))}

			var served uuid.UUID

			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				served = venueID(r.Context())
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/partner/me", nil)

			if tc.key != "" {
				request.Header.Set(authHeader, tc.key)
			}

			authenticate(service, errs)(next).ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}

			if served != tc.wantVenue {
				t.Errorf("venue in the context = %s, want %s", served, tc.wantVenue)
			}

			if tc.wantCode != "" {
				var body httpx.ErrorBody
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
				}

				if body.Code != tc.wantCode {
					t.Errorf("code = %q, want %q", body.Code, tc.wantCode)
				}
			}

			if strings.Contains(logs.String(), demoKey) || strings.Contains(recorder.Body.String(), demoKey) {
				t.Error("the api key left the middleware")
			}
		})
	}
}
