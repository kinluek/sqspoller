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
	}

	// This code should never be reached! Urgent fix
	// required if this error is ever returned!
	return ErrIntegrityIssue
}

func (p *Poller) finishCurrentJob(pollingErrors <-chan error) <-chan error {

	finalErr := make(chan error)

	go func() {
		p.stopRequest <- struct{}{}
		err := <-pollingErrors
		<-p.stopConfirmed
		finalErr <- err
	}()

	return finalErr
}
