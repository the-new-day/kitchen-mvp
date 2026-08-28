package catalog_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMenuUploadNormalize(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		upload  catalog.MenuUpload
		want    catalog.MenuSnapshot
		wantErr error
	}{
		"order of the arrays becomes positions with a gap": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{
					ExternalID: "CAT-PIZZA",
					Name:       "Пицца",
					Items: []catalog.UploadItem{
						{ExternalID: "SKU-MARGHERITA", Name: "Маргарита", Price: 59_000, IsAvailable: true, StockQty: new(20)},
						{ExternalID: "SKU-PEPPERONI", Name: "Пепперони", Price: 69_000},
					},
				},
				{
					ExternalID: "CAT-DRINKS",
					Name:       "Напитки",
					Items: []catalog.UploadItem{
						{ExternalID: "SKU-ESPRESSO", Name: "Эспрессо", Price: 15_000, IsAvailable: true},
					},
				},
			}},
			want: catalog.MenuSnapshot{
				Categories: []catalog.CategoryRow{
					{ExternalID: "CAT-PIZZA", Name: "Пицца", Position: 10},
					{ExternalID: "CAT-DRINKS", Name: "Напитки", Position: 20},
				},
				Items: []catalog.ItemRow{
					{
						CategoryExternalID: "CAT-PIZZA", ExternalID: "SKU-MARGHERITA", Name: "Маргарита",
						Price: 59_000, Position: 10, IsAvailable: true, StockQty: new(20),
					},
					{
						CategoryExternalID: "CAT-PIZZA", ExternalID: "SKU-PEPPERONI", Name: "Пепперони",
						Price: 69_000, Position: 20,
					},
					{
						CategoryExternalID: "CAT-DRINKS", ExternalID: "SKU-ESPRESSO", Name: "Эспрессо",
						Price: 15_000, Position: 10, IsAvailable: true,
					},
				},
			},
		},
		"strings are trimmed": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{{
				ExternalID: "  CAT-PIZZA  ",
				Name:       " Пицца ",
				Items: []catalog.UploadItem{
					{ExternalID: " SKU-MARGHERITA ", Name: " Маргарита ", Description: " Томаты. ", IsAvailable: true},
				},
			}}},
			want: catalog.MenuSnapshot{
				Categories: []catalog.CategoryRow{{ExternalID: "CAT-PIZZA", Name: "Пицца", Position: 10}},
				Items: []catalog.ItemRow{{
					CategoryExternalID: "CAT-PIZZA", ExternalID: "SKU-MARGHERITA", Name: "Маргарита",
					Description: "Томаты.", Position: 10, IsAvailable: true,
				}},
			},
		},
		"an upload without categories takes the whole menu off sale": {
			upload: catalog.MenuUpload{},
			want:   catalog.MenuSnapshot{Categories: []catalog.CategoryRow{}},
		},
		"a category without items is kept": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{ExternalID: "CAT-SOUPS", Name: "Супы"},
			}},
			want: catalog.MenuSnapshot{
				Categories: []catalog.CategoryRow{{ExternalID: "CAT-SOUPS", Name: "Супы", Position: 10}},
			},
		},
		"a category listed twice": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{ExternalID: "CAT-PIZZA", Name: "Пицца"},
				{ExternalID: "CAT-PIZZA", Name: "Пицца ещё раз"},
			}},
			wantErr: domain.ErrInvalidArgument,
		},
		"an item listed twice in two categories": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{
					ExternalID: "CAT-PIZZA", Name: "Пицца",
					Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA", Name: "Маргарита"}},
				},
				{
					ExternalID: "CAT-DRINKS", Name: "Напитки",
					Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA", Name: "Маргарита"}},
				},
			}},
			wantErr: domain.ErrInvalidArgument,
		},
		"a category without an identifier": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{ExternalID: "   ", Name: "Пицца"},
			}},
			wantErr: domain.ErrInvalidArgument,
		},
		"a category without a name": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{ExternalID: "CAT-PIZZA"},
			}},
			wantErr: domain.ErrInvalidArgument,
		},
		"an identifier longer than the contract allows": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{
				{ExternalID: strings.Repeat("C", 129), Name: "Пицца"},
			}},
			wantErr: domain.ErrInvalidArgument,
		},
		"an item name longer than the contract allows": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{{
				ExternalID: "CAT-PIZZA", Name: "Пицца",
				Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA", Name: strings.Repeat("я", 201)}},
			}}},
			wantErr: domain.ErrInvalidArgument,
		},
		"an item without a name": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{{
				ExternalID: "CAT-PIZZA", Name: "Пицца",
				Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA"}},
			}}},
			wantErr: domain.ErrInvalidArgument,
		},
		"a negative price": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{{
				ExternalID: "CAT-PIZZA", Name: "Пицца",
				Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA", Name: "Маргарита", Price: -1}},
			}}},
			wantErr: domain.ErrInvalidArgument,
		},
		"a negative stock": {
			upload: catalog.MenuUpload{Categories: []catalog.UploadCategory{{
				ExternalID: "CAT-PIZZA", Name: "Пицца",
				Items: []catalog.UploadItem{{ExternalID: "SKU-MARGHERITA", Name: "Маргарита", StockQty: new(-1)}},
			}}},
			wantErr: domain.ErrInvalidArgument,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.upload.Normalize()

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got.Categories, tc.want.Categories) {
				t.Errorf("categories = %+v, want %+v", got.Categories, tc.want.Categories)
			}
			if !reflect.DeepEqual(got.Items, tc.want.Items) {
				t.Errorf("items = %+v, want %+v", got.Items, tc.want.Items)
			}
		})
	}
}

func TestMenuItemPatchValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		patch   catalog.MenuItemPatch
		wantErr error
	}{
		"an empty patch changes nothing": {},
		"a price and a stock":            {patch: catalog.MenuItemPatch{Price: new(int64(59_000)), StockQty: new(3)}},
		"taken off the menu":             {patch: catalog.MenuItemPatch{IsAvailable: new(false)}},
		"a negative price":               {patch: catalog.MenuItemPatch{Price: new(int64(-1))}, wantErr: domain.ErrInvalidArgument},
		"a negative stock":               {patch: catalog.MenuItemPatch{StockQty: new(-1)}, wantErr: domain.ErrInvalidArgument},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.patch.Validate()

			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
