// Package catalog holds the entities of the venue catalogue and the rules
// that decide whether a menu item can be ordered right now.
package catalog

import (
	"avito-kitchen/internal/domain"

	"github.com/google/uuid"
)

// Cuisine is an entry of the cuisine reference book.
type Cuisine struct {
	Slug string
	Name string
}

// Venue is a venue card as the catalogue shows it. Money is in kopecks.
type Venue struct {
	ID             uuid.UUID
	Slug           string
	Name           string
	Description    string
	Address        string
	Lat            *float64
	Lon            *float64
	Cuisines       []Cuisine
	IsOpen         bool
	MinOrderAmount int64
	DeliveryFee    int64
	AvgCookMinutes int
}

// MenuItem is a position of a venue menu.
// StockQty is nil when the venue keeps no count of it.
type MenuItem struct {
	ID          uuid.UUID
	ExternalID  string
	Name        string
	Description string
	Price       int64
	Position    int
	IsAvailable bool
	StockQty    *int
}

// MenuCategory groups the items of a menu.
type MenuCategory struct {
	ID         uuid.UUID
	ExternalID string
	Name       string
	Position   int
	Items      []MenuItem
}

// Menu is the whole menu of a venue together with the state of its shift:
// availability of an item depends on both.
type Menu struct {
	VenueID     uuid.UUID
	VenueIsOpen bool
	Categories  []MenuCategory
}

// VenueSort names the ordering of the venue list.
type VenueSort string

// Orderings accepted by VenueFilter.
const (
	SortByName           VenueSort = "name"
	SortByDeliveryFee    VenueSort = "delivery_fee"
	SortByAvgCookMinutes VenueSort = "avg_cook_minutes"
)

// Valid reports whether s is a known ordering.
func (s VenueSort) Valid() bool {
	switch s {
	case SortByName, SortByDeliveryFee, SortByAvgCookMinutes:
		return true
	default:
		return false
	}
}

// VenueFilter selects and orders a page of the venue list. An empty Q or
// Cuisine means the filter is not applied; Limit is the max number of rows the
// repository must return.
type VenueFilter struct {
	Q       string
	Cuisine string
	OpenNow bool
	Sort    VenueSort
	After   *VenueCursor
	Limit   int
}

// VenueCursor is the position of the last row of the previous page: the value
// of the sort key and the identifier that breaks ties between equal keys.
type VenueCursor struct {
	Sort VenueSort
	Key  string
	ID   uuid.UUID
}

// CategorizedMenuItem is a menu item together with the identifier its category
// carries in the system of the venue.
type CategorizedMenuItem struct {
	MenuItem

	CategoryExternalID string
}

// MenuItemPatch changes single fields of one menu item.
// A nil field is left as it is.
type MenuItemPatch struct {
	Price       *int64
	IsAvailable *bool
	StockQty    *int
}

// Validate reports whether the patch may be applied.
func (p MenuItemPatch) Validate() error {
	switch {
	case p.Price != nil && *p.Price < 0:
		return domain.InvalidArgumentf("price must not be negative")
	case p.StockQty != nil && *p.StockQty < 0:
		return domain.InvalidArgumentf("stock_qty must not be negative")
	default:
		return nil
	}
}

// VenueKey is an API key of a venue as the platform stores it: the venue it
// was issued to and the hash of the key itself.
type VenueKey struct {
	VenueID uuid.UUID
	Hash    []byte
}
