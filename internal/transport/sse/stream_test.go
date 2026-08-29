package sse

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamSend(t *testing.T) {
	t.Parallel()

	type body struct {
		Status string `json:"status"`
		Seq    int64  `json:"seq"`
	}

	cases := map[string]struct {
		send func(*Stream) error
		want string
	}{
		"an event of the history is sent under its number": {
			send: func(s *Stream) error {
				return s.Send(Event{ID: 7, Name: "status_changed", Data: body{Status: "READY", Seq: 7}})
			},
			want: "id: 7\nevent: status_changed\ndata: {\"status\":\"READY\",\"seq\":7}\n\n",
		},
		"a snapshot carries no number and leaves the client where it was": {
			send: func(s *Stream) error {
				return s.Send(Event{Name: "snapshot", Data: body{Status: "CREATED"}})
			},
			want: "event: snapshot\ndata: {\"status\":\"CREATED\",\"seq\":0}\n\n",
		},
		"a heartbeat is a comment and nothing else": {
			send: func(s *Stream) error { return s.Heartbeat() },
			want: ": heartbeat\n\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			stream := NewStream(recorder)

			require.NoError(t, tc.send(stream))

			assert.Equal(t, tc.want, recorder.Body.String())
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "text/event-stream; charset=utf-8", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			assert.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
			assert.True(t, recorder.Flushed, "the frame was not flushed to the client")
		})
	}
}

// TestStreamSendToAGoneClient reports the write failure the handler closes the
// stream on.
func TestStreamSendToAGoneClient(t *testing.T) {
	t.Parallel()

	stream := NewStream(brokenWriter{ResponseWriter: httptest.NewRecorder()})

	assert.Error(t, stream.Send(Event{Name: "snapshot", Data: struct{}{}}))
	assert.Error(t, stream.Heartbeat())
}

type brokenWriter struct {
	http.ResponseWriter
}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, http.ErrBodyNotAllowed
}
