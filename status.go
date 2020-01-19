package sqspoller

// resetRunState sets the running and shuttingDown status values
// back to false.
func (p *Poller) resetRunState() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.running = false
	p.shuttingDown = false
}

// isRunning checks the running state of the poller
func (p *Poller) isRunning() bool {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.running
}


// checkAndSetRunningStatus is called at the start of the Run
// method to check whether the poller is already running. If it
// is, the function returns the ErrAlreadyRunning error, else
// it sets the running status value to true.
func (p *Poller) checkAndSetRunningStatus() error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.running {
		return ErrAlreadyRunning
	}
	p.running = true
	return nil
}

// checkAndSetShuttingDownStatus is called at the start of any
// shutdown method to check whether the poller is already in the
// process of shutting down. If it is, the function returns the
// ErrAlreadyShuttingDown error, else it sets the shuttingDown
// value to true.
func (p *Poller) checkAndSetShuttingDownStatus() error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.shuttingDown {
		return ErrAlreadyShuttingDown
	}
	p.shuttingDown = true
	return nil
}
