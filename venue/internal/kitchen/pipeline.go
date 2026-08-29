package kitchen

// Event is what the venue tells the platform about an order.
type Event string

// Events the venue sends.
const (
	EventAccept   Event = "accept"
	EventCooking  Event = "cooking"
	EventReady    Event = "ready"
	EventHandover Event = "handover"
)

// Step is one move of an order through the kitchen: the state it waits in, the
// state it lands in and the event that takes it there.
type Step struct {
	From  State
	To    State
	Event Event
}

// pipeline is the whole life of an order in the kitchen. An order leaves it
// only sideways: cancelled by the platform or rejected.
var pipeline = []Step{
	{From: StateNew, To: StateAccepted, Event: EventAccept},
	{From: StateAccepted, To: StateCooking, Event: EventCooking},
	{From: StateCooking, To: StateReady, Event: EventReady},
	{From: StateReady, To: StateHandedOver, Event: EventHandover},
}

// Pipeline returns the steps an order takes, in order.
func Pipeline() []Step {
	steps := make([]Step, len(pipeline))
	copy(steps, pipeline)

	return steps
}

// Next reports the step an order in this state takes next. A state the kitchen
// does not move on its own has none.
func Next(from State) (Step, bool) {
	for _, step := range pipeline {
		if step.From == from {
			return step, true
		}
	}

	return Step{}, false
}

// Moving reports whether an order in this state is still on its way through
// the kitchen and can be taken away from it.
func Moving(state State) bool {
	_, ok := Next(state)

	return ok
}
