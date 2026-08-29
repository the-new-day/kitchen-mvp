package main

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/api/partnerapi"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The venues of the pilot the demo works with: the bakery runs a service of
// its own and drives its orders itself, the pizzeria has none and answers
// nothing, which is exactly what the refusals need.
const (
	bakerySlug   = "baton"
	pizzeriaSlug = "forno"
)

// The dishes the scenario is built on. The coffee of the bakery has no stock
// at all, so the happy path costs the demo nothing to repeat.
const (
	cappuccino = "SKU-CAPPUCCINO"
	americano  = "SKU-AMERICANO"
	margherita = "SKU-MARGHERITA"
	carbonara  = "SKU-CARBONARA"
)

// Where the order is taken to and who to call about it.
const (
	address = "Москва, Лесная ул., 7, кв. 12"
	phone   = "+79990000000"
)

// Statuses the demo waits for and reacts to.
const (
	statusCreated  = "CREATED"
	statusCooking  = "COOKING"
	statusRejected = "REJECTED"
)

// wanted is the whole life of an order after it has been placed.
var wanted = []string{"ACCEPTED", statusCooking, "READY", "DELIVERING", "DELIVERED"}

func ptr[T any](v T) *T { return &v }

// waitForPlatform waits for the API to answer at all.
func (d *demo) waitForPlatform(ctx context.Context) (string, error) {
	err := d.until(ctx, d.cfg.StartupWait, func(ctx context.Context) (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.cfg.HealthURL, nil)
		if err != nil {
			return false, err
		}

		res, err := d.http.Do(req)
		if err != nil {
			return false, nil //nolint:nilerr // the platform is still starting
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK, nil
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", d.cfg.HealthURL, err)
	}

	return d.cfg.BaseURL, nil
}

// waitForBakery waits for the venue that runs its own service to appear in the
// catalogue: it uploads its menu and opens its shift through the partner API,
// so its presence is the whole first act of the journey of a venue.
func (d *demo) waitForBakery(ctx context.Context) (string, error) {
	err := d.until(ctx, d.cfg.StartupWait, func(ctx context.Context) (bool, error) {
		venue, err := d.find(ctx, bakerySlug, true)
		if err != nil {
			return false, err
		}

		if venue == nil {
			return false, nil
		}

		d.venue = *venue

		return true, nil
	})
	if err != nil {
		return "", fmt.Errorf("пекарня не появилась в выдаче: %w", err)
	}

	if err := d.readMenu(ctx, d.venue.Id); err != nil {
		return "", err
	}

	for _, sku := range []string{cappuccino, americano} {
		if item, ok := d.menu[sku]; !ok || !item.IsAvailable {
			return "", fmt.Errorf("в меню пекарни нет доступной позиции %s", sku)
		}
	}

	return fmt.Sprintf("%s, позиций в меню: %d", d.venue.Name, len(d.menu)), nil
}

// find returns the venue with the given slug, or nothing when the catalogue
// does not show it.
func (d *demo) find(ctx context.Context, slug string, openOnly bool) (*kitchenapi.Venue, error) {
	params := &kitchenapi.ListVenuesParams{Limit: ptr(100)}
	if openOnly {
		params.OpenNow = ptr(true)
	}

	res, err := d.eater.api.ListVenuesWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("список заведений: %w", err)
	}

	if res.JSON200 == nil {
		return nil, fmt.Errorf("список заведений ответил %s", res.HTTPResponse.Status)
	}

	for _, venue := range res.JSON200.Items {
		if venue.Slug == slug {
			return &venue, nil
		}
	}

	return nil, nil
}

// readMenu reads the menu of a venue into the demo, by the identifiers of the
// system the venue keeps its nomenclature in.
func (d *demo) readMenu(ctx context.Context, venueID uuid.UUID) error {
	res, err := d.eater.api.GetVenueMenuWithResponse(ctx, venueID)
	if err != nil {
		return fmt.Errorf("меню заведения: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("меню заведения ответило %s", res.HTTPResponse.Status)
	}

	clear(d.menu)

	for _, category := range res.JSON200.Categories {
		for _, item := range category.Items {
			d.menu[item.ExternalId] = item
		}
	}

	return nil
}

// fillCart puts the order of the demo together: two cappuccinos and an
// americano, which is above the minimum order of the bakery.
func (d *demo) fillCart(ctx context.Context) (string, error) {
	cart, err := d.put(ctx, d.eater, d.venue.Id, cappuccino, 2)
	if err != nil {
		return "", err
	}

	if cart, err = d.put(ctx, d.eater, d.venue.Id, americano, 1); err != nil {
		return "", err
	}

	return fmt.Sprintf("позиций: %d, товары %s, доставка %s",
		len(cart.Items), money(cart.ItemsTotal), money(cart.DeliveryFee)), nil
}

// put sets the quantity of one item of a venue in the cart of a customer.
func (d *demo) put(
	ctx context.Context, who *customer, venueID uuid.UUID, sku string, qty int,
) (kitchenapi.Cart, error) {
	item, ok := d.menu[sku]
	if !ok {
		return kitchenapi.Cart{}, fmt.Errorf("позиции %s нет в меню", sku)
	}

	res, err := who.api.PutCartItemWithResponse(ctx, kitchenapi.PutCartItemRequest{
		VenueId: venueID,
		ItemId:  item.Id,
		Qty:     qty,
	})
	if err != nil {
		return kitchenapi.Cart{}, fmt.Errorf("положить %s в корзину: %w", sku, err)
	}

	if res.JSON200 == nil {
		return kitchenapi.Cart{}, fmt.Errorf("корзина ответила %s на %s",
			res.HTTPResponse.Status, sku)
	}

	return *res.JSON200, nil
}

// checkCart makes the platform recount the cart against the current menu and
// remembers the sum the customer is about to confirm.
func (d *demo) checkCart(ctx context.Context) (string, error) {
	validation, err := d.validate(ctx, d.eater)
	if err != nil {
		return "", err
	}

	if !validation.IsValid {
		return "", fmt.Errorf("корзина невалидна: %s", problems(validation.Problems))
	}

	d.total = validation.Cart.Total

	return fmt.Sprintf("к оплате %s", money(d.total)), nil
}

// validate asks the platform what is wrong with the cart of a customer.
func (d *demo) validate(ctx context.Context, who *customer) (kitchenapi.CartValidation, error) {
	res, err := who.api.ValidateCartWithResponse(ctx)
	if err != nil {
		return kitchenapi.CartValidation{}, fmt.Errorf("проверка корзины: %w", err)
	}

	if res.JSON200 == nil {
		return kitchenapi.CartValidation{}, fmt.Errorf("проверка корзины ответила %s",
			res.HTTPResponse.Status)
	}

	return *res.JSON200, nil
}

// refuseOnPriceChange changes the price under the customer and checks that the
// order is refused rather than placed at a sum they never saw.
func (d *demo) refuseOnPriceChange(ctx context.Context) (string, error) {
	was := d.menu[cappuccino].Price
	now := was + 5_000

	if err := d.patch(ctx, d.bakery, cappuccino, partnerapi.MenuItemPatch{Price: ptr(now)}); err != nil {
		return "", err
	}

	defer func() {
		_ = d.patch(ctx, d.bakery, cappuccino, partnerapi.MenuItemPatch{Price: ptr(was)})
	}()

	res, err := d.create(ctx, d.eater, uuid.New(), d.total, "")
	if err != nil {
		return "", err
	}

	if res.JSON201 != nil || res.JSON200 != nil {
		return "", fmt.Errorf("заказ оформлен по старой сумме %s", money(d.total))
	}

	if err := expect(refusal(res.StatusCode(), res.JSON409),
		http.StatusConflict, "price_mismatch"); err != nil {
		return "", err
	}

	return fmt.Sprintf("цена %s → %s, заказ по старой сумме отклонён", money(was), money(now)), nil
}

// patch changes one item of the menu of a venue on behalf of that venue.
func (d *demo) patch(
	ctx context.Context,
	venue *partnerapi.ClientWithResponses,
	sku string,
	change partnerapi.MenuItemPatch,
) error {
	res, err := venue.PatchMenuItemWithResponse(ctx, sku, change)
	if err != nil {
		return fmt.Errorf("изменить позицию %s: %w", sku, err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("изменение позиции %s ответило %s", sku, res.HTTPResponse.Status)
	}

	return nil
}

// create places an order out of the cart of a customer under the given key.
func (d *demo) create(
	ctx context.Context, who *customer, key uuid.UUID, total int64, comment string,
) (*kitchenapi.CreateOrderResponse, error) {
	request := kitchenapi.CreateOrderRequest{
		Address:       address,
		Phone:         phone,
		ExpectedTotal: total,
	}

	if comment != "" {
		request.Comment = ptr(comment)
	}

	res, err := who.api.CreateOrderWithResponse(ctx,
		&kitchenapi.CreateOrderParams{IdempotencyKey: key}, request)
	if err != nil {
		return nil, fmt.Errorf("оформление заказа: %w", err)
	}

	return res, nil
}

// placeOrder turns the cart into an order and keeps the key it was placed
// with: the two checks after this one repeat it.
func (d *demo) placeOrder(ctx context.Context) (string, error) {
	validation, err := d.validate(ctx, d.eater)
	if err != nil {
		return "", err
	}

	d.total = validation.Cart.Total
	d.key = uuid.New()

	res, err := d.create(ctx, d.eater, d.key, d.total, "")
	if err != nil {
		return "", err
	}

	if res.JSON201 == nil {
		return "", fmt.Errorf("оформление ответило %s: %s",
			res.HTTPResponse.Status, strings.TrimSpace(string(res.Body)))
	}

	d.order = *res.JSON201

	return fmt.Sprintf("%s, %s, статус %s",
		d.order.Number, money(d.order.Total), d.order.Status), nil
}

// repeatKey repeats the very same request and checks that it gives back the
// order that was already placed instead of placing a second one.
func (d *demo) repeatKey(ctx context.Context) (string, error) {
	res, err := d.create(ctx, d.eater, d.key, d.total, "")
	if err != nil {
		return "", err
	}

	if res.JSON200 == nil {
		return "", fmt.Errorf("повтор ответил %s, ожидался 200", res.HTTPResponse.Status)
	}

	if res.JSON200.Id != d.order.Id {
		return "", fmt.Errorf("повтор отдал заказ %s, а не %s", res.JSON200.Id, d.order.Id)
	}

	return fmt.Sprintf("тот же заказ %s", res.JSON200.Number), nil
}

// reuseKey repeats the key with another request and checks that the platform
// refuses to answer for a cart it never saw.
func (d *demo) reuseKey(ctx context.Context) (string, error) {
	res, err := d.create(ctx, d.eater, d.key, d.total, "без сахара")
	if err != nil {
		return "", err
	}

	return "", expect(refusal(res.StatusCode(), res.JSON422),
		http.StatusUnprocessableEntity, "idempotency_key_reuse")
}

// openStream subscribes to the events of the order. Everything that happened
// before the subscription is replayed, so nothing between placing the order and
// opening the stream is lost.
func (d *demo) openStream(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/orders/%s/events", strings.TrimRight(d.cfg.BaseURL, "/"), d.order.Id)

	events, err := follow(ctx, d.streams, url, d.eater.id.String())
	if err != nil {
		return "", err
	}

	d.events = events

	return url, nil
}

// watchStatuses follows the order all the way home. The venue drives it as far
// as the door of the customer and the platform closes it: the whole sequence
// has to arrive, and in this order.
func (d *demo) watchStatuses(ctx context.Context) (string, error) {
	deadline := time.NewTimer(d.cfg.StatusWait)
	defer deadline.Stop()

	notes := make([]string, 0, len(wanted))
	seen := 0

	for seen < len(wanted) {
		select {
		case update, ok := <-d.events.updates:
			if !ok {
				return "", fmt.Errorf("поток закрылся на %d статусе из %d", seen, len(wanted))
			}

			// The status the order was placed in is replayed as well: the
			// stream is followed from the very first entry of the history.
			if update.Status == statusCreated {
				continue
			}

			if update.Status != wanted[seen] {
				return "", fmt.Errorf("пришёл статус %s, ожидался %s", update.Status, wanted[seen])
			}

			notes = append(notes, fmt.Sprintf("%s (%s)", update.Status, update.Actor))
			seen++

			if update.Status != statusCooking {
				continue
			}

			note, err := d.refuseCancel(ctx)
			if err != nil {
				return "", err
			}

			notes = append(notes, note)
		case err := <-d.events.failed:
			return "", fmt.Errorf("поток событий: %w", err)
		case <-deadline.C:
			return "", fmt.Errorf("за %s заказ дошёл только до %s", d.cfg.StatusWait, wanted[seen])
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	return strings.Join(notes, " → "), nil
}

// refuseCancel checks that an order already being cooked can no longer be
// withdrawn by the customer.
func (d *demo) refuseCancel(ctx context.Context) (string, error) {
	res, err := d.eater.api.CancelOrderWithResponse(ctx, d.order.Id,
		kitchenapi.CancelOrderRequest{Reason: ptr("передумал")})
	if err != nil {
		return "", fmt.Errorf("отмена заказа: %w", err)
	}

	if err := expect(refusal(res.StatusCode(), res.JSON409),
		http.StatusConflict, "invalid_transition"); err != nil {
		return "", fmt.Errorf("отмена из COOKING: %w", err)
	}

	return "отмена отклонена (409)", nil
}

// money spells a sum of kopecks the way a receipt would.
func money(amount int64) string {
	return fmt.Sprintf("%d,%02d ₽", amount/100, amount%100)
}

// problems spells out what a check of the cart has found.
func problems(found []kitchenapi.CartProblem) string {
	types := make([]string, 0, len(found))
	for _, problem := range found {
		types = append(types, string(problem.Type))
	}

	return strings.Join(types, ", ")
}
