package sqspoller

import (
	"context"
	"time"
)

// waitOnHandling waits for the handler to return its error
// if a cancellation or timeout signal received before the
// handler can finish processing the current job, then the function
// returns a non nil error to tell the poller to exit.
func (p *Poller) waitOnHandling(ctx context.Context, handlerErrors <-chan error) error {

	timeoutHandling := make(chan time.Time, 1)

	// if TimeoutHandling has been set, spin off a go routine
	// to handle the timeout signal
	if p.TimeoutHandling > 0 {
		go func() {
			timeout := time.After(p.TimeoutHandling)
			t := <-timeout
			timeoutHandling <- t
		}()
	}

	select {
	case err := <-handlerErrors:
		if err != nil {
			return err
		}
	case <-timeoutHandling:
		return ErrTimeoutHandling
	case <-ctx.Done():
		if err := <-handlerErrors; err != nil {
			return err
		}
		return ctx.Err()
	}

	return nil
}

// waitForNextPoll handles the time interval to wait till
// the next poll request is made.
func (p *Poller) waitForNextPoll(ctx context.Context) error {
	nextPoll := time.After(p.Interval)

	select {
	case <-nextPoll:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// checkForStopRequests is called at the end of a poll cycle
// to check whether any stop requests have been made. If a stop
// request is received, the function blocks the poller from making
// anymore requests.
func (p *Poller) checkForStopRequests() {
	select {
	case <-p.stopRequest:
		p.stopConfirmed <- struct{}{}
		<-p.stopRequest
	default:
	}
}


