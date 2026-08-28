package kitchen

import (
	"avito-kitchen/internal/domain"
	"context"
	"net/http"

	"github.com/google/uuid"
)

// userHeader carries the customer the request acts on behalf of. Where it
// comes from is outside the scope of the service: an upstream gateway is
// trusted to have put it there.
// Will be changed in prod, for example to JWT.
const userHeader = "X-User-Id"

type userContextKey struct{}

// withUser puts the customer header of the request into the context.
// The operations of the catalogue serve anonymous callers as well, so a missing
// header stops nothing here: the operations that need a customer ask for one themselves.
func withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get(userHeader)
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, raw)))
	})
}

// requireUser returns the customer of the request.
func requireUser(ctx context.Context) (uuid.UUID, error) {
	raw, _ := ctx.Value(userContextKey{}).(string)
	if raw == "" {
		return uuid.Nil, domain.InvalidArgumentf("%s header is required", userHeader)
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.InvalidArgumentf("%s must be a UUID", userHeader)
	}

	return id, nil
}
