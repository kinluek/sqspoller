package sqspoller

import "sync/atomic"

// resetRunStatus sets the running and shuttingDown status values back to 0.
func (p *Poller) resetRunStatus() {
	atomic.SwapInt64(&p.runStatus, 0)
}

// checkAndSetRunningStatus is called at the start of the Run method to check
// whether the poller is runnable. If not, the function returns ErrNotRunnable,
// else it sets the running status value to 1, ie running.
func (p *Poller) checkAndSetRunningStatus() error {
	if ok := atomic.CompareAndSwapInt64(&p.runStatus, 0, 1); !ok {
		return ErrNotRunnable
	}
	return nil
}

// checkAndSetShuttingDownStatus is called at the start of any shutdown method
// to check whether the poller can be shutdown, ie. in a running state. If not,
// ErrNotCloseable is returned. If it can be shut down, the poller run status is
// set to 2 (shutting down).
func (p *Poller) checkAndSetShuttingDownStatus() error {
	if ok := atomic.CompareAndSwapInt64(&p.runStatus, 1, 2); !ok {
		return ErrNotCloseable
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
