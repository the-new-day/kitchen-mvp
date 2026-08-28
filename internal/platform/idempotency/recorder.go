package idempotency

import (
	"bytes"
	"net/http"
)

// recorder holds the answer of a handler until it is known whether it may be
// stored. Nothing reaches the client before the transaction of the attempt is
// decided, so a stored answer is always the one that was actually sent.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newRecorder() *recorder {
	return &recorder{header: make(http.Header), status: http.StatusOK}
}

func (rec *recorder) Header() http.Header {
	return rec.header
}

func (rec *recorder) WriteHeader(status int) {
	rec.status = status
}

func (rec *recorder) Write(b []byte) (int, error) {
	return rec.body.Write(b)
}

// succeeded reports whether the answer is one worth replaying: a failed
// attempt leaves its key free for the client to try again.
func (rec *recorder) succeeded() bool {
	return rec.status >= http.StatusOK && rec.status < http.StatusMultipleChoices
}

// flush sends the recorded answer to the client as it is.
func (rec *recorder) flush(w http.ResponseWriter) {
	for name, values := range rec.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(rec.status)
	_, _ = w.Write(rec.body.Bytes())
}
