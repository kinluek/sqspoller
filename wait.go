package sqspoller

import (
	"context"
	"time"
)

// waitForSignals knows how to handle signals coming from the handler
// channel, context cancellations and poll time intervals.
func waitForSignals(ctx context.Context, handlerError chan error, interval time.Duration) error {
	//======================================================================
	// Wait for handler to return
	select {
	case err := <-handlerError:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	//======================================================================
	// Set waitForSignals time to next poll
	nextPoll := time.After(interval)

	//======================================================================
	// Handle intervals and timedOuts
	select {
	case <-nextPoll:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitForSignalsWithTimeout knows how to handle signals coming from the handler
// channel, context cancellations, poll time intervals and timeouts.
func waitForSignalsWithTimeout(ctx context.Context, handlerError chan error, interval time.Duration, handlingMsg bool, timedOut <-chan time.Time) error {

	//======================================================================
	// Wait for handler to return
	select {
	case err := <-handlerError:
		if err != nil {
			return err
		}
	case <-timedOut:
		err := <-handlerError
		if err != nil {
			return err
		}
		if !handlingMsg {
			return ErrTimeoutNoMessages
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	//======================================================================
	// Set waitForSignals time to next poll
	nextPoll := time.After(interval)

	//======================================================================
	// Handle intervals and timedOuts
	select {
	case <-nextPoll:
		return nil
	case <-timedOut:
		return ErrTimeoutNoMessages
	case <-ctx.Done():
		return ctx.Err()
	}
}
