// Package partner is how the venue reaches the platform: the generated client
// of the partner specification, an API key on every request and the answers of
// the platform turned into errors the kitchen understands.
package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/venue/internal/config"
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// keyHeader is the header the key the venue was given at onboarding travels in.
const keyHeader = "X-Api-Key" //nolint:gosec // a header name, not a credential

// Client talks to the partner API on behalf of one venue.
type Client struct {
	api *partnerapi.ClientWithResponses
}

// Venue is the profile the platform keeps of the venue holding the key.
type Venue struct {
	ID             uuid.UUID
	Slug           string
	Name           string
	IsOpen         bool
	AvgCookMinutes int
	OrdersTopic    string
}

// Sync counts what an upload of the menu did on the platform.
type Sync struct {
	Categories  int
	Created     int
	Updated     int
	Deactivated int
}

// New returns a client of the partner API configured by cfg.
func New(cfg config.Partner) (*Client, error) {
	key := cfg.APIKey

	api, err := partnerapi.NewClientWithResponses(cfg.BaseURL,
		partnerapi.WithHTTPClient(&http.Client{Timeout: cfg.Timeout}),
		partnerapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set(keyHeader, key)

			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create partner client: %w", err)
	}

	return &Client{api: api}, nil
}

// Me returns the profile of the venue the key belongs to.
func (c *Client) Me(ctx context.Context) (Venue, error) {
	res, err := c.api.GetMeWithResponse(ctx)
	if err != nil {
		return Venue{}, fmt.Errorf("ask the platform who we are: %w", err)
	}

	if err := check("ask the platform who we are", res.StatusCode(), res.Body); err != nil {
		return Venue{}, err
	}

	profile := res.JSON200
	venue := Venue{
		ID:             profile.VenueId,
		Slug:           profile.Slug,
		Name:           profile.Name,
		IsOpen:         profile.IsOpen,
		AvgCookMinutes: profile.AvgCookMinutes,
	}

	if profile.OrdersTopic != nil {
		venue.OrdersTopic = *profile.OrdersTopic
	}

	return venue, nil
}

// SyncMenu hands the platform the whole menu of the venue.
func (c *Client) SyncMenu(ctx context.Context, snapshot partnerapi.MenuSyncRequest) (Sync, error) {
	res, err := c.api.SyncMenuWithResponse(ctx, snapshot)
	if err != nil {
		return Sync{}, fmt.Errorf("upload the menu: %w", err)
	}

	if err := check("upload the menu", res.StatusCode(), res.Body); err != nil {
		return Sync{}, err
	}

	return Sync{
		Categories:  res.JSON200.CategoriesTotal,
		Created:     res.JSON200.ItemsCreated,
		Updated:     res.JSON200.ItemsUpdated,
		Deactivated: res.JSON200.ItemsDeactivated,
	}, nil
}

// SetItemAvailability puts one dish on or off sale on the platform.
func (c *Client) SetItemAvailability(ctx context.Context, sku string, available bool) error {
	res, err := c.api.PatchMenuItemWithResponse(ctx, sku, partnerapi.MenuItemPatch{
		IsAvailable: &available,
	})
	if err != nil {
		return fmt.Errorf("change availability of %s: %w", sku, err)
	}

	return check("change availability of "+sku, res.StatusCode(), res.Body)
}

// OpenShift makes the venue visible to the customers.
func (c *Client) OpenShift(ctx context.Context) error {
	res, err := c.api.OpenShiftWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("open the shift: %w", err)
	}

	return check("open the shift", res.StatusCode(), res.Body)
}

// Accept takes an order into work, promising it in eta minutes.
func (c *Client) Accept(ctx context.Context, orderID uuid.UUID, eta int) error {
	res, err := c.api.AcceptOrderWithResponse(ctx, orderID, partnerapi.AcceptOrderRequest{
		EtaMinutes: eta,
	})
	if err != nil {
		return fmt.Errorf("accept order %s: %w", orderID, err)
	}

	return check("accept order "+orderID.String(), res.StatusCode(), res.Body)
}

// StartCooking tells the platform the order is on the stove.
func (c *Client) StartCooking(ctx context.Context, orderID uuid.UUID) error {
	res, err := c.api.StartCookingWithResponse(ctx, orderID)
	if err != nil {
		return fmt.Errorf("start cooking order %s: %w", orderID, err)
	}

	return check("start cooking order "+orderID.String(), res.StatusCode(), res.Body)
}

// MarkReady tells the platform the order is ready.
func (c *Client) MarkReady(ctx context.Context, orderID uuid.UUID) error {
	res, err := c.api.MarkReadyWithResponse(ctx, orderID)
	if err != nil {
		return fmt.Errorf("mark order %s ready: %w", orderID, err)
	}

	return check("mark order "+orderID.String()+" ready", res.StatusCode(), res.Body)
}

// Handover gives the order to the courier.
func (c *Client) Handover(ctx context.Context, orderID uuid.UUID) error {
	res, err := c.api.HandoverOrderWithResponse(ctx, orderID)
	if err != nil {
		return fmt.Errorf("hand order %s over: %w", orderID, err)
	}

	return check("hand order "+orderID.String()+" over", res.StatusCode(), res.Body)
}

// OrderStatus returns the status the platform keeps for an order.
func (c *Client) OrderStatus(ctx context.Context, orderID uuid.UUID) (string, error) {
	res, err := c.api.GetPartnerOrderWithResponse(ctx, orderID)
	if err != nil {
		return "", fmt.Errorf("look up order %s: %w", orderID, err)
	}

	if err := check("look up order "+orderID.String(), res.StatusCode(), res.Body); err != nil {
		return "", err
	}

	return string(res.JSON200.Status), nil
}
