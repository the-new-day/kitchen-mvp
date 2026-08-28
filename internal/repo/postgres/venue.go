package postgres

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/domain/catalog"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// VenueRepo reads the venue catalogue.
type VenueRepo struct {
	db *DB
}

// NewVenueRepo returns a repository over db.
func NewVenueRepo(db *DB) *VenueRepo {
	return &VenueRepo{db: db}
}

const venueColumns = `
	v.id, v.slug, v.name, v.description, v.address, v.lat, v.lon,
	v.is_open, v.min_order_amount, v.delivery_fee, v.avg_cook_minutes`

// venueOrder is the SQL behind a VenueSort: the expression to order by and
// the type its cursor value is compared as. Only these three expressions ever
// reach the query, so the sort never carries user input into SQL.
var venueOrder = map[catalog.VenueSort]struct {
	expr string
	typ  string
}{
	catalog.SortByName:           {expr: "v.name", typ: "text"},
	catalog.SortByDeliveryFee:    {expr: "v.delivery_fee", typ: "bigint"},
	catalog.SortByAvgCookMinutes: {expr: "v.avg_cook_minutes", typ: "integer"},
}

// ListCuisines returns the cuisine reference book in display order.
func (r *VenueRepo) ListCuisines(ctx context.Context) ([]catalog.Cuisine, error) {
	const query = `SELECT slug, name FROM cuisines ORDER BY position, slug`

	rows, err := r.db.conn(ctx).Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query cuisines: %w", err)
	}

	cuisines, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (catalog.Cuisine, error) {
		var c catalog.Cuisine
		return c, row.Scan(&c.Slug, &c.Name)
	})
	if err != nil {
		return nil, fmt.Errorf("scan cuisines: %w", err)
	}

	return cuisines, nil
}

// ListVenues returns at most f.Limit venues matching the filter, ordered by
// f.Sort and starting after f.After. Cuisines of the page are read by a
// second query, not one per venue.
func (r *VenueRepo) ListVenues(ctx context.Context, f catalog.VenueFilter) ([]catalog.Venue, error) {
	conds := []string{"v.is_active"}
	args := make([]any, 0, 5)

	if f.Q != "" {
		args = append(args, containsPattern(f.Q))
		conds = append(conds, fmt.Sprintf(`v.name ILIKE $%d ESCAPE '\'`, len(args)))
	}

	if f.Cuisine != "" {
		args = append(args, f.Cuisine)
		conds = append(conds, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM venue_cuisines vc
			JOIN cuisines c ON c.id = vc.cuisine_id
			WHERE vc.venue_id = v.id AND c.slug = $%d)`, len(args)))
	}

	if f.OpenNow {
		conds = append(conds, "v.is_open")
	}

	order, ok := venueOrder[f.Sort]
	if !ok {
		order = venueOrder[catalog.SortByName]
	}

	if f.After != nil {
		args = append(args, f.After.Key, f.After.ID)
		conds = append(conds, fmt.Sprintf("(%s, v.id) > ($%d::%s, $%d::uuid)",
			order.expr, len(args)-1, order.typ, len(args)))
	}

	args = append(args, f.Limit)
	query := fmt.Sprintf(`SELECT %s FROM venues v WHERE %s ORDER BY %s, v.id LIMIT $%d`,
		venueColumns, strings.Join(conds, " AND "), order.expr, len(args))

	rows, err := r.db.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query venues: %w", err)
	}

	venues, err := pgx.CollectRows(rows, scanVenue)
	if err != nil {
		return nil, fmt.Errorf("scan venues: %w", err)
	}

	if err := r.attachCuisines(ctx, venues); err != nil {
		return nil, err
	}

	return venues, nil
}

// GetVenue returns one venue card. A venue that does not exist or is not
// active is reported as domain.ErrNotFound.
func (r *VenueRepo) GetVenue(ctx context.Context, id uuid.UUID) (catalog.Venue, error) {
	query := fmt.Sprintf(`SELECT %s FROM venues v WHERE v.id = $1 AND v.is_active`, venueColumns)

	rows, err := r.db.conn(ctx).Query(ctx, query, id)
	if err != nil {
		return catalog.Venue{}, fmt.Errorf("query venue: %w", err)
	}

	venue, err := pgx.CollectExactlyOneRow(rows, scanVenue)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.Venue{}, domain.ErrNotFound
		}

		return catalog.Venue{}, fmt.Errorf("scan venue: %w", err)
	}

	page := []catalog.Venue{venue}
	if err := r.attachCuisines(ctx, page); err != nil {
		return catalog.Venue{}, err
	}

	return page[0], nil
}

// attachCuisines fills the cuisines of every venue of the page with one query.
func (r *VenueRepo) attachCuisines(ctx context.Context, venues []catalog.Venue) error {
	if len(venues) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(venues))
	for _, v := range venues {
		ids = append(ids, v.ID)
	}

	const query = `
		SELECT vc.venue_id, c.slug, c.name
		FROM venue_cuisines vc
		JOIN cuisines c ON c.id = vc.cuisine_id
		WHERE vc.venue_id = ANY($1)
		ORDER BY c.position, c.slug`

	rows, err := r.db.conn(ctx).Query(ctx, query, ids)
	if err != nil {
		return fmt.Errorf("query venue cuisines: %w", err)
	}

	type venueCuisine struct {
		venueID uuid.UUID
		cuisine catalog.Cuisine
	}

	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (venueCuisine, error) {
		var p venueCuisine
		return p, row.Scan(&p.venueID, &p.cuisine.Slug, &p.cuisine.Name)
	})
	if err != nil {
		return fmt.Errorf("scan venue cuisines: %w", err)
	}

	byVenue := make(map[uuid.UUID][]catalog.Cuisine, len(venues))
	for _, p := range pairs {
		byVenue[p.venueID] = append(byVenue[p.venueID], p.cuisine)
	}

	for i := range venues {
		venues[i].Cuisines = byVenue[venues[i].ID]
	}

	return nil
}

func scanVenue(row pgx.CollectableRow) (catalog.Venue, error) {
	var v catalog.Venue
	err := row.Scan(
		&v.ID, &v.Slug, &v.Name, &v.Description, &v.Address, &v.Lat, &v.Lon,
		&v.IsOpen, &v.MinOrderAmount, &v.DeliveryFee, &v.AvgCookMinutes,
	)

	return v, err
}

// containsPattern turns a search term into an ILIKE pattern, escaping the
// wildcards the user may have typed.
func containsPattern(q string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)

	return "%" + escaped + "%"
}
