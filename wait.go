package sqspoller

import (
	"time"
)

// handlePollInterval handles the time to wait according to the IdlePollInterval
// and CurrentInterval. It also handles the adjustment of the CurrentInterval
// based on the queueEmpty flag.
func (p *Poller) handlePollInterval() error {
	// dont wait if no idle poll interval set
	if p.IdlePollInterval == 0 {
		return nil
	}
	// dont wait if queue has messages and set
	// currentInterval back down to 0
	if !p.queueEmpty {
		p.CurrentInterval = 0
		return nil
	}

	p.CurrentInterval = doubleWithLimit(p.CurrentInterval, p.IdlePollInterval)
	return waitForInterval(p.CurrentInterval, p.exitWait)
}

// waitForInterval waits for the given interval time before moving on, unless
// an exit signal is received first.
func waitForInterval(interval time.Duration, exit <-chan struct{}) error {
	nextPoll := time.NewTimer(interval)
	defer nextPoll.Stop()

	select {
	case <-nextPoll.C:
		return nil
	case <-exit:
		return nil
	}
}

// doubleWithLimit takes a current time duration and doubles it, if doubling it
// goes over the limit, then the limit is returned. If the current duration is
// less than 1 second, then 1 second is returned.
func doubleWithLimit(current, limit time.Duration) time.Duration {
	if current > limit {
		return limit
	}
	if current < time.Second {
		current = time.Second
	} else if current < limit {
		current = 2 * current
		if current > limit {
			current = limit
		}
	}
	return current
}
