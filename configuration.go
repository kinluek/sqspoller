package sqspoller

import "time"

// SetPollInterval lets the user set the time interval between
// poll requests.
func (p *Poller) SetPollInterval(t time.Duration) {
	p.PollInterval = t
}

// SetHandlerTimeout lets the user set the timeout for handling
// a message, if the handler function cannot finish execution
// within this time frame, then the Poller will exit and return
// ErrHandlerTimeout.
func (p *Poller) SetHandlerTimeout(t time.Duration) {
	p.HandlerTimeout = t
}
