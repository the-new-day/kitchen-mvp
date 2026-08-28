package order

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/order"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// cursorPayload is what an opaque cursor carries: the position of the last row
// of the page in the ordering the list always uses, newest first.
type cursorPayload struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"id"`
}

// encodeCursor packs the position of the last order of a page. Paging never
// falls back to an offset and stays correct when orders are placed between
// requests.
func encodeCursor(last order.Order) string {
	raw, err := json.Marshal(cursorPayload{CreatedAt: last.CreatedAt, ID: last.ID.String()})
	if err != nil {
		// cursorPayload is a time and a string; marshalling it cannot fail.
		panic(err)
	}

	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodeCursor parses a cursor of the order list.
func decodeCursor(cursor string) (*order.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	var payload cursorPayload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return nil, domain.InvalidArgumentf("cursor is malformed")
	}

	return &order.Cursor{CreatedAt: payload.CreatedAt, ID: id}, nil
}
