package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/internal/domain/catalog"
	"context"
)

// SyncMenu serves POST /partner/menu.
func (s *Server) SyncMenu(
	ctx context.Context, request partnerapi.SyncMenuRequestObject,
) (partnerapi.SyncMenuResponseObject, error) {
	result, err := s.partner.SyncMenu(ctx, venueID(ctx), toMenuUpload(*request.Body))
	if err != nil {
		return nil, err
	}

	return partnerapi.SyncMenu200JSONResponse{
		CategoriesTotal:  result.CategoriesTotal,
		ItemsCreated:     result.ItemsCreated,
		ItemsUpdated:     result.ItemsUpdated,
		ItemsDeactivated: result.ItemsDeactivated,
	}, nil
}

// PatchMenuItem serves PATCH /partner/menu/items/{external_id}.
func (s *Server) PatchMenuItem(
	ctx context.Context, request partnerapi.PatchMenuItemRequestObject,
) (partnerapi.PatchMenuItemResponseObject, error) {
	patch := catalog.MenuItemPatch{
		Price:       request.Body.Price,
		IsAvailable: request.Body.IsAvailable,
		StockQty:    request.Body.StockQty,
	}

	item, err := s.partner.PatchItem(ctx, venueID(ctx), request.ExternalId, patch)
	if err != nil {
		return nil, err
	}

	return partnerapi.PatchMenuItem200JSONResponse(toPartnerMenuItem(item)), nil
}

// toMenuUpload maps an upload to the domain. An item that says nothing about
// its availability is on sale: a venue uploads what it sells.
func toMenuUpload(request partnerapi.MenuSyncRequest) catalog.MenuUpload {
	upload := catalog.MenuUpload{
		Categories: make([]catalog.UploadCategory, 0, len(request.Categories)),
	}

	for _, c := range request.Categories {
		category := catalog.UploadCategory{
			ExternalID: c.ExternalId,
			Name:       c.Name,
			Items:      make([]catalog.UploadItem, 0, len(c.Items)),
		}

		for _, i := range c.Items {
			item := catalog.UploadItem{
				ExternalID:  i.ExternalId,
				Name:        i.Name,
				Price:       i.Price,
				IsAvailable: i.IsAvailable == nil || *i.IsAvailable,
				StockQty:    i.StockQty,
			}

			if i.Description != nil {
				item.Description = *i.Description
			}

			category.Items = append(category.Items, item)
		}

		upload.Categories = append(upload.Categories, category)
	}

	return upload
}

func toPartnerMenuItem(item catalog.CategorizedMenuItem) partnerapi.PartnerMenuItem {
	out := partnerapi.PartnerMenuItem{
		ExternalId:  item.ExternalID,
		Name:        item.Name,
		Price:       item.Price,
		IsAvailable: item.IsAvailable,
		StockQty:    item.StockQty,
	}

	if item.CategoryExternalID != "" {
		out.CategoryExternalId = &item.CategoryExternalID
	}
	if item.Description != "" {
		out.Description = &item.Description
	}

	return out
}
