package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
)

// venueService is the container of the demo venue, as compose names it.
const venueService = "venue-service"

// outageWait is how long the demo gives a venue that has just started to catch
// up with what it missed.
const outageWait = 120 * time.Second

// surviveOutage stops the venue, places an order while it is down and starts it
// again: the order waits in the topic and reaches the kitchen on its own, which
// is the whole point of sending it as an event rather than calling the venue.
func (d *demo) surviveOutage(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "пропущен: docker недоступен", nil
	}

	if err := compose(ctx, "stop", venueService); err != nil {
		return "", err
	}

	defer func() {
		_ = compose(ctx, "start", venueService)
	}()

	venue, err := d.find(ctx, bakerySlug, false)
	if err != nil {
		return "", err
	}

	if venue == nil {
		return "", fmt.Errorf("заведения %s нет в каталоге", bakerySlug)
	}

	if err := d.readMenu(ctx, venue.Id); err != nil {
		return "", err
	}

	eater, err := newCustomer(d.cfg.BaseURL, d.http)
	if err != nil {
		return "", err
	}

	if _, err := d.put(ctx, eater, venue.Id, cappuccino, 2); err != nil {
		return "", err
	}

	if _, err := d.put(ctx, eater, venue.Id, americano, 1); err != nil {
		return "", err
	}

	validation, err := d.validate(ctx, eater)
	if err != nil {
		return "", err
	}

	res, err := d.create(ctx, eater, uuid.New(), validation.Cart.Total, "")
	if err != nil {
		return "", err
	}

	if res.JSON201 == nil {
		return "", fmt.Errorf("заказ при остановленном заведении ответил %s",
			res.HTTPResponse.Status)
	}

	placed := *res.JSON201

	if err := compose(ctx, "start", venueService); err != nil {
		return "", err
	}

	var caught string

	err = d.until(ctx, outageWait, func(ctx context.Context) (bool, error) {
		current, err := d.card(ctx, eater, placed.Id)
		if err != nil {
			return false, err
		}

		caught = string(current.Status)

		return caught != statusCreated, nil
	})
	if err != nil {
		return "", fmt.Errorf("заказ %s не доехал до заведения: %w", placed.Number, err)
	}

	return fmt.Sprintf("%s принят заведением после простоя, статус %s",
		placed.Number, caught), nil
}

// compose runs one compose command of the stand the demo works against.
func compose(ctx context.Context, args ...string) error {
	//nolint:gosec // the arguments are the compose commands of this repository
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return nil
}
