package sqspoller

import (
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
	return p.shutdownForInput(&shutdown{
		sig:     graceful,
		timeout: nil,
	})
}

// ShutdownAfter will attempt to shutdown gracefully, if graceful shutdown cannot
// be achieved within the given time frame, the Poller will exit, potentially
// leaking unhandled resources.
func (p *Poller) ShutdownAfter(t time.Duration) error {
	return p.shutdownForInput(&shutdown{
		sig:     after,
		timeout: time.After(t),
	})
}

// ShutdownNow shuts down the Poller instantly, potentially leaking unhandled
// resources.
func (p *Poller) ShutdownNow() error {
	return p.shutdownForInput(&shutdown{
		sig:     now,
		timeout: nil,
	})
}

// shutdownForInput executes the shutdown for the given input.
func (p *Poller) shutdownForInput(input *shutdown) error {
	if err := p.checkAndSetShuttingDownStatus(); err != nil {
		return err
	}

	p.shutdown <- input
	return <-p.shutdownErrors
}

// handleShutdown handles the shutdown orchestration for the  different shutdown
// modes.
func (p *Poller) handleShutdown(sd *shutdown, pollingErrors <-chan error) error {
	switch sd.sig {
	case now:
		p.stopRequest <- struct{}{}
		p.shutdownErrors <- nil
		return ErrShutdownNow
	case graceful:
		finalErr := p.finishCurrentJob(pollingErrors)
		err := <-finalErr
		p.shutdownErrors <- nil
		return err
	case after:
		finalErr := p.finishCurrentJob(pollingErrors)
		select {
		case err := <-finalErr:
			p.shutdownErrors <- nil
			return err
		case <-sd.timeout:
			p.shutdownErrors <- ErrShutdownGraceful
			return ErrShutdownGraceful
		}
	default:
		// This code should never be reached! Urgent fix
		// required if this error is ever returned!
		p.shutdownErrors <- ErrIntegrityIssue
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

// stopRequestReceived is called at the end of a poll cycle to check whether any
// stop requests have been made. If a stop request is received, the function will
// return true, to tell the poller to break the polling loop. This should happen
// before a graceful shutdown to ensure that no more requests to the queue are made.
func (p *Poller) stopRequestReceived() bool {
	select {
	case <-p.stopRequest:
		p.stopConfirmed <- struct{}{}
		return true
	default:
		return false
	}
}
