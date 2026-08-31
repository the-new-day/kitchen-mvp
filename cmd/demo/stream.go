package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// statusChangedEvent is the frame of the stream that carries a transition.
const statusChangedEvent = "status_changed"

// streamBuffer is how many transitions the reader may run ahead of the demo by.
const streamBuffer = 16

// statusUpdate is one transition of an order as the stream sends it.
type statusUpdate struct {
	Seq    int64  `json:"seq"`
	Status string `json:"status"`
	Actor  string `json:"actor"`
}

// stream is the event stream of one order.
type stream struct {
	updates <-chan statusUpdate
	failed  <-chan error
}

// follow subscribes to the events of an order and reads them until the order
// ends or ctx is cancelled. Everything that happened before the subscription is
// replayed: the stream is asked to continue from the very first entry.
func follow(ctx context.Context, client *http.Client, url, userID string) (*stream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build the stream request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set(userHeader, userID)
	req.Header.Set("Last-Event-ID", "0")

	res, err := client.Do(req) //nolint:bodyclose // the reader closes it
	if err != nil {
		return nil, fmt.Errorf("open the event stream: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()

		return nil, fmt.Errorf("the event stream answered %s", res.Status)
	}

	updates := make(chan statusUpdate, streamBuffer)
	failed := make(chan error, 1)

	go func() {
		defer func() { _ = res.Body.Close() }()
		defer close(updates)

		if err := read(ctx, res.Body, updates); err != nil && ctx.Err() == nil {
			failed <- err
		}
	}()

	return &stream{updates: updates, failed: failed}, nil
}

// read turns the frames of a stream into transitions. A frame of another kind,
// a heartbeat and the identifier of an event are all skipped: the demo follows
// the statuses.
func read(ctx context.Context, body io.Reader, updates chan<- statusUpdate) error {
	var (
		scanner = bufio.NewScanner(body)
		name    string
		data    string
	)

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			if name != statusChangedEvent || data == "" {
				name, data = "", ""

				continue
			}

			var update statusUpdate
			if err := json.Unmarshal([]byte(data), &update); err != nil {
				return fmt.Errorf("read the stream frame %q: %w", data, err)
			}

			select {
			case updates <- update:
			case <-ctx.Done():
				return nil
			}

			name, data = "", ""
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}

	return scanner.Err()
}
