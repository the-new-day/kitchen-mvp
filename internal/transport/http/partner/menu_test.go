package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/catalog"
	"testing"
)

func TestToMenuUploadItem(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		item partnerapi.MenuSyncItem
		want catalog.UploadItem
	}{
		"an item that says nothing about its availability is on sale": {
			item: partnerapi.MenuSyncItem{ExternalId: "SKU-CROISSANT", Name: "Круассан", Price: 12_000},
			want: catalog.UploadItem{
				ExternalID: "SKU-CROISSANT", Name: "Круассан", Price: 12_000, IsAvailable: true,
			},
		},
		"an item taken off the menu": {
			item: partnerapi.MenuSyncItem{
				ExternalId: "SKU-CROISSANT", Name: "Круассан", IsAvailable: new(false),
			},
			want: catalog.UploadItem{ExternalID: "SKU-CROISSANT", Name: "Круассан"},
		},
		"a description and a stock": {
			item: partnerapi.MenuSyncItem{
				ExternalId:  "SKU-CROISSANT",
				Name:        "Круассан",
				Description: new("Слоёный, с маслом."),
				StockQty:    new(7),
			},
			want: catalog.UploadItem{
				ExternalID:  "SKU-CROISSANT",
				Name:        "Круассан",
				Description: "Слоёный, с маслом.",
				IsAvailable: true,
				StockQty:    new(7),
			},
		},
		"an item without a stock is unlimited": {
			item: partnerapi.MenuSyncItem{ExternalId: "SKU-BAGUETTE", Name: "Багет", IsAvailable: new(true)},
			want: catalog.UploadItem{ExternalID: "SKU-BAGUETTE", Name: "Багет", IsAvailable: true},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			upload := toMenuUpload(partnerapi.MenuSyncRequest{
				Categories: []partnerapi.MenuSyncCategory{{
					ExternalId: "CAT-BREAD",
					Name:       "Выпечка",
					Items:      []partnerapi.MenuSyncItem{tc.item},
				}},
			})

			if len(upload.Categories) != 1 || len(upload.Categories[0].Items) != 1 {
				t.Fatalf("upload = %+v, want one category with one item", upload)
			}

			got := upload.Categories[0].Items[0]

			if got.ExternalID != tc.want.ExternalID || got.Name != tc.want.Name ||
				got.Description != tc.want.Description || got.Price != tc.want.Price ||
				got.IsAvailable != tc.want.IsAvailable {
				t.Errorf("item = %+v, want %+v", got, tc.want)
			}

			switch {
			case tc.want.StockQty == nil && got.StockQty != nil:
				t.Errorf("stock_qty = %d, want unlimited", *got.StockQty)
			case tc.want.StockQty != nil && (got.StockQty == nil || *got.StockQty != *tc.want.StockQty):
				t.Errorf("stock_qty = %v, want %d", got.StockQty, *tc.want.StockQty)
			}
		})
	}
}

func TestToPartnerMenuItem(t *testing.T) {
	t.Parallel()

	item := toPartnerMenuItem(catalog.CategorizedMenuItem{
		MenuItem: catalog.MenuItem{
			ExternalID:  "SKU-CROISSANT",
			Name:        "Круассан",
			Price:       12_000,
			IsAvailable: false,
			StockQty:    new(0),
		},
		CategoryExternalID: "CAT-BREAD",
	})

	if item.ExternalId != "SKU-CROISSANT" || item.Price != 12_000 || item.IsAvailable {
		t.Errorf("item = %+v, want the stopped croissant", item)
	}
	if item.CategoryExternalId == nil || *item.CategoryExternalId != "CAT-BREAD" {
		t.Errorf("category_external_id = %v, want CAT-BREAD", item.CategoryExternalId)
	}
	if item.Description != nil {
		t.Errorf("description = %q, want none", *item.Description)
	}
	if item.StockQty == nil || *item.StockQty != 0 {
		t.Errorf("stock_qty = %v, want 0", item.StockQty)
	}
}

func TestToPartnerVenue(t *testing.T) {
	t.Parallel()

	server := &Server{ordersTopic: "kitchen.orders.v1"}

	venue := server.toPartnerVenue(catalog.Venue{
		ID:             demoVenueID,
		Slug:           "baton",
		Name:           "Пекарня «Батон»",
		Address:        "Москва, Лесная ул., 5",
		IsOpen:         true,
		MinOrderAmount: 50_000,
		DeliveryFee:    9_900,
		AvgCookMinutes: 15,
	})

	if venue.VenueId != demoVenueID || venue.Slug != "baton" || !venue.IsOpen {
		t.Errorf("venue = %+v, want the open bakery", venue)
	}
	if venue.Address == nil || *venue.Address != "Москва, Лесная ул., 5" {
		t.Errorf("address = %v, want the address of the bakery", venue.Address)
	}
	if venue.OrdersTopic == nil || *venue.OrdersTopic != "kitchen.orders.v1" {
		t.Errorf("orders_topic = %v, want kitchen.orders.v1", venue.OrdersTopic)
	}
}
