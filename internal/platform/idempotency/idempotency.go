// Package idempotency makes a request safe to repeat: the first attempt of a
// key is carried out and its answer stored, every repeat of the same key gets
// that answer back instead of a second entity. The mechanism knows nothing
// about the operations it guards; they are named to it from the outside.
package idempotency

import (
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
)

// Header carries the key of an attempt.
const Header = "Idempotency-Key"

// Codes of the error envelope a request is refused with.
const (
	CodeKeyReuse = "idempotency_key_reuse"
	CodeInFlight = "idempotency_key_in_flight"
)

// maxHeaderSize is the longest key accepted, so that the header cannot be used
// to write arbitrary amounts into the table.
const maxHeaderSize = 200

// Key identifies one attempt of one customer. The customer is part of the key
// so that a key guessed or repeated by somebody else cannot reach a stored
// answer that is not theirs.
type Key struct {
	UserID uuid.UUID
	Value  string
}

// Record is a stored attempt: what it was made for and, once it has finished,
// what it answered. ResponseStatus is zero while the attempt is still running.
type Record struct {
	Endpoint       string
	RequestHash    []byte
	ResponseStatus int
	ResponseBody   []byte
}

// Done reports whether the attempt has an answer to give back.
func (r Record) Done() bool {
	return r.ResponseStatus != 0
}

// Fingerprint is the hash a repeated key is checked against. It is taken from
// the parsed body rather than from its bytes, so that a client that reordered
// the keys of its JSON or a proxy that reformatted it still gets its answer
// back instead of a refusal.
func Fingerprint(canonical []byte) []byte {
	sum := sha256.Sum256(canonical)
	return sum[:]
}

// expiresAt is when a stored attempt stops being answered from.
func expiresAt(ttl time.Duration) time.Time {
	return time.Now().Add(ttl)
}
