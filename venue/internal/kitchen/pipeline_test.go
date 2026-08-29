package kitchen_test

import (
	"avito-kitchen/venue/internal/kitchen"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNext(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		from  kitchen.State
		step  kitchen.Step
		moves bool
	}{
		"a new order is accepted": {
			from:  kitchen.StateNew,
			step:  kitchen.Step{From: kitchen.StateNew, To: kitchen.StateAccepted, Event: kitchen.EventAccept},
			moves: true,
		},
		"an accepted order goes on the stove": {
			from:  kitchen.StateAccepted,
			step:  kitchen.Step{From: kitchen.StateAccepted, To: kitchen.StateCooking, Event: kitchen.EventCooking},
			moves: true,
		},
		"a cooking order becomes ready": {
			from:  kitchen.StateCooking,
			step:  kitchen.Step{From: kitchen.StateCooking, To: kitchen.StateReady, Event: kitchen.EventReady},
			moves: true,
		},
		"a ready order is handed over": {
			from:  kitchen.StateReady,
			step:  kitchen.Step{From: kitchen.StateReady, To: kitchen.StateHandedOver, Event: kitchen.EventHandover},
			moves: true,
		},
		"a handed over order is done":  {from: kitchen.StateHandedOver},
		"a rejected order stays put":   {from: kitchen.StateRejected},
		"a cancelled order stays put":  {from: kitchen.StateCancelled},
		"an unknown state moves no on": {from: kitchen.State("PLATING")},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			step, ok := kitchen.Next(tc.from)

			assert.Equal(t, tc.moves, ok)
			assert.Equal(t, tc.moves, kitchen.Moving(tc.from))
			assert.Equal(t, tc.step, step)
		})
	}
}

func TestPipelineIsAChain(t *testing.T) {
	t.Parallel()

	steps := kitchen.Pipeline()
	require.NotEmpty(t, steps)

	assert.Equal(t, kitchen.StateNew, steps[0].From)

	for i, step := range steps[1:] {
		assert.Equal(t, steps[i].To, step.From, "step %d starts where the one before it ends", i+1)
	}

	last := steps[len(steps)-1]
	assert.Equal(t, kitchen.StateHandedOver, last.To)
	assert.False(t, kitchen.Moving(last.To))
}
