package kafka

import "errors"

// ErrMalformed is a message that is not an envelope of the platform. Such a
// message is dropped rather than retried: redelivering it changes nothing.
var ErrMalformed = errors.New("malformed event")
