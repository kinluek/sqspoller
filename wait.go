package sqspoller

import (
	"context"
	"time"
)

// waitForSignals knows how to handle signals coming from the handler
// channel, context cancellations and poll time intervals.
func waitForSignals(ctx context.Context, handlerError chan error, interval time.Duration) error {
	//======================================================================
	// Wait for handler or cancellation signal
	select {
	case err := <-handlerError:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		if err := <-handlerError; err != nil {
			return err
		}
		return ctx.Err()
	}

	//======================================================================
	// Set wait time to next poll
	nextPoll := time.After(interval)

	//======================================================================
	// Handle interval, cancellation
	select {
	case <-nextPoll:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Poller) groupStopSignal(ctx context.Context, timeoutNoMsg, timeoutHandling <-chan time.Time) <-chan error {
	stop := make(chan error)

	go func() {
		<-ctx.Done()
		stop <- ctx.Err()
	}()

	if p.TimeoutHandling > 0 {
		go func() {
			<-timeoutHandling
			stop <- ErrTimeoutNoMessages
		}()
	}

	return stop
}
