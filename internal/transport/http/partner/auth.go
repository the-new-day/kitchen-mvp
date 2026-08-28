package partner

import (
	"avito-kitchen/internal/transport/http/apierr"
	"context"
	"net/http"

	"github.com/google/uuid"
)

// authHeader carries the key issued to a venue at onboarding.
const authHeader = "X-Api-Key"

// authenticator resolves an API key into the venue it was issued to.
type authenticator interface {
	Authenticate(ctx context.Context, key string) (uuid.UUID, error)
}

type venueContextKey struct{}

// authenticate rejects a request whose X-Api-Key is missing, unknown or
// revoked, and puts the venue of an accepted key into the context. The key
// itself never leaves this function.
func authenticate(auth authenticator, errs apierr.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := auth.Authenticate(r.Context(), r.Header.Get(authHeader))
			if err != nil {
				errs.Response(w, r, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), venueContextKey{}, id)))
		})
	}
}

// venueID returns the venue the request was authenticated as.
func venueID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(venueContextKey{}).(uuid.UUID)
	return id
}
