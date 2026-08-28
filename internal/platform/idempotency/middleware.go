package idempotency

import (
	"avito-kitchen/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// errIncomplete unwinds the transaction of an attempt that did not succeed, so
// that the key is left free for the client to retry with.
// The answer of the attempt still reaches the client.
var errIncomplete = errors.New("attempt did not succeed")

// Store keeps the attempts.
type Store interface {
	// Reserve claims the key for a new attempt and returns nil. A key already
	// claimed is returned as it is stored, and nothing is written.
	Reserve(ctx context.Context, key Key, record Record, expires time.Time) (*Record, error)

	// Complete writes the answer of an attempt into the row Reserve claimed.
	Complete(ctx context.Context, key Key, status int, body []byte) error
}

// Transactor runs a unit of work in one database transaction. The handler runs
// inside it, so claiming the key and carrying the request out either both
// happen or neither does.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Config is what the middleware needs to guard a set of operations.
type Config struct {
	Store Store
	Tx    Transactor
	TTL   time.Duration

	// Operations are the routes that require a key, as "METHOD /pattern".
	Operations map[string]struct{}

	// User tells whose attempt the request is.
	User func(ctx context.Context) (uuid.UUID, error)

	// OnError renders an error in the envelope of the API being guarded.
	OnError func(w http.ResponseWriter, r *http.Request, err error)
}

// Middleware answers a repeated request with the answer of the first one. A
// request to an operation that is not in Operations passes straight through.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := cfg.Operations[operation(r)]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			if err := cfg.serve(w, r, next); err != nil {
				cfg.OnError(w, r, err)
			}
		})
	}
}

// serve carries one guarded request out. Everything it returns is an error the
// client is told about; a request that was answered, even with a failure of
// its own, returns nil.
func (cfg Config) serve(w http.ResponseWriter, r *http.Request, next http.Handler) error {
	key, err := cfg.key(r)
	if err != nil {
		return err
	}

	fingerprint, err := fingerprintBody(r)
	if err != nil {
		return err
	}

	attempt := Record{Endpoint: operation(r), RequestHash: fingerprint}
	recorded := newRecorder()

	var stored *Record

	err = cfg.Tx.InTx(r.Context(), func(ctx context.Context) error {
		stored, err = cfg.Store.Reserve(ctx, key, attempt, expiresAt(cfg.TTL))
		if err != nil {
			return fmt.Errorf("reserve idempotency key: %w", err)
		}

		if stored != nil {
			return replayable(*stored, attempt)
		}

		next.ServeHTTP(recorded, r.WithContext(ctx))

		if !recorded.succeeded() {
			return errIncomplete
		}

		if failed := cfg.Store.Complete(ctx, key, recorded.status, recorded.body.Bytes()); failed != nil {
			return fmt.Errorf("store idempotent response: %w", failed)
		}

		return nil
	})

	switch {
	case errors.Is(err, errIncomplete):
		recorded.flush(w)
	case err != nil:
		return err
	case stored != nil:
		replay(w, *stored)
	default:
		recorded.flush(w)
	}

	return nil
}

// key reads whose attempt this is and under which key.
func (cfg Config) key(r *http.Request) (Key, error) {
	userID, err := cfg.User(r.Context())
	if err != nil {
		return Key{}, err
	}

	value := r.Header.Get(Header)

	switch {
	case value == "":
		return Key{}, domain.InvalidArgumentf("%s header is required", Header)
	case len(value) > maxHeaderSize:
		return Key{}, domain.InvalidArgumentf("%s must be at most %d bytes", Header, maxHeaderSize)
	}

	return Key{UserID: userID, Value: value}, nil
}

// replayable reports whether a stored attempt may answer the request at hand.
// A key used for something else is a bug in the client: it would otherwise be
// silently given the answer to a request it did not make.
func replayable(stored, attempt Record) error {
	if stored.Endpoint != attempt.Endpoint || !bytes.Equal(stored.RequestHash, attempt.RequestHash) {
		return domain.Unprocessablef(CodeKeyReuse,
			"%s was already used for another request", Header)
	}

	if !stored.Done() {
		return domain.Conflictf(CodeInFlight, "an identical request is still being carried out")
	}

	return nil
}

// replay answers with the stored bytes of the first attempt. The status is the
// one the entity was created with, downgraded to 200: the repeat created nothing.
func replay(w http.ResponseWriter, stored Record) {
	status := stored.ResponseStatus
	if status == http.StatusCreated {
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(stored.ResponseBody)
}

// fingerprintBody hashes the body of the request in a form that does not
// depend on how it was written, and puts the body back for the handler.
func fingerprintBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, domain.InvalidArgumentf("request body could not be read")
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))

	if len(bytes.TrimSpace(raw)) == 0 {
		return Fingerprint(nil), nil
	}

	var parsed any
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, domain.InvalidArgumentf("request body is not valid JSON")
	}

	canonical, err := json.Marshal(parsed)
	if err != nil {
		return nil, domain.InvalidArgumentf("request body could not be read")
	}

	return Fingerprint(canonical), nil
}

// operation names the route the request matched, as the set of guarded
// operations names it, e. g. GET /method
func operation(r *http.Request) string {
	return r.Method + " " + chi.RouteContext(r.Context()).RoutePattern()
}
