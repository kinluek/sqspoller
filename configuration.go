package sqspoller

import "time"

// SetPollInterval lets the user set the time interval between
// poll requests.
func (p *Poller) SetPollInterval(t time.Duration) {
	p.IntervalPoll = t
}

// SetHeartbeat lets the user set the time interval between
// heartbeats.
func (p *Poller) SetHeartbeat(t time.Duration) <-chan Pulse {
	p.IntervalPulse = t
	return p.heartbeat
}

// SetTimeoutHandling lets the user set the time interval between
// poll requests.
func (p *Poller) SetTimeoutHandling(t time.Duration) {
	p.TimeoutHandling = t
}
