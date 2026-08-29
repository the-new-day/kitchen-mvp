// Package bootstrap is the first thing the venue does: it asks the platform
// who it is, hands over its menu and opens the shift. Until that succeeds the
// venue sells nothing, so it keeps trying -- the key it was given reaches the
// platform with a step of its own, and the first attempts are expected to be
// turned down.
package bootstrap

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/venue/internal/kitchen"
	"avito-kitchen/venue/internal/menu"
	"avito-kitchen/venue/internal/partner"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Nomenclature is where the venue keeps the dishes it sells.
type Nomenclature interface {
	Ensure(ctx context.Context, dishes []kitchen.Dish) error
	List(ctx context.Context) ([]kitchen.Dish, error)
}

// Platform is the part of the partner API the start of a shift needs.
type Platform interface {
	Me(ctx context.Context) (partner.Venue, error)
	SyncMenu(ctx context.Context, snapshot partnerapi.MenuSyncRequest) (partner.Sync, error)
	OpenShift(ctx context.Context) error
}

// Open brings the venue online and returns the profile the platform keeps of
// it. It gives up only when ctx is cancelled.
func Open(
	ctx context.Context,
	dishes Nomenclature,
	platform Platform,
	retry time.Duration,
	log *slog.Logger,
) (partner.Venue, error) {
	for {
		venue, err := open(ctx, dishes, platform, log)
		if err == nil {
			return venue, nil
		}

		if ctx.Err() != nil {
			return partner.Venue{}, ctx.Err()
		}

		log.WarnContext(ctx, "venue is not online yet, retrying",
			slog.String("error", err.Error()), slog.Duration("in", retry))

		select {
		case <-ctx.Done():
			return partner.Venue{}, ctx.Err()
		case <-time.After(retry):
		}
	}
}

func open(
	ctx context.Context, dishes Nomenclature, platform Platform, log *slog.Logger,
) (partner.Venue, error) {
	venue, err := platform.Me(ctx)
	if err != nil {
		return partner.Venue{}, err
	}

	if err = dishes.Ensure(ctx, menu.Dishes); err != nil {
		return partner.Venue{}, fmt.Errorf("write down the nomenclature: %w", err)
	}

	stored, err := dishes.List(ctx)
	if err != nil {
		return partner.Venue{}, fmt.Errorf("read the nomenclature: %w", err)
	}

	sync, err := platform.SyncMenu(ctx, menu.Snapshot(stored))
	if err != nil {
		return partner.Venue{}, err
	}

	if err := platform.OpenShift(ctx); err != nil {
		return partner.Venue{}, err
	}

	log.InfoContext(ctx, "shift is open",
		slog.String("venue_id", venue.ID.String()),
		slog.String("venue", venue.Name),
		slog.Int("categories", sync.Categories),
		slog.Int("items_created", sync.Created),
		slog.Int("items_updated", sync.Updated),
		slog.Int("items_deactivated", sync.Deactivated),
	)

	return venue, nil
}
