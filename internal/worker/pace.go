package worker

import "time"

// pace decides how long the outbox job waits before looking at the table
// again. Under load it does not wait at all; on an idle table it backs off to
// max, which is what the delivery of an event costs at worst.
type pace struct {
	min     time.Duration
	max     time.Duration
	current time.Duration
}

func newPace(minPause, maxPause time.Duration) *pace {
	return &pace{min: minPause, max: maxPause, current: minPause}
}

// next reports the pause after a run that published sent messages out of a
// batch of capacity. A full batch means there is a backlog and the loop keeps
// going; a failed run backs off like an empty one, so that a broker that is
// down is not hammered.
func (p *pace) next(sent, capacity int, failed bool) time.Duration {
	if failed || sent == 0 {
		pause := p.current
		p.current = min(p.current*2, p.max)

		return pause
	}

	p.current = p.min

	if sent >= capacity {
		return 0
	}

	return p.min
}
