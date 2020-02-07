package sqspoller

import "sync/atomic"

// resetRunState sets the running and shuttingDown status values back to 0.
func (p *Poller) resetRunState() {
	atomic.SwapInt64(&p.running, 0)
	atomic.SwapInt64(&p.shuttingDown, 0)
}

// isRunning checks the running state of the poller
func (p *Poller) isRunning() bool {
	return atomic.LoadInt64(&p.running) == 1
}

// checkAndSetRunningStatus is called at the start of the Run method to check
// whether the poller is already running. If it is, the function returns the
// ErrAlreadyRunning error, else it sets the running status value to 1.
func (p *Poller) checkAndSetRunningStatus() error {
	if ok := atomic.CompareAndSwapInt64(&p.running, 0, 1); !ok {
		return ErrAlreadyRunning
	}
	return nil
}

// checkAndSetShuttingDownStatus is called at the start of any shutdown method
// to check whether the poller is already in the process of shutting down. If it
// is, the function returns the ErrAlreadyShuttingDown error, else it sets the
// shuttingDown value to 1.
func (p *Poller) checkAndSetShuttingDownStatus() error {
	if ok := atomic.CompareAndSwapInt64(&p.shuttingDown, 0, 1); !ok {
		return ErrAlreadyShuttingDown
	}
	return nil

}

// validateSetup should be executed at the start of the Run() method to validate
// that the poller has all the necessary configurations and handlers set up. if
// a non-nil error is returned then the Run() method should exit.
func (p *Poller) validateSetup() error {
	if p.messageHandler == nil {
		return ErrNoMessageHandler
	}
	if p.errorHandler == nil {
		return ErrNoErrorHandler
	}
	if p.receiveMsgInput == nil {
		return ErrNoReceiveMessageParams
	}

	// set the default request timeout, if the caller has not explicitly set one.
	if p.RequestTimeout <= 0 {
		p.RequestTimeout = defaultRequestTimeout
	}

	return nil
}
