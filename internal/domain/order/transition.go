package order

// Command is a request to move an order: the event to apply and who asks for
// it. Reason and EtaMinutes are what the actor adds to the change and are
// stored with the order.
type Command struct {
	Event      Event
	Actor      Actor
	Reason     string
	EtaMinutes *int
}

// StatusChange is a transition ready to be stored: the order goes from From to
// To, and the status history keeps who moved it and why.
type StatusChange struct {
	From       Status
	To         Status
	Actor      Actor
	Reason     string
	EtaMinutes *int
}

// Applied is a stored transition. Seq is the number of the entry it left in the
// status history: it orders the changes of one order and lets a client resume
// the stream of them.
type Applied struct {
	Order Order
	Seq   int64
}

// ReturnsStock reports whether an order that has reached this status gives the
// quantities it reserved back to the menu.
func (s Status) ReturnsStock() bool {
	switch s {
	case StatusRejected, StatusCancelled:
		return true
	default:
		return false
	}
}
