package main

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/api/partnerapi"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// reaperMargin is how long the demo gives the platform to notice an order
// nobody accepted, on top of the timeout the order itself waits out.
const reaperMargin = 90 * time.Second

// refuseOnEmptyStock takes the last portion of a dish away while it is already
// in the cart and checks that the order is refused instead of promising food
// the venue does not have. The pizzeria is used for it: nobody drives its
// orders, so nothing races with the check.
func (d *demo) refuseOnEmptyStock(ctx context.Context) (string, error) {
	eater, cart, err := d.pizzeriaCart(ctx)
	if err != nil {
		return "", err
	}

	stock := d.menu[carbonara].StockQty
	if stock == nil {
		return "", fmt.Errorf("у позиции %s нет остатка, отказ не воспроизвести", carbonara)
	}

	if err := d.patch(ctx, d.pizzeria, carbonara,
		partnerapi.MenuItemPatch{StockQty: ptr(0)}); err != nil {
		return "", err
	}

	defer func() {
		_ = d.patch(ctx, d.pizzeria, carbonara, partnerapi.MenuItemPatch{StockQty: stock})
	}()

	res, err := d.create(ctx, eater, uuid.New(), cart.Total, "")
	if err != nil {
		return "", err
	}

	if res.JSON201 != nil {
		return "", fmt.Errorf("заказ %s оформлен на кончившуюся позицию", res.JSON201.Number)
	}

	if err := expect(refusal(res.StatusCode(), res.JSON409),
		http.StatusConflict, "out_of_stock"); err != nil {
		return "", err
	}

	return fmt.Sprintf("остаток %s: %d → 0, заказ отклонён", carbonara, *stock), nil
}

// rejectOnTimeout places an order at a venue that answers nothing and checks
// that the platform stops waiting for it on its own: the order is rejected by
// the system and everything it held comes back to the stock.
func (d *demo) rejectOnTimeout(ctx context.Context) (string, error) {
	eater, cart, err := d.pizzeriaCart(ctx)
	if err != nil {
		return "", err
	}

	before := d.stock()

	res, err := d.create(ctx, eater, uuid.New(), cart.Total, "")
	if err != nil {
		return "", err
	}

	if res.JSON201 == nil {
		return "", fmt.Errorf("оформление ответило %s", res.HTTPResponse.Status)
	}

	placed := *res.JSON201

	var rejected kitchenapi.Order

	wait := d.cfg.AcceptTimeout + reaperMargin

	err = d.until(ctx, wait, func(ctx context.Context) (bool, error) {
		current, err := d.card(ctx, eater, placed.Id)
		if err != nil {
			return false, err
		}

		rejected = current

		return string(current.Status) == statusRejected, nil
	})
	if err != nil {
		return "", fmt.Errorf("заказ %s остался в статусе %s: %w",
			placed.Number, rejected.Status, err)
	}

	if rejected.RejectionReason == nil || *rejected.RejectionReason == "" {
		return "", fmt.Errorf("заказ %s отклонён без причины", placed.Number)
	}

	if err := d.readMenu(ctx, placed.Venue.Id); err != nil {
		return "", err
	}

	if after := d.stock(); after != before {
		return "", fmt.Errorf("остатки после отказа: %v, до заказа: %v", after, before)
	}

	return fmt.Sprintf("%s → %s: %s, остатки вернулись",
		placed.Number, rejected.Status, *rejected.RejectionReason), nil
}

// pizzeriaCart gives a customer of their own a cart at the venue that has no
// service behind it, and returns the cart the platform has counted.
func (d *demo) pizzeriaCart(ctx context.Context) (*customer, kitchenapi.Cart, error) {
	venue, err := d.find(ctx, pizzeriaSlug, false)
	if err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	if venue == nil {
		return nil, kitchenapi.Cart{}, fmt.Errorf("заведения %s нет в каталоге", pizzeriaSlug)
	}

	if err := d.readMenu(ctx, venue.Id); err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	eater, err := newCustomer(d.cfg.BaseURL, d.http)
	if err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	if _, err := d.put(ctx, eater, venue.Id, margherita, 1); err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	if _, err := d.put(ctx, eater, venue.Id, carbonara, 1); err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	validation, err := d.validate(ctx, eater)
	if err != nil {
		return nil, kitchenapi.Cart{}, err
	}

	if !validation.IsValid {
		return nil, kitchenapi.Cart{}, fmt.Errorf("корзина в пиццерии невалидна: %s",
			problems(validation.Problems))
	}

	return eater, validation.Cart, nil
}

// card reads an order the way its customer sees it.
func (d *demo) card(
	ctx context.Context, who *customer, orderID uuid.UUID,
) (kitchenapi.Order, error) {
	res, err := who.api.GetOrderWithResponse(ctx, orderID)
	if err != nil {
		return kitchenapi.Order{}, fmt.Errorf("карточка заказа: %w", err)
	}

	if res.JSON200 == nil {
		return kitchenapi.Order{}, fmt.Errorf("карточка заказа ответила %s",
			res.HTTPResponse.Status)
	}

	return *res.JSON200, nil
}

// stock is what the menu the demo has just read holds of the dishes it orders.
func (d *demo) stock() [2]int {
	counted := [2]int{}

	for i, sku := range [2]string{margherita, carbonara} {
		if qty := d.menu[sku].StockQty; qty != nil {
			counted[i] = *qty
		}
	}

	return counted
}
