package sqspoller

import (
	"context"
	"time"
)

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

func (p *Poller) waitForNextPoll(ctx context.Context) error {
	nextPoll := time.After(p.Interval)

	select {
	case <-nextPoll:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}