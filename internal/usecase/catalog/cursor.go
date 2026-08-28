package catalog

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
)

// cursorPayload is what an opaque cursor carries: the ordering it was issued
// for and the position of the last row of the page.
type cursorPayload struct {
	Sort string `json:"s"`
	Key  string `json:"k"`
	ID   string `json:"id"`
}

// encodeCursor packs the position of the last venue of a page. The value is
// keyed on the ordering, so paging never falls back to an offset and stays
// correct when rows are inserted between requests.
func encodeCursor(v catalog.Venue, sort catalog.VenueSort) string {
	payload := cursorPayload{
		Sort: string(sort),
		Key:  sortKey(v, sort),
		ID:   v.ID.String(),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		// cursorPayload is three strings; marshalling it cannot fail.
		panic(err)
	}

	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodeCursor parses a cursor and checks that it belongs
// to the requested ordering.
func decodeCursor(cursor string, sort catalog.VenueSort) (*catalog.VenueCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	var payload cursorPayload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	if catalog.VenueSort(payload.Sort) != sort {
		return nil, domain.InvalidArgumentf("cursor belongs to another sort order")
	}

	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	return &catalog.VenueCursor{Sort: sort, Key: payload.Key, ID: id}, nil
}

func sortKey(v catalog.Venue, sort catalog.VenueSort) string {
	switch sort {
	case catalog.SortByDeliveryFee:
		return strconv.FormatInt(v.DeliveryFee, 10)
	case catalog.SortByAvgCookMinutes:
		return strconv.Itoa(v.AvgCookMinutes)
	case catalog.SortByName:
		return v.Name
	default:
		return v.Name
	}
}
