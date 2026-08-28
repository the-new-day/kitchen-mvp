package worker

import (
	"testing"
	"time"
)

// TestPaceNext covers the adaptive pause of the outbox job: a full batch means
// a backlog and no pause at all, a partial one the shortest pause, and an idle
// or unreachable broker doubles the wait up to the ceiling.
func TestPaceNext(t *testing.T) {
	t.Parallel()

	const (
		minPause = 50 * time.Millisecond
		maxPause = 250 * time.Millisecond
		capacity = 100
	)

	type run struct {
		sent   int
		failed bool
	}

	cases := map[string]struct {
		runs []run
		want []time.Duration
	}{
		"a full batch does not pause": {
			runs: []run{{sent: capacity}, {sent: capacity}},
			want: []time.Duration{0, 0},
		},
		"a partial batch pauses for the shortest time": {
			runs: []run{{sent: 7}},
			want: []time.Duration{minPause},
		},
		"an empty table doubles the pause up to the ceiling": {
			runs: []run{{}, {}, {}, {}, {}},
			want: []time.Duration{minPause, 100 * time.Millisecond, 200 * time.Millisecond, maxPause, maxPause},
		},
		"an unreachable broker backs off like an empty table": {
			runs: []run{{failed: true}, {failed: true}, {failed: true}},
			want: []time.Duration{minPause, 100 * time.Millisecond, 200 * time.Millisecond},
		},
		"work resets the pause the idling has grown": {
			runs: []run{{}, {}, {}, {sent: 3}, {}},
			want: []time.Duration{minPause, 100 * time.Millisecond, 200 * time.Millisecond, minPause, minPause},
		},
		"a partial batch published against a failing broker still backs off": {
			runs: []run{{sent: 3, failed: true}, {sent: 3, failed: true}},
			want: []time.Duration{minPause, 100 * time.Millisecond},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rate := newPace(minPause, maxPause)

			for i, r := range tc.runs {
				if got := rate.next(r.sent, capacity, r.failed); got != tc.want[i] {
					t.Fatalf("run %d paused for %s, want %s", i, got, tc.want[i])
				}
			}
		})
	}
}
