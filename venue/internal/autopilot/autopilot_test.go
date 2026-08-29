package autopilot_test

import (
	"avito-kitchen/venue/internal/autopilot"
	"avito-kitchen/venue/internal/config"
	"avito-kitchen/venue/internal/kitchen"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeadlines(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)

	pace := config.Autopilot{
		AcceptAfter: 2 * time.Second,
		CookAfter:   3 * time.Second,
		ReadyAfter:  5 * time.Second,
		HandAfter:   7 * time.Second,
	}

	cases := map[string]struct {
		cfg  config.Autopilot
		want []kitchen.Due
	}{
		"every step of the kitchen has a deadline of its own": {
			cfg: pace,
			want: []kitchen.Due{
				{State: kitchen.StateNew, Cutoff: now.Add(-2 * time.Second)},
				{State: kitchen.StateAccepted, Cutoff: now.Add(-3 * time.Second)},
				{State: kitchen.StateCooking, Cutoff: now.Add(-5 * time.Second)},
				{State: kitchen.StateReady, Cutoff: now.Add(-7 * time.Second)},
			},
		},
		"a kitchen with no pace set moves everything at once": {
			cfg: config.Autopilot{},
			want: []kitchen.Due{
				{State: kitchen.StateNew, Cutoff: now},
				{State: kitchen.StateAccepted, Cutoff: now},
				{State: kitchen.StateCooking, Cutoff: now},
				{State: kitchen.StateReady, Cutoff: now},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, autopilot.Deadlines(tc.cfg, now))
		})
	}
}

// TestDeadlinesFollowThePipeline keeps the pace of the autopilot tied to the
// states an order actually passes through: a state that ends the order is
// never waited on.
func TestDeadlinesFollowThePipeline(t *testing.T) {
	t.Parallel()

	due := autopilot.Deadlines(config.Autopilot{}, time.Now())

	assert.Len(t, due, len(kitchen.Pipeline()))

	for _, d := range due {
		assert.True(t, kitchen.Moving(d.State), "%s is waited on but leads nowhere", d.State)
	}
}
