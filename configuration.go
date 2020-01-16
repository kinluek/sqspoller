package sqspoller

import "time"

// SetInterval lets the user set the time interval between
// poll requests.
func (p *Poller) SetInterval(t time.Duration) {
	p.Interval = t
}

// SetTimeoutHandling lets the user set the time interval between
// poll requests.
func (p *Poller) SetTimeoutHandling(t time.Duration) {
	p.TimeoutHandling = t
}


