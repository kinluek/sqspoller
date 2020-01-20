package sqspoller

import (
	"context"
	"time"
)

// waitForHandler waits for the handler to return it's error,
// if a cancellation or timeout signal is received before the
// handler can finish processing the current job, then the
// function returns a non nil error to tell the poller to exit.
func (p *Poller) waitForHandler(ctx context.Context, handlerErrors <-chan error) error {
	select {
	case err := <-handlerErrors:
		if err != nil {
			return err
		}
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
	nextPoll := time.NewTimer(p.PollInterval)
	defer nextPoll.Stop()

	select {
	case <-nextPoll.C:
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
