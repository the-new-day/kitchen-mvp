package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Event is one frame of the stream. An ID of zero writes no id field: the
// frame is a state of the order rather than a step of its history, and it must
// not become what a reconnecting client continues from.
type Event struct {
	ID   int64
	Name string
	Data any
}

// Stream writes an event stream into an open response. Every frame is flushed
// on its own: a proxy or a client buffering the answer would defeat the point.
type Stream struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// NewStream answers the request with the headers of an event stream and holds
// the response open.
func NewStream(w http.ResponseWriter) *Stream {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)

	stream := &Stream{w: w, rc: http.NewResponseController(w)}
	stream.flush()

	return stream
}

// Send writes one frame. An error means the client is gone.
func (s *Stream) Send(event Event) error {
	body, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event.Name, err)
	}

	var frame []byte

	if event.ID > 0 {
		frame = fmt.Appendf(frame, "id: %d\n", event.ID)
	}

	frame = fmt.Appendf(frame, "event: %s\ndata: %s\n\n", event.Name, body)

	return s.write(frame)
}

// Heartbeat writes a comment nobody reads, so that an idle stream keeps the
// connection through the proxies alive and a dead one is noticed.
func (s *Stream) Heartbeat() error {
	return s.write([]byte(": heartbeat\n\n"))
}

func (s *Stream) write(frame []byte) error {
	if _, err := s.w.Write(frame); err != nil {
		return fmt.Errorf("write to stream: %w", err)
	}

	s.flush()

	return nil
}

// flush pushes what is written to the client. A response that cannot be
// flushed is served anyway: the frames arrive when its own buffer decides.
func (s *Stream) flush() {
	_ = s.rc.Flush()
}
