package catalog

import (
	"avito-kitchen/internal/domain"
	"strings"
	"unicode/utf8"
)

// Limits of the fields a venue may upload.
const (
	maxExternalIDLen  = 128
	maxNameLen        = 200
	maxDescriptionLen = 1000

	// positionGap is the step between the positions of two neighbours: the
	// order of the upload is spread out so that a row can later be put between
	// them without renumbering the rest.
	positionGap = 10
)

// MenuUpload is the whole menu of a venue as its own system keeps it. The
// order of the arrays is the order the venue shows its menu in.
type MenuUpload struct {
	Categories []UploadCategory
}

// UploadCategory is one category of an upload with the items it holds.
type UploadCategory struct {
	ExternalID string
	Name       string
	Items      []UploadItem
}

// UploadItem is one position of an upload. StockQty is nil when the venue
// keeps no count of it.
type UploadItem struct {
	ExternalID  string
	Name        string
	Description string
	Price       int64
	IsAvailable bool
	StockQty    *int
}

// MenuSnapshot is an upload flattened into the rows of the two menu tables,
// with the order of the arrays already turned into positions.
type MenuSnapshot struct {
	Categories []CategoryRow
	Items      []ItemRow
}

// CategoryRow is a category of a snapshot.
type CategoryRow struct {
	ExternalID string
	Name       string
	Position   int
}

// ItemRow is a position of a snapshot, bound to its category by the
// identifier the venue gave it.
type ItemRow struct {
	CategoryExternalID string
	ExternalID         string
	Name               string
	Description        string
	Price              int64
	Position           int
	IsAvailable        bool
	StockQty           *int
}

// MenuSyncResult counts what an upload changed.
type MenuSyncResult struct {
	CategoriesTotal  int
	ItemsCreated     int
	ItemsUpdated     int
	ItemsDeactivated int
}

// Normalize checks the upload and flattens it into a snapshot: strings are
// trimmed and the index in the array becomes a position. An identifier may
// appear once in the whole upload, both among the categories and among the
// items: two rows with the same identifier cannot be upserted in one
// statement. An upload without categories takes the whole menu off sale.
func (u MenuUpload) Normalize() (MenuSnapshot, error) {
	snapshot := MenuSnapshot{Categories: make([]CategoryRow, 0, len(u.Categories))}
	seenCategories := make(map[string]struct{}, len(u.Categories))
	seenItems := make(map[string]struct{})

	for ci, category := range u.Categories {
		externalID, err := requiredField(category.ExternalID, "category external_id", maxExternalIDLen)
		if err != nil {
			return MenuSnapshot{}, err
		}

		name, err := requiredField(category.Name, "category name", maxNameLen)
		if err != nil {
			return MenuSnapshot{}, err
		}

		if _, seen := seenCategories[externalID]; seen {
			return MenuSnapshot{}, domain.InvalidArgumentf("category %s is listed twice", externalID)
		}
		seenCategories[externalID] = struct{}{}

		snapshot.Categories = append(snapshot.Categories, CategoryRow{
			ExternalID: externalID,
			Name:       name,
			Position:   position(ci),
		})

		for ii, item := range category.Items {
			row, err := normalizeItem(item, externalID, position(ii))
			if err != nil {
				return MenuSnapshot{}, err
			}

			if _, seen := seenItems[row.ExternalID]; seen {
				return MenuSnapshot{}, domain.InvalidArgumentf("item %s is listed twice", row.ExternalID)
			}
			seenItems[row.ExternalID] = struct{}{}

			snapshot.Items = append(snapshot.Items, row)
		}
	}

	return snapshot, nil
}

func normalizeItem(item UploadItem, categoryExternalID string, position int) (ItemRow, error) {
	externalID, err := requiredField(item.ExternalID, "item external_id", maxExternalIDLen)
	if err != nil {
		return ItemRow{}, err
	}

	name, err := requiredField(item.Name, "item name", maxNameLen)
	if err != nil {
		return ItemRow{}, err
	}

	description, err := optionalField(item.Description, "item description", maxDescriptionLen)
	if err != nil {
		return ItemRow{}, err
	}

	if item.Price < 0 {
		return ItemRow{}, domain.InvalidArgumentf("price of item %s must not be negative", externalID)
	}

	if item.StockQty != nil && *item.StockQty < 0 {
		return ItemRow{}, domain.InvalidArgumentf("stock_qty of item %s must not be negative", externalID)
	}

	return ItemRow{
		CategoryExternalID: categoryExternalID,
		ExternalID:         externalID,
		Name:               name,
		Description:        description,
		Price:              item.Price,
		Position:           position,
		IsAvailable:        item.IsAvailable,
		StockQty:           item.StockQty,
	}, nil
}

// position spreads the index of an array over the positions of the menu.
func position(index int) int {
	return (index + 1) * positionGap
}

func requiredField(value, field string, maxLen int) (string, error) {
	trimmed, err := optionalField(value, field, maxLen)
	if err != nil {
		return "", err
	}

	if trimmed == "" {
		return "", domain.InvalidArgumentf("%s must not be empty", field)
	}

	return trimmed, nil
}

func optionalField(value, field string, maxLen int) (string, error) {
	trimmed := strings.TrimSpace(value)

	if !utf8.ValidString(trimmed) {
		return "", domain.InvalidArgumentf("%s must be valid UTF-8", field)
	}

	if utf8.RuneCountInString(trimmed) > maxLen {
		return "", domain.InvalidArgumentf("%s must be at most %d characters", field, maxLen)
	}

	return trimmed, nil
}
