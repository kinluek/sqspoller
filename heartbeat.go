package sqspoller

import "time"

// Pulse holds information on the time of last poll.
// This type is sent out on the heartbeat channel to
// let the user know that the Poller is still running
// as should be.
type Pulse struct {
	LastPollTime time.Time
	PulseTime    time.Time
}

func (p *Poller) getHeartbeat() <-chan time.Time {
	heartbeat := make(chan time.Time)

	if p.IntervalPulse > 0 {
		interval := time.After(p.IntervalPulse)
		go func() {
			t := <-interval
			heartbeat <- t
		}()
	}

	return heartbeat
}
