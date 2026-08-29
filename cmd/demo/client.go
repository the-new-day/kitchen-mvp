package main

import (
	"avito-kitchen/internal/api/kitchenapi"
	"avito-kitchen/internal/api/partnerapi"
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// Headers the demo introduces itself with: a customer of the platform and a
// venue of the partner API.
const (
	userHeader = "X-User-Id"
	keyHeader  = "X-Api-Key" //nolint:gosec // a header name, not a credential
)

// customer is the platform as one customer sees it.
type customer struct {
	id  uuid.UUID
	api *kitchenapi.ClientWithResponses
}

// newCustomer returns a client that acts on behalf of a customer of its own.
func newCustomer(baseURL string, client *http.Client) (*customer, error) {
	id := uuid.New()

	api, err := kitchenapi.NewClientWithResponses(baseURL,
		kitchenapi.WithHTTPClient(client),
		kitchenapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set(userHeader, id.String())

			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create customer client: %w", err)
	}

	return &customer{id: id, api: api}, nil
}

// newVenue returns a client of the partner API holding the key of one venue.
func newVenue(baseURL, apiKey string, client *http.Client) (*partnerapi.ClientWithResponses, error) {
	api, err := partnerapi.NewClientWithResponses(baseURL,
		partnerapi.WithHTTPClient(client),
		partnerapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set(keyHeader, apiKey)

			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create partner client: %w", err)
	}

	return api, nil
}

// answer is what an operation of the platform answered: the demo checks the
// code and the machine-readable reason, not the wording.
type answer struct {
	status int
	code   string
}

func (a answer) String() string {
	if a.code == "" {
		return fmt.Sprintf("%d", a.status)
	}

	return fmt.Sprintf("%d %s", a.status, a.code)
}

// refusal reads the error envelope of a refused request.
func refusal(status int, body *kitchenapi.Error) answer {
	if body == nil {
		return answer{status: status}
	}

	return answer{status: status, code: body.Code}
}

// expect reports whether the platform refused a request the way it had to.
func expect(got answer, status int, code string) error {
	if got.status != status || got.code != code {
		return fmt.Errorf("answered %s, want %d %s", got, status, code)
	}

	return nil
}
