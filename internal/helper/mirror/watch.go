package mirror

import (
	"context"
	"time"
)

// settleTime is how long the folder must be quiet after a wake before a run starts, and burstLimit is the longest a run waits for
// a burst of saves to end.
const (
	settleTime = 300 * time.Millisecond
	burstLimit = 3 * time.Second
)

// Keep keeps the folders in step until the context ends: it runs once, waits, and runs again. The wait is short while there is
// something going on, which is a run that did something or failed, and longer when nothing has changed in a while, so an idle
// folder costs a listing every half minute and a busy one every few seconds. A wake, which is a sign that something may have
// changed, ends the wait: the run starts when the wakes have stopped for a moment, so a burst of saves is one run. A nil wake
// is no wakes, and the timed runs alone keep the folders.
func (e *Engine) Keep(ctx context.Context, quick, slow time.Duration, wake <-chan struct{}, told func(*Report, error)) error {
	wait := quick
	for ctx.Err() == nil {
		report, err := e.Once(ctx)
		if told != nil {
			told(report, err)
		}
		busy := err != nil
		if report != nil {
			for kind, n := range report.Done {
				if kind != Settle && kind != Forget && n > 0 {
					busy = true
				}
			}
			busy = busy || len(report.Errors) > 0
		}
		if busy {
			wait = quick
		} else if wait < slow {
			wait = min(wait*2, slow)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		case <-wake:
			if !settle(ctx, wake) {
				return nil
			}
			wait = quick
		}
	}
	return nil
}

// settle waits until no wake has come for a moment, or a burst has gone on too long, and says whether the context is still on.
func settle(ctx context.Context, wake <-chan struct{}) bool {
	burst := time.NewTimer(burstLimit)
	defer burst.Stop()
	for {
		quiet := time.NewTimer(settleTime)
		select {
		case <-ctx.Done():
			quiet.Stop()
			return false
		case <-burst.C:
			quiet.Stop()
			return true
		case <-wake:
			quiet.Stop()
		case <-quiet.C:
			return true
		}
	}
}
