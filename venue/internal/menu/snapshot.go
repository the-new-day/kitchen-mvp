package menu

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/venue/internal/kitchen"
	"sort"
)

// Snapshot builds the menu upload out of the dishes of the venue. The order of
// the arrays is the order of display: categories by their position, dishes by
// theirs, so the platform needs no sorting rule of its own.
func Snapshot(dishes []kitchen.Dish) partnerapi.MenuSyncRequest {
	sorted := make([]kitchen.Dish, len(dishes))
	copy(sorted, dishes)

	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]

		if left.Category.Position != right.Category.Position {
			return left.Category.Position < right.Category.Position
		}

		if left.Category.ExternalID != right.Category.ExternalID {
			return left.Category.ExternalID < right.Category.ExternalID
		}

		if left.Position != right.Position {
			return left.Position < right.Position
		}

		return left.SKU < right.SKU
	})

	categories := make([]partnerapi.MenuSyncCategory, 0, len(sorted))
	at := make(map[string]int, len(sorted))

	for _, dish := range sorted {
		index, ok := at[dish.Category.ExternalID]
		if !ok {
			categories = append(categories, partnerapi.MenuSyncCategory{
				ExternalId: dish.Category.ExternalID,
				Name:       dish.Category.Name,
				Items:      []partnerapi.MenuSyncItem{},
			})

			index = len(categories) - 1
			at[dish.Category.ExternalID] = index
		}

		categories[index].Items = append(categories[index].Items, item(dish))
	}

	return partnerapi.MenuSyncRequest{Categories: categories}
}

func item(dish kitchen.Dish) partnerapi.MenuSyncItem {
	synced := partnerapi.MenuSyncItem{
		ExternalId:  dish.SKU,
		Name:        dish.Name,
		Price:       dish.Price,
		IsAvailable: &dish.IsAvailable,
		StockQty:    dish.Stock,
	}

	if dish.Description != "" {
		synced.Description = &dish.Description
	}

	return synced
}
