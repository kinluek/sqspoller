package sqspoller

import (
	"context"
	"time"
)

// shutdownMode is the type for a mode of shutdown.
type shutdownMode int

// shutdown modes:
const (
	now shutdownMode = iota
	graceful
	after
)

// shutdown holds information needed to perform the required shutdown.
type shutdown struct {
	sig     shutdownMode
	timeout <-chan time.Time
}

// ShutdownGracefully gracefully shuts down the poller.
func (p *Poller) ShutdownGracefully() error {
	if !p.isRunning() {
		return nil
	}
	if err := p.checkAndSetShuttingDownStatus(); err != nil {
		return err
	}

	p.shutdown <- &shutdown{
		sig:     graceful,
		timeout: nil,
	}
	return <-p.shutdownErrors
}

// ShutdownAfter will attempt to shutdown gracefully, if graceful shutdown cannot
// be achieved within the given time frame, the Poller will exit, potentially
// leaking unhandled resources.
func (p *Poller) ShutdownAfter(t time.Duration) error {
	if !p.isRunning() {
		return nil
	}
	if err := p.checkAndSetShuttingDownStatus(); err != nil {
		return err
	}

	p.shutdown <- &shutdown{
		sig:     after,
		timeout: time.After(t),
	}
	return <-p.shutdownErrors
}

// ShutdownNow shuts down the Poller instantly, potentially leaking unhandled
// resources.
func (p *Poller) ShutdownNow() error {
	if !p.isRunning() {
		return nil
	}
	if err := p.checkAndSetShuttingDownStatus(); err != nil {
		return err
	}

	p.shutdown <- &shutdown{
		sig:     now,
		timeout: nil,
	}
	return <-p.shutdownErrors
}

// handleShutdown handles the shutdown logic for the three different shutdown
// modes.
func (p *Poller) handleShutdown(sd *shutdown, pollingErrors <-chan error, cancel context.CancelFunc) error {
	switch sd.sig {
	case now:
		cancel()
		p.shutdownErrors <- nil
		return ErrShutdownNow
	case graceful:
		finalErr := p.finishCurrentJob(pollingErrors)
		err := <-finalErr
		cancel()
		p.shutdownErrors <- nil
		return err
	case after:
		finalErr := p.finishCurrentJob(pollingErrors)
		for {
			select {
			case err := <-finalErr:
				cancel()
				p.shutdownErrors <- nil
				return err
			case <-sd.timeout:
				cancel()
				p.shutdownErrors <- ErrShutdownGraceful
				return ErrShutdownGraceful
			}
		}
	default:
		// This code should never be reached! Urgent fix
		// required if this error is ever returned!
		return ErrIntegrityIssue
	}
}

// finishCurrentJob sends a stop request to the poller to tell it to stop making
// more polls after it has finished handling its current job. The returned channel
// will return the final error once the poller has been confirmed to have stopped
// polling.
func (p *Poller) finishCurrentJob(pollingErrors <-chan error) <-chan error {
	p.stopRequest <- struct{}{}
	p.exitWait <- struct{}{}

	finalErr := make(chan error)
	go func() {
		err := <-pollingErrors
		<-p.stopConfirmed
		finalErr <- err
	}()
	return finalErr
}

// checkForStopRequests is called at the end of a poll cycle to check whether any
// stop requests have been made. If a stop request is received, the function blocks
// the poller from making anymore requests. This should happen before a graceful
// shutdown to ensure that no more requests to the queue are made.
func (p *Poller) checkForStopRequests() {
	select {
	case <-p.stopRequest:
		p.stopConfirmed <- struct{}{}
		<-p.stopRequest
	default:
	}
}
