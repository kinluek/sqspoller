package sqspoller

import (
	"context"
	"time"
)

type shutdownSig int

const (
	now shutdownSig = iota
	graceful
	after
)

type shutdown struct {
	sig     shutdownSig
	timeout <-chan time.Time
}

// ShutdownGracefully gracefully shuts down the poller.
func (p *Poller) ShutdownGracefully() error {
	p.shutdown <- &shutdown{
		sig:     graceful,
		timeout: nil,
	}
	return <-p.shutdownErrors
}

// ShutdownAfter will attempt to shutdown gracefully,
// if graceful shutdown cannot be achieved within the given
// time frame, the Poller will exit, potentially leaking
// unhandled resources.
func (p *Poller) ShutdownAfter(t time.Duration) error {
	p.shutdown <- &shutdown{
		sig:     after,
		timeout: time.After(t),
	}
	return <-p.shutdownErrors
}

// ShutdownNow shuts down the Poller instantly,
// potentially leaking unhandled resources.
func (p *Poller) ShutdownNow() error {
	p.shutdown <- &shutdown{
		sig:     now,
		timeout: nil,
	}
	return <-p.shutdownErrors
}


func (p *Poller) handleShutdown(sd *shutdown, pollingErrors chan error) error {
	switch sd.sig {
	case now:
		p.shutdownErrors <- nil
		return ErrShutdownNow

	case graceful:
		for err := range pollingErrors {
			if err == context.Canceled {
				p.shutdownErrors <- nil
				return nil
			}
			if err != nil {
				p.shutdownErrors <- nil
				return err
			}
		}
		// This code should never be reached! Urgent fix
		// required if this error is ever returned!
		p.shutdownErrors <- ErrIntegrityIssue

	case after:
		for {
			select {
			case err := <-pollingErrors:
				if err == context.Canceled {
					p.shutdownErrors <- nil
					return nil
				}
				if err != nil {
					p.shutdownErrors <- nil
					return err
				}
			case <-sd.timeout:
				p.shutdownErrors <- ErrShutdownGraceful
				return ErrShutdownGraceful
			}
		}
	}

	// This code should never be reached! Urgent fix
	// required if this error is ever returned!
	return ErrIntegrityIssue
}