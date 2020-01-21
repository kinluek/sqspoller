package sqspoller

import (
	"context"
	"time"
)

// waitForError waits for the error channel to return it's error,
// if a cancellation signal is received before the error from the
// channel is received, the function will exit with a non nil error.
func waitForError(ctx context.Context, errChan <-chan error) error {
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		if err := <-errChan; err != nil {
			return err
		}
		return ctx.Err()
	}

	return nil
}

// waitForInterval waits for the given interval time before moving
// on, unless the context object is cancelled first.
func waitForInterval(ctx context.Context, interval time.Duration) error {
	nextPoll := time.NewTimer(interval)
	defer nextPoll.Stop()

	select {
	case <-nextPoll.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

