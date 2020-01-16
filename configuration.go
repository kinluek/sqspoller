package sqspoller

import "time"

// SetInterval lets the user set the time interval between
// poll requests.
func (p *Poller) SetInterval(t time.Duration) {
	p.Interval = t
}

// ExitAfterNoMessagesReceivedFor, the poller will exit
// after it has not received a message for the given time.
func (p *Poller) ExitAfterNoMessagesReceivedFor(t time.Duration) {
	p.TimeoutNoMessages = t
}

