package sqspoller

import (
	"context"
	"time"
)

func wait(ctx context.Context, handlingMsg bool, handlerError chan error, interval time.Duration) error {
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
	// Set wait time to next poll
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

func waitWithTimeout(ctx context.Context, handlingMsg bool, handlerError chan error, interval time.Duration, timedOut <-chan time.Time) error {

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
	// Set wait time to next poll
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
