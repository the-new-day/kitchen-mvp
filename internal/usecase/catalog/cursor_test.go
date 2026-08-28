package catalog

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var cursorVenue = catalog.Venue{
	ID:             uuid.MustParse("0192f4c1-0000-7000-8000-000000000002"),
	Name:           "Пиццерия «Форно»",
	DeliveryFee:    14_900,
	AvgCookMinutes: 25,
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		sort catalog.VenueSort
		key  string
	}{
		"by name":             {sort: catalog.SortByName, key: cursorVenue.Name},
		"by delivery fee":     {sort: catalog.SortByDeliveryFee, key: "14900"},
		"by avg cook minutes": {sort: catalog.SortByAvgCookMinutes, key: "25"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			decoded, err := decodeCursor(encodeCursor(cursorVenue, tc.sort), tc.sort)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if decoded.Sort != tc.sort {
				t.Errorf("sort = %q, want %q", decoded.Sort, tc.sort)
			}
			if decoded.Key != tc.key {
				t.Errorf("key = %q, want %q", decoded.Key, tc.key)
			}
			if decoded.ID != cursorVenue.ID {
				t.Errorf("id = %s, want %s", decoded.ID, cursorVenue.ID)
			}
		})
	}
}

func TestCursorIsOpaqueAndValidated(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cursor string
		sort   catalog.VenueSort
	}{
		"not base64":             {cursor: "не курсор", sort: catalog.SortByName},
		"base64 of garbage":      {cursor: "Zm9vYmFy", sort: catalog.SortByName},
		"plain venue identifier": {cursor: cursorVenue.ID.String(), sort: catalog.SortByName},
		"issued for another sort": {
			cursor: encodeCursor(cursorVenue, catalog.SortByDeliveryFee),
			sort:   catalog.SortByName,
		},
		"identifier is not a uuid": {
			cursor: base64.RawURLEncoding.EncodeToString(
				[]byte(`{"s":"name","k":"Пекарня","id":"тридцать семь"}`)),
			sort: catalog.SortByName,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := decodeCursor(tc.cursor, tc.sort); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}
