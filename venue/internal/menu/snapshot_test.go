package menu_test

import (
	"avito-kitchen/venue/internal/kitchen"
	"avito-kitchen/venue/internal/menu"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshot(t *testing.T) {
	t.Parallel()

	var (
		first  = kitchen.Category{ExternalID: "CAT-A", Name: "Первая", Position: 10}
		second = kitchen.Category{ExternalID: "CAT-B", Name: "Вторая", Position: 20}
	)

	dish := func(sku string, category kitchen.Category, position int) kitchen.Dish {
		return kitchen.Dish{
			SKU: sku, Name: sku, Price: 1_000, IsAvailable: true,
			Category: category, Position: position,
		}
	}

	cases := map[string]struct {
		dishes     []kitchen.Dish
		categories []string
		items      map[string][]string
	}{
		"empty nomenclature": {
			categories: []string{},
			items:      map[string][]string{},
		},
		"dishes are grouped and ordered by position": {
			dishes: []kitchen.Dish{
				dish("SKU-B2", second, 20),
				dish("SKU-A2", first, 20),
				dish("SKU-B1", second, 10),
				dish("SKU-A1", first, 10),
			},
			categories: []string{"CAT-A", "CAT-B"},
			items: map[string][]string{
				"CAT-A": {"SKU-A1", "SKU-A2"},
				"CAT-B": {"SKU-B1", "SKU-B2"},
			},
		},
		"dishes of one position keep a stable order": {
			dishes: []kitchen.Dish{
				dish("SKU-A2", first, 10),
				dish("SKU-A1", first, 10),
			},
			categories: []string{"CAT-A"},
			items:      map[string][]string{"CAT-A": {"SKU-A1", "SKU-A2"}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			snapshot := menu.Snapshot(tc.dishes)

			names := make([]string, 0, len(snapshot.Categories))
			for _, category := range snapshot.Categories {
				names = append(names, category.ExternalId)

				skus := make([]string, 0, len(category.Items))
				for _, item := range category.Items {
					skus = append(skus, item.ExternalId)
				}

				assert.Equal(t, tc.items[category.ExternalId], skus)
			}

			assert.Equal(t, tc.categories, names)
		})
	}
}

func TestSnapshotCarriesWhatThePlatformNeeds(t *testing.T) {
	t.Parallel()

	limited := 4
	snapshot := menu.Snapshot([]kitchen.Dish{{
		SKU: "SKU-CROISSANT", Name: "Круассан", Description: "Слоёный.",
		Price: 19_000, Stock: &limited, IsAvailable: false,
		Category: kitchen.Category{ExternalID: "CAT-PASTRY", Name: "Выпечка", Position: 10},
	}})

	require.Len(t, snapshot.Categories, 1)
	require.Len(t, snapshot.Categories[0].Items, 1)

	item := snapshot.Categories[0].Items[0]

	assert.Equal(t, "Выпечка", snapshot.Categories[0].Name)
	assert.Equal(t, int64(19_000), item.Price)
	assert.Equal(t, "Слоёный.", *item.Description)
	assert.Equal(t, 4, *item.StockQty)
	assert.False(t, *item.IsAvailable)
}

// TestSnapshotOfTheBakery keeps the shipped nomenclature uploadable: every
// dish belongs to a category and carries a price.
func TestSnapshotOfTheBakery(t *testing.T) {
	t.Parallel()

	snapshot := menu.Snapshot(menu.Dishes)
	require.NotEmpty(t, snapshot.Categories)

	seen := 0

	for _, category := range snapshot.Categories {
		assert.NotEmpty(t, category.ExternalId)
		assert.NotEmpty(t, category.Name)
		assert.NotEmpty(t, category.Items)

		for _, item := range category.Items {
			assert.NotEmpty(t, item.ExternalId)
			assert.NotEmpty(t, item.Name)
			assert.Positive(t, item.Price)

			seen++
		}
	}

	assert.Len(t, menu.Dishes, seen)
}
